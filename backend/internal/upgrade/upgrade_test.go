package upgrade

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"onmaru/backend/config"
)

// fixture의 시계는 실제 시각에서 시작한다(확인 요청의 mtime과 비교한다).
type fixture struct {
	t     *testing.T
	root  string
	state string
	now   time.Time
	m     *Manager
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, root: t.TempDir(), state: t.TempDir(), now: time.Now().UTC().Truncate(time.Second)}
	f.m = New(NewPaths(f.root, f.state), func() time.Time { return f.now })
	return f
}

func (f *fixture) put(path, content string) {
	f.t.Helper()
	full := filepath.Join(f.root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) enable() {
	f.put(config.UpgradeEnvFile, "# comment\nONMARU_UPGRADE_REPO=/home/alice/old\nONMARU_UPGRADE_REPO=\"/home/alice/onmaru\"\nONMARU_UPGRADE_USER=alice\n")
}

func (f *fixture) status(st config.UpgradeStatus) {
	f.t.Helper()
	st.Format = config.UpgradeStatusFormat
	data, err := json.Marshal(st)
	if err != nil {
		f.t.Fatal(err)
	}
	f.put(config.UpgradeStatusFile, string(data))
}

func (f *fixture) stamp(d time.Duration) string { return f.now.Add(d).Format(time.RFC3339) }

func (f *fixture) request() string { return filepath.Join(f.state, config.UpgradeCheckRequestName) }

func TestReadDisabled(t *testing.T) {
	f := newFixture(t)
	s := f.m.Read()
	if s.Enabled || s.Status != nil || s.Pending || s.Busy() {
		t.Fatalf("empty root: %+v", s)
	}
	if err := f.m.RequestCheck(config.UpgradeModeRemote); !errors.Is(err, ErrDisabled) {
		t.Fatalf("request while disabled: %v", err)
	}
	if _, err := os.Stat(f.request()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("request written while disabled: %v", err)
	}
}

func TestReadEnvAndStatus(t *testing.T) {
	f := newFixture(t)
	f.enable()
	f.status(config.UpgradeStatus{State: config.UpgradeStateChecked, UpdatedAt: f.stamp(-time.Hour),
		Check: &config.UpgradeCheck{Mode: config.UpgradeModeRemote, Target: "0123456789abcdef0123456789abcdef01234567"}})
	s := f.m.Read()
	if !s.Enabled || s.Repo != "/home/alice/onmaru" || s.User != "alice" {
		t.Fatalf("env: %+v", s)
	}
	if s.Status == nil || s.Status.State != config.UpgradeStateChecked || s.Status.Check.Target == "" || s.Running() || s.Busy() {
		t.Fatalf("status: %+v", s.Status)
	}
	if s.Installed != nil {
		t.Fatalf("install record without a file: %+v", s.Installed)
	}
	f.put(config.InstallRecordFile, `{"format": 1, "version": "0.3.1", "revision": "0123456789abcdef0123456789abcdef01234567"}`)
	if s := f.m.Read(); s.Installed == nil || s.Installed.Version != "0.3.1" || s.Installed.Revision == "" {
		t.Fatalf("install record: %+v", s.Installed)
	}
	f.put(config.InstallRecordFile, `{"format": 9, "version": "9.9.9"}`)
	if s := f.m.Read(); s.Installed != nil {
		t.Fatalf("unknown install record format read: %+v", s.Installed)
	}
}

func TestReadIgnoresUnknownStatus(t *testing.T) {
	f := newFixture(t)
	f.enable()
	f.put(config.UpgradeStatusFile, `{"format": 99, "state": "checking"}`)
	if s := f.m.Read(); s.Status != nil {
		t.Fatalf("unknown format read: %+v", s.Status)
	}
	f.put(config.UpgradeStatusFile, "not json")
	if s := f.m.Read(); s.Status != nil {
		t.Fatalf("invalid json read: %+v", s.Status)
	}
}

func TestReadDoesNotFollowSymlinks(t *testing.T) {
	f := newFixture(t)
	f.enable()
	secret := filepath.Join(f.root, "secret.json")
	if err := os.WriteFile(secret, []byte(`{"format":1,"state":"checked"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(f.root, config.UpgradeStatusFile)
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	if s := f.m.Read(); s.Status != nil {
		t.Fatalf("followed symlink: %+v", s.Status)
	}
}

func TestInterrupted(t *testing.T) {
	f := newFixture(t)
	f.enable()
	f.status(config.UpgradeStatus{State: config.UpgradeStateApplying, Stage: config.UpgradeStageBuild, UpdatedAt: f.stamp(-time.Hour)})
	if s := f.m.Read(); !s.Running() || s.Interrupted || !s.Busy() {
		t.Fatalf("running an hour: %+v", s)
	}
	f.now = f.now.Add(config.UpgradeServiceTimeout)
	if s := f.m.Read(); !s.Interrupted || s.Busy() {
		t.Fatalf("past the service timeout: %+v", s)
	}
}

func TestRequestCheck(t *testing.T) {
	f := newFixture(t)
	f.enable()
	if err := f.m.RequestCheck("sideways"); err == nil {
		t.Fatal("unknown mode accepted")
	}
	if err := f.m.RequestCheck(config.UpgradeModeLocal); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(f.request())
	if err != nil {
		t.Fatal(err)
	}
	var req config.UpgradeCheckRequest
	if err := json.Unmarshal(data, &req); err != nil || req.Format != config.UpgradeRequestFormat || req.Mode != config.UpgradeModeLocal {
		t.Fatalf("request %s: %v", data, err)
	}
	if fi, err := os.Stat(f.request()); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("request mode: %v %v", fi.Mode(), err)
	}
	entries, _ := os.ReadDir(f.state)
	if len(entries) != 1 {
		t.Fatalf("temporary files left: %v", entries)
	}

	// 서비스가 가져가기 전에는 바쁘다.
	if s := f.m.Read(); !s.Pending || s.PendingStalled || !s.Busy() {
		t.Fatalf("pending: %+v", s)
	}
	if err := f.m.RequestCheck(config.UpgradeModeRemote); !errors.Is(err, ErrBusy) {
		t.Fatalf("second request while pending: %v", err)
	}

	// 서비스가 요청을 가져가 확인 중이어도 바쁘다.
	if err := os.Remove(f.request()); err != nil {
		t.Fatal(err)
	}
	f.status(config.UpgradeStatus{State: config.UpgradeStateChecking, Stage: config.UpgradeStageFetch, UpdatedAt: f.stamp(0)})
	if err := f.m.RequestCheck(config.UpgradeModeRemote); !errors.Is(err, ErrBusy) {
		t.Fatalf("request while checking: %v", err)
	}

	// 끝난 뒤에도 마지막 요청에서 checkInterval 안에는 받지 않는다.
	f.status(config.UpgradeStatus{State: config.UpgradeStateChecked, UpdatedAt: f.stamp(0)})
	f.now = f.now.Add(10 * time.Second)
	var rl *RateLimitedError
	if err := f.m.RequestCheck(config.UpgradeModeRemote); !errors.As(err, &rl) || rl.RetryAfter != 20*time.Second {
		t.Fatalf("rate limit: %v", err)
	}
	f.now = f.now.Add(20 * time.Second)
	if err := f.m.RequestCheck(config.UpgradeModeRemote); err != nil {
		t.Fatalf("after the interval: %v", err)
	}
}

func TestStalledRequestCanBeReplaced(t *testing.T) {
	f := newFixture(t)
	f.enable()
	if err := f.m.RequestCheck(config.UpgradeModeRemote); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(2 * time.Minute)
	if s := f.m.Read(); !s.Pending || !s.PendingStalled || s.Busy() {
		t.Fatalf("stalled: %+v", s)
	}
	if err := f.m.RequestCheck(config.UpgradeModeLocal); err != nil {
		t.Fatalf("replace stalled request: %v", err)
	}
}

func TestNoStateDir(t *testing.T) {
	m := New(NewPaths(t.TempDir(), ""), nil)
	if m.CanRequest() {
		t.Fatal("can request without a state directory")
	}
	if err := m.RequestCheck(config.UpgradeModeRemote); !errors.Is(err, ErrDisabled) {
		t.Fatalf("request without a state directory: %v", err)
	}
}
