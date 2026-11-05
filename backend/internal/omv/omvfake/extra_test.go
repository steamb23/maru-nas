package omvfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testUA = "omvfake-test/1"

// rpcReply는 가짜 rpc.php 응답이다.
type rpcReply struct {
	status int
	result json.RawMessage
	err    string
}

// call은 sid 세션으로 RPC 하나를 부른다. sid가 비면 세션 없이 부른다.
func call(t *testing.T, s *Server, sid, service, method string, params any) rpcReply {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"service": service, "method": method, "params": params})
	req := httptest.NewRequest(http.MethodPost, "/rpc.php", strings.NewReader(string(body)))
	req.Header.Set("User-Agent", testUA)
	if sid != "" {
		req.Header.Set("X-OpenMediaVault-SessionID", sid)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	var resp struct {
		Response json.RawMessage `json:"response"`
		Error    *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s.%s: %v: %s", service, method, err, rec.Body.String())
	}
	r := rpcReply{status: rec.Code, result: resp.Response}
	if resp.Error != nil {
		r.err = resp.Error.Message
	}
	return r
}

// login은 가짜 계정으로 로그인한 세션 ID다.
func login(t *testing.T, s *Server, user string) string {
	t.Helper()
	r := call(t, s, "", "Session", "login", map[string]string{"username": user, "password": user})
	var res struct {
		SessionID string `json:"sessionid"`
	}
	if r.status != http.StatusOK || json.Unmarshal(r.result, &res) != nil || res.SessionID == "" {
		t.Fatalf("login %s: %d %s %s", user, r.status, r.result, r.err)
	}
	return res.SessionID
}

func TestParseExtraRPC(t *testing.T) {
	got, err := ParseExtraRPC([]byte(`{
		"Example.getStatus": {"result": {"running": true}, "admin": true},
		"example.getNothing": {"result": null}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if e := got["Example.getStatus"]; !e.Admin || string(e.Result) != `{"running": true}` {
		t.Errorf("getStatus = %+v", e)
	}
	if e := got["example.getNothing"]; e.Admin || string(e.Result) != "null" {
		t.Errorf("getNothing = %+v", e)
	}

	for name, in := range map[string]string{
		"not an object":     `[]`,
		"null":              `null`,
		"trailing":          `{} {}`,
		"bad key":           `{"Example": {"result": 1}}`,
		"bad key chars":     `{"Example.get-status": {"result": 1}}`,
		"session":           `{"session.login": {"result": 1}}`,
		"no result":         `{"Example.get": {"admin": true}}`,
		"unknown field":     `{"Example.get": {"result": 1, "status": 500}}`,
		"admin not bool":    `{"Example.get": {"result": 1, "admin": "yes"}}`,
		"entry null":        `{"Example.get": null}`,
		"same method twice": `{"Example.get": {"result": 1}, "EXAMPLE.get": {"result": 2}}`,
	} {
		if _, err := ParseExtraRPC([]byte(in)); err == nil {
			t.Errorf("%s: accepted %s", name, in)
		}
	}
}

func TestExtraRPC(t *testing.T) {
	s := New(DefaultUsers(), 0)
	extra, err := ParseExtraRPC([]byte(`{
		"Example.getStatus": {"result": {"running": true}},
		"Example.getSecret": {"result": "s", "admin": true},
		"System.getInformation": {"result": "never"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	s.SetExtraRPC(extra)
	if got := s.ExtraRPCNames(); strings.Join(got, ",") != "example.getSecret,example.getStatus,system.getInformation" {
		t.Errorf("names = %v", got)
	}

	// 세션이 없으면 실제 OMV처럼 401이다.
	if r := call(t, s, "", "Example", "getStatus", nil); r.status != http.StatusUnauthorized {
		t.Errorf("without session: %d %s", r.status, r.err)
	}
	user := login(t, s, "user")
	// 서비스 이름은 대소문자를 가리지 않는다.
	if r := call(t, s, user, "example", "getStatus", map[string]any{"ignored": 1}); r.status != http.StatusOK || string(r.result) != `{"running":true}` {
		t.Errorf("getStatus: %d %s %s", r.status, r.result, r.err)
	}
	if r := call(t, s, user, "Example", "getSecret", nil); r.status != http.StatusForbidden || r.err != "Invalid context role." {
		t.Errorf("admin-only as user: %d %s", r.status, r.err)
	}
	// 메서드 이름은 그대로 비교한다.
	if r := call(t, s, user, "Example", "getstatus", nil); r.status != http.StatusNotFound {
		t.Errorf("method case: %d %s", r.status, r.result)
	}
	admin := login(t, s, "admin")
	if r := call(t, s, admin, "Example", "getSecret", nil); r.status != http.StatusOK || string(r.result) != `"s"` {
		t.Errorf("admin-only as admin: %d %s %s", r.status, r.result, r.err)
	}
	// 가짜 OMV가 구현한 메서드는 추가 메서드보다 먼저다.
	if r := call(t, s, admin, "System", "getInformation", nil); r.status != http.StatusOK || string(r.result) == `"never"` {
		t.Errorf("built-in overridden: %d %s", r.status, r.result)
	}
	// 없는 메서드는 그대로 404다.
	if r := call(t, s, admin, "Example", "getOther", nil); r.status != http.StatusNotFound {
		t.Errorf("unknown method: %d %s", r.status, r.result)
	}
}

// TestPerfStats는 SDK 예제 hello-backend가 쓰는 성능 통계 설정(engined/rpc/perfstats.inc)의 가짜 응답을 본다.
func TestPerfStats(t *testing.T) {
	s := New(DefaultUsers(), 0)
	user := login(t, s, "user")
	if r := call(t, s, user, "PerfStats", "get", nil); r.status != http.StatusForbidden {
		t.Errorf("get as user: %d %s", r.status, r.result)
	}
	admin := login(t, s, "admin")
	if r := call(t, s, admin, "PerfStats", "get", nil); r.status != http.StatusOK || string(r.result) != `{"enable":true}` {
		t.Fatalf("get: %d %s %s", r.status, r.result, r.err)
	}
	// rpc.perfstats.set은 enable(불) 하나다.
	for _, bad := range []any{map[string]any{}, map[string]any{"enable": "no"}, map[string]any{"enable": false, "extra": 1}} {
		if r := call(t, s, admin, "PerfStats", "set", bad); r.status == http.StatusOK {
			t.Errorf("set %v accepted", bad)
		}
	}
	if r := call(t, s, admin, "PerfStats", "set", map[string]any{"enable": false}); r.status != http.StatusOK {
		t.Fatalf("set: %d %s", r.status, r.err)
	}
	if r := call(t, s, admin, "Config", "isDirty", nil); string(r.result) != "true" {
		t.Errorf("isDirty after set = %s", r.result)
	}
	if r := call(t, s, admin, "PerfStats", "get", nil); string(r.result) != `{"enable":false}` {
		t.Errorf("get after set = %s", r.result)
	}
	r := call(t, s, admin, "Config", "applyChangesBg", map[string]any{"modules": []string{}, "force": false})
	var file string
	if json.Unmarshal(r.result, &file) != nil || file == "" {
		t.Fatalf("applyChangesBg: %d %s", r.status, r.result)
	}
	for i := 0; i < 10; i++ {
		var st struct{ Running bool }
		if json.Unmarshal(call(t, s, admin, "Exec", "isRunning", map[string]string{"filename": file}).result, &st) != nil || !st.Running {
			break
		}
	}
	if r := call(t, s, admin, "Config", "isDirty", nil); string(r.result) != "false" {
		t.Errorf("isDirty after apply = %s", r.result)
	}
}
