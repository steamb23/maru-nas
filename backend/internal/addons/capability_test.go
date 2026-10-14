package addons

import (
	"bytes"
	"log"
	"maps"
	"os"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"onmaru/backend/internal/api"
)

func TestParseInstalledPackages(t *testing.T) {
	f, err := os.Open("testdata/dpkg-status")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseInstalledPackages(f)
	if err != nil {
		t.Fatal(err)
	}
	// 설명의 연속 줄에 든 "Package:"는 필드가 아니다. 마지막 문단은 빈 줄 없이 끝난다.
	want := []string{"openmediavault", "openmediavault-bcache", "openmediavault-compose", "openmediavault-kvm"}
	if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, want) {
		t.Errorf("installed = %v, want %v", keys, want)
	}
}

type detectorEnv struct {
	d    *Detector
	rpc  fstest.MapFS
	dpkg fstest.MapFS
	now  time.Time
}

func newDetectorEnv(t *testing.T) *detectorEnv {
	t.Helper()
	status, err := os.ReadFile("testdata/dpkg-status")
	if err != nil {
		t.Fatal(err)
	}
	e := &detectorEnv{
		// cputemp는 파일은 있지만 dpkg 상태가 설치가 아니고, bcache는 설치됐지만 파일이 없다.
		rpc: fstest.MapFS{
			"compose.inc": {},
			"cputemp.inc": {},
			"kvm.inc":     {},
		},
		dpkg: fstest.MapFS{"status": {Data: status}},
		now:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	e.d = NewDetectorFS(e.rpc, e.dpkg)
	e.d.Now = func() time.Time { return e.now }
	return e
}

func pluginNames(m map[string]bool) []string { return slices.Sorted(maps.Keys(m)) }

func TestDetectorNeedsFileAndPackage(t *testing.T) {
	e := newDetectorEnv(t)
	want := []string{"openmediavault-compose", "openmediavault-kvm"}
	if got := pluginNames(e.d.Plugins()); !slices.Equal(got, want) {
		t.Errorf("plugins = %v, want %v", got, want)
	}
}

func TestDetectorRechecks(t *testing.T) {
	e := newDetectorEnv(t)
	e.d.Plugins()

	// 플러그인을 지워도 TTL 안에서는 이전 결과를 쓴다.
	delete(e.rpc, "kvm.inc")
	e.now = e.now.Add(e.d.TTL - time.Second)
	if got := pluginNames(e.d.Plugins()); len(got) != 2 {
		t.Errorf("within ttl: %v", got)
	}
	// TTL이 지나면 다시 확인한다.
	e.now = e.now.Add(time.Second)
	if got := pluginNames(e.d.Plugins()); !slices.Equal(got, []string{"openmediavault-compose"}) {
		t.Errorf("after ttl: %v", got)
	}

	// Invalidate 뒤에는 TTL과 상관없이 다시 확인한다.
	delete(e.rpc, "compose.inc")
	e.d.Invalidate()
	if got := e.d.Plugins(); len(got) != 0 {
		t.Errorf("after invalidate: %v", got)
	}

	// Refresh는 바로 다시 확인한다.
	e.rpc["kvm.inc"] = &fstest.MapFile{}
	e.d.Refresh()
	e.rpc["compose.inc"] = &fstest.MapFile{}
	if got := pluginNames(e.d.Plugins()); !slices.Equal(got, []string{"openmediavault-kvm"}) {
		t.Errorf("after refresh: %v", got)
	}
}

func TestDetectorKeepsResultOnReadError(t *testing.T) {
	logs := &bytes.Buffer{}
	prev := log.Writer()
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	e := newDetectorEnv(t)
	e.d.Plugins()
	delete(e.dpkg, "status")
	e.d.Refresh()
	if got := e.d.Plugins(); len(got) != 2 {
		t.Errorf("plugins after read error = %v", got)
	}
	if logs.Len() == 0 {
		t.Error("read error not logged")
	}

	// 처음부터 읽지 못하면 아무것도 설치되지 않은 것으로 본다.
	empty := NewDetectorFS(fstest.MapFS{}, fstest.MapFS{})
	if got := empty.Plugins(); got == nil || len(got) != 0 {
		t.Errorf("plugins without status = %v", got)
	}
}

func TestCapability(t *testing.T) {
	compose := "openmediavault-compose"
	cputemp := "openmediavault-cputemp"
	installed := map[string]bool{compose: true}
	prov := func(name string, admin bool, plugins ...string) Provider {
		return fakeProvider{name: name, req: api.Requirements{Admin: admin, Plugins: plugins}}
	}
	cases := []struct {
		name  string
		addon Addon
		admin bool
		want  api.AddonCapability
	}{
		{
			name:  "no requirements",
			addon: addon("a", nil),
			want:  api.AddonCapability{ID: "a", Available: true, Sources: []string{"direct"}},
		},
		{
			name:  "plugin installed",
			addon: addon("a", func(a *Addon) { a.Manifest.Requires.Plugins = []string{compose} }),
			want:  api.AddonCapability{ID: "a", Available: true, Sources: []string{"direct"}},
		},
		{
			name:  "plugin missing",
			addon: addon("a", func(a *Addon) { a.Manifest.Requires.Plugins = []string{cputemp} }),
			admin: true,
			want:  api.AddonCapability{ID: "a", Reason: api.ReasonPluginMissing, Sources: []string{}},
		},
		{
			name:  "admin required",
			addon: addon("a", func(a *Addon) { a.Manifest.Requires.Admin = true }),
			want:  api.AddonCapability{ID: "a", Reason: api.ReasonAdminRequired, Sources: []string{}},
		},
		{
			name: "admin required before plugin missing",
			addon: addon("a", func(a *Addon) {
				a.Manifest.Requires = api.Requirements{Admin: true, Plugins: []string{cputemp}}
			}),
			want: api.AddonCapability{ID: "a", Reason: api.ReasonAdminRequired, Sources: []string{}},
		},
		{
			name: "providers filtered",
			addon: addon("a", func(a *Addon) {
				a.Providers = []Provider{prov("smart", true), prov("hwmon", false), prov("cputemp", false, cputemp)}
			}),
			want: api.AddonCapability{ID: "a", Available: true, Sources: []string{"hwmon"}},
		},
		{
			name: "all providers unavailable",
			addon: addon("a", func(a *Addon) {
				a.Providers = []Provider{prov("cputemp", false, cputemp), prov("smart", true)}
			}),
			want: api.AddonCapability{ID: "a", Reason: api.ReasonPluginMissing, Sources: []string{}},
		},
		{
			name: "admin sees admin providers",
			addon: addon("a", func(a *Addon) {
				a.Providers = []Provider{prov("smart", true), prov("hwmon", false)}
			}),
			admin: true,
			want:  api.AddonCapability{ID: "a", Available: true, Sources: []string{"smart", "hwmon"}},
		},
	}
	for _, c := range cases {
		got := Capability(c.addon, installed, c.admin)
		if got.ID != c.want.ID || got.Available != c.want.Available || got.Reason != c.want.Reason ||
			!slices.Equal(got.Sources, c.want.Sources) || got.Sources == nil {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestRegistryCapabilities(t *testing.T) {
	r, err := NewRegistry(
		addon("b", nil),
		addon("a", func(a *Addon) { a.Manifest.Requires.Admin = true }),
	)
	if err != nil {
		t.Fatal(err)
	}
	plugins := map[string]bool{"openmediavault-kvm": true, "openmediavault-compose": true}
	res := r.Capabilities(plugins, false)
	if len(res.Addons) != 2 || res.Addons[0].ID != "b" || !res.Addons[0].Available || res.Addons[1].Available {
		t.Errorf("addons = %+v", res.Addons)
	}
	if !slices.Equal(res.Plugins, []string{"openmediavault-compose", "openmediavault-kvm"}) {
		t.Errorf("plugins = %v", res.Plugins)
	}
	if empty, _ := NewRegistry(); empty.Capabilities(nil, true).Plugins == nil {
		t.Error("plugins must be an empty list, not null")
	}
}
