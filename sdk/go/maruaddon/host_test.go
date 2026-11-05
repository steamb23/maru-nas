// SPDX-License-Identifier: MIT

package maruaddon

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// 백엔드가 받은 요청의 호출 토큰으로 호스트를 부르는지, 요청 모양이 계약과 같은지 본다.
func TestHostRPCThroughRequest(t *testing.T) {
	fh := startFakeHost(t, func(path, body string) (int, string) {
		return 200, `{"result":{"hostname":"nas","extra":[1]}}`
	})
	ts := startServer(t, Handler{Data: func(ctx context.Context, r *Request) (any, error) {
		return r.Host.RPC(ctx, "System", "getInformation", nil)
	}}, fh.path)
	resp, body := ts.do(t, "GET", "/v1/data", nil, "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}
	jsonEqual(t, body, `{"hostname":"nas","extra":[1]}`)
	reqs := fh.requests()
	if len(reqs) != 1 {
		t.Fatalf("host got %d requests", len(reqs))
	}
	r := reqs[0]
	if r.Method != "POST" || r.Path != "/v1/rpc" || r.Host != "maru" || r.ContentType != "application/json" {
		t.Errorf("host request = %+v", r)
	}
	if r.Token != testToken {
		t.Error("host request does not carry the call token")
	}
	jsonEqual(t, r.Body, `{"service":"System","method":"getInformation","params":null}`)
}

func TestHostRPC(t *testing.T) {
	reply := `{"result":[{"uuid":"u1"}]}`
	fh := startFakeHost(t, func(path, body string) (int, string) { return 200, reply })
	h := fh.host("tok")
	raw, err := h.RPC(context.Background(), "ShareMgmt", "enumerateSharedFolders", map[string]any{"start": 0})
	if err != nil {
		t.Fatal(err)
	}
	jsonEqual(t, string(raw), `[{"uuid":"u1"}]`)
	jsonEqual(t, fh.requests()[0].Body, `{"service":"ShareMgmt","method":"enumerateSharedFolders","params":{"start":0}}`)

	// null 결과는 null 그대로, result가 없으면 오류다
	reply = `{"result":null}`
	raw, err = h.RPC(context.Background(), "S", "m", nil)
	if err != nil || string(raw) != "null" {
		t.Errorf("null result: %q, %v", raw, err)
	}
	reply = `{}`
	if _, err := h.RPC(context.Background(), "S", "m", nil); err == nil {
		t.Error("missing result: no error")
	}
	reply = `not json`
	if _, err := h.RPC(context.Background(), "S", "m", nil); err == nil {
		t.Error("invalid JSON: no error")
	}
	// 이미 인코딩한 params도 그대로 보낸다
	reply = `{"result":1}`
	if _, err := h.RPC(context.Background(), "S", "m", json.RawMessage(`{"a":[true]}`)); err != nil {
		t.Fatal(err)
	}
	reqs := fh.requests()
	jsonEqual(t, reqs[len(reqs)-1].Body, `{"service":"S","method":"m","params":{"a":[true]}}`)
}

func TestHostRPCInto(t *testing.T) {
	fh := startFakeHost(t, func(path, body string) (int, string) {
		return 200, `{"result":{"enable":true,"workgroup":"WORKGROUP","unknown":"ignored"}}`
	})
	h := fh.host("tok")
	var s struct {
		Enable    bool   `json:"enable"`
		Workgroup string `json:"workgroup"`
	}
	if err := h.RPCInto(context.Background(), "Smb", "getSettings", nil, &s); err != nil {
		t.Fatal(err)
	}
	if !s.Enable || s.Workgroup != "WORKGROUP" {
		t.Errorf("decoded %+v", s)
	}
	if err := h.RPCInto(context.Background(), "Smb", "getSettings", nil, nil); err != nil {
		t.Errorf("nil out: %v", err)
	}
	var wrong []int
	err := h.RPCInto(context.Background(), "Smb", "getSettings", nil, &wrong)
	if err == nil || !strings.Contains(err.Error(), "Smb.getSettings") {
		t.Errorf("type mismatch: %v", err)
	}
}

func TestHostRPCBg(t *testing.T) {
	reply := `{"output":"line 1\nline 2\n","truncated":true}`
	fh := startFakeHost(t, func(path, body string) (int, string) { return 200, reply })
	h := fh.host("tok")
	out, truncated, err := h.RPCBg(context.Background(), "Smart", "doSelfTest", map[string]string{"devicefile": "/dev/sda"}, true)
	if err != nil || out != "line 1\nline 2\n" || !truncated {
		t.Fatalf("RPCBg = %q, %v, %v", out, truncated, err)
	}
	r := fh.requests()[0]
	if r.Path != "/v1/rpc-bg" || r.Token != "tok" {
		t.Errorf("host request = %+v", r)
	}
	jsonEqual(t, r.Body, `{"service":"Smart","method":"doSelfTest","params":{"devicefile":"/dev/sda"},"output":true}`)

	reply = `{"output":"","truncated":false}`
	if _, _, err := h.RPCBg(context.Background(), "S", "m", nil, false); err != nil {
		t.Fatal(err)
	}
	reqs := fh.requests()
	jsonEqual(t, reqs[len(reqs)-1].Body, `{"service":"S","method":"m","params":null,"output":false}`)

	reply = `{"truncated":false}`
	if _, _, err := h.RPCBg(context.Background(), "S", "m", nil, false); err == nil {
		t.Error("missing output: no error")
	}
}

func TestHostApply(t *testing.T) {
	reply := `{"results":[{"uuid":"a"},null]}`
	fh := startFakeHost(t, func(path, body string) (int, string) { return 200, reply })
	h := fh.host("tok")
	results, err := h.Apply(context.Background(),
		Call{Service: "Smb", Method: "setSettings", Params: map[string]any{"enable": true}},
		Call{Service: "Smb", Method: "setShare"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || string(results[1]) != "null" {
		t.Fatalf("results = %q", results)
	}
	jsonEqual(t, string(results[0]), `{"uuid":"a"}`)
	r := fh.requests()[0]
	if r.Path != "/v1/apply" {
		t.Errorf("path = %s", r.Path)
	}
	jsonEqual(t, r.Body, `{"calls":[{"service":"Smb","method":"setSettings","params":{"enable":true}},`+
		`{"service":"Smb","method":"setShare","params":null}]}`)

	reply = `{"results":[1]}`
	if _, err := h.Apply(context.Background(), Call{Service: "S", Method: "a"}, Call{Service: "S", Method: "b"}); err == nil {
		t.Error("result count mismatch: no error")
	}
}

func TestHostLocalLimits(t *testing.T) {
	fh := startFakeHost(t, func(path, body string) (int, string) { return 200, `{"result":1,"results":[]}` })
	h := fh.host("tok")
	ctx := context.Background()
	if _, err := h.Apply(ctx); err == nil {
		t.Error("Apply with no calls: no error")
	}
	calls := make([]Call, MaxApplyCalls+1)
	for i := range calls {
		calls[i] = Call{Service: "S", Method: fmt.Sprint("m", i)}
	}
	if _, err := h.Apply(ctx, calls...); err == nil {
		t.Error("Apply with too many calls: no error")
	}
	big := strings.Repeat("x", maxHostRequestBytes)
	if _, err := h.RPC(ctx, "S", "m", map[string]string{"v": big}); err == nil || !strings.Contains(err.Error(), "64 KiB") {
		t.Errorf("large request: %v", err)
	}
	if _, err := h.RPC(ctx, "S", "m", make(chan int)); err == nil {
		t.Error("unencodable params: no error")
	}
	if n := len(fh.requests()); n != 0 {
		t.Errorf("host got %d requests, want 0", n)
	}
}

func TestHostErrors(t *testing.T) {
	status, reply := 403, `{"error":"method not declared","code":"rpc_forbidden"}`
	fh := startFakeHost(t, func(path, body string) (int, string) { return status, reply })
	h := fh.host("tok")
	ctx := context.Background()

	_, err := h.RPC(ctx, "Smb", "setSettings", nil)
	var he *HostError
	if !errors.As(err, &he) || he.Status != 403 || he.Code != "rpc_forbidden" || he.Message != "method not declared" {
		t.Fatalf("err = %#v", err)
	}
	if !IsCode(err, "rpc_forbidden") || !IsCode(fmt.Errorf("wrapped: %w", err), "rpc_forbidden") {
		t.Error("IsCode does not match")
	}
	if IsCode(err, "busy") || IsCode(errBoom, "rpc_forbidden") || IsCode(nil, "") {
		t.Error("IsCode matches the wrong error")
	}
	if !strings.Contains(err.Error(), "403 rpc_forbidden: method not declared") {
		t.Errorf("Error() = %q", err.Error())
	}

	for _, p := range []func() error{
		func() error { return h.RPCInto(ctx, "S", "m", nil, nil) },
		func() error { _, _, err := h.RPCBg(ctx, "S", "m", nil, true); return err },
		func() error { _, err := h.Apply(ctx, Call{Service: "S", Method: "m"}); return err },
	} {
		status, reply = 409, `{"error":"pending changes","code":"config_dirty"}`
		if err := p(); !IsCode(err, "config_dirty") {
			t.Errorf("err = %v, want config_dirty", err)
		}
	}

	// JSON이 아닌 오류 본문은 상태 문구로 둔다
	status, reply = 502, `<html>bad gateway</html>`
	_, err = h.RPC(ctx, "S", "m", nil)
	if !errors.As(err, &he) || he.Status != 502 || he.Code != "" || he.Message != "Bad Gateway" {
		t.Errorf("err = %#v", err)
	}
}

func TestHostToken(t *testing.T) {
	fh := startFakeHost(t, func(path, body string) (int, string) { return 200, `{"result":null}` })
	if _, err := fh.host("").RPC(context.Background(), "S", "m", nil); err != nil {
		t.Fatal(err)
	}
	if r := fh.requests()[0]; r.HasToken {
		t.Errorf("empty token sent as a header: %+v", r)
	}
	// 토큰은 오류 문구에 넣지 않는다
	gone := &Host{c: newHostClient(filepath.Join(shortTempDir(t), "missing.sock")), token: testToken}
	_, err := gone.RPC(context.Background(), "S", "m", nil)
	var he *HostError
	if err == nil || errors.As(err, &he) || strings.Contains(err.Error(), testToken) {
		t.Errorf("unreachable host: %v", err)
	}
}

// TestHostFormatRedacted는 Host를 어떻게 출력해도(포인터·값, Request의 필드, 모든 동사) 토큰이 나오지 않는지 본다.
// 패키지 코드가 log.Printf("%v", r.Host) 같은 줄로 토큰을 journal에 남기지 않게 한다.
func TestHostFormatRedacted(t *testing.T) {
	h := &Host{c: newHostClient("/nonexistent/host.sock"), token: testToken}
	r := &Request{User: "user", Host: h}
	// %x·%X는 문자열을 16진수로 출력하므로 그 꼴도 찾는다
	leaks := []string{testToken, hex.EncodeToString([]byte(testToken)), strings.ToUpper(hex.EncodeToString([]byte(testToken)))}
	verbs := []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%t", "%10.3v"}
	for _, verb := range verbs {
		for _, arg := range []any{h, *h, r, *r} {
			got := fmt.Sprintf(verb, arg)
			for _, leak := range leaks {
				if strings.Contains(got, leak) {
					t.Errorf("Sprintf(%q, %T) leaks the token: %s", verb, arg, got)
				}
			}
		}
	}
	if got := h.String(); got != redactedHost {
		t.Errorf("String() = %q", got)
	}
	if got := fmt.Sprint(h); got != redactedHost {
		t.Errorf("Sprint(*Host) = %q", got)
	}
	// nil *Host는 panic 없이 출력된다
	if got := fmt.Sprint((*Host)(nil)); got != "<nil>" {
		t.Errorf("Sprint(nil *Host) = %q", got)
	}
}

func TestHostNil(t *testing.T) {
	var h *Host
	if _, err := h.RPC(context.Background(), "S", "m", nil); err == nil {
		t.Error("nil Host: no error")
	}
	if _, err := (&Host{}).Apply(context.Background(), Call{}); err == nil {
		t.Error("zero Host: no error")
	}
}

func TestHostContextCanceled(t *testing.T) {
	fh := startFakeHost(t, func(path, body string) (int, string) { return 200, `{"result":1}` })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fh.host("tok").RPC(ctx, "S", "m", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestHostSocketPath(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if p := hostSocketPath(getenv); p != DefaultHostSocket || DefaultHostSocket != "/run/maru-host/host.sock" {
		t.Errorf("default = %q", p)
	}
	env["MARU_ADDON_HOST"] = "/srv/dev/addon-host/x.sock"
	if p := hostSocketPath(getenv); p != "/srv/dev/addon-host/x.sock" {
		t.Errorf("MARU_ADDON_HOST = %q", p)
	}
}

// ServeContext가 MARU_ADDON_HOST를 읽어 그 소켓을 쓰는지 본다.
func TestServeContextHostEnv(t *testing.T) {
	fh := startFakeHost(t, func(path, body string) (int, string) { return 200, `{"result":"from env"}` })
	t.Setenv("MARU_ADDON_HOST", fh.path)
	h := Handler{Data: func(ctx context.Context, r *Request) (any, error) {
		return r.Host.RPC(ctx, "S", "m", nil)
	}}
	path := filepath.Join(shortTempDir(t), "b.sock")
	l, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ServeContext(ctx, l, h) }()
	ts := &testServer{path: path, client: unixClient(path)}
	resp, body := ts.do(t, "GET", "/v1/data", nil, "")
	ts.client.CloseIdleConnections()
	cancel()
	if err := <-done; err != nil {
		t.Error(err)
	}
	if resp.StatusCode != 200 || body != `"from env"` {
		t.Errorf("status = %d, body %s", resp.StatusCode, body)
	}
}
