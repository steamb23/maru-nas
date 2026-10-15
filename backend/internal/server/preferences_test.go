package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/prefs"
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

func TestPreferencesStoreError(t *testing.T) {
	e := prefsEnv(t, failingStore{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/preferences", nil), http.StatusInternalServerError)
	for path, body := range map[string]any{
		"/api/preferences/widgets":   api.WidgetsRequest{Widgets: []api.WidgetPlacement{}},
		"/api/preferences/shortcuts": api.ShortcutsRequest{Shortcuts: []api.Shortcut{}},
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
