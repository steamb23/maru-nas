package addons

import (
	"slices"
	"testing"

	"omv-desktop-dashboard/backend/internal/api"
)

type fakeProvider struct {
	name string
	req  api.Requirements
}

func (p fakeProvider) Name() string               { return p.name }
func (p fakeProvider) Requires() api.Requirements { return p.req }

// addon은 검사를 통과하는 최소 애드온을 만든다. edit으로 필드를 바꾼다.
func addon(id string, edit func(*Addon)) Addon {
	a := Addon{
		Manifest: api.Manifest{
			ID:    id,
			Title: "apps.test",
			Icon:  "folder",
			Views: api.AddonViews{Window: &api.WindowView{DefaultSize: api.Size{Width: 400, Height: 300}}},
		},
		Providers: []Provider{fakeProvider{name: "direct"}},
	}
	if edit != nil {
		edit(&a)
	}
	return a
}

func TestNewRegistryKeepsOrder(t *testing.T) {
	r, err := NewRegistry(addon("b", nil), addon("a", nil))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, a := range r.All() {
		ids = append(ids, a.Manifest.ID)
	}
	if !slices.Equal(ids, []string{"b", "a"}) {
		t.Errorf("order = %v", ids)
	}
	if _, ok := r.Get("a"); !ok {
		t.Error("Get(a) not found")
	}
	if _, ok := r.Get("c"); ok {
		t.Error("Get(c) found")
	}
}

func TestNewRegistryRejectsInvalid(t *testing.T) {
	cases := map[string][]Addon{
		"duplicate id": {addon("a", nil), addon("a", nil)},
		"bad id":       {addon("System_Info", nil)},
		"empty id":     {addon("", nil)},
		"no title":     {addon("a", func(a *Addon) { a.Manifest.Title = "" })},
		"no icon":      {addon("a", func(a *Addon) { a.Manifest.Icon = "" })},
		"no view":      {addon("a", func(a *Addon) { a.Manifest.Views.Window = nil })},
		"zero size": {addon("a", func(a *Addon) {
			a.Manifest.Views.Window.DefaultSize = api.Size{}
		})},
		"min over default": {addon("a", func(a *Addon) {
			a.Manifest.Views.Window.MinSize = &api.Size{Width: 500, Height: 100}
		})},
		"bad method": {addon("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Smart getList"} })},
		"no method":  {addon("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Smart"} })},
		"method twice": {addon("a", func(a *Addon) {
			a.Manifest.RPC.Read = []string{"Smart.getList"}
			a.Manifest.RPC.Write = []string{"Smart.getList"}
		})},
		"read and write across addons": {
			addon("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Compose.getContainers"} }),
			addon("b", func(a *Addon) { a.Manifest.RPC.Write = []string{"Compose.getContainers"} }),
		},
		"empty plugin": {addon("a", func(a *Addon) { a.Manifest.Requires.Plugins = []string{""} })},
		"no provider":  {addon("a", func(a *Addon) { a.Providers = nil })},
		"provider name twice": {addon("a", func(a *Addon) {
			a.Providers = []Provider{fakeProvider{name: "x"}, fakeProvider{name: "x"}}
		})},
		"provider no name": {addon("a", func(a *Addon) { a.Providers = []Provider{fakeProvider{}} })},
	}
	for name, list := range cases {
		if _, err := NewRegistry(list...); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestClassify(t *testing.T) {
	r, err := NewRegistry(
		addon("a", func(a *Addon) {
			a.Manifest.RPC.Read = []string{"Smart.getList", "Compose.getContainers"}
			a.Manifest.RPC.Write = []string{"Compose.startContainer"}
		}),
		addon("b", func(a *Addon) { a.Manifest.RPC.Read = []string{"Smart.getList"} }),
	)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		service, method string
		kind            RPCKind
		addons          []string
	}{
		{"Smart", "getList", RPCRead, []string{"a", "b"}},
		{"Compose", "getContainers", RPCRead, []string{"a"}},
		{"Compose", "startContainer", RPCWrite, []string{"a"}},
		{"Compose", "stopContainer", RPCNone, nil},
		// 대소문자가 다르면 허용하지 않는다.
		{"smart", "getList", RPCNone, nil},
		{"Smart", "getlist", RPCNone, nil},
		// 서비스와 메서드를 합친 문자열이 같아도 나눈 위치가 다르면 다른 메서드다.
		{"Smart.getList", "", RPCNone, nil},
	}
	for _, c := range cases {
		kind, ids := r.Classify(c.service, c.method)
		if kind != c.kind || !slices.Equal(ids, c.addons) {
			t.Errorf("Classify(%q, %q) = %v %v, want %v %v", c.service, c.method, kind, ids, c.kind, c.addons)
		}
	}
}
