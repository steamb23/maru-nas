package server

import (
	"bytes"
	"net/http"
	"slices"
	"testing"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/prefs"
)

func TestSiteInfoIsPublic(t *testing.T) {
	e := newEnv(t, envOptions{workbenchURL: "https://nas.example.lan/"})
	b := e.browser()
	// 로그인하지 않아도 받는다. 기본값은 대기 연출 켜짐, 배경·기본 화면 없음이다.
	rec := b.do(http.MethodGet, "/api/site", nil)
	wantStatus(t, rec, http.StatusOK)
	got := decode[api.SiteInfo](t, rec)
	if !got.LoginEasterEgg || got.LoginBackground != nil || got.DefaultTheme != "" || got.DefaultWallpaper != nil ||
		got.Images.LoginBackground != nil || got.Images.DefaultWallpaper != nil {
		t.Fatalf("defaults %+v", got)
	}
	// 가동 설정은 공개 정보에 넣지 않는다.
	if bytes.Contains(rec.Body.Bytes(), []byte("nas.example.lan")) {
		t.Errorf("public site info leaks the workbench url: %s", rec.Body.String())
	}
	wantStatus(t, b.do(http.MethodGet, "/api/site/images/login-background", nil), http.StatusNotFound)
	wantCode(t, b.do(http.MethodGet, "/api/site/images/default-wallpaper", nil), http.StatusUnauthorized, api.ErrUnauthorized)
}

func TestSiteSettingsRequireAdmin(t *testing.T) {
	site := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{site: site})
	anon := e.browser()
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	login := api.SiteLoginRequest{Login: &api.SiteLogin{EasterEgg: false}}
	defaults := api.SiteDefaultsRequest{Defaults: &api.SiteDefaults{Theme: api.ThemeDark}}
	server := api.ServerSettingsRequest{Server: &api.ServerOverrides{}}
	for _, c := range []struct {
		b    *browser
		code int
		err  string
	}{{anon, http.StatusUnauthorized, api.ErrUnauthorized}, {user, http.StatusForbidden, api.ErrAdminRequired}} {
		wantCode(t, c.b.do(http.MethodPut, "/api/site/login", login), c.code, c.err)
		wantCode(t, c.b.do(http.MethodPut, "/api/site/defaults", defaults), c.code, c.err)
		wantCode(t, c.b.do(http.MethodGet, "/api/site/server", nil), c.code, c.err)
		wantCode(t, c.b.do(http.MethodPut, "/api/site/server", server), c.code, c.err)
		wantCode(t, c.b.raw(http.MethodPut, "/api/site/images/login-background", "image/png", testPNG(t)), c.code, c.err)
		wantCode(t, c.b.do(http.MethodDelete, "/api/site/images/login-background", nil), c.code, c.err)
	}
	if got, _ := site.GetSite(); got.Login != nil || got.Defaults != nil || got.Server != nil {
		t.Errorf("non-admin changed site settings: %+v", got)
	}
	// 기본 바탕화면 이미지는 로그인한 사용자 누구나 받는다(없으면 404).
	wantStatus(t, user.do(http.MethodGet, "/api/site/images/default-wallpaper", nil), http.StatusNotFound)
}

