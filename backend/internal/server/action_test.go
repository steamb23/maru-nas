package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/addons/containers"
	"maru/backend/internal/addons/services"
	"maru/backend/internal/api"
	"maru/backend/internal/omv/omvfake"
)

// actionEnv는 테스트용 조작 애드온을 등록한 환경이다.
//
// jobs 애드온의 run 조작은 대상에 따라 동작한다: ok(쓰기 RPC 후 백그라운드 작업 성공),
// fail(백그라운드 작업 실패), block(release가 닫힐 때까지 대기), dirty(ActionError), 그 밖은 잘못된 대상.
type actionEnv struct {
	*env
	rec     *recordingOMV
	plugins *fakePlugins
	started chan struct{}
	release chan struct{}
}

func newActionEnv(t *testing.T) *actionEnv {
	t.Helper()
	ae := &actionEnv{started: make(chan struct{}, 1), release: make(chan struct{})}
	run := func(ctx context.Context, req addons.ActionRequest) error {
		switch req.Target {
		case "ok", "fail":
			if _, err := req.RPC.Call(ctx, "UserMgmt", "getUserByContext", nil); err != nil {
				return err
			}
			fail := ""
			if req.Target == "fail" {
				fail = "docker: no such container"
			}
			return addons.WaitBg(ctx, req.RPC, ae.fake.StartBg(fail), time.Millisecond)
		case "block":
			ae.started <- struct{}{}
			<-ae.release
			return nil
		case "dirty":
			return &addons.ActionError{Status: http.StatusConflict, Code: api.ErrConfigDirty, Message: "pending changes"}
		case "sneaky":
			// 다른 애드온이 선언한 쓰기 메서드
			_, err := req.RPC.Call(ctx, "Compose", "doContainerCommand", nil)
			return err
		}
		return addons.ErrInvalidTarget
	}
	jobs := testAddon("jobs", api.Requirements{}, api.RPCMethods{
		Read:  []string{"Exec.isRunning"},
		Write: []string{"UserMgmt.getUserByContext"},
	})
	jobs.Manifest.Actions = []api.ActionDecl{{Name: "run", Admin: true}, {Name: "slow"}}
	jobs.Actions = map[string]addons.Action{
		"run": {Run: run},
		"slow": {Timeout: 30 * time.Millisecond, Run: func(ctx context.Context, req addons.ActionRequest) error {
			return addons.WaitBg(ctx, req.RPC, ae.fake.StartBg(""), time.Millisecond)
		}},
	}
	other := testAddon("other", api.Requirements{}, api.RPCMethods{Write: []string{"Compose.doContainerCommand"}})
	compose := testAddon("needs-compose", api.Requirements{Plugins: []string{"openmediavault-compose"}},
		api.RPCMethods{Write: []string{"Compose.doContainerCommand"}})
	compose.Manifest.Actions = []api.ActionDecl{{Name: "run"}}
	compose.Actions = map[string]addons.Action{"run": {Run: run}}

	ae.plugins = &fakePlugins{installed: map[string]bool{}}
	ae.rec = &recordingOMV{}
	ae.env = newEnv(t, envOptions{
		addons:  testRegistry(t, jobs, other, compose),
		plugins: ae.plugins,
		wrapOMV: func(c OMV) OMV { ae.rec.OMV = c; return ae.rec },
	})
	return ae
}

func act(b *browser, addon, action, target string) *httptest.ResponseRecorder {
	return b.do(http.MethodPost, "/api/addons/"+addon+"/actions/"+action, api.ActionRequest{Target: target})
}

func TestActionRuns(t *testing.T) {
	e := newActionEnv(t)
	e.fake.SetBgPolls(2)
	b := e.browser()
	b.header.Set("X-Forwarded-For", "192.0.2.10")
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	wantStatus(t, act(b, "jobs", "run", "ok"), http.StatusNoContent)
	var methods []string
	for _, c := range e.rec.calls {
		methods = append(methods, c.Service+"."+c.Method)
		if !c.NoTouch || c.ClientIP != "192.0.2.10" {
			t.Errorf("omv request %+v", c)
		}
	}
	want := "UserMgmt.getUserByContext Exec.isRunning Exec.isRunning Exec.isRunning"
	if got := strings.Join(methods, " "); got != want {
		t.Errorf("calls = %s, want %s", got, want)
	}
	if !strings.Contains(e.logs.String(), `action: user="admin" addon=jobs action=run target="ok" result=ok`) {
		t.Errorf("log %q", e.logs.String())
	}
}

