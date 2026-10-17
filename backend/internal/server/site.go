package server

import (
	"log"
	"net/http"
	"strconv"
	"sync"

	"onmaru/backend/internal/addons/smart"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/prefs"
	"onmaru/backend/internal/terminal"
)

// 전역 설정(관리자): 로그인 화면, 기본 화면, 웹에서 덮어쓰는 가동 설정.
// 로그인 화면이 로그인 전에 쓰는 정보(GET /api/site, 로그인 배경 이미지)만 인증 없이 준다.

// defaultSmartRefreshAt은 -smart-refresh-at의 기본값이다.
const defaultSmartRefreshAt = "00:00"

// overrides는 웹에서 덮어쓴 가동 설정의 지금 값이다. 이 프로세스만 site.json을 쓰므로 메모리에 두고
// 저장할 때 함께 바꾼다. nil이면 덮어쓰지 않은 것이다.
type overrides struct {
	mu        sync.Mutex
	workbench *string
	smartAt   *string
	terminal  *string
}

// requireAdmin은 관리자 세션이어야 next를 부른다. 관리자가 아니면 403 admin_required다.
func (h *handler) requireAdmin(next func(http.ResponseWriter, *http.Request, auth.Session)) http.HandlerFunc {
	return h.requireSession(func(w http.ResponseWriter, r *http.Request, s auth.Session) {
		if !s.Admin {
			writeError(w, http.StatusForbidden, api.ErrAdminRequired, "administrator required")
			return
		}
		next(w, r, s)
	})
}

// loadOverrides는 시작할 때 저장된 가동 설정 덮어쓰기를 반영한다. 읽을 수 없으면 플래그 값으로 시작한다.
func (h *handler) loadOverrides() {
	site, err := h.Site.GetSite()
	if err != nil {
		log.Printf("site settings: read: %v", err)
		return
	}
	if site.Server != nil {
		h.applyOverrides(*site.Server)
	}
}

// applyOverrides는 덮어쓰기 값을 반영한다. 형식이 틀린 값(손으로 고친 파일 등)은 로그만 남기고 무시한다.
func (h *handler) applyOverrides(o api.ServerOverrides) {
	if o.WorkbenchURL != nil {
		if err := CheckWorkbenchURL(*o.WorkbenchURL); err != nil {
			log.Printf("site settings: ignoring workbench url override: %v", err)
			o.WorkbenchURL = nil
		}
	}
	if o.Terminal != nil && !validBool(*o.Terminal) {
		log.Printf("site settings: ignoring terminal override %q", *o.Terminal)
		o.Terminal = nil
	}
	smartAt := h.smartBase()
	if o.SmartRefreshAt != nil {
		if _, _, err := smart.ParseRefreshAt(*o.SmartRefreshAt); err != nil {
			log.Printf("site settings: ignoring smart refresh time override: %v", err)
			o.SmartRefreshAt = nil
		} else {
			smartAt = *o.SmartRefreshAt
		}
	}
	h.over.mu.Lock()
	h.over.workbench, h.over.smartAt, h.over.terminal = o.WorkbenchURL, o.SmartRefreshAt, o.Terminal
	h.over.mu.Unlock()
	if h.Terminal != nil && !h.terminalEnabled() {
		// 웹 터미널을 끄면 열린 터미널을 바로 닫는다.
		h.Terminal.CloseAll(terminal.ReasonDisabled)
	}
	if h.SetSmartRefreshAt != nil {
		if hour, minute, err := smart.ParseRefreshAt(smartAt); err == nil {
			h.SetSmartRefreshAt(hour, minute)
		}
	}
}

// validBool은 bool 가동 설정의 덮어쓰기 값("true" 또는 "false")인지다.
func validBool(v string) bool { return v == "true" || v == "false" }

func (h *handler) smartBase() string {
	if h.SmartRefreshAt == "" {
		return defaultSmartRefreshAt
	}
	return h.SmartRefreshAt
}

