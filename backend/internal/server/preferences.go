package server

import (
	"log"
	"net/http"
	"regexp"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/prefs"
)

// maxPreferencesBodyBytes는 PUT /api/preferences/* 본문의 상한이다. 위젯·바로가기 64개도 넉넉히 들어간다.
const maxPreferencesBodyBytes = 64 << 10

// preferences는 로그인한 사용자의 대시보드 설정을 돌려준다. 올린 바탕화면 이미지가 있으면 그 정보도 붙인다.
func (h *handler) preferences(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	p, err := h.Prefs.Get(s.Username)
	if err == nil {
		p.WallpaperImage, err = h.userImageInfo(prefs.UserWallpaper, s.Username)
	}
	if err == nil {
		p.AvatarImage, err = h.userImageInfo(prefs.UserAvatar, s.Username)
	}
	if err != nil {
		log.Printf("preferences: read: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to read preferences")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// userImageInfo는 사용자 이미지 자리(slot: prefs.UserWallpaper 등)의 정보다. 없으면 nil.
func (h *handler) userImageInfo(slot func(string) (prefs.ImageKey, error), username string) (*api.ImageInfo, error) {
	key, err := slot(username)
	if err != nil {
		return nil, err
	}
	return h.Images.ImageInfo(key)
}

// presetPattern은 배경 프리셋 id의 형식이다. 어떤 id가 있는지는 프론트가 정한다.
var presetPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// validWallpaper는 배경 서술자의 형식을 검사한다. 없음(nil)은 기본값을 따른다는 뜻이라 받는다.
func validWallpaper(w *api.Wallpaper) bool {
	if w == nil {
		return true
	}
	switch w.Kind {
	case api.WallpaperPreset:
		return presetPattern.MatchString(w.Preset)
	case api.WallpaperImage:
		return w.Preset == ""
	}
	return false
}

// validTheme은 테마 값을 검사한다. 빈 값은 기본값을 따른다는 뜻이라 받는다.
func validTheme(theme string) bool {
	return theme == "" || theme == api.ThemeAuto || theme == api.ThemeLight || theme == api.ThemeDark
}

// validTaskbarAlign은 작업표시줄 정렬 값을 검사한다. 빈 값은 기본값(가운데)을 따른다는 뜻이라 받는다.
func validTaskbarAlign(align string) bool {
	return align == "" || align == api.TaskbarAlignCenter || align == api.TaskbarAlignLeft
}

// setAppearance는 화면 설정 전체를 바꾼다. 다른 설정은 그대로 둔다. 바탕화면을 올린 이미지로 고르는 것은
// 이미지가 없어도 받는다(프론트가 기본 배경으로 그린다).
func (h *handler) setAppearance(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.AppearanceRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Appearance == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "appearance is required")
		return
	}
	if !validTheme(req.Appearance.Theme) || !validWallpaper(req.Appearance.Wallpaper) ||
		!validTaskbarAlign(req.Appearance.TaskbarAlign) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid appearance")
		return
	}
	next := *req.Appearance
	if err := h.Prefs.UpdateAppearance(s.Username, func(a *api.AppearancePrefs) { *a = next }); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// wallpaperImage는 로그인한 사용자가 올린 바탕화면 이미지를 보낸다.
func (h *handler) wallpaperImage(w http.ResponseWriter, r *http.Request, s auth.Session) {
	key, err := prefs.UserWallpaper(s.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "failed to read image")
		return
	}
	h.serveImage(w, r, key)
}

// putWallpaperImage는 바탕화면 이미지를 올리고 바탕화면을 그 이미지로 바꾼다. 응답은 이미지 정보다.
func (h *handler) putWallpaperImage(w http.ResponseWriter, r *http.Request, s auth.Session) {
	key, err := prefs.UserWallpaper(s.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "failed to save image")
		return
	}
	info, ok := h.putImage(w, r, key, maxImageBytes)
	if !ok {
		return
	}
	if err := h.Prefs.UpdateAppearance(s.Username, func(a *api.AppearancePrefs) {
		a.Wallpaper = &api.Wallpaper{Kind: api.WallpaperImage}
	}); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// deleteWallpaperImage는 올린 바탕화면 이미지를 지운다. 바탕화면이 그 이미지였으면 기본값을 따르게 한다.
