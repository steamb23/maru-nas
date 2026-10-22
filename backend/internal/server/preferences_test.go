package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/prefs"
)

// widgetAddon은 위젯 보기가 있는 테스트용 애드온이다(크기 2×2~4×3, 기본 3×2).
func widgetAddon(id string) addons.Addon {
	a := testAddon(id, api.Requirements{}, api.RPCMethods{})
	a.Manifest.Views.Widget = &api.WidgetView{
		DefaultSize: api.GridSize{Cols: 3, Rows: 2},
		MinSize:     &api.GridSize{Cols: 2, Rows: 2},
		MaxSize:     &api.GridSize{Cols: 4, Rows: 3},
	}
	return a
}

func prefsEnv(t *testing.T, store prefs.Store) *env {
	t.Helper()
	return newEnv(t, envOptions{
		addons: testRegistry(t, widgetAddon("cpu"), widgetAddon("disk"), testAddon("files", api.Requirements{}, api.RPCMethods{})),
		prefs:  store,
	})
}

func TestPreferencesRoundTrip(t *testing.T) {
	e := prefsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.Widgets != nil {
		t.Fatalf("new user widgets = %+v, want none", *got.Widgets)
	}

	layout := []api.WidgetPlacement{
		{Addon: "cpu", Right: 0, Top: 0, Cols: 3, Rows: 2},
		{Addon: "disk", Right: 0, Top: 2, Cols: 2, Rows: 2},
	}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: layout}), http.StatusNoContent)
	got = decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.Widgets == nil || !slices.Equal(*got.Widgets, layout) {
		t.Fatalf("widgets = %+v", got.Widgets)
	}

	// 모두 지운 배치는 "저장한 적 없음"과 다르게 빈 목록으로 돌아온다.
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: []api.WidgetPlacement{}}), http.StatusNoContent)
	rec := b.do(http.MethodGet, "/api/preferences", nil)
	if !strings.Contains(rec.Body.String(), `"widgets":[]`) {
		t.Errorf("cleared body = %s", rec.Body.String())
	}
}

func TestPreferencesPerUser(t *testing.T) {
	e := prefsEnv(t, nil)
	admin, user := e.browser(), e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, user.login("user", "user"), http.StatusOK)

	layout := []api.WidgetPlacement{{Addon: "cpu", Cols: 3, Rows: 2}}
	wantStatus(t, admin.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: layout}), http.StatusNoContent)
	if got := decode[api.Preferences](t, user.do(http.MethodGet, "/api/preferences", nil)); got.Widgets != nil {
		t.Errorf("user sees admin's widgets: %+v", *got.Widgets)
	}

	// 로그아웃 후 다시 로그인해도 배치가 남는다.
	wantStatus(t, admin.do(http.MethodPost, "/api/logout", nil), http.StatusNoContent)
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	if got := decode[api.Preferences](t, admin.do(http.MethodGet, "/api/preferences", nil)); got.Widgets == nil || len(*got.Widgets) != 1 {
		t.Errorf("after relogin: %+v", got.Widgets)
	}
}

func TestPreferencesRequiresSession(t *testing.T) {
	e := prefsEnv(t, nil)
	b := e.browser()
	wantCode(t, b.do(http.MethodGet, "/api/preferences", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	wantCode(t, b.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: []api.WidgetPlacement{}}),
		http.StatusUnauthorized, api.ErrUnauthorized)
}

