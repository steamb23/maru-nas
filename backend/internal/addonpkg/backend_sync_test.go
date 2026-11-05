package addonpkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/config"
	"maru/backend/internal/addonpkg/format"
)

// syncEnv는 임시 디렉터리의 동기화 파일과 시계, 레지스트리(active)다. 시계는 실제 시각에서 시작한다(요청 파일의 수정
// 시각과 비교한다).
type syncEnv struct {
	t      *testing.T
	paths  SyncPaths
	now    time.Time
	active []*Installed
	s      *Sync
}

func newSyncEnv(t *testing.T) *syncEnv {
	t.Helper()
	dir := t.TempDir()
	e := &syncEnv{t: t, now: time.Now(), paths: SyncPaths{
		Desired: filepath.Join(dir, config.AddonBackendsFile),
		Request: filepath.Join(dir, config.AddonSyncRequest),
		Status:  filepath.Join(dir, "status.json"),
	}}
	e.restart()
	return e
}

// restart는 maru를 다시 시작한 것처럼 Sync를 새로 만든다(디스크의 원하는 상태 파일에서 이어 간다).
func (e *syncEnv) restart() {
	e.s = NewSync(e.paths, func() time.Time { return e.now }, func() []*Installed { return e.active })
}

// update는 레지스트리를 list로 바꾸고 Update를 부른다(Manager.OnChange).
func (e *syncEnv) update(list ...*Installed) error {
	e.active = list
	return e.s.Update()
}

// state는 Sync가 보는 p의 백엔드 상태다.
func (e *syncEnv) state(p *Installed) (string, string) {
	return e.s.Backend(p.ID, p.Rev, p.InstallUID)
}

// run은 root 동기화가 요청을 가져가 지금 원하는 상태를 맞춘 것처럼 한다. failed는 켜지 못한 id다.
func (e *syncEnv) run(purged []string, failed ...string) {
	e.t.Helper()
	if err := os.Remove(e.paths.Request); err != nil && !errors.Is(err, fs.ErrNotExist) {
		e.t.Fatal(err)
	}
	st := config.AddonSyncStatus{UpdatedAt: e.now, Digest: e.digest(), Purged: purged}
	for _, b := range e.desired().Backends {
		bs := config.AddonBackendStatus{ID: b.ID, Rev: b.Rev, Unit: config.AddonUnitPrefix(b.Network) + "@" + b.ID + ".socket",
			State: config.AddonBackendEnabled}
		if slices.Contains(failed, b.ID) {
			bs.State, bs.Error = config.AddonBackendFailed, "enable failed"
		}
		st.Backends = append(st.Backends, bs)
	}
	e.writeStatus(st)
}

// backendPkg는 레지스트리에 들어간 백엔드 패키지 하나다.
func backendPkg(id, version, sha, instance string, network ...string) *Installed {
	return &Installed{
		ID: id, Rev: version + "-" + sha, InstallUID: instance,
		Manifest: &format.Manifest{ID: id, Version: version, Backend: &format.Backend{Network: network}},
	}
}

const (
	inst1 = "0123456789abcdef0123456789abcdef"
	inst2 = "fedcba9876543210fedcba9876543210"
)

func (e *syncEnv) desired() config.AddonBackends {
	e.t.Helper()
	data, err := os.ReadFile(e.paths.Desired)
	if err != nil {
		e.t.Fatal(err)
	}
	b, err := config.ParseAddonBackends(data)
	if err != nil {
		e.t.Fatalf("desired file does not parse: %v\n%s", err, data)
	}
	return b
}

func (e *syncEnv) digest() string {
	e.t.Helper()
	data, err := os.ReadFile(e.paths.Desired)
	if err != nil {
		e.t.Fatal(err)
	}
	return config.AddonBackendsDigest(data)
}

func (e *syncEnv) requested() bool {
	_, err := os.Lstat(e.paths.Request)
	return err == nil
}

// consume는 root 동기화가 요청을 가져간 것처럼 요청을 지운다.
func (e *syncEnv) consume() {
	e.t.Helper()
	if err := os.Remove(e.paths.Request); err != nil {
		e.t.Fatal(err)
	}
}