func TestSiteLoginAndDefaults(t *testing.T) {
	e := newEnv(t, envOptions{})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, admin.do(http.MethodPut, "/api/site/login", api.SiteLoginRequest{Login: &api.SiteLogin{
		Background: &api.Wallpaper{Kind: api.WallpaperPreset, Preset: "ocean"},
	}}), http.StatusNoContent)
	wantStatus(t, admin.do(http.MethodPut, "/api/site/defaults", api.SiteDefaultsRequest{Defaults: &api.SiteDefaults{
		Theme: api.ThemeLight, Wallpaper: &api.Wallpaper{Kind: api.WallpaperPreset, Preset: "forest"},
	}}), http.StatusNoContent)
	got := decode[api.SiteInfo](t, e.browser().do(http.MethodGet, "/api/site", nil))
	if got.LoginEasterEgg || got.LoginBackground == nil || got.LoginBackground.Preset != "ocean" ||
		got.DefaultTheme != api.ThemeLight || got.DefaultWallpaper == nil || got.DefaultWallpaper.Preset != "forest" {
		t.Fatalf("site info %+v", got)
	}

	for name, body := range map[string]any{
		"login missing":     map[string]any{},
		"login bad preset":  map[string]any{"login": map[string]any{"background": map[string]any{"kind": "preset", "preset": "A B"}}},
		"login bad kind":    map[string]any{"login": map[string]any{"background": map[string]any{"kind": "file"}}},
		"defaults missing":  map[string]any{"defaults": nil},
		"defaults bad mode": map[string]any{"defaults": map[string]any{"theme": "sepia"}},
	} {
		path := "/api/site/login"
		if name[0] == 'd' {
			path = "/api/site/defaults"
		}
		if rec := admin.do(http.MethodPut, path, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
}

func TestSiteImages(t *testing.T) {
	site := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{site: site})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, admin.raw(http.MethodPut, "/api/site/images/other", "image/png", testPNG(t)), http.StatusNotFound)

	// 로그인 배경 이미지를 올리면 로그인 배경이 그 이미지가 되고, 대기 연출은 기본값(켜짐)을 유지한다.
	img := testPNG(t)
	rec := admin.raw(http.MethodPut, "/api/site/images/login-background", "image/png", img)
	wantStatus(t, rec, http.StatusOK)
	info := decode[api.ImageInfo](t, rec)
	anon := e.browser()
	got := decode[api.SiteInfo](t, anon.do(http.MethodGet, "/api/site", nil))
	if got.LoginBackground == nil || got.LoginBackground.Kind != api.WallpaperImage || !got.LoginEasterEgg ||
		got.Images.LoginBackground == nil || got.Images.LoginBackground.Rev != info.Rev {
		t.Fatalf("after upload %+v", got)
	}
	// 로그인 화면에 보이는 그림이라 인증 없이 받는다.
	rec = anon.do(http.MethodGet, "/api/site/images/login-background", nil)
	wantStatus(t, rec, http.StatusOK)
	if !bytes.Equal(rec.Body.Bytes(), img) || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("login background %d bytes, headers %v", rec.Body.Len(), rec.Header())
	}

	// 기본 바탕화면 이미지는 로그인한 사용자만 받는다.
	wantStatus(t, admin.raw(http.MethodPut, "/api/site/images/default-wallpaper", "image/jpeg", testJPEG(t)), http.StatusOK)
	wantCode(t, anon.do(http.MethodGet, "/api/site/images/default-wallpaper", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	if rec := user.do(http.MethodGet, "/api/site/images/default-wallpaper", nil); rec.Code != http.StatusOK ||
		rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("default wallpaper %d %v", rec.Code, rec.Header())
	}

	// 지우면 그 이미지를 가리키던 배경만 지운다. 프리셋으로 바꾼 배경은 그대로다.
	wantStatus(t, admin.do(http.MethodPut, "/api/site/defaults", api.SiteDefaultsRequest{Defaults: &api.SiteDefaults{
		Wallpaper: &api.Wallpaper{Kind: api.WallpaperPreset, Preset: "dusk"},
	}}), http.StatusNoContent)
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/images/login-background", nil), http.StatusNoContent)
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/images/default-wallpaper", nil), http.StatusNoContent)
	got = decode[api.SiteInfo](t, anon.do(http.MethodGet, "/api/site", nil))
	if got.LoginBackground != nil || !got.LoginEasterEgg || got.Images.LoginBackground != nil ||
		got.DefaultWallpaper == nil || got.DefaultWallpaper.Preset != "dusk" || got.Images.DefaultWallpaper != nil {
		t.Errorf("after delete %+v", got)
	}
}