func TestSetWidgetsRejectsInvalid(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := prefsEnv(t, store)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	cases := map[string]any{
		"missing widgets": map[string]any{},
		"null widgets":    map[string]any{"widgets": nil},
		"not an array":    map[string]any{"widgets": "cpu"},
		"unknown addon":   api.WidgetsRequest{Widgets: []api.WidgetPlacement{{Addon: "nope", Cols: 3, Rows: 2}}},
		"window only":     api.WidgetsRequest{Widgets: []api.WidgetPlacement{{Addon: "files", Cols: 3, Rows: 2}}},
		"size too large":  api.WidgetsRequest{Widgets: []api.WidgetPlacement{{Addon: "cpu", Cols: 5, Rows: 2}}},
		"overlap": api.WidgetsRequest{Widgets: []api.WidgetPlacement{
			{Addon: "cpu", Cols: 3, Rows: 2},
			{Addon: "disk", Right: 1, Top: 1, Cols: 2, Rows: 2},
		}},
	}
	for name, body := range cases {
		rec := b.do(http.MethodPut, "/api/preferences/widgets", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d (body %s)", name, rec.Code, rec.Body.String())
		}
	}
	if p, _ := store.Get("admin"); p.Widgets != nil {
		t.Errorf("invalid layout was saved: %+v", *p.Widgets)
	}

	// JSON이 아닌 본문은 받지 않는다(CSRF 보조 장치).
	b.header.Set("Content-Type", "text/plain")
	wantCode(t, b.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: []api.WidgetPlacement{}}),
		http.StatusUnsupportedMediaType, api.ErrBadRequest)
}

func TestSetWidgetsBodyLimit(t *testing.T) {
	e := prefsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	big := map[string]string{"widgets": strings.Repeat("x", maxPreferencesBodyBytes)}
	wantCode(t, b.do(http.MethodPut, "/api/preferences/widgets", big), http.StatusBadRequest, api.ErrBadRequest)
}

// failingStore는 항상 실패하는 저장소다.
type failingStore struct{}

func (failingStore) Get(string) (api.Preferences, error) {
	return api.Preferences{}, errors.New("disk on fire")
}
func (failingStore) SetWidgets(string, []api.WidgetPlacement) error {
	return errors.New("disk on fire")
}
func (failingStore) SetShortcuts(string, []api.Shortcut) error {
	return errors.New("disk on fire")
}
func (failingStore) SetDrawer(string, api.DrawerPrefs) error {
	return errors.New("disk on fire")
}
func (failingStore) SetSystemInfo(string, api.SystemInfoPrefs) error {
	return errors.New("disk on fire")
}
func (failingStore) SetFiles(string, api.FilesPrefs) error {
	return errors.New("disk on fire")
}
func (failingStore) UpdateAppearance(string, func(*api.AppearancePrefs)) error {
	return errors.New("disk on fire")
}

func TestPreferencesStoreError(t *testing.T) {
	e := prefsEnv(t, failingStore{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/preferences", nil), http.StatusInternalServerError)
	for path, body := range map[string]any{
		"/api/preferences/widgets":   api.WidgetsRequest{Widgets: []api.WidgetPlacement{}},
		"/api/preferences/shortcuts": api.ShortcutsRequest{Shortcuts: []api.Shortcut{}},
		"/api/preferences/drawer":    api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: api.DrawerSortName}},
		"/api/preferences/system-info": api.SystemInfoRequest{
			SystemInfo: &api.SystemInfoPrefs{CPUScale: api.CPUScaleMachine, CPUGraph: api.CPUGraphOverall},
		},
		"/api/preferences/files": api.FilesPrefsRequest{Files: &api.FilesPrefs{View: api.FilesViewGrid}},
	} {
		rec := b.do(http.MethodPut, path, body)
		wantStatus(t, rec, http.StatusInternalServerError)
		if strings.Contains(rec.Body.String(), "disk on fire") {
			t.Errorf("%s: internal error leaked: %s", path, rec.Body.String())
		}
	}
}