// writeStatus는 root 동기화의 결과를 쓴다. 같은 이름 바꾸기(새 inode)로 쓴다.
func (e *syncEnv) writeStatus(st config.AddonSyncStatus) {
	e.t.Helper()
	st.Format = config.AddonSyncStatusFormat
	data, _ := json.Marshal(st)
	tmp := e.paths.Status + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		e.t.Fatal(err)
	}
	if err := os.Rename(tmp, e.paths.Status); err != nil {
		e.t.Fatal(err)
	}
}

func TestSyncDesiredFile(t *testing.T) {
	e := newSyncEnv(t)
	dev := backendPkg("devpkg", "1.0.0", "aaaaaaaa", inst1)
	dev.Dev, dev.Rev, dev.InstallUID = true, "dev-0123abcd", "dev"
	noBackend := &Installed{ID: "plain", Rev: "1.0.0-bbbbbbbb", InstallUID: inst1, Manifest: &format.Manifest{ID: "plain", Version: "1.0.0"}}
	active := []*Installed{
		backendPkg("zeta", "2.0.0-beta.1", "cccccccc", inst2, "loopback", "lan"),
		dev, noBackend,
		backendPkg("alpha", "1.0.0", "dddddddd", inst1),
	}
	if err := e.update(active...); err != nil {
		t.Fatal(err)
	}
	got := e.desired()
	want := config.AddonBackends{Format: 1, Purge: []string{}, Backends: []config.AddonBackend{
		{ID: "alpha", Version: "1.0.0", Rev: "1.0.0-dddddddd", Instance: inst1, Network: []string{}},
		{ID: "zeta", Version: "2.0.0-beta.1", Rev: "2.0.0-beta.1-cccccccc", Instance: inst2, Network: []string{"lan", "loopback"}},
	}}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("desired = %s, want %s", gotJSON, wantJSON)
	}
	fi, err := os.Stat(e.paths.Desired)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("desired file mode = %v %v", fi.Mode(), err)
	}
	// 결과가 없으므로(설치기가 아직 빈 결과도 쓰지 않음) 요청한다.
	if !e.requested() {
		t.Error("no sync request")
	}
	if b, _ := os.ReadFile(e.paths.Request); string(b) != config.AddonSyncRequestBody {
		t.Errorf("request body = %q", b)
	}

	// 내용이 같으면 파일을 다시 쓰지 않는다.
	before, _ := os.Stat(e.paths.Desired)
	time.Sleep(10 * time.Millisecond)
	if err := e.update(slices.Clone(active)...); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(e.paths.Desired)
	if !before.ModTime().Equal(after.ModTime()) || !os.SameFile(before, after) {
		t.Error("unchanged desired state was rewritten")
	}
	// 남은 임시 파일이 없다.
	entries, _ := os.ReadDir(filepath.Dir(e.paths.Desired))
	for _, en := range entries {
		if strings.HasPrefix(en.Name(), ".tmp-") {
			t.Errorf("temporary file %s left", en.Name())
		}
	}
}

func TestSyncRequestOnlyWhenNeeded(t *testing.T) {
	e := newSyncEnv(t)
	// 설치기가 켤 때 써 둔 빈 결과(digest 없음): 시작할 때 첫 동기화를 요청한다.
	e.writeStatus(config.AddonSyncStatus{UpdatedAt: e.now})
	hello := backendPkg("hello", "1.0.0", "aaaaaaaa", inst1)
	if err := e.update(hello); err != nil {
		t.Fatal(err)
	}
	if !e.requested() {
		t.Fatal("first sync not requested")
	}
	// 요청이 남아 있으면 내용이 바뀌어도 새로 쓰지 않는다(동기화가 지금 파일을 읽는다).
	reqInfo, _ := os.Stat(e.paths.Request)
	other := backendPkg("other", "1.0.0", "bbbbbbbb", inst2)
	if err := e.update(hello, other); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(e.paths.Request); !os.SameFile(reqInfo, fi) {
		t.Error("pending request rewritten")
	}
	if len(e.desired().Backends) != 2 {
		t.Error("desired state not updated while a request is pending")
	}

	// 동기화가 지금 파일을 맞추면 요청하지 않는다.
	digest := e.digest()
	e.run(nil, "other")
	if err := e.update(hello, other); err != nil {
		t.Fatal(err)
	}
	if e.requested() {
		t.Error("request written although the status matches")
	}
	st := e.s.State()
	if !st.Applied() || !st.Synced() || st.Pending || st.Digest != digest || st.StatusErr != nil || st.WriteErr != nil {
		t.Errorf("state = %+v", st)
	}
	if s, _ := st.Backend("hello", hello.Rev, inst1); s != BackendRunning {
		t.Errorf("hello = %s", s)
	}
	if s, why := e.state(other); s != BackendFailed || why != "enable failed" {
		t.Errorf("other = %s %q", s, why)
	}

	// 업데이트(새 rev)는 결과가 반영할 때까지 pending이고 실패가 아니다.
	updated := backendPkg("other", "1.1.0", "cccccccc", inst2)
	if err := e.update(hello, updated); err != nil {
		t.Fatal(err)
	}
	if !e.requested() {
		t.Error("changed desired state not requested")
	}
	if st := e.s.State(); st.Applied() {
		t.Error("applied before the sync ran")
	}
	if s, _ := e.state(updated); s != BackendPending {
		t.Errorf("updated package = %s", s)
	}
	// 지난 결과라도 그 파일에서 바뀌지 않은 백엔드(같은 rev·설치)는 그대로 따른다.
	if s, _ := e.state(hello); s != BackendRunning {
		t.Errorf("hello after another change = %s", s)
	}
}