// workbenchURL은 지금 쓰는 OMV 웹 UI 주소다(웹에서 덮어쓴 값, 없으면 -workbench-url).
func (h *handler) workbenchURL() string {
	h.over.mu.Lock()
	defer h.over.mu.Unlock()
	if h.over.workbench != nil {
		return *h.over.workbench
	}
	return h.WorkbenchURL
}

// siteInfo는 로그인 화면이 쓰는 공개 전역 정보다. 로그인 전에도 준다.
func (h *handler) siteInfo(w http.ResponseWriter, _ *http.Request) {
	site, err := h.Site.GetSite()
	if err != nil {
		log.Printf("site settings: read: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to read site settings")
		return
	}
	info := api.SiteInfo{LoginEasterEgg: true}
	if site.Login != nil {
		info.LoginBackground = site.Login.Background
		info.LoginEasterEgg = site.Login.EasterEgg
	}
	if site.Defaults != nil {
		info.DefaultTheme = site.Defaults.Theme
		info.DefaultWallpaper = site.Defaults.Wallpaper
	}
	for name, dst := range map[string]**api.ImageInfo{
		prefs.SiteLoginBackground:  &info.Images.LoginBackground,
		prefs.SiteDefaultWallpaper: &info.Images.DefaultWallpaper,
	} {
		key, _ := prefs.SiteImage(name)
		if *dst, err = h.Images.ImageInfo(key); err != nil {
			log.Printf("site settings: image info: %v", err)
			writeError(w, http.StatusInternalServerError, "", "failed to read site settings")
			return
		}
	}
	writeJSON(w, http.StatusOK, info)
}

// loginBackgroundImage는 관리자가 올린 로그인 배경 이미지다. 로그인 화면에 보이는 그림이라 인증 없이 준다.
func (h *handler) loginBackgroundImage(w http.ResponseWriter, r *http.Request) {
	key, _ := prefs.SiteImage(prefs.SiteLoginBackground)
	h.serveImage(w, r, key)
}

// defaultWallpaperImage는 관리자가 올린 기본 바탕화면 이미지다. 로그인한 사용자에게만 준다.
func (h *handler) defaultWallpaperImage(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	key, _ := prefs.SiteImage(prefs.SiteDefaultWallpaper)
	h.serveImage(w, r, key)
}

// updateSite는 전역 설정을 바꾼다. 실패하면 오류 응답을 쓰고 false다.
func (h *handler) updateSite(w http.ResponseWriter, apply func(*api.SiteSettings)) bool {
	if err := h.Site.UpdateSite(apply); err != nil {
		log.Printf("site settings: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save site settings")
		return false
	}
	return true
}

// setSiteLogin은 로그인 화면 설정 전체를 바꾼다.
func (h *handler) setSiteLogin(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	var req api.SiteLoginRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Login == nil || !validWallpaper(req.Login.Background) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid login screen settings")
		return
	}
	next := *req.Login
	if h.updateSite(w, func(v *api.SiteSettings) { v.Login = &next }) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// setSiteDefaults는 기본 화면 전체를 바꾼다.
func (h *handler) setSiteDefaults(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	var req api.SiteDefaultsRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Defaults == nil || !validTheme(req.Defaults.Theme) || !validWallpaper(req.Defaults.Wallpaper) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid default screen settings")
		return
	}
	next := *req.Defaults
	if h.updateSite(w, func(v *api.SiteSettings) { v.Defaults = &next }) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// siteImageKey는 경로의 전역 이미지 이름으로 자리를 찾는다. 모르는 이름이면 404를 쓰고 false다.
func siteImageKey(w http.ResponseWriter, r *http.Request) (prefs.ImageKey, string, bool) {
	name := r.PathValue("name")
	key, ok := prefs.SiteImage(name)
	if !ok {
		writeError(w, http.StatusNotFound, "", "not found")
	}
	return key, name, ok
}