func TestShortcutsRoundTrip(t *testing.T) {
	e := prefsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.Shortcuts != nil {
		t.Fatalf("new user shortcuts = %+v, want none", *got.Shortcuts)
	}

	// 레지스트리에 없는 앱 id(프론트의 준비 중 앱)도 형식만 맞으면 저장한다.
	list := []api.Shortcut{{App: "placeholder-app"}, {App: "files"}, {App: "cpu"}}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/shortcuts", api.ShortcutsRequest{Shortcuts: list}), http.StatusNoContent)
	got = decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.Shortcuts == nil || !slices.Equal(*got.Shortcuts, list) {
		t.Fatalf("shortcuts = %+v", got.Shortcuts)
	}

	// 위젯 배치를 저장해도 바로가기는 남는다.
	layout := []api.WidgetPlacement{{Addon: "cpu", Cols: 3, Rows: 2}}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/widgets", api.WidgetsRequest{Widgets: layout}), http.StatusNoContent)
	got = decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if got.Widgets == nil || got.Shortcuts == nil || !slices.Equal(*got.Shortcuts, list) {
		t.Fatalf("after widgets: %+v", got)
	}

	// 모두 지운 목록은 "저장한 적 없음"과 다르게 빈 목록으로 돌아온다.
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/shortcuts", api.ShortcutsRequest{Shortcuts: []api.Shortcut{}}), http.StatusNoContent)
	rec := b.do(http.MethodGet, "/api/preferences", nil)
	if !strings.Contains(rec.Body.String(), `"shortcuts":[]`) {
		t.Errorf("cleared body = %s", rec.Body.String())
	}
	if got := decode[api.Preferences](t, rec); got.Widgets == nil || len(*got.Widgets) != 1 {
		t.Errorf("clearing shortcuts changed widgets: %+v", got.Widgets)
	}
}

func TestShortcutsPerUser(t *testing.T) {
	e := prefsEnv(t, nil)
	admin, user := e.browser(), e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, user.login("user", "user"), http.StatusOK)

	list := []api.Shortcut{{App: "files"}}
	wantStatus(t, admin.do(http.MethodPut, "/api/preferences/shortcuts", api.ShortcutsRequest{Shortcuts: list}), http.StatusNoContent)
	if got := decode[api.Preferences](t, user.do(http.MethodGet, "/api/preferences", nil)); got.Shortcuts != nil {
		t.Errorf("user sees admin's shortcuts: %+v", *got.Shortcuts)
	}
	wantCode(t, e.browser().do(http.MethodPut, "/api/preferences/shortcuts", api.ShortcutsRequest{Shortcuts: list}),
		http.StatusUnauthorized, api.ErrUnauthorized)
}

func TestSetShortcutsRejectsInvalid(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := prefsEnv(t, store)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	tooMany := make([]api.Shortcut, addons.MaxShortcuts+1)
	for i := range tooMany {
		tooMany[i] = api.Shortcut{App: fmt.Sprintf("app-%d", i)}
	}
	cases := map[string]any{
		"missing shortcuts": map[string]any{},
		"null shortcuts":    map[string]any{"shortcuts": nil},
		"not an array":      map[string]any{"shortcuts": "files"},
		"array of strings":  map[string]any{"shortcuts": []string{"files"}},
		"bad app id":        api.ShortcutsRequest{Shortcuts: []api.Shortcut{{App: "../files"}}},
		"empty app id":      api.ShortcutsRequest{Shortcuts: []api.Shortcut{{}}},
		"duplicate":         api.ShortcutsRequest{Shortcuts: []api.Shortcut{{App: "files"}, {App: "files"}}},
		"too many":          api.ShortcutsRequest{Shortcuts: tooMany},
	}
	for name, body := range cases {
		rec := b.do(http.MethodPut, "/api/preferences/shortcuts", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d (body %s)", name, rec.Code, rec.Body.String())
		}
	}
	if p, _ := store.Get("admin"); p.Shortcuts != nil {
		t.Errorf("invalid shortcuts were saved: %+v", *p.Shortcuts)
	}

	big := map[string]string{"shortcuts": strings.Repeat("x", maxPreferencesBodyBytes)}
	wantCode(t, b.do(http.MethodPut, "/api/preferences/shortcuts", big), http.StatusBadRequest, api.ErrBadRequest)

	// JSON이 아닌 본문은 받지 않는다(CSRF 보조 장치).
	b.header.Set("Content-Type", "text/plain")
	wantCode(t, b.do(http.MethodPut, "/api/preferences/shortcuts", api.ShortcutsRequest{Shortcuts: []api.Shortcut{}}),
		http.StatusUnsupportedMediaType, api.ErrBadRequest)
}