// 결과에는 설치 인스턴스가 없으므로 결과가 맞춘 파일로 확인한다: 같은 rev라도 다시 설치(새 인스턴스)한 패키지는 동기화가
// 새 파일을 맞출 때까지 pending이다(이전 설치의 유닛이 아직 그 소켓을 쥐고 있을 수 있다).
func TestSyncBackendInstance(t *testing.T) {
	e := newSyncEnv(t)
	first := backendPkg("hello", "1.0.0", "aaaaaaaa", inst1)
	if err := e.update(first); err != nil {
		t.Fatal(err)
	}
	e.run(nil)
	if s, _ := e.state(first); s != BackendRunning {
		t.Fatalf("first install = %s", s)
	}
	// 데이터를 남기고 지운 뒤 같은 파일(같은 rev)을 다시 설치했다. 결과는 아직 첫 설치의 파일을 맞춘 것이다.
	if err := e.update(); err != nil {
		t.Fatal(err)
	}
	again := backendPkg("hello", "1.0.0", "aaaaaaaa", inst2)
	if err := e.update(again); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.state(again); s != BackendPending {
		t.Errorf("reinstall before the sync = %s", s)
	}
	e.run(nil)
	if s, _ := e.state(again); s != BackendRunning {
		t.Errorf("reinstall after the sync = %s", s)
	}

	// 다시 시작하면 디스크의 원하는 상태 파일을 기억하므로 그 파일을 맞춘 결과를 따른다.
	e.restart()
	if s, _ := e.state(again); s != BackendRunning {
		t.Errorf("after a restart = %s", s)
	}
	// 기억하지 않는 파일(지난 실행의 다른 파일)을 맞춘 결과는 확인으로 보지 않는다.
	e.writeStatus(config.AddonSyncStatus{UpdatedAt: e.now, Digest: strings.Repeat("0", 64), Backends: []config.AddonBackendStatus{
		{ID: "hello", Rev: again.Rev, Unit: "maru-addon@hello.socket", State: config.AddonBackendEnabled},
	}})
	if s, _ := e.state(again); s != BackendPending {
		t.Errorf("status of an unknown file = %s", s)
	}
	// 동기화 전체가 실패한 결과는 반영이 아니고, 항목도 믿지 않는다.
	e.writeStatus(config.AddonSyncStatus{UpdatedAt: e.now, Digest: e.digest(), Error: "addon backends: locked", Backends: []config.AddonBackendStatus{
		{ID: "hello", Rev: again.Rev, Unit: "maru-addon@hello.socket", State: config.AddonBackendEnabled},
	}})
	if st := e.s.State(); st.Applied() {
		t.Error("a failed sync counted as applied")
	}
	if s, _ := e.state(again); s != BackendPending {
		t.Errorf("status with an error = %s", s)
	}
	// 결과가 없으면 pending이다(fail closed).
	os.Remove(e.paths.Status)
	if s, _ := e.state(again); s != BackendPending {
		t.Errorf("missing status = %s", s)
	}
}

