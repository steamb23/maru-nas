// Package server는 HTTP 라우트와 핸들러를 연결한다.
//
// 라우트는 대시보드 루트(/api/...)에 둔다. 외부 접두사 /desk는
// 리버스 프록시(개발: Vite, 운영: Caddy)가 제거한다.
package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"omv-desktop-dashboard/backend/internal/api"
	"omv-desktop-dashboard/backend/internal/auth"
	"omv-desktop-dashboard/backend/internal/omv"
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
}

// Config는 New의 설정이다. System, Static, OMV는 필수다.
type Config struct {
	System SystemSource
	// Static은 /api/ 밖의 요청을 처리한다(프론트엔드 빌드 결과).
	Static http.Handler
	OMV    OMV

	// Sessions, Limiter가 nil이면 새로 만든다.
	Sessions *auth.Store
	Limiter  *auth.Limiter
	// CookiePath는 브라우저가 보는 대시보드 경로다(리버스 프록시가 떼기 전). 기본 /desk.
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
		cfg.CookiePath = "/desk"
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
	h := &handler{cfg}

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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