func TestDrawerRoundTrip(t *testing.T) {
	e := prefsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	if got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil)); got.Drawer != nil {
		t.Fatalf("new user drawer = %+v, want none", *got.Drawer)
	}
	list := []api.Shortcut{{App: "files"}}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/shortcuts", api.ShortcutsRequest{Shortcuts: list}), http.StatusNoContent)
	for _, sort := range []string{api.DrawerSortName, api.DrawerSortRecommended} {
		req := api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: sort}}
		wantStatus(t, b.do(http.MethodPut, "/api/preferences/drawer", req), http.StatusNoContent)
		got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
		if got.Drawer == nil || got.Drawer.Sort != sort {
			t.Fatalf("drawer = %+v, want sort %q", got.Drawer, sort)
		}
		// 다른 설정은 그대로다.
		if got.Shortcuts == nil || !slices.Equal(*got.Shortcuts, list) {
			t.Errorf("drawer save changed shortcuts: %+v", got.Shortcuts)
		}
	}
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	if got := decode[api.Preferences](t, user.do(http.MethodGet, "/api/preferences", nil)); got.Drawer != nil {
		t.Errorf("user sees admin's drawer settings: %+v", *got.Drawer)
	}
}

func TestSetDrawerRejectsInvalid(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := prefsEnv(t, store)
	b := e.browser()
	wantCode(t, b.do(http.MethodPut, "/api/preferences/drawer", api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: api.DrawerSortName}}),
		http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	cases := map[string]any{
		"missing drawer": map[string]any{},
		"null drawer":    map[string]any{"drawer": nil},
		"not an object":  map[string]any{"drawer": "name"},
		"missing sort":   map[string]any{"drawer": map[string]any{}},
		"unknown sort":   api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: "random"}},
	}
	for name, body := range cases {
		rec := b.do(http.MethodPut, "/api/preferences/drawer", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d (body %s)", name, rec.Code, rec.Body.String())
		}
	}
	if p, _ := store.Get("admin"); p.Drawer != nil {
		t.Errorf("invalid drawer settings were saved: %+v", *p.Drawer)
	}

	b.header.Set("Content-Type", "text/plain")
	wantCode(t, b.do(http.MethodPut, "/api/preferences/drawer", api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: api.DrawerSortName}}),
		http.StatusUnsupportedMediaType, api.ErrBadRequest)
}

func TestSystemInfoPrefsRoundTrip(t *testing.T) {
	e := prefsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	if got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil)); got.SystemInfo != nil {
		t.Fatalf("new user system info = %+v, want none", *got.SystemInfo)
	}
	drawer := api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: api.DrawerSortName}}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/drawer", drawer), http.StatusNoContent)
	hide, show := false, true
	// kernel은 CPUKernel을 비교하려고 포인터를 값으로 바꾼다(nil은 "없음").
	kernel := func(p *bool) string {
		if p == nil {
			return "unset"
		}
		return strconv.FormatBool(*p)
	}
	for _, want := range []api.SystemInfoPrefs{
		{CPUScale: api.CPUScaleCore, CPUGraph: api.CPUGraphCores, CPUKernel: &hide},
		{CPUScale: api.CPUScaleCore, CPUGraph: api.CPUGraphCores, CPUKernel: &show},
		// 옛 탭은 cpuKernel을 보내지 않는다.
		{CPUScale: api.CPUScaleMachine, CPUGraph: api.CPUGraphOverall},
	} {
		wantStatus(t, b.do(http.MethodPut, "/api/preferences/system-info", api.SystemInfoRequest{SystemInfo: &want}), http.StatusNoContent)
		got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
		if got.SystemInfo == nil || got.SystemInfo.CPUScale != want.CPUScale || got.SystemInfo.CPUGraph != want.CPUGraph ||
			kernel(got.SystemInfo.CPUKernel) != kernel(want.CPUKernel) {
			t.Fatalf("system info = %+v, want %+v", got.SystemInfo, want)
		}
		// 다른 설정은 그대로다.
		if got.Drawer == nil || got.Drawer.Sort != api.DrawerSortName {
			t.Errorf("system info save changed drawer: %+v", got.Drawer)
		}
	}
	// 일반 사용자도 자기 설정을 저장한다(개요의 CPU 그래프 모양).
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	if got := decode[api.Preferences](t, user.do(http.MethodGet, "/api/preferences", nil)); got.SystemInfo != nil {
		t.Errorf("user sees admin's system info settings: %+v", *got.SystemInfo)
	}
	own := api.SystemInfoPrefs{CPUScale: api.CPUScaleMachine, CPUGraph: api.CPUGraphCores}
	wantStatus(t, user.do(http.MethodPut, "/api/preferences/system-info", api.SystemInfoRequest{SystemInfo: &own}), http.StatusNoContent)
}