func TestActionErrors(t *testing.T) {
	e := newActionEnv(t)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	wantStatus(t, act(b, "jobs", "nope", "ok"), http.StatusNotFound)
	wantStatus(t, act(b, "nope", "run", "ok"), http.StatusNotFound)
	wantCode(t, b.do(http.MethodPost, "/api/addons/jobs/actions/run", "ok"), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(t, act(b, "jobs", "run", "-rf"), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(t, act(b, "jobs", "run", "dirty"), http.StatusConflict, api.ErrConfigDirty)
	wantCode(t, act(b, "needs-compose", "run", "ok"), http.StatusForbidden, api.ErrPluginMissing)

	got := wantCode(t, act(b, "jobs", "run", "fail"), http.StatusBadGateway, api.ErrActionFailed)
	if strings.Contains(got.Error, "docker") {
		t.Errorf("omv error leaked: %q", got.Error)
	}
	if strings.Contains(e.logs.String(), "trace") {
		t.Error("omv trace logged")
	}

	// 다른 애드온이 선언한 쓰기 메서드는 OMV에 보내지 않는다.
	before := len(e.fake.Calls())
	wantStatus(t, act(b, "jobs", "run", "sneaky"), http.StatusInternalServerError)
	if len(e.fake.Calls()) != before {
		t.Error("undeclared write reached omv")
	}

	e.fake.SetBgPolls(1000)
	wantCode(t, act(b, "jobs", "slow", ""), http.StatusGatewayTimeout, api.ErrActionTimeout)
}

func TestActionRequiresAdmin(t *testing.T) {
	e := newActionEnv(t)
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	before := len(e.fake.Calls())
	wantCode(t, act(b, "jobs", "run", "ok"), http.StatusForbidden, api.ErrAdminRequired)
	if len(e.fake.Calls()) != before {
		t.Error("rejected action reached omv")
	}
	// admin 조건이 없는 조작은 일반 사용자도 실행한다. 역할은 OMV가 최종 검사한다
	// (가짜 OMV의 Exec.isRunning도 관리자 전용이라 실패한다).
	e.fake.SetBgPolls(0)
	wantCode(t, act(b, "jobs", "slow", ""), http.StatusBadGateway, api.ErrActionFailed)
}

func TestActionBusy(t *testing.T) {
	e := newActionEnv(t)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- act(b, "jobs", "run", "block") }()
	<-e.started
	wantCode(t, act(b, "jobs", "run", "block"), http.StatusConflict, api.ErrActionBusy)
	// 다른 대상은 막지 않는다.
	wantStatus(t, act(b, "jobs", "run", "ok"), http.StatusNoContent)
	close(e.release)
	wantStatus(t, <-done, http.StatusNoContent)
	// 끝나면 다시 실행할 수 있다.
	e.release = make(chan struct{})
	close(e.release)
	wantStatus(t, act(b, "jobs", "run", "block"), http.StatusNoContent)
}

func TestActionOMVSessionExpired(t *testing.T) {
	e := newActionEnv(t)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	e.fake.Expire("admin")
	wantCode(t, act(b, "jobs", "run", "ok"), http.StatusUnauthorized, api.ErrUnauthorized)
	if e.store.Len() != 0 {
		t.Error("dashboard session not deleted")
	}
}

// 실제 내장 애드온(containers)의 조작이 가짜 OMV까지 가는 경로.
func TestActionBuiltinContainers(t *testing.T) {
	e, _, _ := newDataEnv(t, containers.New())
	e.fake.SetBgPolls(0)
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)

	wantStatus(t, act(admin, "containers", "stop", "web"), http.StatusNoContent)
	var params string
	for _, c := range e.fake.Calls() {
		if c.Method == "doContainerCommand" {
			params = string(c.Params)
		}
	}
	if params != `{"command":"stop","command2":"","id":"web"}` {
		t.Errorf("params %s", params)
	}
	r := admin.do(http.MethodGet, "/api/addons/containers", nil)
	wantStatus(t, r, http.StatusOK)
	for _, c := range decode[api.ContainersResponse](t, r).Containers {
		if c.Name == "web" && c.State != "exited" {
			t.Errorf("web state %s after stop", c.State)
		}
	}

	wantCode(t, act(admin, "containers", "start", "broken"), http.StatusBadGateway, api.ErrActionFailed)
	wantCode(t, act(admin, "containers", "restart", "web; reboot"), http.StatusBadRequest, api.ErrBadRequest)

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, act(user, "containers", "start", "web"), http.StatusForbidden, api.ErrAdminRequired)
}

