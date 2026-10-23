package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"maru/backend/config"
	"maru/backend/internal/api"
	"maru/backend/internal/prefs"
)

const upgradeTarget = "0123456789abcdef0123456789abcdef01234567"

// enableUpgrade는 설치기가 대시보드에서 업그레이드를 켠 것처럼 가짜 루트에 root 쪽 설정 파일을 둔다.
func (e *env) enableUpgrade() {
	e.t.Helper()
	path := filepath.Join(e.upgradeRoot, config.UpgradeEnvFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	data := "MARU_UPGRADE_REPO=/home/alice/maru-nas\nMARU_UPGRADE_USER=alice\nMARU_UPGRADE_PATH=/usr/bin:/bin\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// upgradeStatus는 업그레이드 서비스가 쓴 것처럼 가짜 루트에 상태 파일을 둔다.
func (e *env) upgradeStatus(st config.UpgradeStatus) {
	e.t.Helper()
	st.Format = config.UpgradeStatusFormat
	if st.UpdatedAt == "" {
		st.UpdatedAt = e.clk.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(st)
	if err != nil {
		e.t.Fatal(err)
	}
	path := filepath.Join(e.upgradeRoot, config.UpgradeStatusFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// checkedStatus는 대상으로 업그레이드할 수 있다고 끝난 확인이다(새 설정 질문 하나).
func checkedStatus(target string) config.UpgradeStatus {
	return config.UpgradeStatus{State: config.UpgradeStateChecked, Check: &config.UpgradeCheck{
		Mode: config.UpgradeModeRemote, Target: target, Web: true, Commits: []config.UpgradeCommit{}, Manual: []config.UpgradeReason{},
		Plan: &config.UpgradePlan{Format: config.UpgradePlanFormat, Version: "0.4.0", Web: true,
			Questions: []config.UpgradeQuestion{{Name: "public-origin", Kind: config.UpgradeQuestionInput, Dash: true, EmptyOK: true}}},
	}}
}

func TestUpgradeAdminOnly(t *testing.T) {
	e := newEnv(t, envOptions{upgrade: true})
	e.enableUpgrade()
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantCode(t, b.do(http.MethodGet, "/api/upgrade", nil), http.StatusForbidden, api.ErrAdminRequired)
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeRemote}),
		http.StatusForbidden, api.ErrAdminRequired)
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: upgradeTarget}),
		http.StatusForbidden, api.ErrAdminRequired)
	wantCode(t, b.do(http.MethodPut, "/api/upgrade/channel", api.SetUpgradeChannelRequest{Channel: config.UpgradeChannelMain}),
		http.StatusForbidden, api.ErrAdminRequired)
}

func TestUpgradeDisabled(t *testing.T) {
	// Upgrade를 주지 않으면 실제 /etc/default/maru-upgrade를 보지 않고 꺼져 있다.
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	info := decode[api.UpgradeInfo](t, b.do(http.MethodGet, "/api/upgrade", nil))
	if info.Enabled || info.DisabledReason != api.UpgradeNotEnabled || info.Status != nil || info.Version != config.Version {
		t.Fatalf("default = %+v", info)
	}
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeRemote}),
		http.StatusConflict, api.ErrUpgradeDisabled)

	// 설정 파일이 있어도 도우미가 없으면 업그레이드를 요청할 수 없다.
	e = newEnv(t, envOptions{upgrade: true})
	e.enableUpgrade()
	b = e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	info = decode[api.UpgradeInfo](t, b.do(http.MethodGet, "/api/upgrade", nil))
	if info.Enabled || info.DisabledReason != api.UpgradeNoHelper || info.Repo != "/home/alice/maru-nas" || info.User != "alice" {
		t.Fatalf("no helper = %+v", info)
	}
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeLocal}),
		http.StatusConflict, api.ErrUpgradeDisabled)
	if _, err := os.Stat(filepath.Join(e.upgradeState, config.UpgradeCheckRequestName)); !os.IsNotExist(err) {
		t.Fatalf("check request written while disabled: %v", err)
	}
}

