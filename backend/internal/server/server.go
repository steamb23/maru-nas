// Package server는 HTTP 라우트와 핸들러를 연결한다.
//
// 라우트는 대시보드 루트(/api/...)에 둔다. 대시보드를 경로 접두사 아래에 둘 때는
// 리버스 프록시가 접두사를 제거한다.
package server

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"time"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/buildinfo"
	"maru/backend/internal/files"
	"maru/backend/internal/jobs"
	"maru/backend/internal/jobs/omvjob"
	"maru/backend/internal/notify"
	"maru/backend/internal/omv"
	"maru/backend/internal/prefs"
	"maru/backend/internal/security"
	"maru/backend/internal/terminal"
	"maru/backend/internal/upgrade"
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

// PluginDetector는 OMV 플러그인 설치 여부와 OMV 버전을 알려 준다. *addons.Detector가 구현한다.
type PluginDetector interface {
	// Plugins는 설치된 플러그인 패키지 집합이다.
	Plugins() map[string]bool
	// OMV는 설치된 OMV의 버전과 코드명이다. 모르는 값은 빈 문자열이다.
	OMV() addons.OMVInfo
	// Refresh는 바로 다시 확인한다.
	Refresh()
	// Invalidate는 다음 Plugins 호출 때 다시 확인하게 한다.
	Invalidate()
}

// noPlugins는 아무 플러그인도 설치되지 않고 OMV 버전도 모르는 것으로 보는 PluginDetector다.
type noPlugins struct{}

func (noPlugins) Plugins() map[string]bool { return map[string]bool{} }
func (noPlugins) OMV() addons.OMVInfo      { return addons.OMVInfo{} }
func (noPlugins) Refresh()                 {}
func (noPlugins) Invalidate()              {}

