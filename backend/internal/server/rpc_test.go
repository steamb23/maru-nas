package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"omv-desktop-dashboard/backend/internal/api"
	"omv-desktop-dashboard/backend/internal/omv"
	"omv-desktop-dashboard/backend/internal/omv/omvfake"
)

// recordingOMV는 실제 클라이언트로 넘기면서 Call 요청을 기록한다.
type recordingOMV struct {
	OMV
	mu    sync.Mutex
	calls []omv.Request
}

func (r *recordingOMV) Call(ctx context.Context, req omv.Request) (omv.Response, error) {
	r.mu.Lock()
	r.calls = append(r.calls, req)
	r.mu.Unlock()
	return r.OMV.Call(ctx, req)
}

type rpcEnv struct {
	*env
	plugins *fakePlugins
	rec     *recordingOMV
}

// newRPCEnv는 읽기·쓰기·관리자 전용·플러그인 필요 애드온을 등록한 환경이다.
func newRPCEnv(t *testing.T) *rpcEnv {
	t.Helper()
	plugins := &fakePlugins{installed: map[string]bool{}}
	rec := &recordingOMV{}
	e := newEnv(t, envOptions{
		addons: testRegistry(t,
			testAddon("reader", api.Requirements{}, api.RPCMethods{Read: []string{"WebGui.getAutoLogoutSettings"}}),
			testAddon("writer", api.Requirements{}, api.RPCMethods{Write: []string{"UserMgmt.getUserByContext"}}),
			testAddon("admin-only", api.Requirements{Admin: true}, api.RPCMethods{Read: []string{"Smart.getList"}}),
			testAddon("needs-compose", api.Requirements{Plugins: []string{"openmediavault-compose"}},
				api.RPCMethods{Read: []string{"Compose.getContainers"}}),
		),
		plugins: plugins,
		wrapOMV: func(c OMV) OMV { rec.OMV = c; return rec },
	})
	return &rpcEnv{env: e, plugins: plugins, rec: rec}
}

func (e *rpcEnv) engineCalls() int {
	n := 0
	for _, c := range e.fake.Calls() {
		if c.Service != "Session" {
			n++
		}
	}
	return n
}

func rpcReq(service, method string, params string) api.RPCRequest {
	r := api.RPCRequest{Service: service, Method: method}
	if params != "" {
		r.Params = []byte(params)
	}
	return r
}