// 동기화는 요청을 먼저 지우고 일하므로, 요청을 가져간 뒤 결과를 쓰기 전까지도 대기 중이다.
func TestSyncPendingUntilResult(t *testing.T) {
	e := newSyncEnv(t)
	e.writeStatus(config.AddonSyncStatus{UpdatedAt: e.now})
	if err := e.update(backendPkg("hello", "1.0.0", "aaaaaaaa", inst1)); err != nil {
		t.Fatal(err)
	}
	if st := e.s.State(); !st.Pending || st.Stalled {
		t.Fatalf("after the request = %+v", st)
	}
	e.consume()
	if st := e.s.State(); !st.Pending || st.Stalled || st.Applied() {
		t.Errorf("request taken, no result yet = %+v", st)
	}
	e.run(nil)
	if st := e.s.State(); st.Pending || !st.Applied() {
		t.Errorf("after the result = %+v", st)
	}

	// 강제 요청도 결과 파일이 바뀔 때까지 대기다(지금 파일을 이미 맞췄어도).
	if wrote, err := e.s.Request(); err != nil || !wrote {
		t.Fatalf("Request = %v %v", wrote, err)
	}
	e.consume()
	if st := e.s.State(); !st.Pending {
		t.Errorf("forced request taken, no result yet = %+v", st)
	}
	// 결과 없이 SyncStall이 지나면 대기도 멈춤도 아니다(관리자 페이지는 다시 동기화를 권한다).
	e.now = e.now.Add(SyncStall + time.Second)
	if st := e.s.State(); st.Pending || st.Stalled {
		t.Errorf("no result after a minute = %+v", st)
	}
}

func TestSyncForceRequest(t *testing.T) {
	e := newSyncEnv(t)
	hello := backendPkg("hello", "1.0.0", "aaaaaaaa", inst1)
	if err := e.update(hello); err != nil {
		t.Fatal(err)
	}
	e.run(nil)
	// 결과가 지금 파일을 맞췄어도 다시 동기화를 요청할 수 있다(실패한 백엔드를 다시 켜 본다).
	if wrote, err := e.s.Request(); err != nil || !wrote || !e.requested() {
		t.Fatalf("Request = %v %v", wrote, err)
	}
	// 아직 가져가지 않은 요청이 있으면 새로 쓰지 않는다.
	if wrote, err := e.s.Request(); err != nil || wrote {
		t.Errorf("Request with a pending request = %v %v", wrote, err)
	}
	// 1분 넘게 남은 요청은 멈춤이고, 강제 요청은 덮어쓴다. 자동 갱신은 덮어쓰지 않는다.
	if err := os.Chtimes(e.paths.Request, e.now.Add(-2*time.Minute), e.now.Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st := e.s.State(); !st.Pending || !st.Stalled {
		t.Errorf("stalled state = %+v", st)
	}
	other := backendPkg("other", "1.0.0", "bbbbbbbb", inst2)
	if err := e.update(hello, other); err != nil {
		t.Fatal(err)
	}
	if st := e.s.State(); !st.Stalled {
		t.Error("automatic update rewrote a stalled request")
	}
	// 요청은 잠금을 쥔 뒤의 레지스트리로 원하는 상태를 맞춘다(그 전에 읽은 목록으로 덮어쓰지 않는다).
	e.active = []*Installed{other}
	if wrote, err := e.s.Request(); err != nil || !wrote {
		t.Errorf("Request over a stalled request = %v %v", wrote, err)
	}
	if b := e.desired(); len(b.Backends) != 1 || b.Backends[0].ID != "other" {
		t.Errorf("Request did not use the current registry: %+v", b)
	}
	if st := e.s.State(); !st.Pending || st.Stalled {
		t.Errorf("state after rewriting = %+v", st)
	}
	// 시계로 1분이 지나면 다시 멈춤이다.
	e.now = e.now.Add(SyncStall + time.Second)
	if st := e.s.State(); !st.Stalled {
		t.Error("not stalled after a minute")
	}
}

func TestSyncPurge(t *testing.T) {
	e := newSyncEnv(t)
	hello := backendPkg("hello", "1.0.0", "aaaaaaaa", inst1)
	other := backendPkg("other", "1.0.0", "bbbbbbbb", inst2)
	if err := e.update(hello, other); err != nil {
		t.Fatal(err)
	}
	// 제거한 뒤 데이터를 지우게 한다(OnChange가 먼저 백엔드를 빼고, 그다음 purge).
	if err := e.update(other); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Purge("hello"); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); !slices.Equal(got.Purge, []string{"hello"}) || len(got.Backends) != 1 {
		t.Fatalf("desired after purge = %+v", got)
	}
	// 다시 시작해도 지난 파일에서 purge 목록을 이어 간다.
	e.restart()
	if err := e.update(other); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); !slices.Equal(got.Purge, []string{"hello"}) {
		t.Fatalf("purge list lost on restart: %+v", got)
	}

	// 다른 파일을 맞춘 결과의 purged는 믿지 않는다.
	e.writeStatus(config.AddonSyncStatus{UpdatedAt: e.now, Digest: strings.Repeat("0", 64), Purged: []string{"hello"}})
	if err := e.update(other); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); !slices.Equal(got.Purge, []string{"hello"}) {
		t.Errorf("purge dropped by a status of another file: %+v", got)
	}
	// 지금 파일을 맞춘 결과가 지운 것을 확인하면 뺀다.
	e.run([]string{"hello"})
	if err := e.update(other); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); len(got.Purge) != 0 {
		t.Errorf("confirmed purge kept: %+v", got)
	}
	// 확인된 id라도 다시 고르면 다시 넣는다.
	if err := e.s.Purge("hello"); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); !slices.Equal(got.Purge, []string{"hello"}) {
		t.Errorf("purge chosen again not kept: %+v", got)
	}
	// 다시 설치해 켜진 id는 purge에서 뺀다(새 인스턴스라 동기화가 켜기 전에 데이터를 지운다).
	again := backendPkg("hello", "1.0.0", "aaaaaaaa", inst2)
	if err := e.update(other, again); err != nil {
		t.Fatal(err)
	}
	got := e.desired()
	if len(got.Purge) != 0 || len(got.Backends) != 2 || got.Backends[0].Instance != inst2 {
		t.Errorf("desired after reinstall = %+v", got)
	}
	if err := e.s.Purge("hello"); err != nil || len(e.desired().Purge) != 0 {
		t.Errorf("purge of an active id kept: %v %+v", err, e.desired())
	}
	if err := e.s.Purge("../x"); err == nil {
		t.Error("invalid purge id accepted")
	}
}