// Config는 New의 설정이다. System, Static, OMV는 필수다.
type Config struct {
	System SystemSource
	// Static은 /api/ 밖의 요청을 처리한다(프론트엔드 빌드 결과).
	Static http.Handler
	OMV    OMV
	// Addons는 내장 애드온이다. nil이면 빈 목록이다. /api/rpc 중계는 이 레지스트리만 본다.
	Addons *addons.Registry
	// Packages는 애드온 패키지다(설치본, 개발본). 기준 레지스트리는 Addons와 같아야 한다. nil이면 패키지를 쓰지 않는다.
	Packages *addonpkg.Manager
	// AddonHost는 애드온 패키지 백엔드의 소켓 위치다(백엔드 소켓, 호스트 소켓). nil이면 이 설치에서 백엔드가 꺼져 있다
	// (백엔드 패키지는 capability backend_unavailable, 데이터·조작은 503 addon_backend_disabled). 설정하면 백엔드
	// 패키지마다 호스트 소켓을 열므로 NewServer로 만들고 끝낼 때 Close한다. Packages의 BackendsEnabled와 같아야 한다.
	AddonHost *addonpkg.BackendPaths
	// AddonSync는 운영 백엔드의 root 동기화 파일 위치다(설치기 --addon-backends가 켠 설치). 있으면 레지스트리가 바뀔
	// 때마다 원하는 상태를 쓰고 동기화를 요청하며, 관리자 페이지가 동기화 결과를 보인다. nil이면 쓰지 않는다(백엔드가
	// 꺼진 설치, 개발 서버의 개발 백엔드). AddonHost가 있을 때만 쓴다.
	AddonSync *addonpkg.SyncPaths
	// AddonSDK는 애드온 프레임에 주는 SDK 런타임(sdk.js, maru.css)이 든 파일 시스템이다. nil이면 404다.
	AddonSDK fs.FS
	// Plugins가 nil이면 아무 플러그인도 설치되지 않은 것으로 본다.
	Plugins PluginDetector
	// Prefs는 사용자 설정 저장소다. nil이면 메모리에 둔다(재시작하면 사라진다).
	Prefs prefs.Store
	// AddonData는 애드온 패키지의 사용자별 저장값이다. nil이면 Prefs가 구현하면 그것을, 아니면 메모리를 쓴다.
	AddonData prefs.AddonDataStore
	// Site는 전역 설정, Images는 배경 이미지 저장소다. nil이면 메모리에 둔다.
	Site   prefs.SiteStore
	Images prefs.ImageStore

	// Sessions, Limiter가 nil이면 새로 만든다.
	Sessions *auth.Store
	Limiter  *auth.Limiter
	// CookiePath는 브라우저가 보는 대시보드 경로다(리버스 프록시가 떼기 전). 기본 /.
	CookiePath string
	// WorkbenchURL은 브라우저에서 OMV 웹 UI를 열 주소다(-workbench-url). /api/capabilities로 알려 준다.
	// 비우면 알리지 않는다. 설정의 서버 페이지에서 덮어쓴 값(site.json)이 있으면 그 값이 우선한다.
	WorkbenchURL string
	// SmartRefreshAt은 -smart-refresh-at(HH:MM)이다. 비우면 00:00. 덮어쓴 값이 있으면 그 값이 우선한다.
	SmartRefreshAt string
	// SetSmartRefreshAt은 SMART 캐시의 기준 시각을 바꾼다. 시작할 때와 덮어쓰기를 저장할 때 부른다. nil이면 부르지 않는다.
	SetSmartRefreshAt func(hour, minute int)
	// Runtime은 시작할 때 정해진 가동 설정 전체다. 설정의 서버 페이지(관리자)에 보여 준다.
	Runtime []api.RuntimeSetting
	// Helper는 권한 도우미 클라이언트다(-helper-socket). nil이면 도우미 기능을 끈다.
	// 설정하면 Sessions의 삭제 훅(OnRemove, OnRemovePending)을 이 핸들러가 쓴다.
	Helper Helper
	// Terminal은 웹 터미널의 SSH 접속이다. nil이면 웹 터미널을 쓸 수 없다(테스트 등).
	// 설정하면 Sessions의 삭제 훅(OnRemove)을 이 핸들러가 쓴다.
	Terminal *terminal.Manager
	// TerminalEnabled는 -terminal이다. 설정의 서버 페이지에서 덮어쓴 값이 있으면 그 값이 우선한다.
	TerminalEnabled bool
	// Mounts는 /srv 아래 마운트(파일 탐색기의 디스크)다. nil이면 디스크를 보이지 않는다.
	Mounts MountSource
	// FilesSrvDir는 데이터 디스크가 마운트되는 디렉터리다. 비우면 /srv. 개발 서버가 가짜 도우미의 루트로 바꾼다.
	FilesSrvDir string
	// Upgrade는 대시보드에서 업그레이드의 파일(상태 파일, 설정 파일, 확인 요청)이다. nil이면 꺼진 것으로 본다.
	Upgrade *upgrade.Manager
	// Notify는 서버 알림을 모은다. nil이면 알림 출처가 없다.
	Notify *notify.Center
	// Jobs는 서버 백그라운드 작업(시스템 업데이트)이다. nil이면 메모리에만 기록한다. New가 OMV 세션 감시자를
	// 등록하고 남은 작업을 이어 지켜본다(Resume). 서비스를 끝낼 때 부르는 쪽이 Close한다.
	Jobs *jobs.Manager
	// Security는 보안 강화 모드의 수단 기록 저장소다. nil이면 메모리에 둔다.
	Security security.Store
	// RP는 패스키의 신뢰 당사자(-public-origin)다. nil이면 패스키를 쓸 수 없다(TOTP·복구 코드만 쓴다).
	RP *security.RP
	// Hostname은 인증 앱에 보이는 계정 이름(<사용자>@<호스트>)에 쓴다. 비우면 사용자 이름만 쓴다.
	Hostname string
	// CheckInterval마다 요청 처리 중에 OMV 세션이 살아 있는지 확인한다. 기본 60초.
	// 예상 만료 시각이 지났으면 주기와 상관없이 확인한다.
	CheckInterval time.Duration
	// FailDelay는 로그인 실패 응답 전에 기다리는 시간이다. 기본 1초, 음수면 기다리지 않는다.
	FailDelay time.Duration
	// JobInterval은 작업 감시자가 OMV 작업 출력을 읽는 간격이다. 0이면 1초(테스트에서 줄인다).
	JobInterval time.Duration
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time
}

