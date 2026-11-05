package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/config"
	"maru/backend/internal/addonpkg"
	"maru/backend/internal/api"
)

// 운영 백엔드의 root 동기화(addon_sync.go) 시험. root 동기화는 시험이 흉내 낸다(요청을 지우고 결과를 쓴다).

// syncFixture는 동기화 파일을 둔 임시 디렉터리다. 결과 파일은 설치기처럼 미리 빈 결과로 써 둔다.
type syncFixture struct {
	t     *testing.T
	paths addonpkg.SyncPaths
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	state, root := t.TempDir(), t.TempDir()
	f := &syncFixture{t: t, paths: addonpkg.SyncPaths{
		Desired: filepath.Join(state, config.AddonBackendsFile),
		Request: filepath.Join(state, config.AddonSyncRequest),
		Status:  filepath.Join(root, "status.json"),
	}}
	f.status(config.AddonSyncStatus{})
	return f
}

func (f *syncFixture) desired() (config.AddonBackends, string) {
	f.t.Helper()
	data, err := os.ReadFile(f.paths.Desired)
	if err != nil {
		f.t.Fatal(err)
	}
	b, err := config.ParseAddonBackends(data)
	if err != nil {
		f.t.Fatalf("desired state does not parse: %v", err)
	}
	return b, config.AddonBackendsDigest(data)
}

func (f *syncFixture) requested() bool {
	_, err := os.Lstat(f.paths.Request)
	return err == nil
}

// status는 root 동기화의 결과를 쓴다(새 파일로 바꿔 넣는다).
func (f *syncFixture) status(st config.AddonSyncStatus) {
	f.t.Helper()
	st.Format = config.AddonSyncStatusFormat
	data, _ := json.Marshal(st)
	tmp := f.paths.Status + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Rename(tmp, f.paths.Status); err != nil {
		f.t.Fatal(err)
	}
}

// run은 root 동기화가 요청을 가져가 지금 원하는 상태를 맞춘 것처럼 한다. states는 id별 결과(enabled·failed)다.
func (f *syncFixture) run(at time.Time, states map[string]string, purged ...string) {
	f.t.Helper()
	if err := os.Remove(f.paths.Request); err != nil && !errors.Is(err, fs.ErrNotExist) {
		f.t.Fatal(err)
	}
	b, digest := f.desired()
	st := config.AddonSyncStatus{UpdatedAt: at, Digest: digest, Purged: purged}
	for _, e := range b.Backends {
		s := states[e.ID]
		if s == "" {
			s = config.AddonBackendEnabled
		}
		bs := config.AddonBackendStatus{ID: e.ID, Rev: e.Rev, Unit: config.AddonUnitPrefix(e.Network) + "@" + e.ID + ".socket", State: s}
		if s == config.AddonBackendFailed {
			bs.Error = "enable " + bs.Unit + ": exit status 1"
		}
		st.Backends = append(st.Backends, bs)
	}
	f.status(st)
}

func syncedPackage(t *testing.T, list api.AddonPackagesResponse, id string) api.AddonPackageStatus {
	t.Helper()
	for _, p := range list.Packages {
		if p.ID == id {
			return p
		}
	}
	t.Fatalf("package %s not listed", id)
	return api.AddonPackageStatus{}
}

func (b *browser) addonList() api.AddonPackagesResponse {
	b.e.t.Helper()
	rec := b.do(http.MethodGet, "/api/site/addons", nil)
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.AddonPackagesResponse](b.e.t, rec)
}

