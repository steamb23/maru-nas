// Package server는 HTTP 라우트와 핸들러를 연결한다.
//
// 라우트는 대시보드 루트(/api/...)에 둔다. 대시보드를 경로 접두사 아래에 둘 때는
// 리버스 프록시가 접두사를 제거한다.
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/omv"
	"onmaru/backend/internal/prefs"
)

// SystemSource는 시스템 통계를 제공한다. *sysinfo.Reader가 구현한다.
type SystemSource interface {
	System() (api.SystemResponse, error)
}

// OMV는 인증에 필요한 OMV RPC 호출이다. *omv.Client가 구현한다.
type OMV interface {
	Login(ctx context.Context, username, password, clientIP string) (omv.LoginResult, error)
	Verify(ctx context.Context, sessionID, username string, response any, clientIP string) (omv.LoginResult, error)
	Logout(ctx context.Context, sessionID string) error
	Probe(ctx context.Context, sessionID string, touch bool) (omv.SessionState, error)
	// Call은 rpc.php를 한 번 호출한다. /api/rpc 중계에 쓴다.
	Call(ctx context.Context, r omv.Request) (omv.Response, error)
}

// PluginDetector는 OMV 플러그인 설치 여부를 알려 준다. *addons.Detector가 구현한다.
type PluginDetector interface {
	// Plugins는 설치된 플러그인 패키지 집합이다.
	Plugins() map[string]bool
	// Refresh는 바로 다시 확인한다.
	Refresh()
	// Invalidate는 다음 Plugins 호출 때 다시 확인하게 한다.
	Invalidate()
}

// noPlugins는 아무 플러그인도 설치되지 않은 것으로 보는 PluginDetector다.
type noPlugins struct{}

func (noPlugins) Plugins() map[string]bool { return map[string]bool{} }
func (noPlugins) Refresh()                 {}
func (noPlugins) Invalidate()              {}

// Config는 New의 설정이다. System, Static, OMV는 필수다.
type Config struct {
	System SystemSource
	// Static은 /api/ 밖의 요청을 처리한다(프론트엔드 빌드 결과).
	Static http.Handler
	OMV    OMV
	// Addons는 등록된 애드온이다. nil이면 빈 목록이다.
	Addons *addons.Registry
	// Plugins가 nil이면 아무 플러그인도 설치되지 않은 것으로 본다.
	Plugins PluginDetector
	// Prefs는 사용자 설정 저장소다. nil이면 메모리에 둔다(재시작하면 사라진다).
	Prefs prefs.Store

	// Sessions, Limiter가 nil이면 새로 만든다.
	Sessions *auth.Store
	Limiter  *auth.Limiter
	// CookiePath는 브라우저가 보는 대시보드 경로다(리버스 프록시가 떼기 전). 기본 /.
	CookiePath string
	// CheckInterval마다 요청 처리 중에 OMV 세션이 살아 있는지 확인한다. 기본 60초.
	// 예상 만료 시각이 지났으면 주기와 상관없이 확인한다.
	CheckInterval time.Duration
	// FailDelay는 로그인 실패 응답 전에 기다리는 시간이다. 기본 1초, 음수면 기다리지 않는다.
	FailDelay time.Duration
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time
}

// handler는 라우트가 공유하는 상태다.
type handler struct {
	Config
	// actions는 진행 중인 애드온 조작이다.
	actions *inflight
}

// New는 루트 핸들러를 반환한다. /api/ 밖의 요청은 cfg.Static이 처리한다.
func New(cfg Config) http.Handler {
	if cfg.Sessions == nil {
		cfg.Sessions = auth.NewStore()
	}
	if cfg.Limiter == nil {
		cfg.Limiter = auth.NewLimiter()
	}
	if cfg.CookiePath == "" {
		cfg.CookiePath = "/"
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 60 * time.Second
	}
	if cfg.FailDelay == 0 {
		cfg.FailDelay = time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Addons == nil {
		cfg.Addons, _ = addons.NewRegistry()
	}
	if cfg.Plugins == nil {
		cfg.Plugins = noPlugins{}
	}
	if cfg.Prefs == nil {
		cfg.Prefs = prefs.NewMemoryStore()
	}
	h := &handler{Config: cfg, actions: newInflight()}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
	})
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/login/verify", h.verify)
	mux.HandleFunc("POST /api/logout", h.logout)
	mux.HandleFunc("GET /api/session", h.requireSession(h.session))
	mux.HandleFunc("POST /api/session/activity", h.requireSession(h.activity))
	mux.HandleFunc("GET /api/system", h.requireSession(h.system))
	mux.HandleFunc("GET /api/capabilities", h.requireSession(h.capabilities))
	mux.HandleFunc("GET /api/addons/{id}", h.requireSession(h.addonData))
	mux.HandleFunc("POST /api/addons/{id}/actions/{action}", h.requireSession(h.addonAction))
	mux.HandleFunc("POST /api/rpc", h.requireSession(h.rpc))
	mux.HandleFunc("GET /api/preferences", h.requireSession(h.preferences))
	mux.HandleFunc("PUT /api/preferences/widgets", h.requireSession(h.setWidgets))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
	})
	mux.Handle("/", cfg.Static)
	return mux
}

func (h *handler) system(w http.ResponseWriter, _ *http.Request, _ auth.Session) {
	info, err := h.System.System()
	if err != nil {
		log.Printf("system info: %v", err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to read system info"})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// capabilities는 플러그인 설치 상태와 세션 역할로 애드온마다 쓸 수 있는지 알려 준다.
func (h *handler) capabilities(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	writeJSON(w, http.StatusOK, h.Addons.Capabilities(h.Plugins.Plugins(), s.Admin))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