func TestRPCRelaysAllowedRead(t *testing.T) {
	e := newRPCEnv(t)
	b := e.browser()
	b.header.Set("X-Forwarded-For", "192.0.2.10")
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	before := len(e.fake.Calls())

	rec := b.do(http.MethodPost, "/api/rpc", rpcReq("WebGui", "getAutoLogoutSettings", `{"x": 12345678901234567890}`))
	wantStatus(t, rec, http.StatusOK)
	if got := strings.TrimSpace(rec.Body.String()); got != `{"timeout":5}` {
		t.Errorf("body = %q", got)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	calls := e.fake.Calls()[before:]
	if len(calls) != 1 || calls[0].Service != "WebGui" || calls[0].Method != "getAutoLogoutSettings" {
		t.Fatalf("omv calls = %+v", calls)
	}
	// 주기 요청이 OMV 세션을 연장하지 않게 updatelastaccess=false로 보낸다.
	if calls[0].Touch {
		t.Error("relay touched the omv session")
	}
	if calls[0].ForwardedFor != "192.0.2.10" {
		t.Errorf("X-Forwarded-For = %q", calls[0].ForwardedFor)
	}
	// 파라미터는 숫자 정밀도를 잃지 않고 그대로 넘긴다(공백은 테스트 브라우저의 json.Marshal이 없앤다).
	last := e.rec.calls[len(e.rec.calls)-1]
	if p, ok := last.Params.(json.RawMessage); !ok || string(p) != `{"x":12345678901234567890}` {
		t.Errorf("params = %#v", last.Params)
	}
}

func TestRPCEmptyParams(t *testing.T) {
	e := newRPCEnv(t)
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	for _, p := range []string{"", "null"} {
		wantStatus(t, b.do(http.MethodPost, "/api/rpc", rpcReq("WebGui", "getAutoLogoutSettings", p)), http.StatusOK)
		if last := e.rec.calls[len(e.rec.calls)-1]; last.Params != nil {
			t.Errorf("params %q relayed as %#v", p, last.Params)
		}
	}
}

func TestRPCRejectsWithoutCallingOMV(t *testing.T) {
	e := newRPCEnv(t)
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	before := e.engineCalls()

	cases := []struct {
		req    api.RPCRequest
		status int
		code   string
	}{
		{rpcReq("System", "reboot", ""), http.StatusForbidden, api.ErrRPCForbidden},
		{rpcReq("Session", "logout", ""), http.StatusForbidden, api.ErrRPCForbidden},
		{rpcReq("webgui", "getAutoLogoutSettings", ""), http.StatusForbidden, api.ErrRPCForbidden},
		// 3단계에서는 manifest에 선언한 쓰기 메서드도 막는다.
		{rpcReq("UserMgmt", "getUserByContext", ""), http.StatusForbidden, api.ErrRPCForbidden},
		{rpcReq("Smart", "getList", ""), http.StatusForbidden, api.ErrAdminRequired},
		{rpcReq("Compose", "getContainers", ""), http.StatusForbidden, api.ErrPluginMissing},
	}
	for _, c := range cases {
		wantCode(t, b.do(http.MethodPost, "/api/rpc", c.req), c.status, c.code)
	}
	if n := e.engineCalls(); n != before {
		t.Errorf("rejected requests reached omv (%d calls)", n-before)
	}

	anon := e.browser()
	wantCode(t, anon.do(http.MethodPost, "/api/rpc", rpcReq("WebGui", "getAutoLogoutSettings", "")),
		http.StatusUnauthorized, api.ErrUnauthorized)
}

func TestRPCBadRequests(t *testing.T) {
	e := newRPCEnv(t)
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)

	wantCode(t, b.do(http.MethodPost, "/api/rpc", "not an object"), http.StatusBadRequest, api.ErrBadRequest)
	big := rpcReq("WebGui", "getAutoLogoutSettings", `{"pad":"`+strings.Repeat("x", maxRPCBodyBytes)+`"}`)
	wantCode(t, b.do(http.MethodPost, "/api/rpc", big), http.StatusBadRequest, api.ErrBadRequest)

	b.header.Set("Content-Type", "text/plain")
	wantStatus(t, b.do(http.MethodPost, "/api/rpc", rpcReq("WebGui", "getAutoLogoutSettings", "")),
		http.StatusUnsupportedMediaType)
	// GET은 다른 /api 경로와 마찬가지로 404다.
	wantStatus(t, b.do(http.MethodGet, "/api/rpc", nil), http.StatusNotFound)
}

func TestRPCOMVErrors(t *testing.T) {
	e := newRPCEnv(t)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	// Smart 서비스가 없다 → 404. 플러그인 감지를 다시 하도록 표시한다.
	data := omvfake.DefaultPluginData()
	data.Missing = []string{"Smart"}
	e.fake.SetPluginData(data)
	wantCode(t, b.do(http.MethodPost, "/api/rpc", rpcReq("Smart", "getList", "")), http.StatusBadGateway, api.ErrOMVError)
	if e.plugins.invalidates != 1 {
		t.Errorf("invalidates = %d, want 1", e.plugins.invalidates)
	}
	if strings.Contains(e.logs.String(), "trace") {
		t.Error("omv trace logged")
	}

	// OMV 세션이 만료됐으면 대시보드 세션도 지운다.
	e.fake.Expire("admin")
	wantCode(t, b.do(http.MethodPost, "/api/rpc", rpcReq("WebGui", "getAutoLogoutSettings", "")),
		http.StatusUnauthorized, api.ErrUnauthorized)
	if e.store.Len() != 0 {
		t.Error("dashboard session not deleted")
	}
}

func TestRPCOMVUnreachable(t *testing.T) {
	e := newRPCEnv(t)
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	e.omvTS.Close()
	wantCode(t, b.do(http.MethodPost, "/api/rpc", rpcReq("WebGui", "getAutoLogoutSettings", "")),
		http.StatusBadGateway, api.ErrOMVError)
	if e.store.Len() != 1 {
		t.Error("session dropped while omv is unreachable")
	}
}