func TestAddonBackendSync(t *testing.T) {
	f := newSyncFixture(t)
	paths := backendPaths(shortSocketDir(t))
	e := newEnv(t, envOptions{packages: true, addonHost: &paths, addonSync: &f.paths})

	// 시작할 때(설치기가 빈 결과를 써 둔 뒤 maru가 다시 시작) 빈 원하는 상태를 쓰고 첫 동기화를 요청한다.
	if b, _ := f.desired(); len(b.Backends) != 0 || len(b.Purge) != 0 {
		t.Errorf("initial desired state = %+v", b)
	}
	if !f.requested() {
		t.Fatal("first sync not requested at startup")
	}
	f.run(e.clk.Now(), nil)

	admin := e.adminWithManage()
	list := admin.addonList()
	if bs := list.Backends; !bs.Enabled || !bs.Managed || bs.Pending || bs.Stalled || !bs.Applied || bs.SyncedAt == nil ||
		bs.Error != "" || bs.FileError != "" {
		t.Errorf("backends after the first sync = %+v", bs)
	}

	// 설치하면 원하는 상태에 백엔드가 들어가고 동기화를 요청한다. 반영 전에는 pending이고 capability는 막지 않는다.
	admin.installAddon(backendArchive(t))
	inst, ok := e.packages.Get("hello")
	if !ok {
		t.Fatal("hello not active")
	}
	b, _ := f.desired()
	want := []config.AddonBackend{{ID: "hello", Version: inst.Manifest.Version, Rev: inst.Rev, Instance: inst.InstallUID, Network: []string{}}}
	if !slices.EqualFunc(b.Backends, want, func(x, y config.AddonBackend) bool {
		return x.ID == y.ID && x.Version == y.Version && x.Rev == y.Rev && x.Instance == y.Instance && slices.Equal(x.Network, y.Network)
	}) || !config.ValidAddonInstance(inst.InstallUID) {
		t.Errorf("desired backends = %+v, want %+v", b.Backends, want)
	}
	if !f.requested() {
		t.Error("install did not request a sync")
	}
	list = admin.addonList()
	if bs := list.Backends; !bs.Pending || bs.Applied {
		t.Errorf("backends after install = %+v", bs)
	}
	if p := syncedPackage(t, list, "hello"); p.Backend == nil || p.Backend.State != addonpkg.BackendPending {
		t.Errorf("hello backend = %+v", p.Backend)
	}
	if c := packageView(t, admin.capabilities(), "hello"); c.ID == "" {
		t.Error("pending package missing from capabilities")
	}
	if c := capabilityOf(t, admin.capabilities(), "hello"); !c.Available {
		t.Errorf("pending package capability = %+v", c)
	}
	// 반영을 확인하기 전에는 백엔드로 보내지 않는다(이전 설치·rev의 유닛이 아직 그 소켓을 쥐고 있을 수 있다).
	if got := wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable); got.Error != errBackendNotReady {
		t.Errorf("pending data request error = %q", got.Error)
	}
	if got := wantCode(t, admin.do(http.MethodPost, "/api/addons/hello/actions/run", api.ActionRequest{}), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable); got.Error != errBackendNotReady {
		t.Errorf("pending action error = %q", got.Error)
	}
	if !strings.Contains(e.logs.String(), "action=run target=\"\" result=addon_backend_unavailable") {
		t.Errorf("pending action not in the audit log: %s", e.logs.String())
	}
	// 동기화가 요청을 가져간 뒤 결과를 쓰기 전에도 대기 중이다(관리자 페이지가 계속 다시 받는다).
	if err := os.Remove(f.paths.Request); err != nil {
		t.Fatal(err)
	}
	if bs := admin.addonList().Backends; !bs.Pending || bs.Stalled || bs.Applied {
		t.Errorf("request taken, no result yet = %+v", bs)
	}

	// 동기화가 켜지 못하면 failed와 이유, capability backend_unavailable이다.
	f.run(e.clk.Now(), map[string]string{"hello": config.AddonBackendFailed})
	list = admin.addonList()
	p := syncedPackage(t, list, "hello")
	if p.Backend == nil || p.Backend.State != addonpkg.BackendFailed || p.Backend.Error == "" || !list.Backends.Applied {
		t.Errorf("failed backend = %+v, sync %+v", p.Backend, list.Backends)
	}
	if c := capabilityOf(t, admin.capabilities(), "hello"); c.Available || c.Reason != api.ReasonBackendUnavailable {
		t.Errorf("failed package capability = %+v", c)
	}
	wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusForbidden, api.ErrBackendUnavailable)

	// "다시 동기화"는 결과가 같아도 요청하고, 아직 가져가지 않은 요청은 다시 쓰지 않는다.
	rec := admin.do(http.MethodPost, "/api/site/addons/sync", struct{}{})
	wantStatus(t, rec, http.StatusOK)
	if bs := decode[api.AddonBackendSync](t, rec); !bs.Pending || !bs.Managed || bs.Stalled {
		t.Errorf("sync response = %+v", bs)
	}
	before, err := os.Stat(f.paths.Request)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, admin.do(http.MethodPost, "/api/site/addons/sync", struct{}{}), http.StatusOK)
	if after, _ := os.Stat(f.paths.Request); !os.SameFile(before, after) {
		t.Error("pending request rewritten")
	}
	// 1분 넘게 남은 요청은 멈춤이고, 다시 동기화하면 덮어쓴다.
	old := e.clk.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(f.paths.Request, old, old); err != nil {
		t.Fatal(err)
	}
	if bs := admin.addonList().Backends; !bs.Stalled || !bs.Pending {
		t.Errorf("stalled request = %+v", bs)
	}
	rec = admin.do(http.MethodPost, "/api/site/addons/sync", struct{}{})
	wantStatus(t, rec, http.StatusOK)
	if bs := decode[api.AddonBackendSync](t, rec); bs.Stalled || !bs.Pending {
		t.Errorf("after rewriting a stalled request = %+v", bs)
	}
	// 본문은 다른 사이트 경로와 같이 JSON이어야 하고, 관리자만 부른다.
	wantStatus(t, admin.raw(http.MethodPost, "/api/site/addons/sync", "text/plain", []byte("{}")), http.StatusUnsupportedMediaType)
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, user.do(http.MethodPost, "/api/site/addons/sync", struct{}{}), http.StatusForbidden, api.ErrAdminRequired)

	// 켜지면 running이고 쓸 수 있다(시험에는 백엔드가 없어 연결이 실패한다).
	f.run(e.clk.Now(), nil)
	list = admin.addonList()
	if p := syncedPackage(t, list, "hello"); p.Backend == nil || p.Backend.State != addonpkg.BackendRunning || p.Backend.Error != "" {
		t.Errorf("running backend = %+v", p.Backend)
	}
	if c := capabilityOf(t, admin.capabilities(), "hello"); !c.Available {
		t.Errorf("running package capability = %+v", c)
	}
	if got := wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable); got.Error == errBackendNotReady {
		t.Error("running backend not dialed")
	}

	// 끄면 원하는 상태에서 빠진다(데이터는 남는다).
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: false}), http.StatusOK)
	if b, _ := f.desired(); len(b.Backends) != 0 || len(b.Purge) != 0 {
		t.Errorf("desired after disabling = %+v", b)
	}
	if p := syncedPackage(t, admin.addonList(), "hello"); p.Backend != nil {
		t.Errorf("disabled package backend = %+v", p.Backend)
	}
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: true}), http.StatusOK)
	f.run(e.clk.Now(), nil)

	// 데이터 지우기를 골라 제거하면 백엔드가 빠지고 purge에 들어간다. 동기화가 지운 것을 확인하면 다음 변경 때 뺀다.
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello?purge=1", nil), http.StatusNoContent)
	b, _ = f.desired()
	if len(b.Backends) != 0 || !slices.Equal(b.Purge, []string{"hello"}) {
		t.Errorf("desired after purge = %+v", b)
	}
	if !f.requested() {
		t.Error("purge did not request a sync")
	}
	f.run(e.clk.Now(), nil, "hello")
	admin.installAddon(addonArchive(t, nil))
	if b, _ := f.desired(); len(b.Purge) != 0 {
		t.Errorf("confirmed purge kept: %+v", b)
	}

	// 데이터 지우기를 고르지 않은 제거는 purge에 넣지 않는다.
	admin.installAddon(backendArchive(t))
	f.run(e.clk.Now(), nil)
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	if b, _ := f.desired(); len(b.Backends) != 0 || len(b.Purge) != 0 {
		t.Errorf("desired after uninstall without purge = %+v", b)
	}
}