// 지울 목록은 파일에 config.AddonBackendsMax개까지 들어간다. 그만큼 차면 PurgeFull이 새 purge를 막고, 그래도 들어온 것은
// 메모리에서 기다렸다가 앞의 것이 확인되면 파일에 들어간다.
func TestSyncPurgeLimit(t *testing.T) {
	e := newSyncEnv(t)
	if err := e.update(); err != nil {
		t.Fatal(err)
	}
	id := func(i int) string { return fmt.Sprintf("gone%02d", i) }
	for i := 0; i < config.AddonBackendsMax; i++ {
		if err := e.s.Purge(id(i)); err != nil {
			t.Fatal(err)
		}
	}
	if !e.s.PurgeFull("another") {
		t.Error("PurgeFull with a full list = false")
	}
	if e.s.PurgeFull(id(3)) {
		t.Error("PurgeFull for an id already in the list = true")
	}
	// 켜져 있는(아직 제거하기 전인) 패키지 자신은 목록에 없으므로 막힌다. 다시 켜진 id는 목록에서 빠져 자리가 난다.
	e.active = []*Installed{backendPkg(id(0), "1.0.0", "aaaaaaaa", inst1)}
	if e.s.PurgeFull("another") {
		t.Error("an active id still counted in the purge list")
	}
	e.active = nil
	// 경쟁으로 하나가 더 들어오면 파일에는 상한만큼만 쓰고 나머지는 기다린다.
	if err := e.s.Purge("zlast"); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); len(got.Purge) != config.AddonBackendsMax || slices.Contains(got.Purge, "zlast") {
		t.Errorf("file purge list = %d entries %v", len(got.Purge), got.Purge)
	}
	// 지금 파일의 purge가 확인되면 기다리던 것이 들어간다.
	e.run(e.desired().Purge)
	if err := e.update(); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); !slices.Equal(got.Purge, []string{"zlast"}) {
		t.Errorf("waiting purge = %v", got.Purge)
	}
	if e.s.PurgeFull("another") {
		t.Error("PurgeFull after confirmations = true")
	}
}