func TestUpgradeCheck(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, upgrade: true})
	e.enableUpgrade()
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	info := decode[api.UpgradeInfo](t, b.do(http.MethodGet, "/api/upgrade", nil))
	if !info.Enabled || info.DisabledReason != "" || info.Pending {
		t.Fatalf("enabled = %+v", info)
	}
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: "sideways"}),
		http.StatusBadRequest, api.ErrBadRequest)

	// 확인에는 권한 상승이 필요 없다(OMV 내장 admin도 확인할 수 있다).
	rec := b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeRemote})
	wantStatus(t, rec, http.StatusOK)
	if info := decode[api.UpgradeInfo](t, rec); !info.Pending {
		t.Fatalf("after request = %+v", info)
	}
	data, err := os.ReadFile(filepath.Join(e.upgradeState, config.UpgradeCheckRequestName))
	if err != nil {
		t.Fatal(err)
	}
	var req config.UpgradeCheckRequest
	if json.Unmarshal(data, &req) != nil || req.Mode != config.UpgradeModeRemote || req.Format != config.UpgradeRequestFormat {
		t.Fatalf("check request %s", data)
	}
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeLocal}),
		http.StatusConflict, api.ErrUpgradeBusy)
	if !strings.Contains(e.logs.String(), `upgrade check requested: user="admin" mode=remote`) {
		t.Fatalf("check request not logged:\n%s", e.logs.String())
	}

	// 서비스가 가져가 성공한 뒤 같은 방식으로 바로 다시 요청하면 요청을 쓰지 않고 그 결과를 돌려준다.
	request := filepath.Join(e.upgradeState, config.UpgradeCheckRequestName)
	if err := os.Remove(request); err != nil {
		t.Fatal(err)
	}
	now := e.clk.Now().UTC().Format(time.RFC3339)
	e.upgradeStatus(config.UpgradeStatus{State: config.UpgradeStateChecked,
		Check: &config.UpgradeCheck{Mode: config.UpgradeModeRemote, CheckedAt: now, UpToDate: true}})
	logs := e.logs.Len()
	rec = b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeRemote})
	wantStatus(t, rec, http.StatusOK)
	if info := decode[api.UpgradeInfo](t, rec); info.Pending || info.Status == nil || info.Status.State != config.UpgradeStateChecked {
		t.Fatalf("reused = %+v", info)
	}
	if _, err := os.Stat(request); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("request written for a recent check: %v", err)
	}
	if strings.Contains(e.logs.String()[logs:], "upgrade check requested") {
		t.Fatalf("reused check logged as a request:\n%s", e.logs.String())
	}

	// 다른 방식은 바로 다시 확인한다.
	rec = b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeLocal})
	wantStatus(t, rec, http.StatusOK)
	if info := decode[api.UpgradeInfo](t, rec); !info.Pending {
		t.Fatalf("other mode = %+v", info)
	}
}

// 업데이트 채널은 site.json에 두고, 원격 확인 요청이 그 채널을 담는다. 확인한 뒤 채널을 바꾸면 그 결과로 업그레이드하지 않는다.
func TestUpgradeChannel(t *testing.T) {
	site := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{helper: true, upgrade: true, site: site})
	e.enableUpgrade()
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	if info := decode[api.UpgradeInfo](t, b.do(http.MethodGet, "/api/upgrade", nil)); info.Channel != config.UpgradeChannelCurrent {
		t.Fatalf("default channel %q", info.Channel)
	}
	wantCode(t, b.do(http.MethodPut, "/api/upgrade/channel", api.SetUpgradeChannelRequest{Channel: "origin/main"}),
		http.StatusBadRequest, api.ErrBadRequest)

	rec := b.do(http.MethodPut, "/api/upgrade/channel", api.SetUpgradeChannelRequest{Channel: config.UpgradeChannelMain})
	wantStatus(t, rec, http.StatusOK)
	if info := decode[api.UpgradeInfo](t, rec); info.Channel != config.UpgradeChannelMain {
		t.Fatalf("after set %q", info.Channel)
	}
	if v, err := site.GetSite(); err != nil || v.Upgrade == nil || v.Upgrade.Channel != config.UpgradeChannelMain {
		t.Fatalf("site %+v %v", v.Upgrade, err)
	}
	if !strings.Contains(e.logs.String(), `upgrade channel set: user="admin" channel="main"`) {
		t.Fatalf("channel change not logged:\n%s", e.logs.String())
	}

	// 원격 확인은 고른 채널을 담는다.
	wantStatus(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeRemote}), http.StatusOK)
	request := filepath.Join(e.upgradeState, config.UpgradeCheckRequestName)
	data, err := os.ReadFile(request)
	if err != nil {
		t.Fatal(err)
	}
	var req config.UpgradeCheckRequest
	if json.Unmarshal(data, &req) != nil || req.Mode != config.UpgradeModeRemote || req.Channel != config.UpgradeChannelMain {
		t.Fatalf("check request %s", data)
	}
	// 확인을 기다리는 동안에는 바꾸지 않는다.
	wantCode(t, b.do(http.MethodPut, "/api/upgrade/channel", api.SetUpgradeChannelRequest{Channel: config.UpgradeChannelStable}),
		http.StatusConflict, api.ErrUpgradeBusy)
	if err := os.Remove(request); err != nil {
		t.Fatal(err)
	}

	// main으로 끝난 확인이 있는데 채널을 stable로 바꾸면 그 결과로 업그레이드하지 않는다.
	st := checkedStatus(upgradeTarget)
	st.Check.Channel = config.UpgradeChannelMain
	e.upgradeStatus(st)
	wantStatus(t, b.do(http.MethodPut, "/api/upgrade/channel", api.SetUpgradeChannelRequest{Channel: config.UpgradeChannelStable}), http.StatusOK)
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: upgradeTarget,
		Settings: map[string]string{"public-origin": "-"}}), http.StatusConflict, api.ErrUpgradeOutdated)

	// 빈 값은 채널을 고르지 않은 것(지금 브랜치)으로 되돌린다.
	rec = b.do(http.MethodPut, "/api/upgrade/channel", api.SetUpgradeChannelRequest{Channel: config.UpgradeChannelCurrent})
	wantStatus(t, rec, http.StatusOK)
	if v, _ := site.GetSite(); v.Upgrade != nil || decode[api.UpgradeInfo](t, rec).Channel != config.UpgradeChannelCurrent {
		t.Fatalf("cleared %+v", v.Upgrade)
	}

	// 도우미가 없으면(업그레이드를 쓸 수 없으면) 바꾸지 않는다.
	e = newEnv(t, envOptions{upgrade: true})
	e.enableUpgrade()
	b = e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantCode(t, b.do(http.MethodPut, "/api/upgrade/channel", api.SetUpgradeChannelRequest{Channel: config.UpgradeChannelMain}),
		http.StatusConflict, api.ErrUpgradeDisabled)
}