// capabilityOf는 capabilities의 id 항목이다.
func capabilityOf(t *testing.T, caps api.CapabilitiesResponse, id string) api.AddonCapability {
	t.Helper()
	for _, c := range caps.Addons {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("capability %s missing", id)
	return api.AddonCapability{}
}

func TestAddonBackendSyncReinstallNewInstance(t *testing.T) {
	f := newSyncFixture(t)
	paths := backendPaths(shortSocketDir(t))
	e := newEnv(t, envOptions{packages: true, addonHost: &paths, addonSync: &f.paths})
	admin := e.adminWithManage()
	admin.installAddon(backendArchive(t))
	first, _ := f.desired()
	// 업데이트(같은 패키지 다시 올리기)는 인스턴스를 유지한다.
	admin.installAddon(backendArchive(t))
	if again, _ := f.desired(); again.Backends[0].Instance != first.Backends[0].Instance {
		t.Error("update changed the instance")
	}
	f.run(e.clk.Now(), nil)
	if p := syncedPackage(t, admin.addonList(), "hello"); p.Backend == nil || p.Backend.State != addonpkg.BackendRunning {
		t.Fatalf("installed backend = %+v", p.Backend)
	}
	// 지웠다가 다시 설치하면 인스턴스가 바뀐다(동기화가 켜기 전에 이전 데이터를 지운다). 같은 rev라도 동기화가 새 파일을
	// 맞출 때까지 pending이고 백엔드로 보내지 않는다(그 소켓은 아직 이전 설치의 유닛일 수 있다).
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	admin.installAddon(backendArchive(t))
	if again, _ := f.desired(); again.Backends[0].Instance == first.Backends[0].Instance || again.Backends[0].Rev != first.Backends[0].Rev {
		t.Error("reinstall kept the instance")
	}
	if p := syncedPackage(t, admin.addonList(), "hello"); p.Backend == nil || p.Backend.State != addonpkg.BackendPending {
		t.Errorf("reinstalled backend before the sync = %+v", p.Backend)
	}
	if got := wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable); got.Error != errBackendNotReady {
		t.Errorf("reinstalled backend dialed before the sync: %q", got.Error)
	}
	f.run(e.clk.Now(), nil)
	if p := syncedPackage(t, admin.addonList(), "hello"); p.Backend == nil || p.Backend.State != addonpkg.BackendRunning {
		t.Errorf("reinstalled backend after the sync = %+v", p.Backend)
	}
	// 백엔드가 없는 패키지는 purge=1로 지워도 purge에 넣지 않는다.
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	admin.installAddon(addonArchive(t, nil))
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello?purge=1", nil), http.StatusNoContent)
	if b, _ := f.desired(); len(b.Purge) != 0 || len(b.Backends) != 0 {
		t.Errorf("desired after purging a package without a backend = %+v", b)
	}
	// 감사 로그는 실제로 지울 목록에 넣었는지를 남긴다.
	if !strings.Contains(e.logs.String(), "action=uninstall id=hello purge=false result=ok") {
		t.Errorf("purge of a package without a backend logged as queued: %s", e.logs.String())
	}
}

