package server

import (
	"net/http"
	"slices"
	"sync"
	"testing"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

type testProvider struct {
	name string
	req  api.Requirements
}

func (p testProvider) Name() string               { return p.name }
func (p testProvider) Requires() api.Requirements { return p.req }

// testAddon은 창 보기 하나와 provider 하나를 가진 테스트용 애드온이다.
func testAddon(id string, req api.Requirements, rpc api.RPCMethods) addons.Addon {
	return addons.Addon{
		Manifest: api.Manifest{
			ID:       id,
			Title:    "apps.test",
			Icon:     "folder",
			Views:    api.AddonViews{Window: &api.WindowView{DefaultSize: api.Size{Width: 400, Height: 300}}},
			Requires: req,
			RPC:      rpc,
		},
		Providers: []addons.Provider{testProvider{name: "direct"}},
	}
}

func testRegistry(t *testing.T, list ...addons.Addon) *addons.Registry {
	t.Helper()
	r, err := addons.NewRegistry(list...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// fakePlugins는 고정된 플러그인 집합을 돌려주고 호출 횟수를 센다.
type fakePlugins struct {
	mu          sync.Mutex
	installed   map[string]bool
	refreshes   int
	invalidates int
}

func (f *fakePlugins) Plugins() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.installed
}

func (f *fakePlugins) Refresh() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshes++
}

func (f *fakePlugins) Invalidate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidates++
}

func TestCapabilities(t *testing.T) {
	plugins := &fakePlugins{installed: map[string]bool{"openmediavault-compose": true}}
	e := newEnv(t, envOptions{
		addons: testRegistry(t,
			testAddon("free", api.Requirements{}, api.RPCMethods{}),
			testAddon("admin-only", api.Requirements{Admin: true}, api.RPCMethods{}),
			testAddon("needs-compose", api.Requirements{Plugins: []string{"openmediavault-compose"}}, api.RPCMethods{}),
			testAddon("needs-cputemp", api.Requirements{Plugins: []string{"openmediavault-cputemp"}}, api.RPCMethods{}),
		),
		plugins: plugins,
	})

	anon := e.browser()
	wantCode(t, anon.do(http.MethodGet, "/api/capabilities", nil), http.StatusUnauthorized, api.ErrUnauthorized)

	type row struct {
		available bool
		reason    string
	}
	check := func(user string, want map[string]row) {
		t.Helper()
		b := e.browser()
		wantStatus(t, b.login(user, user), http.StatusOK)
		rec := b.do(http.MethodGet, "/api/capabilities", nil)
		wantStatus(t, rec, http.StatusOK)
		got := decode[api.CapabilitiesResponse](t, rec)
		if !slices.Equal(got.Plugins, []string{"openmediavault-compose"}) {
			t.Errorf("%s: plugins = %v", user, got.Plugins)
		}
		var ids []string
		for _, c := range got.Addons {
			ids = append(ids, c.ID)
			if w := want[c.ID]; c.Available != w.available || c.Reason != w.reason {
				t.Errorf("%s: %s = %+v, want %+v", user, c.ID, c, w)
			}
		}
		if !slices.Equal(ids, []string{"free", "admin-only", "needs-compose", "needs-cputemp"}) {
			t.Errorf("%s: order = %v", user, ids)
		}
	}
	check("admin", map[string]row{
		"free":          {true, ""},
		"admin-only":    {true, ""},
		"needs-compose": {true, ""},
		"needs-cputemp": {false, api.ReasonPluginMissing},
	})
	check("user", map[string]row{
		"free":          {true, ""},
		"admin-only":    {false, api.ReasonAdminRequired},
		"needs-compose": {true, ""},
		"needs-cputemp": {false, api.ReasonPluginMissing},
	})
	// 로그인할 때마다 플러그인 설치 여부를 다시 확인한다.
	if plugins.refreshes != 2 {
		t.Errorf("refreshes = %d, want 2", plugins.refreshes)
	}
}

func TestCapabilitiesDefaults(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	rec := b.do(http.MethodGet, "/api/capabilities", nil)
	wantStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); body != "{\"addons\":[],\"plugins\":[]}\n" {
		t.Errorf("body = %q", body)
	}
}