// siteImageSlot은 전역 이미지 이름에 맞는 배경 서술자 자리다. 저장한 적 없는 부분은 기본값으로 만든다
// (로그인 화면은 대기 연출이 켜진 상태가 기본이다).
func siteImageSlot(v *api.SiteSettings, name string) **api.Wallpaper {
	if name == prefs.SiteLoginBackground {
		if v.Login == nil {
			v.Login = &api.SiteLogin{EasterEgg: true}
		}
		return &v.Login.Background
	}
	if v.Defaults == nil {
		v.Defaults = &api.SiteDefaults{}
	}
	return &v.Defaults.Wallpaper
}

// putSiteImage는 전역 배경 이미지를 올리고 그 자리의 배경을 이 이미지로 바꾼다. 응답은 이미지 정보다.
func (h *handler) putSiteImage(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	key, name, ok := siteImageKey(w, r)
	if !ok {
		return
	}
	info, ok := h.putImage(w, r, key, maxImageBytes)
	if !ok {
		return
	}
	if h.updateSite(w, func(v *api.SiteSettings) {
		*siteImageSlot(v, name) = &api.Wallpaper{Kind: api.WallpaperImage}
	}) {
		writeJSON(w, http.StatusOK, info)
	}
}

// deleteSiteImage는 전역 배경 이미지를 지운다. 그 자리의 배경이 이 이미지였으면 기본 배경으로 되돌린다.
func (h *handler) deleteSiteImage(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	key, name, ok := siteImageKey(w, r)
	if !ok || !h.deleteImage(w, key) {
		return
	}
	if h.updateSite(w, func(v *api.SiteSettings) {
		slot := siteImageSlot(v, name)
		if *slot != nil && (*slot).Kind == api.WallpaperImage {
			*slot = nil
		}
	}) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// serverSettings는 웹에서 덮어쓸 수 있는 가동 설정과 시작할 때 정해진 가동 설정 전체다.
func (h *handler) serverSettings(w http.ResponseWriter, _ *http.Request, _ auth.Session) {
	h.over.mu.Lock()
	wb, sa, term := h.over.workbench, h.over.smartAt, h.over.terminal
	h.over.mu.Unlock()
	writeJSON(w, http.StatusOK, api.ServerSettings{
		WorkbenchURL:   overridable(h.WorkbenchURL, wb),
		SmartRefreshAt: overridable(h.smartBase(), sa),
		Terminal:       overridable(strconv.FormatBool(h.TerminalEnabled), term),
		Runtime:        append([]api.RuntimeSetting{}, h.Runtime...),
	})
}

func overridable(base string, over *string) api.OverridableSetting {
	s := api.OverridableSetting{Value: base, Base: base}
	if over != nil {
		s.Value, s.Overridden = *over, true
	}
	return s
}

// setServerSettings는 덮어쓰기 값 전체를 바꾸고 바로 반영한다. 없는 값은 플래그·환경 변수 값으로 돌아간다.
func (h *handler) setServerSettings(w http.ResponseWriter, r *http.Request, _ auth.Session) {
	var req api.ServerSettingsRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Server == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "server is required")
		return
	}
	next := *req.Server
	if next.WorkbenchURL != nil {
		if err := CheckWorkbenchURL(*next.WorkbenchURL); err != nil {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid workbench url")
			return
		}
	}
	if next.SmartRefreshAt != nil {
		if _, _, err := smart.ParseRefreshAt(*next.SmartRefreshAt); err != nil {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid smart refresh time")
			return
		}
	}
	if next.Terminal != nil && !validBool(*next.Terminal) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid terminal setting")
		return
	}
	if !h.updateSite(w, func(v *api.SiteSettings) {
		if next.WorkbenchURL == nil && next.SmartRefreshAt == nil && next.Terminal == nil {
			v.Server = nil
		} else {
			v.Server = &next
		}
	}) {
		return
	}
	h.applyOverrides(next)
	w.WriteHeader(http.StatusNoContent)
}