func TestSetSystemInfoRejectsInvalid(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := prefsEnv(t, store)
	b := e.browser()
	valid := api.SystemInfoRequest{SystemInfo: &api.SystemInfoPrefs{CPUScale: api.CPUScaleMachine, CPUGraph: api.CPUGraphOverall}}
	wantCode(t, b.do(http.MethodPut, "/api/preferences/system-info", valid), http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	cases := map[string]any{
		"missing":       map[string]any{},
		"null":          map[string]any{"systemInfo": nil},
		"not an object": map[string]any{"systemInfo": "core"},
		"missing scale": map[string]any{"systemInfo": map[string]any{"cpuGraph": "cores"}},
		"missing graph": map[string]any{"systemInfo": map[string]any{"cpuScale": "core"}},
		"unknown scale": api.SystemInfoRequest{SystemInfo: &api.SystemInfoPrefs{CPUScale: "irix", CPUGraph: api.CPUGraphOverall}},
		"unknown graph": api.SystemInfoRequest{SystemInfo: &api.SystemInfoPrefs{CPUScale: api.CPUScaleCore, CPUGraph: "heatmap"}},
	}
	for name, body := range cases {
		rec := b.do(http.MethodPut, "/api/preferences/system-info", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d (body %s)", name, rec.Code, rec.Body.String())
		}
	}
	if p, _ := store.Get("admin"); p.SystemInfo != nil {
		t.Errorf("invalid system info settings were saved: %+v", *p.SystemInfo)
	}

	b.header.Set("Content-Type", "text/plain")
	wantCode(t, b.do(http.MethodPut, "/api/preferences/system-info", valid), http.StatusUnsupportedMediaType, api.ErrBadRequest)
}

func TestSetFilesPrefs(t *testing.T) {
	store := prefs.NewMemoryStore()
	e := prefsEnv(t, store)
	b := e.browser()
	valid := api.FilesPrefsRequest{Files: &api.FilesPrefs{View: api.FilesViewGrid}}
	wantCode(t, b.do(http.MethodPut, "/api/preferences/files", valid), http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	if got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil)); got.Files != nil {
		t.Fatalf("new user files prefs = %+v, want none", *got.Files)
	}
	drawer := api.DrawerRequest{Drawer: &api.DrawerPrefs{Sort: api.DrawerSortName}}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/drawer", drawer), http.StatusNoContent)
	for _, view := range []string{api.FilesViewGrid, api.FilesViewList} {
		req := api.FilesPrefsRequest{Files: &api.FilesPrefs{View: view}}
		wantStatus(t, b.do(http.MethodPut, "/api/preferences/files", req), http.StatusNoContent)
		got := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
		if got.Files == nil || got.Files.View != view {
			t.Fatalf("files prefs = %+v, want %s", got.Files, view)
		}
		if got.Drawer == nil || got.Drawer.Sort != api.DrawerSortName {
			t.Errorf("files prefs save changed drawer: %+v", got.Drawer)
		}
	}

	for name, body := range map[string]any{
		"missing":      map[string]any{},
		"null":         map[string]any{"files": nil},
		"missing view": map[string]any{"files": map[string]any{}},
		"unknown view": api.FilesPrefsRequest{Files: &api.FilesPrefs{View: "columns"}},
	} {
		if rec := b.do(http.MethodPut, "/api/preferences/files", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d (body %s)", name, rec.Code, rec.Body.String())
		}
	}
	if p, _ := store.Get("admin"); p.Files == nil || p.Files.View != api.FilesViewList {
		t.Errorf("invalid files prefs changed the saved value: %+v", p.Files)
	}
}
