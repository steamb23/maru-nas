package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/addons/containers"
	"maru/backend/internal/addons/services"
	"maru/backend/internal/addons/smart"
	"maru/backend/internal/addons/sysinfo"
	"maru/backend/internal/api"
	"maru/backend/internal/omv/omvfake"
)

// dataAddon은 Fetch가 있는 테스트용 애드온이다.
func dataAddon(id string, req api.Requirements, rpc api.RPCMethods,
	fetch func(context.Context, addons.Request) (any, error)) addons.Addon {
	a := testAddon(id, req, rpc)
	a.Fetch = fetch
	return a
}

type fetchResult struct {
	Admin   bool            `json:"admin"`
	Sources []string        `json:"sources"`
	Refresh bool            `json:"refresh"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// callFetch는 RPC 하나를 부르고 결과나 오류를 그대로 응답에 담는다(부분 실패 흉내).
func callFetch(service, method string, params any) func(context.Context, addons.Request) (any, error) {
	return func(ctx context.Context, req addons.Request) (any, error) {
		out := fetchResult{Admin: req.Admin, Sources: req.Sources, Refresh: req.Refresh}
		raw, err := req.RPC.Call(ctx, service, method, params)
		if err != nil {
			out.Error = err.Error()
		} else {
			out.Result = raw
		}
		return out, nil
	}
}

func newDataEnv(t *testing.T, list ...addons.Addon) (*env, *fakePlugins, *recordingOMV) {
	t.Helper()
	plugins := &fakePlugins{installed: map[string]bool{"openmediavault-compose": true}}
	rec := &recordingOMV{}
	e := newEnv(t, envOptions{
		addons:  testRegistry(t, list...),
		plugins: plugins,
		wrapOMV: func(c OMV) OMV { rec.OMV = c; return rec },
	})
	return e, plugins, rec
}

func TestAddonDataCallsDeclaredRPC(t *testing.T) {
	e, _, rec := newDataEnv(t,
		dataAddon("containers", api.Requirements{Plugins: []string{"openmediavault-compose"}},
			api.RPCMethods{Read: []string{"Compose.getContainers"}}, callFetch("Compose", "getContainers", nil)),
	)
	b := e.browser()
	b.header.Set("X-Forwarded-For", "192.0.2.10")
	wantStatus(t, b.login("user", "user"), http.StatusOK)

	r := b.do(http.MethodGet, "/api/addons/containers?refresh=1", nil)
	wantStatus(t, r, http.StatusOK)
	got := decode[fetchResult](t, r)
	if got.Error != "" || got.Admin || !got.Refresh || len(got.Sources) != 1 || got.Sources[0] != "direct" {
		t.Fatalf("got %+v", got)
	}
	var rows []omvfake.Container
	if err := json.Unmarshal(got.Result, &rows); err != nil || len(rows) == 0 || rows[0].Name != "Docker" {
		t.Fatalf("rows %s (%v)", got.Result, err)
	}
	last := rec.calls[len(rec.calls)-1]
	if !last.NoTouch || last.Service != "Compose" || last.ClientIP != "192.0.2.10" {
		t.Errorf("omv request %+v", last)
	}
	// 주기 요청이 OMV 세션을 연장하지 않는다.
	calls := e.fake.Calls()
	if c := calls[len(calls)-1]; c.Touch {
		t.Error("provider rpc touched the omv session")
	}
}

func TestAddonDataRejectsUndeclaredRPC(t *testing.T) {
	e, _, _ := newDataEnv(t,
		// other가 선언한 메서드를 sneaky가 부르려 한다.
		dataAddon("other", api.Requirements{}, api.RPCMethods{Read: []string{"WebGui.getAutoLogoutSettings"}},
			callFetch("WebGui", "getAutoLogoutSettings", nil)),
		dataAddon("sneaky", api.Requirements{}, api.RPCMethods{}, callFetch("WebGui", "getAutoLogoutSettings", nil)),
		// Fetch는 자기가 선언한 쓰기 메서드도 부를 수 없다(조작에서만 부른다).
		dataAddon("writer", api.Requirements{}, api.RPCMethods{Write: []string{"UserMgmt.getUserByContext"}},
			callFetch("UserMgmt", "getUserByContext", nil)),
	)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	before := len(e.fake.Calls())

	for _, id := range []string{"sneaky", "writer"} {
		r := b.do(http.MethodGet, "/api/addons/"+id, nil)
		wantStatus(t, r, http.StatusOK)
		if got := decode[fetchResult](t, r); !strings.Contains(got.Error, addons.ErrRPCNotAllowed.Error()) {
			t.Errorf("%s: error %q", id, got.Error)
		}
	}
	if n := len(e.fake.Calls()) - before; n != 0 {
		t.Errorf("rejected rpc reached omv (%d calls)", n)
	}
}

func TestAddonDataCapability(t *testing.T) {
	e, _, _ := newDataEnv(t,
		dataAddon("smart", api.Requirements{Admin: true}, api.RPCMethods{Read: []string{"Smart.getList"}},
			callFetch("Smart", "getList", map[string]any{"start": 0, "limit": nil})),
		dataAddon("needs-cputemp", api.Requirements{Plugins: []string{"openmediavault-cputemp"}},
			api.RPCMethods{Read: []string{"CpuTemp.get"}}, callFetch("CpuTemp", "get", nil)),
		testAddon("no-fetch", api.Requirements{}, api.RPCMethods{}),
	)
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantCode(t, b.do(http.MethodGet, "/api/addons/smart", nil), http.StatusForbidden, api.ErrAdminRequired)
	wantCode(t, b.do(http.MethodGet, "/api/addons/needs-cputemp", nil), http.StatusForbidden, api.ErrPluginMissing)
	wantStatus(t, b.do(http.MethodGet, "/api/addons/no-fetch", nil), http.StatusNotFound)
	wantStatus(t, b.do(http.MethodGet, "/api/addons/missing", nil), http.StatusNotFound)

	anon := e.browser()
	wantCode(t, anon.do(http.MethodGet, "/api/addons/smart", nil), http.StatusUnauthorized, api.ErrUnauthorized)

	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	r := admin.do(http.MethodGet, "/api/addons/smart", nil)
	wantStatus(t, r, http.StatusOK)
	got := decode[fetchResult](t, r)
	var list struct {
		Total int `json:"total"`
	}
	if got.Error != "" || json.Unmarshal(got.Result, &list) != nil || list.Total == 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestAddonDataOMVErrors(t *testing.T) {
	failing := func(ctx context.Context, req addons.Request) (any, error) {
		_, err := req.RPC.Call(ctx, "Compose", "getContainers", nil)
		return nil, err
	}
	// 오류를 삼키는 provider도 세션 만료는 알린다.
	swallow := func(ctx context.Context, req addons.Request) (any, error) {
		req.RPC.Call(ctx, "Compose", "getContainers", nil)
		return map[string]string{}, nil
	}
	e, plugins, _ := newDataEnv(t,
		dataAddon("containers", api.Requirements{}, api.RPCMethods{Read: []string{"Compose.getContainers"}}, failing),
		dataAddon("swallow", api.Requirements{}, api.RPCMethods{Read: []string{"Compose.getContainers"}}, swallow),
		dataAddon("local", api.Requirements{}, api.RPCMethods{}, func(context.Context, addons.Request) (any, error) {
			return nil, errors.New("read failed")
		}),
	)
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)

	wantStatus(t, b.do(http.MethodGet, "/api/addons/local", nil), http.StatusInternalServerError)

	data := omvfake.DefaultPluginData()
	data.Missing = []string{"Compose"}
	e.fake.SetPluginData(data)
	wantCode(t, b.do(http.MethodGet, "/api/addons/containers", nil), http.StatusBadGateway, api.ErrOMVError)
	if plugins.invalidates != 1 {
		t.Errorf("invalidates = %d, want 1", plugins.invalidates)
	}
	if strings.Contains(e.logs.String(), "trace") {
		t.Error("omv trace logged")
	}

	e.fake.Expire("user")
	wantCode(t, b.do(http.MethodGet, "/api/addons/swallow", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	if e.store.Len() != 0 {
		t.Error("dashboard session not deleted")
	}
}

// 실제 내장 애드온(containers)이 가짜 OMV까지 provider 내부 RPC로 가는 경로.
func TestAddonDataBuiltinContainers(t *testing.T) {
	e, _, _ := newDataEnv(t, containers.New())
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	r := b.do(http.MethodGet, "/api/addons/containers", nil)
	wantStatus(t, r, http.StatusOK)
	got := decode[api.ContainersResponse](t, r)
	if got.Runtime != "docker" || len(got.Containers) != len(omvfake.DefaultPluginData().Containers) {
		t.Errorf("got %+v", got)
	}
}

// 시스템 정보 창의 데이터: 요약은 누구나, 프로세스 목록은 관리자에게만 준다(provider 조건).
func TestAddonDataBuiltinSystemInfo(t *testing.T) {
	proc := fstest.MapFS{
		"stat":      {Data: []byte("cpu 10 0 10 80 0\ncpu0 10 0 10 80 0\n")},
		"meminfo":   {Data: []byte("MemTotal: 1000 kB\nMemAvailable: 500 kB\n")},
		"uptime":    {Data: []byte("5.0 1.0\n")},
		"diskstats": {Data: []byte("8 0 sda 1 0 8 0 1 0 16 0 0 0 0\n")},
		"1/stat":    {Data: []byte("1 (init) S 0 0 0 0 0 4194560 0 0 0 0 5 5 0 0 0 0 1 0 2 0 0\n")},
		"1/status":  {Data: []byte("Uid:\t0\t0\t0\t0\nVmRSS:\t100 kB\n")},
		"1/cmdline": {Data: []byte("/sbin/init\x00")},
	}
	block := fstest.MapFS{"sda/device": {Data: []byte("../../0:0:0:0"), Mode: os.ModeSymlink}}
	e, _, _ := newDataEnv(t, sysinfo.New(
		sysinfo.NewReaderFS(proc, block, nil),
		sysinfo.NewProcessesFS(proc, func(string) string { return "root" }),
	))
	for _, c := range []struct {
		user    string
		sources []string
		procs   int
	}{
		{"user", []string{"proc"}, 0},
		{"admin", []string{"proc", sysinfo.SourceProcesses}, 1},
	} {
		b := e.browser()
		wantStatus(t, b.login(c.user, c.user), http.StatusOK)
		caps := decode[api.CapabilitiesResponse](t, b.do(http.MethodGet, "/api/capabilities", nil))
		if len(caps.Addons) != 1 || !caps.Addons[0].Available || !slices.Equal(caps.Addons[0].Sources, c.sources) {
			t.Errorf("%s: capability %+v", c.user, caps.Addons)
		}
		// 목록은 processes=1로 요청할 때만 준다(관리자만).
		r := b.do(http.MethodGet, "/api/addons/system-info?processes=1", nil)
		wantStatus(t, r, http.StatusOK)
		got := decode[api.SystemStatsResponse](t, r)
		if got.CPU.TotalTicks != 100 || len(got.CPU.Cores) != 1 || got.Disk == nil || got.Disk.WriteBytes != 16*512 {
			t.Errorf("%s: stats %+v", c.user, got)
		}
		if len(got.Processes) != c.procs || !slices.Equal(got.Sources, c.sources) {
			t.Errorf("%s: processes %+v, sources %v", c.user, got.Processes, got.Sources)
		}
		if plain := decode[api.SystemStatsResponse](t, b.do(http.MethodGet, "/api/addons/system-info", nil)); plain.Processes != nil {
			t.Errorf("%s: processes without query", c.user)
		}
	}
}

// 컨테이너 목록은 세션 사이에 잠깐 함께 쓴다. 새로고침과 조작 뒤에는 OMV에 다시 묻는다.
func TestAddonDataContainersSharedCache(t *testing.T) {
	e, _, _ := newDataEnv(t, containers.New())
	e.fake.SetBgPolls(0)
	listCalls := func() int {
		n := 0
		for _, c := range e.fake.Calls() {
			if c.Method == "getContainers" {
				n++
			}
		}
		return n
	}
	state := func(b *browser, name string) string {
		t.Helper()
		r := b.do(http.MethodGet, "/api/addons/containers", nil)
		wantStatus(t, r, http.StatusOK)
		for _, c := range decode[api.ContainersResponse](t, r).Containers {
			if c.Name == name {
				return c.State
			}
		}
		t.Fatalf("container %s not found", name)
		return ""
	}

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	if s := state(user, "web"); s != "running" {
		t.Fatalf("web state %s", s)
	}
	state(admin, "web")
	if n := listCalls(); n != 1 {
		t.Errorf("getContainers calls = %d, want 1", n)
	}
	wantStatus(t, admin.do(http.MethodGet, "/api/addons/containers?refresh=1", nil), http.StatusOK)
	if n := listCalls(); n != 2 {
		t.Errorf("getContainers calls after refresh = %d, want 2", n)
	}

	wantStatus(t, act(admin, "containers", "stop", "web"), http.StatusNoContent)
	if s := state(user, "web"); s != "exited" {
		t.Errorf("web state %s after stop, want exited (cache not invalidated)", s)
	}
}

// SMART는 관리자만 쓰고, 결과를 세션 사이에 함께 써서 OMV(smartctl)를 다시 부르지 않는다.
func TestAddonDataBuiltinSmart(t *testing.T) {
	cache := smart.NewCache(0, 0)
	e, _, _ := newDataEnv(t, smart.New(cache))
	cache.Now = e.clk.Now
	smartCalls := func() int {
		n := 0
		for _, c := range e.fake.Calls() {
			if c.Service == "Smart" {
				n++
			}
		}
		return n
	}

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, user.do(http.MethodGet, "/api/addons/smart", nil), http.StatusForbidden, api.ErrAdminRequired)

	a := e.browser()
	wantStatus(t, a.login("admin", "admin"), http.StatusOK)
	r := a.do(http.MethodGet, "/api/addons/smart", nil)
	wantStatus(t, r, http.StatusOK)
	if strings.Contains(r.Body.String(), "FAKE-SERIAL") {
		t.Error("serial number sent to browser")
	}
	got := decode[api.SmartResponse](t, r)
	if len(got.Disks) != len(omvfake.DefaultPluginData().Smart) || got.CheckedAt == "" {
		t.Errorf("got %+v", got)
	}

	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/addons/smart", nil), http.StatusOK)
	// 새로고침은 1분이 지나야 다시 부른다.
	wantStatus(t, b.do(http.MethodGet, "/api/addons/smart?refresh=1", nil), http.StatusOK)
	if n := smartCalls(); n != 1 {
		t.Errorf("smart calls = %d, want 1", n)
	}
	e.clk.Add(2 * time.Minute)
	wantStatus(t, b.do(http.MethodGet, "/api/addons/smart?refresh=1", nil), http.StatusOK)
	if n := smartCalls(); n != 2 {
		t.Errorf("smart calls = %d, want 2", n)
	}
}

// 서비스 상태는 관리자만 본다.
func TestAddonDataBuiltinServices(t *testing.T) {
	e, _, _ := newDataEnv(t, services.New())
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	r := admin.do(http.MethodGet, "/api/addons/services", nil)
	wantStatus(t, r, http.StatusOK)
	if got := decode[api.ServicesResponse](t, r); len(got.Services) != len(omvfake.DefaultPluginData().Services) {
		t.Errorf("got %+v", got)
	}

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, user.do(http.MethodGet, "/api/addons/services", nil), http.StatusForbidden, api.ErrAdminRequired)
}
