package upgradefake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"maru/backend/config"
	"maru/backend/internal/upgrade"
)

type fixture struct {
	t     *testing.T
	o     Options
	now   time.Time
	m     *upgrade.Manager
	state string
}

func start(t *testing.T, scenario string) *fixture {
	t.Helper()
	f := &fixture{t: t, state: t.TempDir(), now: time.Now()}
	f.o = Options{Root: t.TempDir(), CheckRequest: filepath.Join(f.state, config.UpgradeCheckRequestName),
		ApplyDir: t.TempDir(), Scenario: scenario, Poll: 5 * time.Millisecond}
	s, err := Start(f.o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	f.m = upgrade.New(upgrade.NewPaths(f.o.Root, f.state), func() time.Time { return f.now })
	return f
}

// wait는 상태 파일이 state가 될 때까지 기다린다.
func (f *fixture) wait(state string) *config.UpgradeStatus {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := f.m.Read(); s.Status != nil && s.Status.State == state && !s.Pending {
			return s.Status
		}
		time.Sleep(5 * time.Millisecond)
	}
	f.t.Fatalf("status did not become %s: %+v", state, f.m.Read().Status)
	return nil
}

func (f *fixture) check(mode string) {
	f.t.Helper()
	f.now = f.now.Add(time.Minute) // 최근 확인 결과를 다시 쓰지 않게 한다.
	if ok, err := f.m.RequestCheck(mode); !ok || err != nil {
		f.t.Fatal(ok, err)
	}
}

func (f *fixture) apply(target string, settings map[string]string) {
	f.t.Helper()
	data, err := json.Marshal(config.UpgradeApplyRequest{Format: config.UpgradeRequestFormat, Target: target,
		Settings: settings, User: "totp", RequestedAt: time.Now().UTC().Format(time.RFC3339)})
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.o.ApplyDir, config.UpgradeApplyRequestName), data, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func TestWebScenario(t *testing.T) {
	f := start(t, "")
	if s := f.m.Read(); !s.Enabled || s.Repo == "" || s.User == "" || s.Status != nil {
		t.Fatalf("start = %+v", s)
	}
	f.check(config.UpgradeModeRemote)
	st := f.wait(config.UpgradeStateChecked)
	chk := st.Check
	if chk == nil || !chk.Web || chk.Upstream == "" || chk.Target != targetRevision || len(chk.Commits) != 5 ||
		chk.Plan == nil || len(chk.Plan.Questions) != 3 || len(chk.Plan.Notes) != 1 || chk.Plan.Version != nextVersion() {
		t.Fatalf("check = %+v", chk)
	}

	// 답이 틀리면 계획 단계에서 실패한다.
	f.apply(targetRevision, map[string]string{"public-origin": "-", "smart-refresh-at": "25:00", "example-toggle": "yes"})
	if st := f.wait(config.UpgradeStateApplyFailed); st.Stage != config.UpgradeStagePlan || st.Error == nil {
		t.Fatalf("bad settings = %+v", st)
	}

	f.check(config.UpgradeModeLocal)
	f.wait(config.UpgradeStateChecked)
	f.apply(targetRevision, map[string]string{"public-origin": "-", "smart-refresh-at": "03:00", "example-toggle": "no"})
	st = f.wait(config.UpgradeStateApplied)
	if st.Apply == nil || st.Apply.Version != nextVersion() || st.Apply.User != "totp" || st.Apply.FinishedAt == "" || st.Check == nil {
		t.Fatalf("applied = %+v", st)
	}

	// 올린 뒤에는 최신이다.
	f.check(config.UpgradeModeRemote)
	if st := f.wait(config.UpgradeStateChecked); !st.Check.UpToDate || st.Check.Web {
		t.Fatalf("after upgrade = %+v", st.Check)
	}
}

func TestManualAndLatest(t *testing.T) {
	f := start(t, ScenarioManual)
	f.check(config.UpgradeModeRemote)
	chk := f.wait(config.UpgradeStateChecked).Check
	if chk.Web || len(chk.Manual) != 2 || chk.Command != config.UpgradeCommandReinstall {
		t.Fatalf("manual = %+v", chk)
	}
	// 확인이 웹 업그레이드를 허락하지 않았으면 적용 요청은 실패한다.
	f.apply(targetRevision, nil)
	if st := f.wait(config.UpgradeStateApplyFailed); st.Stage != config.UpgradeStageInspect {
		t.Fatalf("apply after manual = %+v", st)
	}

	f = start(t, ScenarioLatest)
	f.check(config.UpgradeModeLocal)
	if chk := f.wait(config.UpgradeStateChecked).Check; !chk.UpToDate || chk.Target != installedRevision {
		t.Fatalf("latest = %+v", chk)
	}
}

func TestFailures(t *testing.T) {
	f := start(t, ScenarioCheckFail)
	f.check(config.UpgradeModeRemote)
	if st := f.wait(config.UpgradeStateCheckFailed); st.Stage != config.UpgradeStageFetch || st.Error == nil || st.Error.Log == "" {
		t.Fatalf("check-fail = %+v", st)
	}

	f = start(t, ScenarioApplyFail)
	f.check(config.UpgradeModeRemote)
	f.wait(config.UpgradeStateChecked)
	f.apply(targetRevision, map[string]string{"public-origin": "https://desk.example.com", "smart-refresh-at": "00:00", "example-toggle": "yes"})
	if st := f.wait(config.UpgradeStateApplyFailed); st.Stage != config.UpgradeStageBuild || st.Error.Log == "" {
		t.Fatalf("apply-fail = %+v", st)
	}
}

func TestDisabled(t *testing.T) {
	f := start(t, ScenarioDisabled)
	if s := f.m.Read(); s.Enabled {
		t.Fatalf("disabled = %+v", s)
	}
	if _, err := Start(Options{Root: t.TempDir(), CheckRequest: "x", ApplyDir: "y", Scenario: "nope"}); err == nil {
		t.Fatal("unknown scenario accepted")
	}
}
