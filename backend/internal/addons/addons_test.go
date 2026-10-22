package addons

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"maru/backend/internal/api"
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

func TestNewRegistryAcceptsViews(t *testing.T) {
	widget := &api.WidgetView{
		DefaultSize: api.GridSize{Cols: 3, Rows: 2},
		MinSize:     &api.GridSize{Cols: 2, Rows: 2},
		MaxSize:     &api.GridSize{Cols: MaxWidgetSpan, Rows: MaxWidgetSpan},
	}
	cases := map[string]Addon{
		"window only": addon("a", nil),
		"widget only": addon("a", func(a *Addon) {
			a.Manifest.Views.Window = nil
			a.Manifest.Views.Widget = widget
		}),
		"window and widget": addon("a", func(a *Addon) { a.Manifest.Views.Widget = widget }),
		"fixed widget": addon("a", func(a *Addon) {
			a.Manifest.Views.Widget = &api.WidgetView{DefaultSize: api.GridSize{Cols: 1, Rows: 1}}
		}),
	}
	for name, a := range cases {
		if _, err := NewRegistry(a); err != nil {
			t.Errorf("%s: %v", name, err)
		}
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
		"zero widget size": {addon("a", func(a *Addon) {
			a.Manifest.Views.Widget = &api.WidgetView{}
		})},
		"widget too large": {addon("a", func(a *Addon) {
			a.Manifest.Views.Widget = &api.WidgetView{DefaultSize: api.GridSize{Cols: MaxWidgetSpan + 1, Rows: 1}}
		})},
		"widget min over default": {addon("a", func(a *Addon) {
			a.Manifest.Views.Widget = &api.WidgetView{
				DefaultSize: api.GridSize{Cols: 2, Rows: 2},
				MinSize:     &api.GridSize{Cols: 3, Rows: 1},
			}
		})},
		"widget max below default": {addon("a", func(a *Addon) {
			a.Manifest.Views.Widget = &api.WidgetView{
				DefaultSize: api.GridSize{Cols: 2, Rows: 2},
				MaxSize:     &api.GridSize{Cols: 3, Rows: 1},
			}
		})},
		"widget max too large": {addon("a", func(a *Addon) {
			a.Manifest.Views.Widget = &api.WidgetView{
				DefaultSize: api.GridSize{Cols: 2, Rows: 2},
				MaxSize:     &api.GridSize{Cols: 2, Rows: MaxWidgetSpan + 1},
			}
		})},
		"bad method": {addon("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Smart getList"} })},
		"no method":  {addon("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Smart"} })},
		"session":    {addon("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Session.login"} })},
		"method twice": {addon("a", func(a *Addon) {
			a.Manifest.RPC.Read = []string{"Smart.getList"}
			a.Manifest.RPC.Write = []string{"Smart.getList"}
		})},
		"read and write across addons": {
			addon("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Compose.getContainers"} }),
			addon("b", func(a *Addon) { a.Manifest.RPC.Write = []string{"Compose.getContainers"} }),
		},
		"empty plugin":   {addon("a", func(a *Addon) { a.Manifest.Requires.Plugins = []string{""} })},
		"unknown plugin": {addon("a", func(a *Addon) { a.Manifest.Requires.Plugins = []string{"openmediavault-nope"} })},
		"unknown provider plugin": {addon("a", func(a *Addon) {
			a.Providers = []Provider{fakeProvider{name: "x", req: api.Requirements{Plugins: []string{"nope"}}}}
		})},
		"no provider": {addon("a", func(a *Addon) { a.Providers = nil })},
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

// withAction은 쓰기 RPC 하나와 조작 name을 선언하고 구현한다.
func withAction(name string) func(*Addon) {
	return func(a *Addon) {
		a.Manifest.RPC.Write = []string{"Compose.doContainerCommand"}
		a.Manifest.Actions = append(a.Manifest.Actions, api.ActionDecl{Name: name, Admin: true})
		if a.Actions == nil {
			a.Actions = map[string]Action{}
		}
		a.Actions[name] = Action{Run: func(context.Context, ActionRequest) error { return nil }}
	}
}

func TestNewRegistryActions(t *testing.T) {
	r, err := NewRegistry(addon("a", func(a *Addon) { withAction("start")(a); withAction("stop")(a) }))
	if err != nil {
		t.Fatal(err)
	}
	if d, act, ok := r.ActionDecl("a", "stop"); !ok || d.Name != "stop" || !d.Admin || act.Run == nil {
		t.Errorf("ActionDecl(a, stop) = %+v %v", d, ok)
	}
	if _, _, ok := r.ActionDecl("a", "restart"); ok {
		t.Error("ActionDecl(a, restart) found")
	}
	if _, _, ok := r.ActionDecl("b", "stop"); ok {
		t.Error("ActionDecl(b, stop) found")
	}

	cases := map[string]Addon{
		"no write rpc": addon("a", func(a *Addon) {
			withAction("start")(a)
			a.Manifest.RPC.Write = nil
		}),
		"bad name":       addon("a", withAction("Start")),
		"declared twice": addon("a", func(a *Addon) { withAction("start")(a); withAction("start")(a) }),
		"no implementation": addon("a", func(a *Addon) {
			withAction("start")(a)
			delete(a.Actions, "start")
		}),
		"nil run": addon("a", func(a *Addon) {
			withAction("start")(a)
			a.Actions["start"] = Action{}
		}),
		"undeclared implementation": addon("a", func(a *Addon) {
			withAction("start")(a)
			a.Actions["stop"] = Action{Run: func(context.Context, ActionRequest) error { return nil }}
		}),
	}
	for name, a := range cases {
		if _, err := NewRegistry(a); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// bgCaller는 Exec.isRunning에 정해진 응답을 차례로 주는 가짜 RPCCaller다.
type bgCaller struct {
	running []bool
	err     error
	calls   int
}

func (c *bgCaller) Call(_ context.Context, service, method string, params any) (json.RawMessage, error) {
	if service+"."+method != "Exec.isRunning" {
		return nil, errors.New("unexpected rpc " + service + "." + method)
	}
	if p, _ := params.(map[string]string); p["filename"] != "bgstatus123" {
		return nil, errors.New("unexpected params")
	}
	c.calls++
	if c.err != nil && c.calls > len(c.running) {
		return nil, c.err
	}
	running := c.running[min(c.calls, len(c.running))-1]
	return json.Marshal(map[string]any{"filename": "bgstatus123", "running": running})
}

func TestWaitBg(t *testing.T) {
	ctx := context.Background()

	c := &bgCaller{running: []bool{true, true, false}}
	if err := WaitBg(ctx, c, "bgstatus123", time.Millisecond); err != nil || c.calls != 3 {
		t.Errorf("done: err=%v calls=%d", err, c.calls)
	}

	c = &bgCaller{running: []bool{true}, err: errors.New("omv rpc error (HTTP 500): docker failed")}
	if err := WaitBg(ctx, c, "bgstatus123", time.Millisecond); !errors.Is(err, ErrActionFailed) {
		t.Errorf("failed: err=%v", err)
	}

	if err := WaitBg(ctx, &bgCaller{}, "", time.Millisecond); !errors.Is(err, ErrActionFailed) {
		t.Errorf("empty filename: err=%v", err)
	}

	tctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	if err := WaitBg(tctx, &bgCaller{running: []bool{true}}, "bgstatus123", 5*time.Millisecond); !errors.Is(err, ErrActionTimeout) {
		t.Errorf("timeout: err=%v", err)
	}
}
