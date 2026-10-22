package addons

import (
	"bytes"
	"log"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/api"
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
	// 버전은 문단마다 따로 읽는다. 설명 뒤에 와도 된다.
	if got["openmediavault"] != "8.0.0-1" || got["openmediavault-compose"] != "8.0.0" {
		t.Errorf("versions = %v", got)
	}
}

func TestParseProductInfo(t *testing.T) {
	f, err := os.Open("testdata/productinfo.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseProductInfo(f)
	if err != nil || got != "Synthetic" {
		t.Errorf("version name = %q, %v", got, err)
	}

	// 다른 XML 문서는 제품 정보가 아니다.
	if _, err := ParseProductInfo(strings.NewReader("<other><versionname>X</versionname></other>")); err == nil {
		t.Error("accepted a non-productinfo document")
	}
}

type detectorEnv struct {
	d    *Detector
	omv  fstest.MapFS
	dpkg fstest.MapFS
	now  time.Time
}

// rpcFile은 OMV 디렉터리 안의 RPC 파일 경로다.
func rpcFile(name string) string { return "engined/rpc/" + name }

func newDetectorEnv(t *testing.T) *detectorEnv {
	t.Helper()
	status, err := os.ReadFile("testdata/dpkg-status")
	if err != nil {
		t.Fatal(err)
	}
	product, err := os.ReadFile("testdata/productinfo.xml")
	if err != nil {
		t.Fatal(err)
	}
	e := &detectorEnv{
		// cputemp는 파일은 있지만 dpkg 상태가 설치가 아니고, bcache는 설치됐지만 파일이 없다.
		omv: fstest.MapFS{
			rpcFile("compose.inc"): {},
			rpcFile("cputemp.inc"): {},
			rpcFile("kvm.inc"):     {},
			"productinfo.xml":      {Data: product},
		},
		dpkg: fstest.MapFS{"status": {Data: status}},
		now:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	e.d = NewDetectorFS(e.omv, e.dpkg)
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
	delete(e.omv, rpcFile("kvm.inc"))
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
	delete(e.omv, rpcFile("compose.inc"))
	e.d.Invalidate()
	if got := e.d.Plugins(); len(got) != 0 {
		t.Errorf("after invalidate: %v", got)
	}

	// Refresh는 바로 다시 확인한다.
	e.omv[rpcFile("kvm.inc")] = &fstest.MapFile{}
	e.d.Refresh()
	e.omv[rpcFile("compose.inc")] = &fstest.MapFile{}
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
	if got := empty.OMV(); got != (OMVInfo{}) {
		t.Errorf("omv without files = %+v", got)
	}
}

func TestDetectorOMV(t *testing.T) {
	e := newDetectorEnv(t)
	want := OMVInfo{Version: "8.0.0-1", VersionName: "Synthetic"}
	if got := e.d.OMV(); got != want {
		t.Errorf("omv = %+v, want %+v", got, want)
	}

	// 실행 중에 업그레이드하면 TTL이 지난 뒤 새 버전을 읽는다.
	e.dpkg["status"] = &fstest.MapFile{Data: []byte("Package: openmediavault\nStatus: install ok installed\nVersion: 8.1.0-1\n")}
	e.now = e.now.Add(e.d.TTL)
	if got := e.d.OMV().Version; got != "8.1.0-1" {
		t.Errorf("version after upgrade = %q", got)
	}

	// OMV 패키지가 없으면 버전을 모른다.
	e.dpkg["status"] = &fstest.MapFile{Data: []byte("Package: other\nStatus: install ok installed\nVersion: 1\n")}
	e.d.Refresh()
	if got := e.d.OMV().Version; got != "" {
		t.Errorf("version without package = %q", got)
	}
}

func TestDetectorProductInfoErrors(t *testing.T) {
	logs := &bytes.Buffer{}
	prev := log.Writer()
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	e := newDetectorEnv(t)
	e.d.OMV()

	// 깨진 제품 정보는 코드명만 직전 값을 유지하고 남긴다. 플러그인·버전은 그대로 읽는다.
	e.omv["productinfo.xml"] = &fstest.MapFile{Data: []byte("<productinfo>")}
	delete(e.omv, rpcFile("kvm.inc"))
	e.d.Refresh()
	if got := e.d.OMV(); got != (OMVInfo{Version: "8.0.0-1", VersionName: "Synthetic"}) {
		t.Errorf("omv after bad product info = %+v", got)
	}
	if got := pluginNames(e.d.Plugins()); !slices.Equal(got, []string{"openmediavault-compose"}) {
		t.Errorf("plugins after bad product info = %v", got)
	}
	if logs.Len() == 0 {
		t.Error("product info error not logged")
	}

	// 파일이 없으면(OMV가 없는 개발 환경) 코드명 없이 조용히 넘어간다.
	logs.Reset()
	delete(e.omv, "productinfo.xml")
	e.d.Refresh()
	if got := e.d.OMV(); got != (OMVInfo{Version: "8.0.0-1"}) {
		t.Errorf("omv without product info = %+v", got)
	}
	if logs.Len() != 0 {
		t.Errorf("missing product info logged: %s", logs)
	}

	// dpkg 상태를 읽지 못하면 버전도 직전 값을 유지한다.
	delete(e.dpkg, "status")
	e.d.Refresh()
	if got := e.d.OMV().Version; got != "8.0.0-1" {
		t.Errorf("version after read error = %q", got)
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