// handler는 라우트가 공유하는 상태다.
type handler struct {
	Config
	// actions는 진행 중인 애드온 조작이다.
	actions *inflight
	// uiTokens는 애드온 프레임 파일 경로의 토큰이다(세션·패키지마다 하나, addon_ui.go).
	uiTokens *uiTokens
	// uploads는 검토를 기다리는 애드온 패키지다(addon_admin.go).
	uploads *pendingUploads
	// backends는 패키지 백엔드 클라이언트, hosts는 백엔드 패키지마다 연 호스트 소켓이다(addon_host.go). 둘 다
	// Config.AddonHost가 있을 때만 있다. hostCalls는 호스트 소켓의 호출 토큰이다.
	backends  *addonpkg.BackendClient
	hosts     *addonHosts
	hostCalls *hostCalls
	// sync는 운영 백엔드의 root 동기화다(Config.AddonSync, addon_sync.go). 없으면 nil이다.
	sync *addonpkg.Sync
	// backendDataTimeout은 패키지 데이터 요청의 시간 제한이다(테스트에서 줄인다).
	backendDataTimeout time.Duration
	// about은 실행 파일의 버전·빌드 정보다. 실행 중에 바뀌지 않으므로 시작할 때 한 번 읽는다.
	about api.AboutResponse
	// over는 웹에서 덮어쓴 가동 설정이다(site.go).
	over overrides
	// terminalLimiter는 웹 터미널의 비밀번호 실패 제한이다. sshd·fail2ban에는 접속지가 루프백으로만
	// 보여 원래 IP로 막을 수 없으므로 여기서 센다. 로그인 제한과 따로 둔다(입력을 기다리는 동안
	// 동시 시도 칸을 차지하므로).
	terminalLimiter *auth.Limiter
	// securityLimiter는 2차 수단(TOTP, 복구 코드, 패스키) 확인의 실패 제한이다. 비밀번호 재확인은 로그인 제한을 쓴다.
	securityLimiter *auth.Limiter
	// policy는 지금 보안 강화 정책이다(security.go).
	policy policyState
	// files는 파일 탐색기의 세션별 폴더 목록과 홈 폴더, filesSite는 파일 탐색기 전역 설정이다(files.go).
	files     *files.Cache
	filesSite filesSiteState
	// challenges는 진행 중인 패스키 확인·등록, pendingPasskeys는 등록 값은 맞았지만 아직 시험 확인을 하지 않은
	// 패스키, totpSetup은 설정 중인 TOTP 비밀이다. 대시보드 세션에 묶인다.
	challenges      *security.Ephemeral[ceremony]
	pendingPasskeys *security.Ephemeral[pendingPasskey]
	totpSetup       *security.Ephemeral[string]
	// deviceCodes는 기기 추가 코드의 해시다. 다른 기기의 세션이 쓰므로 사용자 이름에 묶인다(security_manage.go).
	deviceCodes *security.Ephemeral[string]
	// loginCeremonies는 진행 중인 패스키로 로그인(options → 확인), passkeyUnlocks는 확인을 마치고 share를 내준 뒤
	// 비밀번호를 기다리는 로그인이다. 대시보드 세션이 아직 없으므로 의식 쿠키(PasskeyLoginCookie)에 묶인다.
	loginCeremonies *security.Ephemeral[loginCeremony]
	passkeyUnlocks  *security.Ephemeral[passkeyUnlock]
}

// Server는 NewServer가 만든 루트 핸들러와 종료 처리다.
type Server struct {
	http.Handler
	h *handler
}

