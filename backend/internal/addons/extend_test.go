package addons

import (
	"slices"
	"strings"
	"testing"

	"maru/backend/internal/api"
)

// pkg는 검사를 통과하는 최소 패키지 애드온을 만든다.
func pkg(id string, edit func(*Addon)) Addon {
	a := Addon{
		Kind: KindPackage,
		Manifest: api.Manifest{
			ID:    id,
			Views: api.AddonViews{Window: &api.WindowView{DefaultSize: api.Size{Width: 400, Height: 300}}},
		},
		Providers: []Provider{fakeProvider{name: "package"}},
	}
	if edit != nil {
		edit(&a)
	}
	return a
}

func builtinBase(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(
		addon("storage", func(a *Addon) { a.Manifest.RPC.Read = []string{"FileSystemMgmt.enumerateFilesystems"} }),
		addon("containers", withAction("start")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func ids(r *Registry) []string {
	var out []string
	for _, a := range r.All() {
		out = append(out, a.Manifest.ID)
	}
	return out
}

func TestExtendAcceptsPackages(t *testing.T) {
	base := builtinBase(t)
	r, rej := Extend(base, []Addon{
		pkg("hello", func(a *Addon) { a.Manifest.RPC.Read = []string{"System.getInformation"} }),
		pkg("ups", func(a *Addon) {
			a.Manifest.Requires.Plugins = []string{"openmediavault-nut"}
			a.Plugins = map[string]string{"openmediavault-nut": "nut.inc"}
			a.Manifest.RPC.Read = []string{"Nut.getStats"}
		}),
	})
	if len(rej) > 0 {
		t.Fatalf("rejected: %+v", rej)
	}
	if got := ids(r); !slices.Equal(got, []string{"storage", "containers", "hello", "ups"}) {
		t.Errorf("ids = %v", got)
	}
	if got := ids(base); len(got) != 2 {
		t.Errorf("base changed: %v", got)
	}
	if kind, who := r.Classify("Nut", "getStats"); kind != RPCRead || !slices.Equal(who, []string{"ups"}) {
		t.Errorf("Classify = %v %v", kind, who)
	}
}

func TestExtendRejects(t *testing.T) {
	cases := map[string]struct {
		a    Addon
		code string
	}{
		"builtin id":     {pkg("storage", nil), RejectConflictBuiltin},
		"reserved id":    {pkg("settings", nil), RejectReservedID},
		"id too long":    {pkg(strings.Repeat("a", MaxPackageIDLen+1), nil), RejectInvalid},
		"bad id":         {pkg("Hello", nil), RejectInvalid},
		"denied service": {pkg("x", func(a *Addon) { a.Manifest.RPC.Read = []string{"Exec.isRunning"} }), RejectRPCDenied},
		"denied case": {pkg("x", func(a *Addon) {
			a.Manifest.RPC.Read = []string{"exec.isRunning"}
		}), RejectRPCDenied},
		"denied method": {pkg("x", func(a *Addon) {
			a.Manifest.RPC.Read = []string{"Compose.getContainerLog"}
		}), RejectRPCDenied},
		"denied prefix": {pkg("x", func(a *Addon) {
			a.Manifest.RPC.Write = []string{"UserMgmt.setUser"}
			a.Manifest.Actions = []api.ActionDecl{{Name: "go"}}
		}), RejectRPCDenied},
		"write without action": {pkg("x", func(a *Addon) { a.Manifest.RPC.Write = []string{"Foo.set"} }), RejectInvalid},
		"kind conflict with builtin": {pkg("x", func(a *Addon) {
			a.Manifest.RPC.Write = []string{"fileSystemMgmt.enumerateFilesystems"}
			a.Manifest.Actions = []api.ActionDecl{{Name: "go"}}
		}), RejectRPCKind},
		"same method twice by case": {pkg("x", func(a *Addon) {
			a.Manifest.RPC.Read = []string{"Foo.get", "foo.get"}
		}), RejectInvalid},
		"bad rpc file": {pkg("x", func(a *Addon) {
			a.Manifest.Requires.Plugins = []string{"openmediavault-nut"}
			a.Plugins = map[string]string{"openmediavault-nut": "../nut.inc"}
		}), RejectInvalid},
		"known plugin other file": {pkg("x", func(a *Addon) {
			a.Manifest.Requires.Plugins = []string{"openmediavault-compose"}
			a.Plugins = map[string]string{"openmediavault-compose": "other.inc"}
		}), RejectInvalid},
		"undeclared plugin": {pkg("x", func(a *Addon) {
			a.Manifest.Requires.Plugins = []string{"openmediavault-nut"}
		}), RejectInvalid},
		"go implementation": {pkg("x", func(a *Addon) {
			withAction("go")(a)
			a.Manifest.RPC.Write = []string{"Foo.set"}
		}), RejectInvalid},
		"builtin kind": {addon("x", nil), RejectInvalid},
	}
	base := builtinBase(t)
	for name, c := range cases {
		r, rej := Extend(base, []Addon{c.a})
		if len(rej) != 1 || rej[0].Code != c.code || rej[0].ID != c.a.Manifest.ID {
			t.Errorf("%s: rejections = %+v, want code %s", name, rej, c.code)
			continue
		}
		if got := ids(r); !slices.Equal(got, []string{"storage", "containers"}) {
			t.Errorf("%s: ids = %v", name, got)
		}
	}
}

// 하나가 나빠도 나머지는 받고, 같은 id나 종류가 겹치면 앞의 것이 이긴다.
func TestExtendOrder(t *testing.T) {
	base := builtinBase(t)
	r, rej := Extend(base, []Addon{
		pkg("a", func(a *Addon) { a.Manifest.RPC.Read = []string{"Foo.get"} }),
		pkg("bad", func(a *Addon) { a.Manifest.RPC.Read = []string{"Session.login"} }),
		pkg("a", nil),
		pkg("b", func(a *Addon) {
			a.Manifest.RPC.Write = []string{"Foo.get"}
			a.Manifest.Actions = []api.ActionDecl{{Name: "go"}}
		}),
		pkg("c", func(a *Addon) { a.Manifest.RPC.Read = []string{"Foo.get"} }),
	})
	if got := ids(r); !slices.Equal(got, []string{"storage", "containers", "a", "c"}) {
		t.Errorf("ids = %v", got)
	}
	var codes []string
	for _, x := range rej {
		codes = append(codes, x.ID+":"+x.Code)
	}
	want := []string{"bad:" + RejectRPCDenied, "a:" + RejectConflictPackage, "b:" + RejectRPCKind}
	if !slices.Equal(codes, want) {
		t.Errorf("rejections = %v, want %v", codes, want)
	}
	if _, who := r.Classify("Foo", "get"); !slices.Equal(who, []string{"a", "c"}) {
		t.Errorf("Foo.get declared by %v", who)
	}
}

// 패키지 조작은 선언만 있고 Go 구현이 없으며 OMV 쓰기 없이도 된다.
func TestExtendPackageActions(t *testing.T) {
	r, rej := Extend(builtinBase(t), []Addon{
		pkg("plug", func(a *Addon) { a.Manifest.Actions = []api.ActionDecl{{Name: "toggle", Confirm: true}} }),
	})
	if len(rej) > 0 {
		t.Fatalf("rejected: %+v", rej)
	}
	d, act, ok := r.ActionDecl("plug", "toggle")
	if !ok || !d.Confirm || act.Run != nil || act.Query != nil {
		t.Errorf("ActionDecl = %+v %+v %v", d, act, ok)
	}
}

func TestDeniedForPackage(t *testing.T) {
	denied := []string{
		"Session.login", "session.logout", "Exec.getOutput", "Config.applyChangesBg", "Cron.execute",
		"Scripts.doRun", "PluginMgmt.install", "Apttool.doCommand", "Omvextras.setRepo", "EnvVars.set",
		"Apt.install", "apt.upgrade", "Apt.getChangeLog", "Compose.doContainerCommand", "COMPOSE.getContainerLog",
		"UserMgmt.setUser", "UserMgmt.deleteGroup", "UserMgmt.importUsers", "UserMgmt.authUser",
		"EmailNotification.get", "Rsync.get", "Network.getProxy", "Network.getInterfaceList",
		"Symlinks.setSymlink", "Kvm.setVmXml",
	}
	for _, m := range denied {
		if reason, ok := DeniedForPackage(m); !ok || reason == "" {
			t.Errorf("%s: not denied", m)
		}
	}
	allowed := []string{
		"System.getInformation", "Compose.getContainers", "UserMgmt.getUserList", "Apt.enumerateUpgraded",
		"Rsync.getList", "Network.getGeneralSettings", "Kvm.getVmNameStateList", "Nut.getStats",
		// 메서드 이름은 대소문자를 구분한다(OMV의 hasMethod). 다른 메서드다.
		"Compose.GetContainerLog",
	}
	for _, m := range allowed {
		if _, ok := DeniedForPackage(m); ok {
			t.Errorf("%s: denied", m)
		}
	}
}

func TestReservedAppIDs(t *testing.T) {
	got := ReservedAppIDs()
	if !slices.IsSorted(got) || !slices.Contains(got, "files") || !slices.Contains(got, "workbench") {
		t.Errorf("ReservedAppIDs = %v", got)
	}
	for _, id := range got {
		if !idPattern.MatchString(id) {
			t.Errorf("reserved id %q has an invalid format", id)
		}
	}
}
