package addonpkg

import (
	"archive/tar"
	"bytes"
	"debug/elf"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"maru/backend/internal/addons"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(filepath.Join(t.TempDir(), "addons"))
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestStoreIndex(t *testing.T) {
	s := newStore(t)
	if list, err := s.Load(); err != nil || list != nil {
		t.Fatalf("empty store: %v %v", list, err)
	}
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	want := []Record{{ID: "hello", Version: "1.0.0", Rev: "1.0.0-0123abcd", SHA256: "0123abcd", Enabled: true, InstalledAt: now, UpdatedAt: now, InstallUID: "u1"}}
	if err := s.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || len(got) != 1 || got[0].ID != "hello" || !got[0].InstalledAt.Equal(now) {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	fi, err := os.Stat(filepath.Join(s.Dir(), indexFile))
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("index mode = %v, %v", fi.Mode(), err)
	}

	// 손상된 기록은 옮기거나 비우지 않고 오류로 둔다.
	for name, body := range map[string]string{
		"not json":  "{",
		"version":   `{"version":2,"packages":[]}`,
		"bad id":    `{"version":1,"packages":[{"id":"../x","rev":"1.0.0-0123abcd"}]}`,
		"bad rev":   `{"version":1,"packages":[{"id":"x","rev":"../../etc"}]}`,
		"duplicate": `{"version":1,"packages":[{"id":"x","rev":"1.0.0-0123abcd"},{"id":"x","rev":"1.0.0-0123abcd"}]}`,
	} {
		if err := os.WriteFile(filepath.Join(s.Dir(), indexFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Load(); !errors.Is(err, ErrIndex) {
			t.Errorf("%s: err = %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(s.Dir(), indexFile)); err != nil {
			t.Errorf("%s: index moved: %v", name, err)
		}
	}
}

func TestStoreCommit(t *testing.T) {
	s := newStore(t)
	install := func(version string) *Package {
		m := manifestJSON(t, func(m map[string]any) {
			m["version"] = version
			m["backend"] = map[string]any{"network": []string{}}
		})
		data := pack(t, append(basic(m),
			entry{name: "backend/linux-amd64", typ: tar.TypeReg, body: fakeELF(elf.EM_X86_64)},
			entry{name: "backend/linux-arm64", typ: tar.TypeReg, body: fakeELF(elf.EM_AARCH64)},
		))
		st, err := s.NewStaging()
		if err != nil {
			t.Fatal(err)
		}
		p, err := Read(bytes.NewReader(data), DefaultLimits, st.Root, "amd64")
		if err != nil {
			st.Discard()
			t.Fatal(err)
		}
		if err := s.Commit(st, "hello", p.Rev(), "amd64"); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p1 := install("1.0.0")
	dir := s.Path("hello", p1.Rev())
	fi, err := os.Stat(filepath.Join(dir, "backend", "run"))
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("backend/run: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend", "linux-arm64")); !os.IsNotExist(err) {
		t.Errorf("other arch kept: %v", err)
	}
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("rev dir mode: %v %v", fi, err)
	}
	if target, err := os.Readlink(filepath.Join(s.Dir(), "hello", currentLink)); err != nil || target != p1.Rev() {
		t.Errorf("current = %q, %v", target, err)
	}

	p2 := install("1.1.0")
	if target, _ := os.Readlink(filepath.Join(s.Dir(), "hello", currentLink)); target != p2.Rev() {
		t.Errorf("current = %q after update", target)
	}
	if err := s.Prune("hello", p2.Rev()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Path("hello", p1.Rev())); !os.IsNotExist(err) {
		t.Errorf("old rev kept: %v", err)
	}
	if _, err := os.Stat(s.Path("hello", p2.Rev())); err != nil {
		t.Errorf("new rev removed: %v", err)
	}

	// 같은 파일을 다시 설치하면 있는 rev를 그대로 쓴다.
	install("1.1.0")
	if err := s.Remove("hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "hello")); !os.IsNotExist(err) {
		t.Errorf("package dir kept: %v", err)
	}
}

func TestStoreInitCleansStaging(t *testing.T) {
	s := newStore(t)
	st, err := s.NewStaging()
	if err != nil {
		t.Fatal(err)
	}
	st.Root.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.Dir); !os.IsNotExist(err) {
		t.Errorf("staging kept: %v", err)
	}
}

func TestStoreRejectsBadNames(t *testing.T) {
	s := newStore(t)
	if err := s.Remove("../x"); err == nil {
		t.Error("Remove accepted a bad id")
	}
	if err := s.Prune("a/b", "x"); err == nil {
		t.Error("Prune accepted a bad id")
	}
	for _, rev := range []string{"..", "1.0.0", "1.0.0-0123ABCD", "../1.0.0-0123abcd", "1..0-0123abcd"} {
		if ValidRev(rev) {
			t.Errorf("ValidRev(%q) = true", rev)
		}
	}
	if !ValidRev("1.0.0-rc.1-0123abcd") || !ValidRev("dev-0123abcd") {
		t.Error("ValidRev rejected a valid rev")
	}
}

func TestSummarize(t *testing.T) {
	m := manifestJSON(t, func(m map[string]any) {
		m["rpc"] = map[string]any{"read": []string{"Nut.getStats", "System.getInformation"}, "write": []string{"Nut.set"}}
		m["actions"] = []any{map[string]any{"name": "toggle", "confirm": true, "confirmText": map[string]any{"en": "Sure?"}}}
		m["backend"] = map[string]any{"network": []string{"loopback", "lan"}}
		m["requires"] = map[string]any{"admin": true, "plugins": []any{map[string]any{"package": "openmediavault-nut", "rpcFile": "nut.inc"}}}
	})
	data := pack(t, append(basic(m), entry{name: "backend/linux-amd64", typ: tar.TypeReg, body: fakeELF(elf.EM_X86_64)}))
	p, _, err := readPkg(t, data, DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	s := Summarize(p.Manifest)
	if !s.Admin || !s.Backend || !s.Elevated() || len(s.Plugins) != 1 {
		t.Errorf("summary = %+v", s)
	}
	wantRisks := []string{RiskOMVWrite, RiskBackend, RiskBackendSharedData, RiskNetLAN, RiskNetLoopback}
	if !slices.Equal(s.Risks, wantRisks) || !slices.Equal(s.Network, []string{"lan", "loopback"}) {
		t.Errorf("risks %v network %v", s.Risks, s.Network)
	}
	if s.Digest() != Summarize(p.Manifest).Digest() {
		t.Error("digest is not stable")
	}

	base := Summarize(mustManifest(t, manifestJSON(t, nil)))
	if base.Elevated() || len(base.Risks) != 0 {
		t.Errorf("plain package: %+v", base)
	}
	d := Diff(base, s)
	if !d.Widens() || !d.BackendAdded || !slices.Equal(d.AddedWrite, []string{"Nut.set"}) {
		t.Errorf("diff = %+v", d)
	}
	if Diff(s, s).Widens() {
		t.Error("same summary widens")
	}

	a := ToAddon(p.Manifest)
	if a.Kind != addons.KindPackage || a.Plugins["openmediavault-nut"] != "nut.inc" || len(a.Manifest.Actions) != 1 || a.Manifest.Views.Window == nil {
		t.Errorf("addon = %+v", a)
	}
	if _, rej := addons.Extend(mustRegistry(t), []addons.Addon{a}); len(rej) != 0 {
		t.Errorf("converted addon rejected: %+v", rej)
	}
}

func mustRegistry(t *testing.T) *addons.Registry {
	t.Helper()
	r, err := addons.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