func TestServerOverrides(t *testing.T) {
	var smartCalls [][2]int
	runtime := []api.RuntimeSetting{{Flag: "listen", Env: "ONMARU_LISTEN", Value: "127.0.0.1:8090", Source: api.RuntimeSourceDefault}}
	site := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{
		site:         site,
		workbenchURL: "https://nas.example.lan/",
		smartAt:      "03:30",
		setSmart:     func(h, m int) { smartCalls = append(smartCalls, [2]int{h, m}) },
		runtime:      runtime,
	})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	got := decode[api.ServerSettings](t, admin.do(http.MethodGet, "/api/site/server", nil))
	if got.WorkbenchURL != (api.OverridableSetting{Value: "https://nas.example.lan/", Base: "https://nas.example.lan/"}) ||
		got.SmartRefreshAt != (api.OverridableSetting{Value: "03:30", Base: "03:30"}) || !slices.Equal(got.Runtime, runtime) {
		t.Fatalf("initial %+v", got)
	}

	// 덮어쓰면 바로 반영된다: 워크벤치 주소는 capability로, SMART 시각은 캐시로.
	off, at := "", "06:15"
	wantStatus(t, admin.do(http.MethodPut, "/api/site/server", api.ServerSettingsRequest{
		Server: &api.ServerOverrides{WorkbenchURL: &off, SmartRefreshAt: &at},
	}), http.StatusNoContent)
	got = decode[api.ServerSettings](t, admin.do(http.MethodGet, "/api/site/server", nil))
	if !got.WorkbenchURL.Overridden || got.WorkbenchURL.Value != "" || got.WorkbenchURL.Base != "https://nas.example.lan/" ||
		!got.SmartRefreshAt.Overridden || got.SmartRefreshAt.Value != "06:15" {
		t.Fatalf("overridden %+v", got)
	}
	if caps := decode[api.CapabilitiesResponse](t, admin.do(http.MethodGet, "/api/capabilities", nil)); caps.WorkbenchURL != "" {
		t.Errorf("capabilities workbench %q", caps.WorkbenchURL)
	}
	if last := smartCalls[len(smartCalls)-1]; last != [2]int{6, 15} {
		t.Errorf("smart calls %v", smartCalls)
	}

	// 잘못된 값은 저장하지 않는다.
	bad, badAt := "nas.example.lan/", "25:00"
	for _, o := range []api.ServerOverrides{{WorkbenchURL: &bad}, {SmartRefreshAt: &badAt}} {
		wantCode(t, admin.do(http.MethodPut, "/api/site/server", api.ServerSettingsRequest{Server: &o}),
			http.StatusBadRequest, api.ErrBadRequest)
	}
	wantCode(t, admin.do(http.MethodPut, "/api/site/server", map[string]any{}), http.StatusBadRequest, api.ErrBadRequest)

	// 다시 시작해도(저장소가 같으면) 덮어쓴 값으로 시작한다.
	smartCalls = nil
	e2 := newEnv(t, envOptions{site: site, workbenchURL: "https://nas.example.lan/", smartAt: "03:30",
		setSmart: func(h, m int) { smartCalls = append(smartCalls, [2]int{h, m}) }})
	admin2 := e2.browser()
	wantStatus(t, admin2.login("admin", "admin"), http.StatusOK)
	if caps := decode[api.CapabilitiesResponse](t, admin2.do(http.MethodGet, "/api/capabilities", nil)); caps.WorkbenchURL != "" {
		t.Errorf("restart: capabilities workbench %q", caps.WorkbenchURL)
	}
	if len(smartCalls) != 1 || smartCalls[0] != [2]int{6, 15} {
		t.Errorf("restart: smart calls %v", smartCalls)
	}

	// 덮어쓰기를 지우면 플래그 값으로 돌아간다.
	wantStatus(t, admin2.do(http.MethodPut, "/api/site/server", api.ServerSettingsRequest{Server: &api.ServerOverrides{}}),
		http.StatusNoContent)
	if caps := decode[api.CapabilitiesResponse](t, admin2.do(http.MethodGet, "/api/capabilities", nil)); caps.WorkbenchURL != "https://nas.example.lan/" {
		t.Errorf("reset: capabilities workbench %q", caps.WorkbenchURL)
	}
	if last := smartCalls[len(smartCalls)-1]; last != [2]int{3, 30} {
		t.Errorf("reset: smart calls %v", smartCalls)
	}
	if s, _ := site.GetSite(); s.Server != nil {
		t.Errorf("reset: stored %+v", s.Server)
	}
}
