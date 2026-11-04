package addonpkg

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

type testProvider struct{}

func (testProvider) Name() string               { return "direct" }
func (testProvider) Requires() api.Requirements { return api.Requirements{} }

func builtinRegistry(t *testing.T) *addons.Registry {
	t.Helper()
	r, err := addons.NewRegistry(addons.Addon{
		Manifest: api.Manifest{
			ID: "storage", Title: "apps.storage", Icon: "drive-harddisk",
			Views: api.AddonViews{Widget: &api.WidgetView{DefaultSize: api.GridSize{Cols: 2, Rows: 2}}},
			RPC:   api.RPCMethods{Read: []string{"FileSystemMgmt.enumerateFilesystems"}},
		},
		Providers: []addons.Provider{testProvider{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type fakeDetector struct{ extra map[string]string }

func (d *fakeDetector) SetExtra(m map[string]string) { d.extra = m }

func newManager(t *testing.T, opt Options) *Manager {
	t.Helper()
	if opt.Store == nil {
		opt.Store = NewStore(filepath.Join(t.TempDir(), "addons"))
	}
	if opt.Arch == "" {
		opt.Arch = "amd64"
	}
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	opt.Now = func() time.Time { now = now.Add(time.Minute); return now }
	return NewManager(builtinRegistry(t), opt)
}

func archive(t *testing.T, edit func(map[string]any)) []byte {
	t.Helper()
	return pack(t, basic(manifestJSON(t, edit)))
}

func install(t *testing.T, m *Manager, data []byte) *Status {
	t.Helper()
	p, err := m.Prepare(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	s, err := m.Install(p, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestManagerInstallLifecycle(t *testing.T) {
	det := &fakeDetector{}
	m := newManager(t, Options{Plugins: det})
	if _, ok := m.Registry().Get("hello"); ok {
		t.Fatal("hello registered before install")
	}
	s := install(t, m, archive(t, func(mf map[string]any) {
		mf["rpc"] = map[string]any{"read": []string{"Nut.getStats"}}
		mf["requires"] = map[string]any{"plugins": []any{map[string]any{"package": "openmediavault-nut", "rpcFile": "nut.inc"}}}
	}))
	if s.State != StateActive || s.Record.InstallUID == "" || s.Record.InstalledBy != "admin" {
		t.Fatalf("status = %+v", s)
	}
	if _, ok := m.Registry().Get("hello"); !ok {
		t.Fatal("hello not registered")
	}
	inst, ok := m.Get("hello")
	if !ok || inst.Rev != s.Rev || inst.Dir != m.opt.Store.Path("hello", s.Rev) || !slices.Equal(inst.Languages, []string{"en"}) {
		t.Fatalf("installed = %+v", inst)
	}
	if det.extra["openmediavault-nut"] != "nut.inc" {
		t.Errorf("plugin table = %v", det.extra)
	}
	uid := s.Record.InstallUID

	// 업데이트는 설치 ID와 처음 설치 시각을 유지하고 옛 rev를 지운다.
	p, err := m.Prepare(bytes.NewReader(archive(t, func(mf map[string]any) {
		mf["version"] = "1.1.0"
		mf["author"] = map[string]any{"name": "Someone else"}
		mf["rpc"] = map[string]any{"read": []string{"Nut.getStats", "System.getInformation"}}
		mf["requires"] = map[string]any{"plugins": []any{map[string]any{"package": "openmediavault-nut", "rpcFile": "nut.inc"}}}
	})))
	if err != nil {
		t.Fatal(err)
	}
	if p.Previous == nil || p.Diff == nil || !slices.Equal(p.Diff.AddedRead, []string{"System.getInformation"}) || !slices.Contains(p.Warnings, WarnAuthorChanged) {
		t.Errorf("pending = %+v diff %+v", p, p.Diff)
	}
	s2, err := m.Install(p, "admin2")
	if err != nil {
		t.Fatal(err)
	}
	if s2.Record.InstallUID != uid || !s2.Record.InstalledAt.Equal(s.Record.InstalledAt) || s2.Version != "1.1.0" {
		t.Errorf("updated = %+v", s2.Record)
	}
	if _, err := os.Stat(m.opt.Store.Path("hello", s.Rev)); !os.IsNotExist(err) {
		t.Errorf("old rev kept: %v", err)
	}

	// 낮은 버전은 경고와 함께 받는다.
	p, err = m.Prepare(bytes.NewReader(archive(t, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Warnings, WarnDowngrade) {
		t.Errorf("warnings = %v", p.Warnings)
	}
	p.Discard()

	// 끄면 레지스트리에서 빠지고 목록에는 남는다.
	if s, err := m.SetEnabled("hello", false); err != nil || s.State != StateDisabled {
		t.Fatalf("disable: %+v %v", s, err)
	}
	if _, ok := m.Registry().Get("hello"); ok {
		t.Error("disabled package still registered")
	}
	if len(det.extra) != 0 {
		t.Errorf("plugin table kept: %v", det.extra)
	}
	if s, err := m.SetEnabled("hello", true); err != nil || s.State != StateActive {
		t.Fatalf("enable: %+v %v", s, err)
	}

	// 다른 관리자가 다시 시작해도(새 Manager) 같은 상태를 읽는다.
	m2 := NewManager(builtinRegistry(t), Options{Store: m.opt.Store, Arch: "amd64"})
	if got, ok := m2.Get("hello"); !ok || got.Rev != s2.Rev {
		t.Errorf("reloaded = %+v %v", got, ok)
	}

	if err := m.Uninstall("hello"); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Registry().Get("hello"); ok {
		t.Error("uninstalled package still registered")
	}
	if list, _ := m.List(); len(list) != 0 {
		t.Errorf("list = %+v", list)
	}
	if err := m.Uninstall("hello"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second uninstall: %v", err)
	}
}

func TestManagerPrepareRejects(t *testing.T) {
	m := newManager(t, Options{})
	var rj *RejectError
	if _, err := m.Prepare(bytes.NewReader(archive(t, func(mf map[string]any) { mf["id"] = "storage" }))); !errors.As(err, &rj) || rj.Code != addons.RejectConflictBuiltin {
		t.Errorf("builtin id: %v", err)
	}
	if _, err := m.Prepare(bytes.NewReader(archive(t, func(mf map[string]any) {
		mf["rpc"] = map[string]any{"read": []string{"Exec.getOutput"}}
	}))); !errors.As(err, &rj) || rj.Code != addons.RejectRPCDenied {
		t.Errorf("denied rpc: %v", err)
	}
	var e *Error
	data := pack(t, append(basic(manifestJSON(t, func(mf map[string]any) { mf["backend"] = map[string]any{"network": []string{}} })),
		entry{name: "backend/linux-amd64", typ: tar.TypeReg, body: fakeELF(0x3e)}))
	if _, err := m.Prepare(bytes.NewReader(data)); !errors.As(err, &e) || e.Reason != ReasonBackendUnsupported {
		t.Errorf("backend: %v", err)
	}
	// 실패한 대기는 스테이징을 남기지 않는다.
	entries, _ := os.ReadDir(m.opt.Store.Dir())
	for _, en := range entries {
		if en.Name() != indexFile {
			t.Errorf("left %s", en.Name())
		}
	}
	// 메서드 종류가 먼저 설치한 패키지와 다르면 받지 않는다.
	install(t, m, archive(t, func(mf map[string]any) { mf["rpc"] = map[string]any{"read": []string{"Foo.get"}} }))
	if _, err := m.Prepare(bytes.NewReader(archive(t, func(mf map[string]any) {
		mf["id"] = "other"
		mf["rpc"] = map[string]any{"write": []string{"Foo.get"}}
		mf["actions"] = []any{map[string]any{"name": "go"}}
		mf["backend"] = map[string]any{"network": []string{}}
	}))); err == nil {
		t.Error("kind conflict accepted")
	}
}

func TestManagerNoStore(t *testing.T) {
	m := NewManager(builtinRegistry(t), Options{Arch: "amd64"})
	if m.CanInstall() {
		t.Error("CanInstall without a store")
	}
	if _, err := m.Prepare(bytes.NewReader(nil)); !errors.Is(err, ErrNoStore) {
		t.Errorf("Prepare: %v", err)
	}
}

func TestManagerDevPackages(t *testing.T) {
	dev := t.TempDir()
	write := func(name, body string) {
		p := filepath.Join(dev, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range basic(manifestJSON(t, nil)) {
		if e.typ == tar.TypeReg {
			write(e.name, string(e.body))
		}
	}
	write("dev/fake-rpc.json", `{}`)
	// 먼저 설치한 같은 id의 패키지가 있어도 개발본이 이긴다(개발자가 고치는 중이다).
	plain := newManager(t, Options{})
	install(t, plain, archive(t, nil))
	m := newManager(t, Options{Store: plain.opt.Store, DevDirs: []string{dev}})
	inst, ok := m.Get("hello")
	if !ok || !inst.Dev || !ValidRev(inst.Rev) || inst.Dir != dev {
		t.Fatalf("dev package = %+v %v", inst, ok)
	}
	rev := inst.Rev
	if s, _ := m.Status("hello"); s.State != StateRejected || s.Reason != addons.RejectConflictPackage {
		t.Errorf("installed status = %+v", s)
	}
	// 개발 패키지와 같은 id는 설치할 수 없다.
	p, err := m.Prepare(bytes.NewReader(archive(t, func(mf map[string]any) { mf["version"] = "2.0.0" })))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Install(p, "admin"); !errors.Is(err, ErrDev) {
		t.Errorf("install over a dev package: %v", err)
	}
	if _, err := m.SetEnabled("nope", true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetEnabled(nope): %v", err)
	}

	// 파일이 바뀌면 다시 불러와 rev가 바뀐다.
	time.Sleep(10 * time.Millisecond)
	write("ui/window.html", "<!doctype html><p>changed</p>")
	m.RescanDev()
	if got, _ := m.Get("hello"); got.Rev == rev {
		t.Error("rev did not change after editing a dev file")
	}

	// 잘못된 개발 디렉터리는 목록에 이유와 함께 남는다.
	write("ui/.hidden", "x")
	m.RescanDev()
	// 개발본이 잘못되면 설치본이 대신 들어간다.
	if got, ok := m.Get("hello"); !ok || got.Dev {
		t.Fatalf("after invalid dev package: %+v %v", got, ok)
	}
	list, _ := m.List()
	found := false
	for _, s := range list {
		if s.Dev && s.State == StateInvalid && s.Reason == ReasonArchive {
			found = true
		}
	}
	if !found {
		t.Errorf("list = %+v", list)
	}
}

func TestManagerCorruptIndex(t *testing.T) {
	m := newManager(t, Options{})
	install(t, m, archive(t, nil))
	if err := os.WriteFile(filepath.Join(m.opt.Store.Dir(), indexFile), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.Reload()
	if _, err := m.List(); !errors.Is(err, ErrIndex) {
		t.Errorf("List err = %v", err)
	}
	if _, ok := m.Get("hello"); ok {
		t.Error("package registered from a corrupt index")
	}
	if _, ok := m.Registry().Get("storage"); !ok {
		t.Error("built-ins lost")
	}
}