// 원하는 상태에 넣지 못한 백엔드(손으로 고친 설치 기록이 상한을 넘김)는 failed로 보인다.
func TestSyncLeftOut(t *testing.T) {
	e := newSyncEnv(t)
	var list []*Installed
	for i := 0; i <= config.AddonBackendsMax; i++ {
		list = append(list, backendPkg(fmt.Sprintf("pkg%02d", i), "1.0.0", "aaaaaaaa", inst1))
	}
	if err := e.update(list...); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); len(got.Backends) != config.AddonBackendsMax || len(got.Purge) != 0 {
		t.Errorf("desired = %d backends, purge %v", len(got.Backends), got.Purge)
	}
	last := list[config.AddonBackendsMax]
	if s, why := e.state(last); s != BackendFailed || why == "" {
		t.Errorf("left-out backend = %s %q", s, why)
	}
	if s, _ := e.state(list[0]); s != BackendPending {
		t.Errorf("kept backend = %s", s)
	}
	if err := e.update(list[:config.AddonBackendsMax]...); err != nil {
		t.Fatal(err)
	}
	if s, _ := e.state(last); s != BackendPending {
		t.Errorf("left-out state kept after the registry changed = %s", s)
	}
}

func TestSyncStatusErrors(t *testing.T) {
	e := newSyncEnv(t)
	if st := e.s.State(); st.Status != nil || !errors.Is(st.StatusErr, fs.ErrNotExist) {
		t.Errorf("missing status = %+v", st)
	}
	// 심볼릭 링크는 따라가지 않는다.
	target := filepath.Join(t.TempDir(), "real.json")
	os.WriteFile(target, []byte(`{"format":1}`), 0o644)
	if err := os.Symlink(target, e.paths.Status); err != nil {
		t.Fatal(err)
	}
	if st := e.s.State(); st.Status != nil || st.StatusErr == nil {
		t.Errorf("symlinked status = %+v", st)
	}
	os.Remove(e.paths.Status)
	// 형식이 틀리면 오류다. 바뀐 파일은 다시 읽는다.
	os.WriteFile(e.paths.Status, []byte(`{"format":9}`), 0o644)
	if st := e.s.State(); st.Status != nil || st.StatusErr == nil {
		t.Errorf("unknown format = %+v", st)
	}
	e.writeStatus(config.AddonSyncStatus{Error: "addon backends: invalid id"})
	st := e.s.State()
	if st.Status == nil || st.StatusErr != nil || st.Status.Error == "" || st.Synced() {
		t.Errorf("status with an error = %+v", st)
	}
	// 너무 큰 파일은 읽지 않는다.
	os.WriteFile(e.paths.Status, []byte(`{"format":1,"error":"`+strings.Repeat("x", config.AddonSyncStatusMaxBytes)+`"}`), 0o644)
	if st := e.s.State(); st.Status != nil || st.StatusErr == nil {
		t.Errorf("oversized status = %+v", st)
	}

	// 원하는 상태를 쓰지 못하면 State가 이유를 보인다(읽기 전용 디렉터리).
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Dir(e.paths.Desired)
	os.Chmod(dir, 0o500)
	defer os.Chmod(dir, 0o700)
	if err := e.update(backendPkg("hello", "1.0.0", "aaaaaaaa", inst1)); err == nil {
		t.Fatal("write into a read-only directory succeeded")
	}
	if st := e.s.State(); st.WriteErr == nil || st.Digest != "" {
		t.Errorf("write error not reported: %+v", st)
	}
}

func TestSyncStartsFromInvalidFile(t *testing.T) {
	e := newSyncEnv(t)
	os.WriteFile(e.paths.Desired, []byte(`{"format":1,"backends":[],"purge":["../x"]}`), 0o600)
	e.restart()
	if err := e.update(); err != nil {
		t.Fatal(err)
	}
	if got := e.desired(); len(got.Purge) != 0 || len(got.Backends) != 0 {
		t.Errorf("desired = %+v", got)
	}
}
