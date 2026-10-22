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

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/buildinfo"
	"maru/backend/internal/files"
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
	// Addons는 등록된 애드온이다. nil이면 빈 목록이다.
	Addons *addons.Registry
	// Plugins가 nil이면 아무 플러그인도 설치되지 않은 것으로 본다.
	Plugins PluginDetector
	// Prefs는 사용자 설정 저장소다. nil이면 메모리에 둔다(재시작하면 사라진다).
	Prefs prefs.Store
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
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time
}

// handler는 라우트가 공유하는 상태다.
type handler struct {
	Config
	// actions는 진행 중인 애드온 조작이다.
	actions *inflight
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
	if cfg.Site == nil {
		cfg.Site = prefs.NewMemoryStore()
	}
	if cfg.Images == nil {
		cfg.Images = prefs.NewMemoryStore()
	}
	if cfg.Security == nil {
		cfg.Security = prefs.NewMemoryStore()
	}
	if cfg.Upgrade == nil {
		// 경로가 비어 있으면 설정 파일이 없는 것으로 읽혀 꺼짐(not-enabled)이다.
		cfg.Upgrade = upgrade.New(upgrade.Paths{}, cfg.Now)
	}
	h := &handler{
		Config:          cfg,
		actions:         newInflight(),
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
	if cfg.Helper != nil || cfg.Terminal != nil {
		// 세션이 어떻게 끝나든(로그아웃, 만료, 밀려남, 2차 인증 포기) 도우미 티켓을 폐기하고
		// 그 세션의 터미널과 파일 탐색기 목록을 닫는다.
		cfg.Sessions.OnRemove = func(s auth.Session) {
			h.closeHelper(s.HelperTicket)
			h.files.RemoveSession(s.ID)
			if cfg.Terminal != nil {
				cfg.Terminal.CloseSession(s.ID, terminal.ReasonSessionEnded)
			}
		}
		cfg.Sessions.OnRemovePending = func(p auth.Pending) { h.closeHelper(p.HelperTicket) }
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
	})
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/login/verify", h.verify)
	mux.HandleFunc("POST /api/login/passkey/options", h.passkeyLoginOptions)
	mux.HandleFunc("POST /api/login/passkey", h.passkeyLoginUnlock)
	mux.HandleFunc("POST /api/login/passkey/finish", h.passkeyLoginFinish)
	mux.HandleFunc("POST /api/logout", h.logout)
	// 로그인 화면이 로그인 전에 쓰는 공개 정보. 화면 꾸밈에 쓰는 전역 설정만 담는다.
	mux.HandleFunc("GET /api/site", h.siteInfo)
	mux.HandleFunc("GET /api/site/images/login-background", h.loginBackgroundImage)
	mux.HandleFunc("GET /api/session", h.requireSession(h.session))
	mux.HandleFunc("POST /api/session/activity", h.requireSession(h.activity))
	mux.HandleFunc("GET /api/system", h.requireSession(h.system))
	mux.HandleFunc("GET /api/capabilities", h.requireSession(h.capabilities))
	mux.HandleFunc("GET /api/about", h.requireSession(h.aboutInfo))
	mux.HandleFunc("GET /api/profile", h.requireSession(h.profile))
	mux.HandleFunc("PUT /api/profile", h.requireSession(h.setProfile))
	mux.HandleFunc("GET /api/helper", h.requireSession(h.helperStatus))
	mux.HandleFunc("GET /api/helper/identity", h.requireSession(h.helperIdentity))
	mux.HandleFunc("POST /api/helper/elevation", h.requireAdmin(h.elevate))
	mux.HandleFunc("DELETE /api/helper/elevation", h.requireSession(h.dropElevation))
	mux.HandleFunc("GET /api/terminal", h.requireSession(h.terminal))
	// 파일 탐색기. 읽기는 requireFiles, 쓰기는 requireFilesWrite(보안 강화 정책을 쓰기에만 적용할 때도 막는다).
	mux.HandleFunc("GET /api/files/roots", h.requireFiles(h.filesRoots))
	mux.HandleFunc("GET /api/files/list", h.requireFiles(h.filesList))
	mux.HandleFunc("GET /api/files/list/{id}", h.requireFiles(h.filesPage))
	mux.HandleFunc("POST /api/files/list/{id}/prepare", h.requireFiles(h.filesPrepare))
	mux.HandleFunc("GET /api/files/info", h.requireFiles(h.filesInfo))
	mux.HandleFunc("GET /api/files/download", h.requireFiles(h.filesDownload))
	mux.HandleFunc("GET /api/files/preview", h.requireFiles(h.filesPreview))
	mux.HandleFunc("GET /api/files/search", h.requireFiles(h.filesSearch))
	// 올리기는 본문을 다 받기 전에 거절해도 응답을 먼저 보내고 남은 본문을 받는다(drainRejected). 사전 확인은 본문 없이
	// 같은 가드와 검사를 거친다.
	mux.HandleFunc("PUT /api/files/upload", drainRejected(h.requireFilesWrite(h.filesUpload)))
	mux.HandleFunc("POST /api/files/upload/check", h.requireFilesWrite(h.filesUploadCheck))
	mux.HandleFunc("GET /api/files/list/{id}/find", h.requireFiles(h.filesFind))
	mux.HandleFunc("POST /api/files/mkdir", h.requireFilesWrite(h.filesMkdir))
	mux.HandleFunc("POST /api/files/rename", h.requireFilesWrite(h.filesRename))
	mux.HandleFunc("POST /api/files/copy", h.requireFilesWrite(h.filesCopy))
	mux.HandleFunc("POST /api/files/delete", h.requireFilesWrite(h.filesDelete))
	mux.HandleFunc("GET /api/files/trash", h.requireFiles(h.filesTrashList))
	mux.HandleFunc("GET /api/files/trash/volumes", h.requireFiles(h.filesTrashVolumes))
	mux.HandleFunc("POST /api/files/trash", h.requireFilesWrite(h.filesTrash))
	mux.HandleFunc("POST /api/files/trash/restore", h.requireFilesWrite(h.filesTrashRestore))
	mux.HandleFunc("POST /api/files/trash/purge", h.requireFilesWrite(h.filesTrashPurge))
	mux.HandleFunc("POST /api/files/trash/empty", h.requireFilesWrite(h.filesTrashEmpty))
	mux.HandleFunc("GET /api/security", h.requireSession(h.securityInfo))
	mux.HandleFunc("POST /api/security/verify", h.requireSession(h.verifySecurity))
	mux.HandleFunc("POST /api/security/manage", h.requireSession(h.manageSecurity))
	mux.HandleFunc("POST /api/security/passkeys/challenge", h.requireSession(h.passkeyChallenge))
	mux.HandleFunc("POST /api/security/passkeys", h.requireSession(h.registerPasskey))
	mux.HandleFunc("POST /api/security/passkeys/confirm", h.requireSession(h.confirmPasskey))
	mux.HandleFunc("DELETE /api/security/passkeys/{id}", h.requireSession(h.deletePasskey))
	mux.HandleFunc("POST /api/security/totp/setup", h.requireSession(h.setupTOTP))
	mux.HandleFunc("POST /api/security/totp", h.requireSession(h.confirmTOTP))
	mux.HandleFunc("DELETE /api/security/totp", h.requireSession(h.deleteTOTP))
	mux.HandleFunc("POST /api/security/recovery-codes", h.requireSession(h.regenerateRecovery))
	mux.HandleFunc("POST /api/security/device-code", h.requireSession(h.createDeviceCode))
	mux.HandleFunc("POST /api/security/passkey-logins", h.requireSession(h.enrollPasskeyLogin))
	mux.HandleFunc("DELETE /api/security/passkey-logins/{id}", h.requireSession(h.deletePasskeyLogin))
	mux.HandleFunc("GET /api/security/users", h.requireAdmin(h.securityUsers))
	mux.HandleFunc("DELETE /api/security/users/{name}", h.requireAdmin(h.resetSecurityUser))
	mux.HandleFunc("GET /api/upgrade", h.requireAdmin(h.upgradeState))
	mux.HandleFunc("POST /api/upgrade/check", h.requireAdmin(h.upgradeCheck))
	mux.HandleFunc("POST /api/upgrade/apply", h.requireAdmin(h.upgradeApply))
	mux.HandleFunc("GET /api/addons/{id}", h.requireSession(h.addonData))
	mux.HandleFunc("POST /api/addons/{id}/actions/{action}", h.requireSession(h.addonAction))
	mux.HandleFunc("POST /api/rpc", h.requireSession(h.rpc))
	mux.HandleFunc("GET /api/preferences", h.requireSession(h.preferences))
	mux.HandleFunc("PUT /api/preferences/widgets", h.requireSession(h.setWidgets))
	mux.HandleFunc("PUT /api/preferences/shortcuts", h.requireSession(h.setShortcuts))
	mux.HandleFunc("PUT /api/preferences/drawer", h.requireSession(h.setDrawer))
	mux.HandleFunc("PUT /api/preferences/system-info", h.requireSession(h.setSystemInfo))
	mux.HandleFunc("PUT /api/preferences/files", h.requireSession(h.setFilesPrefs))
	mux.HandleFunc("PUT /api/preferences/appearance", h.requireSession(h.setAppearance))
	mux.HandleFunc("GET /api/preferences/wallpaper-image", h.requireSession(h.wallpaperImage))
	mux.HandleFunc("PUT /api/preferences/wallpaper-image", h.requireSession(h.putWallpaperImage))
	mux.HandleFunc("DELETE /api/preferences/wallpaper-image", h.requireSession(h.deleteWallpaperImage))
	mux.HandleFunc("GET /api/preferences/avatar-image", h.requireSession(h.avatarImage))
	mux.HandleFunc("PUT /api/preferences/avatar-image", h.requireSession(h.putAvatarImage))
	mux.HandleFunc("DELETE /api/preferences/avatar-image", h.requireSession(h.deleteAvatarImage))
	mux.HandleFunc("GET /api/site/images/default-wallpaper", h.requireSession(h.defaultWallpaperImage))
	mux.HandleFunc("PUT /api/site/login", h.requireAdmin(h.setSiteLogin))
	mux.HandleFunc("PUT /api/site/defaults", h.requireAdmin(h.setSiteDefaults))
	mux.HandleFunc("PUT /api/site/images/{name}", h.requireAdmin(h.putSiteImage))
	mux.HandleFunc("DELETE /api/site/images/{name}", h.requireAdmin(h.deleteSiteImage))
	mux.HandleFunc("GET /api/site/server", h.requireAdmin(h.serverSettings))
	mux.HandleFunc("PUT /api/site/server", h.requireAdmin(h.setServerSettings))
	mux.HandleFunc("GET /api/site/security", h.requireAdmin(h.securityPolicyInfo))
	mux.HandleFunc("PUT /api/site/security", h.requireAdmin(h.setSecurityPolicy))
	mux.HandleFunc("GET /api/site/files", h.requireAdmin(h.filesSettings))
	mux.HandleFunc("PUT /api/site/files", h.requireAdmin(h.setFilesSettings))
	mux.HandleFunc("POST /api/site/files/refresh", h.requireAdmin(h.refreshFilesShares))
	mux.HandleFunc("GET /api/site/files/trash", h.requireAdmin(h.getSiteTrash))
	mux.HandleFunc("POST /api/site/files/trash", h.requireAdmin(h.prepareSiteTrash))
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
// OMV 웹 UI 주소(설정했으면), 권한 도우미 설정 여부, 웹 터미널 허용 여부도 함께 알려 준다.
func (h *handler) capabilities(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	resp := h.Addons.Capabilities(h.Plugins.Plugins(), s.Admin)
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