func TestUpgradeCheckWhileRunning(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, upgrade: true})
	e.enableUpgrade()
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	e.upgradeStatus(config.UpgradeStatus{State: config.UpgradeStateApplying, Stage: config.UpgradeStageBuild})
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeRemote}),
		http.StatusConflict, api.ErrUpgradeBusy)

	// 서비스의 시간 상한이 지나도록 바뀌지 않았으면 중단된 실행이다. 다시 확인할 수 있다.
	e.clk.Add(config.UpgradeServiceTimeout + time.Minute)
	b = e.browser() // 그사이 세션은 만료됐다.
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	info := decode[api.UpgradeInfo](t, b.do(http.MethodGet, "/api/upgrade", nil))
	if !info.Interrupted || info.Status == nil || info.Status.State != config.UpgradeStateApplying {
		t.Fatalf("interrupted = %+v", info)
	}
	wantStatus(t, b.do(http.MethodPost, "/api/upgrade/check", api.StartUpgradeCheckRequest{Mode: config.UpgradeModeRemote}), http.StatusOK)
}

func TestUpgradeApply(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, upgrade: true})
	e.enableUpgrade()
	e.upgradeStatus(checkedStatus(upgradeTarget))
	b := e.browser()
	loginTOTP(t, b)
	settings := map[string]string{"public-origin": "https://desk.example.com"}

	// 꼴이 틀리면 도우미까지 가지 않는다.
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: "HEAD"}),
		http.StatusBadRequest, api.ErrBadRequest)
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: upgradeTarget,
		Settings: map[string]string{"public-origin": "has space"}}), http.StatusBadRequest, api.ErrBadRequest)
	// 끝난 확인과 대상이 다르다.
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: strings.Repeat("a", 40)}),
		http.StatusConflict, api.ErrUpgradeOutdated)
	// 상승하지 않았다.
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: upgradeTarget, Settings: settings}),
		http.StatusForbidden, api.ErrElevationRequired)
	if _, err := os.Stat(filepath.Join(e.helper.UpgradeDir(), config.UpgradeApplyRequestName)); !os.IsNotExist(err) {
		t.Fatalf("apply request written before elevation: %v", err)
	}

	wantStatus(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}), http.StatusOK)
	rec := b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: upgradeTarget, Settings: settings})
	wantStatus(t, rec, http.StatusAccepted)
	data, err := os.ReadFile(filepath.Join(e.helper.UpgradeDir(), config.UpgradeApplyRequestName))
	if err != nil {
		t.Fatal(err)
	}
	var req config.UpgradeApplyRequest
	if err := json.Unmarshal(data, &req); err != nil || req.Target != upgradeTarget || req.User != "totp" ||
		req.Settings["public-origin"] != "https://desk.example.com" {
		t.Fatalf("apply request %s: %v", data, err)
	}
	logs := e.logs.String()
	if !strings.Contains(logs, `upgrade requested: user="totp" target=`+upgradeTarget+` settings="public-origin"`) ||
		strings.Contains(logs, "desk.example.com") {
		t.Fatalf("apply logs:\n%s", logs)
	}

	// 서비스가 진행하는 동안에는 받지 않는다.
	e.upgradeStatus(config.UpgradeStatus{State: config.UpgradeStateApplying, Stage: config.UpgradeStageBuild, Check: checkedStatus(upgradeTarget).Check})
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: upgradeTarget, Settings: settings}),
		http.StatusConflict, api.ErrUpgradeBusy)
}

func TestUpgradeApplyHelperDisabled(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, upgrade: true})
	e.enableUpgrade()
	e.upgradeStatus(checkedStatus(upgradeTarget))
	// 도우미 유닛에 요청 디렉터리가 없다(설정 파일과 도우미 유닛이 어긋난 설치).
	if err := os.Remove(e.helper.UpgradeDir()); err != nil {
		t.Fatal(err)
	}
	b := e.browser()
	loginTOTP(t, b)
	wantStatus(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}), http.StatusOK)
	wantCode(t, b.do(http.MethodPost, "/api/upgrade/apply", api.StartUpgradeRequest{Target: upgradeTarget,
		Settings: map[string]string{"public-origin": "-"}}), http.StatusConflict, api.ErrUpgradeDisabled)
}