func (h *handler) deleteWallpaperImage(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	key, err := prefs.UserWallpaper(s.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "failed to delete image")
		return
	}
	if !h.deleteImage(w, key) {
		return
	}
	if err := h.Prefs.UpdateAppearance(s.Username, func(a *api.AppearancePrefs) {
		if a.Wallpaper != nil && a.Wallpaper.Kind == api.WallpaperImage {
			a.Wallpaper = nil
		}
	}); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setWidgets는 위젯 배치 전체를 바꾼다. 다른 설정은 그대로 둔다.
func (h *handler) setWidgets(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.WidgetsRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Widgets == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "widgets is required")
		return
	}
	if err := h.Addons.ValidateWidgets(req.Widgets); err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, err.Error())
		return
	}
	if err := h.Prefs.SetWidgets(s.Username, req.Widgets); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setShortcuts는 바로가기 목록 전체를 바꾼다. 다른 설정은 그대로 둔다.
func (h *handler) setShortcuts(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.ShortcutsRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Shortcuts == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "shortcuts is required")
		return
	}
	if err := addons.ValidateShortcuts(req.Shortcuts); err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, err.Error())
		return
	}
	if err := h.Prefs.SetShortcuts(s.Username, req.Shortcuts); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validDrawerSort는 저장할 수 있는 "모든 앱" 정렬 방식인지다.
func validDrawerSort(sort string) bool {
	return sort == api.DrawerSortRecommended || sort == api.DrawerSortName
}

// setDrawer는 "모든 앱" 화면 설정 전체를 바꾼다. 다른 설정은 그대로 둔다.
func (h *handler) setDrawer(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.DrawerRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Drawer == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "drawer is required")
		return
	}
	if !validDrawerSort(req.Drawer.Sort) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown drawer sort")
		return
	}
	if err := h.Prefs.SetDrawer(s.Username, *req.Drawer); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setSystemInfo는 시스템 정보 창 설정 전체를 바꾼다. 다른 설정은 그대로 둔다.
func (h *handler) setSystemInfo(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.SystemInfoRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.SystemInfo == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "systemInfo is required")
		return
	}
	p := *req.SystemInfo
	if p.CPUScale != api.CPUScaleMachine && p.CPUScale != api.CPUScaleCore {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown cpu scale")
		return
	}
	if p.CPUGraph != api.CPUGraphOverall && p.CPUGraph != api.CPUGraphCores {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown cpu graph")
		return
	}
	if err := h.Prefs.SetSystemInfo(s.Username, p); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setFilesPrefs는 파일 탐색기 보기 설정 전체를 바꾼다. 다른 설정은 그대로 둔다.
func (h *handler) setFilesPrefs(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.FilesPrefsRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Files == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "files is required")
		return
	}
	if v := req.Files.View; v != api.FilesViewList && v != api.FilesViewGrid {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown files view")
		return
	}
	if err := h.Prefs.SetFiles(s.Username, *req.Files); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// avatarImage는 로그인한 사용자의 프로필 사진을 보낸다. 자기 사진만 받는다.
func (h *handler) avatarImage(w http.ResponseWriter, r *http.Request, s auth.Session) {
	key, err := prefs.UserAvatar(s.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "failed to read image")
		return
	}
	h.serveImage(w, r, key)
}

// putAvatarImage는 프로필 사진을 바꾼다. 브라우저가 잘라 줄인 이미지를 받는다(maxAvatarBytes 이하). 응답은 이미지 정보다.
func (h *handler) putAvatarImage(w http.ResponseWriter, r *http.Request, s auth.Session) {
	key, err := prefs.UserAvatar(s.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "failed to save image")
		return
	}
	if info, ok := h.putImage(w, r, key, maxAvatarBytes); ok {
		writeJSON(w, http.StatusOK, info)
	}
}

// deleteAvatarImage는 프로필 사진을 지운다. 이후 기본 아바타를 그린다.
func (h *handler) deleteAvatarImage(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	key, err := prefs.UserAvatar(s.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "", "failed to delete image")
		return
	}
	if h.deleteImage(w, key) {
		w.WriteHeader(http.StatusNoContent)
	}
}