// Close는 애드온 호스트 소켓을 닫는다(Config.AddonHost). 처리 중인 호스트 요청도 끊긴다. 그 밖의 정리(작업 기록 등)는
// 각 설정의 주인이 한다.
func (s *Server) Close() {
	h := s.h
	if h.hosts == nil {
		return
	}
	// 더 이상 레지스트리 변경을 받지 않게 한 뒤(진행 중인 reconcile이 끝날 때까지 기다린다) 소켓을 닫는다.
	h.Packages.OnChange(nil)
	h.hosts.close()
}

// New는 루트 핸들러를 반환한다. /api/ 밖의 요청은 cfg.Static이 처리한다. 애드온 호스트 소켓(Config.AddonHost)을
// 쓰면 닫을 수 있게 NewServer를 쓴다.
func New(cfg Config) http.Handler { return NewServer(cfg).Handler }

// NewServer는 루트 핸들러를 만든다. Config.AddonHost가 있으면 백엔드 패키지마다 호스트 소켓을 열고, 레지스트리가
// 바뀔 때마다 맞춘다(Close로 닫는다).
func NewServer(cfg Config) *Server {
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
	if cfg.AddonData == nil {
		if ad, ok := cfg.Prefs.(prefs.AddonDataStore); ok {
			cfg.AddonData = ad
		} else {
			cfg.AddonData = prefs.NewMemoryStore()
		}
	}
	if cfg.Site == nil {
		cfg.Site = prefs.NewMemoryStore()
	}
	if cfg.Images == nil {
		cfg.Images = prefs.NewMemoryStore()
	}
	if cfg.Security == nil {
		cfg.Security = prefs.NewMemoryStore()
	}
	if cfg.Notify == nil {
		cfg.Notify = notify.New()
	}
	if cfg.Upgrade == nil {
		// 경로가 비어 있으면 설정 파일이 없는 것으로 읽혀 꺼짐(not-enabled)이다.
		cfg.Upgrade = upgrade.New(upgrade.Paths{}, cfg.Now)
	}
	if cfg.Jobs == nil {
		cfg.Jobs = jobs.New(jobs.Config{Now: cfg.Now})
	}
	h := &handler{
		Config:          cfg,
		actions:         newInflight(),
		uiTokens:        newUITokens(),
		uploads:         newPendingUploads(cfg.Now),
		about:           buildinfo.Read(),
		terminalLimiter: auth.NewLimiter(),
		securityLimiter: auth.NewLimiter(),
		challenges:      security.NewEphemeral[ceremony](challengeTTL),
		pendingPasskeys: security.NewEphemeral[pendingPasskey](challengeTTL),
		totpSetup:       security.NewEphemeral[string](totpSetupTTL),
		deviceCodes:     security.NewEphemeral[string](deviceCodeTTL),
		loginCeremonies: security.NewEphemeral[loginCeremony](challengeTTL),
		passkeyUnlocks:  security.NewEphemeral[passkeyUnlock](passkeyUnlockTTL),
		files:           files.NewCache(),
		hostCalls:       newHostCalls(),
		// 패키지 데이터 요청의 시간 제한(.claude/docs/addon-packages.md의 "maru ↔ 백엔드").
		backendDataTimeout: addonpkg.BackendDataTimeout,
	}
	h.files.Now = cfg.Now
	for _, l := range []*auth.Limiter{h.terminalLimiter, h.securityLimiter} {
		l.Now = cfg.Now
	}
	h.challenges.Now, h.pendingPasskeys.Now, h.totpSetup.Now, h.deviceCodes.Now = cfg.Now, cfg.Now, cfg.Now, cfg.Now
	h.loginCeremonies.Now, h.passkeyUnlocks.Now = cfg.Now, cfg.Now
	h.loadOverrides()
	h.loadPolicy()
	h.loadFilesSite()
	// 작업은 관리자 OMV 세션으로 지켜본다. 상태 디렉터리에 남은 작업(재시작 전에 시작한 것)을 이어 지켜본다.
	runner := omvjob.New(jobSessions{h}, cfg.Jobs.IndexModTime)
	runner.Now = cfg.Now
	if d := cfg.JobInterval; d > 0 {
		runner.Interval, runner.Backoff, runner.WaitRetry = d, []time.Duration{d}, 100*d
	}
	cfg.Jobs.Register(runner)
	cfg.Jobs.Resume()
	// 세션이 어떻게 끝나든(로그아웃, 만료, 밀려남, 2차 인증 포기) 도우미 티켓을 폐기하고
	// 그 세션의 터미널, 파일 탐색기 목록, 애드온 프레임 토큰, 검토 대기를 닫는다.
	cfg.Sessions.OnRemove = func(s auth.Session) {
		h.uiTokens.removeSession(s.ID)
		h.uploads.removeSession(s.ID)
		h.hostCalls.removeSession(s.ID)
		if cfg.Helper != nil {
			h.closeHelper(s.HelperTicket)
		}
		h.files.RemoveSession(s.ID)
		if cfg.Terminal != nil {
			cfg.Terminal.CloseSession(s.ID, terminal.ReasonSessionEnded)
		}
	}
	if cfg.Helper != nil {
		cfg.Sessions.OnRemovePending = func(p auth.Pending) { h.closeHelper(p.HelperTicket) }
	}
	if cfg.Packages != nil {
		if cfg.Packages.BackendsEnabled() != (cfg.AddonHost != nil) {
			log.Printf("addon packages: backend setting of the package manager does not match the server")
		}
		if cfg.AddonHost != nil {
			h.backends = addonpkg.NewBackendClient(cfg.AddonHost.Backend)
			h.hosts = newAddonHosts(h, *cfg.AddonHost)
			if cfg.AddonSync != nil {
				h.sync = addonpkg.NewSync(*cfg.AddonSync, cfg.Now, cfg.Packages.Active)
			}
			// 지금 레지스트리로 한 번 맞추고(시작할 때의 첫 동기화 요청도 여기서 나간다), 바뀔 때마다 다시 맞춘다.
			cfg.Packages.OnChange(h.packagesChanged)
		} else if cfg.AddonSync != nil {
			log.Printf("addon packages: backend sync paths without backend paths are ignored")
		}
	}
	return &Server{Handler: h.routes(), h: h}
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
// OMV 웹 UI 주소(설정했으면), 권한 도우미 설정 여부, 웹 터미널 허용 여부도 함께 알려 준다.
func (h *handler) capabilities(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	if h.Packages != nil {
		// 개발 패키지 파일이 바뀌었으면 다시 불러온다(rev가 바뀌어 열린 프레임이 새로 뜬다).
		h.Packages.RescanDev()
	}
	resp := h.addons().Capabilities(h.Plugins.Plugins(), s.Admin)
	// 이 설치에서 백엔드가 꺼져 있으면 백엔드 패키지는 쓸 수 없다(backend_unavailable).
	for i, c := range resp.Addons {
		if c.Available && h.backendDisabled(c.ID) {
			resp.Addons[i] = api.AddonCapability{ID: c.ID, Reason: api.ReasonBackendUnavailable, Sources: []string{}}
		}
	}
	resp.AddonPackages = h.packageViews(s)
	resp.WorkbenchURL = h.workbenchURL()
	resp.Helper = h.Helper != nil
	resp.Terminal = h.terminalEnabled()
	writeJSON(w, http.StatusOK, resp)
}

// aboutInfo는 마루 NAS 버전과 빌드 정보다. 로그인한 사용자는 누구나 본다.
// OMV 버전·코드명은 관리자에게만 준다. OMV도 System.getInformation에서 version을 관리자에게만 준다.
// 실행 중에 OMV를 업그레이드할 수 있으므로 시작할 때 읽어 둔 h.about에 넣지 않고 요청마다 채운다.
func (h *handler) aboutInfo(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	resp := h.about
	if s.Admin {
		omv := h.Plugins.OMV()
		resp.OMVVersion, resp.OMVVersionName = omv.Version, omv.VersionName
	}
	writeJSON(w, http.StatusOK, resp)
}