// 실제 내장 애드온(services)의 켜기·끄기가 가짜 OMV까지 가는 경로.
func TestActionBuiltinServices(t *testing.T) {
	e, _, _ := newDataEnv(t, services.New())
	e.fake.SetBgPolls(0)
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	status := func(name string) api.Service {
		t.Helper()
		r := admin.do(http.MethodGet, "/api/addons/services", nil)
		wantStatus(t, r, http.StatusOK)
		for _, s := range decode[api.ServicesResponse](t, r).Services {
			if s.Name == name {
				return s
			}
		}
		t.Fatalf("service %s not found", name)
		return api.Service{}
	}

	// 가짜 OMV는 set 스키마 밖의 속성(NFS의 shares)을 거절하므로 통과하면 골라 보낸 것이다.
	wantStatus(t, act(admin, "services", "enable", "nfs"), http.StatusNoContent)
	if s := status("nfs"); !s.Enabled || !s.Running || !s.Toggleable {
		t.Errorf("nfs after enable %+v", s)
	}
	wantStatus(t, act(admin, "services", "disable", "ssh"), http.StatusNoContent)
	if s := status("ssh"); s.Enabled || s.Running {
		t.Errorf("ssh after disable %+v", s)
	}
	if s := status("fail2ban"); s.Toggleable {
		t.Error("fail2ban toggleable")
	}

	// 적용하지 않은 변경이 있으면 설정을 바꾸지 않는다.
	data := omvfake.DefaultPluginData()
	data.Dirty = []string{"samba"}
	e.fake.SetPluginData(data)
	before := len(e.fake.Calls())
	wantCode(t, act(admin, "services", "disable", "samba"), http.StatusConflict, api.ErrConfigDirty)
	for _, c := range e.fake.Calls()[before:] {
		if c.Method == "setSettings" {
			t.Error("setSettings called while dirty")
		}
	}

	data = omvfake.DefaultPluginData()
	data.FailApply = true
	e.fake.SetPluginData(data)
	wantCode(t, act(admin, "services", "disable", "samba"), http.StatusBadGateway, api.ErrActionFailed)
	wantCode(t, act(admin, "services", "enable", "fail2ban"), http.StatusBadRequest, api.ErrBadRequest)

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, act(user, "services", "enable", "nfs"), http.StatusForbidden, api.ErrAdminRequired)
}

// 서비스 켜기·끄기를 연달아 하면 뒤의 것은 앞의 적용을 기다린다. 앞 요청이 남긴 적용 대기 때문에
// config_dirty로 거절하지 않는다.
func TestActionServicesSerialized(t *testing.T) {
	e, _, _ := newDataEnv(t, services.New())
	e.fake.SetBgPolls(3)
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)

	first := make(chan *httptest.ResponseRecorder)
	go func() { first <- act(admin, "services", "disable", "ssh") }()
	// 첫 요청이 적용을 시작할 때까지(가짜 OMV에 applyChangesBg가 올 때까지) 기다린다.
	deadline := time.Now().Add(5 * time.Second)
	for !slices.ContainsFunc(e.fake.Calls(), func(c omvfake.Call) bool { return c.Method == "applyChangesBg" }) {
		if time.Now().After(deadline) {
			t.Fatal("first toggle did not start applying")
		}
		time.Sleep(5 * time.Millisecond)
	}
	wantStatus(t, act(admin, "services", "enable", "nfs"), http.StatusNoContent)
	wantStatus(t, <-first, http.StatusNoContent)
}