// 지울 데이터 목록이 원하는 상태 파일에 다 들어가지 않으면 데이터 지우기를 고른 제거를 하기 전에 거절한다(패키지는
// 그대로다). 데이터를 남기는 제거는 그대로 된다.
func TestAddonBackendSyncPurgeFull(t *testing.T) {
	f := newSyncFixture(t)
	paths := backendPaths(shortSocketDir(t))
	e := newEnv(t, envOptions{packages: true, addonHost: &paths, addonSync: &f.paths})
	admin := e.adminWithManage()
	admin.installAddon(backendArchive(t))
	for i := 0; i < config.AddonBackendsMax; i++ {
		if err := e.srv.h.sync.Purge(fmt.Sprintf("gone%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	wantCode(t, admin.do(http.MethodDelete, "/api/site/addons/hello?purge=1", nil), http.StatusConflict, api.ErrAddonPurgeFull)
	if _, ok := e.packages.Get("hello"); !ok {
		t.Fatal("package removed although the purge was refused")
	}
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	if b, _ := f.desired(); slices.Contains(b.Purge, "hello") || len(b.Purge) != config.AddonBackendsMax {
		t.Errorf("desired purge = %v", b.Purge)
	}
}

func TestAddonBackendSyncStatusProblems(t *testing.T) {
	f := newSyncFixture(t)
	paths := backendPaths(shortSocketDir(t))
	e := newEnv(t, envOptions{packages: true, addonHost: &paths, addonSync: &f.paths})
	admin := e.adminWithManage()
	// 동기화 전체가 실패하면 그 이유를 보인다.
	// 동기화 전체가 실패하면 그 이유와 그 시각을 보인다.
	f.status(config.AddonSyncStatus{UpdatedAt: e.clk.Now(), Error: "addon backends: unsupported format 2"})
	if bs := admin.addonList().Backends; bs.Error == "" || bs.FileError != "" || bs.SyncedAt == nil || !bs.SyncedAt.Equal(e.clk.Now()) || bs.Applied {
		t.Errorf("sync error = %+v", bs)
	}
	// 결과 파일을 읽지 못하면 그 이유를 보이고, 페이지와 capability는 그대로 돈다. 반영을 확인할 수 없으므로 백엔드로는
	// 보내지 않는다.
	os.Remove(f.paths.Status)
	if bs := admin.addonList().Backends; bs.FileError == "" || bs.Applied {
		t.Errorf("missing status = %+v", bs)
	}
	admin.installAddon(backendArchive(t))
	if c := capabilityOf(t, admin.capabilities(), "hello"); !c.Available {
		t.Errorf("capability with an unreadable status = %+v", c)
	}
	if got := wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable); got.Error != errBackendNotReady {
		t.Errorf("data request with an unreadable status = %q", got.Error)
	}
}

// 개발 백엔드(동기화 없음)와 백엔드가 꺼진 설치는 운영 파일을 쓰지 않고, 다시 동기화는 409다.
func TestAddonBackendSyncNotManaged(t *testing.T) {
	paths := backendPaths(shortSocketDir(t))
	dev := newEnv(t, envOptions{packages: true, addonHost: &paths})
	admin := dev.adminWithManage()
	admin.installAddon(backendArchive(t))
	list := admin.addonList()
	if bs := list.Backends; !bs.Enabled || bs.Managed || bs.Pending || bs.Applied {
		t.Errorf("dev backends = %+v", bs)
	}
	if p := syncedPackage(t, list, "hello"); p.Backend != nil {
		t.Errorf("dev package backend = %+v", p.Backend)
	}
	wantCode(t, admin.do(http.MethodPost, "/api/site/addons/sync", struct{}{}), http.StatusConflict, api.ErrAddonBackendDisabled)
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello?purge=1", nil), http.StatusNoContent)
	for _, name := range []string{config.AddonBackendsFile, config.AddonSyncRequest} {
		if _, err := os.Lstat(filepath.Join(dev.stateDir, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s written without sync paths: %v", name, err)
		}
	}

	off := packagesEnv(t)
	offAdmin := off.adminWithManage()
	if bs := offAdmin.addonList().Backends; bs.Enabled || bs.Managed {
		t.Errorf("disabled backends = %+v", bs)
	}
	wantCode(t, offAdmin.do(http.MethodPost, "/api/site/addons/sync", struct{}{}), http.StatusConflict, api.ErrAddonBackendDisabled)
}
