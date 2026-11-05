// SPDX-License-Identifier: MIT

package maruaddon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDataRequest(t *testing.T) {
	var got *Request
	ts := startServer(t, Handler{Data: func(ctx context.Context, r *Request) (any, error) {
		got = r
		return map[string]any{"n": 1, "user": r.User}, nil
	}}, "")
	resp, body := ts.do(t, "GET", "/v1/data?a=1&b=x&b=y", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	jsonEqual(t, body, `{"n":1,"user":"alice"}`)
	if got.User != "alice" || got.Admin || got.Lang != "ko" {
		t.Errorf("request = %+v", got)
	}
	if want := (url.Values{"a": {"1"}, "b": {"x", "y"}}); !reflect.DeepEqual(got.Query, want) {
		t.Errorf("Query = %v, want %v", got.Query, want)
	}
	if got.Action != "" || got.Target != "" || got.Input != nil {
		t.Errorf("data request has action fields: %+v", got)
	}
	if got.Host == nil || got.Host.token != testToken {
		t.Error("Host does not carry the call token")
	}
}

func TestRequestHeaders(t *testing.T) {
	var got *Request
	ts := startServer(t, Handler{Data: func(ctx context.Context, r *Request) (any, error) {
		got = r
		return nil, nil
	}}, "")
	cases := []struct {
		admin, lang string
		wantAdmin   bool
		wantLang    string
	}{
		{"1", "ko", true, "ko"},
		{"0", "en", false, "en"},
		// "1"만 관리자다
		{"true", "", false, "en"},
		{"", "de", false, "de"},
	}
	for _, c := range cases {
		h := defaultHeaders()
		h.Set("X-Maru-Admin", c.admin)
		h.Set("X-Maru-Lang", c.lang)
		resp, body := ts.do(t, "GET", "/v1/data", h, "")
		if resp.StatusCode != http.StatusOK || body != "null" {
			t.Fatalf("status = %d, body %q", resp.StatusCode, body)
		}
		if got.Admin != c.wantAdmin || got.Lang != c.wantLang {
			t.Errorf("admin %q lang %q: Admin = %v, Lang = %q", c.admin, c.lang, got.Admin, got.Lang)
		}
		if len(got.Query) != 0 || got.Query == nil {
			t.Errorf("Query = %#v, want empty", got.Query)
		}
	}
}

func TestMissingUser(t *testing.T) {
	called := false
	ts := startServer(t, Handler{Data: func(ctx context.Context, r *Request) (any, error) {
		called = true
		return nil, nil
	}}, "")
	h := defaultHeaders()
	h.Del("X-Maru-User")
	resp, body := ts.do(t, "GET", "/v1/data", h, "")
	wantError(t, resp, body, http.StatusBadRequest, "bad_request")
	if called {
		t.Error("handler ran without a user")
	}
}

func TestInvalidQuery(t *testing.T) {
	ts := startServer(t, Handler{Data: func(ctx context.Context, r *Request) (any, error) { return 1, nil }}, "")
	resp, body := ts.do(t, "GET", "/v1/data?a=%zz", nil, "")
	wantError(t, resp, body, http.StatusBadRequest, "bad_request")
}

func TestNoData(t *testing.T) {
	ts := startServer(t, Handler{Actions: map[string]HandlerFunc{
		"run": func(ctx context.Context, r *Request) (any, error) { return nil, nil },
	}}, "")
	resp, body := ts.do(t, "GET", "/v1/data", nil, "")
	wantError(t, resp, body, http.StatusNotFound, "no_data")
}

func TestActionResults(t *testing.T) {
	var got *Request
	type result struct {
		OK bool `json:"ok"`
	}
	ts := startServer(t, Handler{Actions: map[string]HandlerFunc{
		"none": func(ctx context.Context, r *Request) (any, error) {
			got = r
			return nil, nil
		},
		"value": func(ctx context.Context, r *Request) (any, error) { return result{OK: true}, nil },
		// 타입이 있는 nil은 값이라 204가 아니라 null이다
		"typed-nil": func(ctx context.Context, r *Request) (any, error) { return (*result)(nil), nil },
	}}, "")

	resp, body := ts.do(t, "POST", "/v1/actions/none", nil, `{"target":"disk-1","input":{"a":1}}`)
	if resp.StatusCode != http.StatusNoContent || body != "" {
		t.Fatalf("none: status = %d, body %q", resp.StatusCode, body)
	}
	if got.Action != "none" || got.Target != "disk-1" || string(got.Input) != `{"a":1}` || got.User != "alice" {
		t.Errorf("request = %+v, input %s", got, got.Input)
	}
	if got.Query != nil {
		t.Errorf("action Query = %v, want nil", got.Query)
	}

	resp, body = ts.do(t, "POST", "/v1/actions/value", nil, `{"target":"","input":null}`)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("value: status = %d, body %q", resp.StatusCode, body)
	}
	jsonEqual(t, body, `{"ok":true}`)

	resp, body = ts.do(t, "POST", "/v1/actions/typed-nil", nil, `{}`)
	if resp.StatusCode != http.StatusOK || body != "null" {
		t.Fatalf("typed-nil: status = %d, body %q", resp.StatusCode, body)
	}
}

func TestActionBody(t *testing.T) {
	var got *Request
	ts := startServer(t, Handler{Actions: map[string]HandlerFunc{
		"run": func(ctx context.Context, r *Request) (any, error) {
			got = r
			return nil, nil
		},
	}}, "")
	cases := []struct {
		name       string
		body       string
		wantStatus int
		wantTarget string
		wantInput  string // "" = nil
	}{
		{"full", `{"target":"t","input":{"x":[1,2]}}`, 204, "t", `{"x":[1,2]}`},
		{"input null", `{"target":"t","input":null}`, 204, "t", ""},
		{"input absent", `{"target":"t"}`, 204, "t", ""},
		{"empty object", `{}`, 204, "", ""},
		{"spaces", " \n{ \"input\" :  { } }\n ", 204, "", `{ }`},
		// maru가 나중에 더하는 봉투 필드는 무시한다
		{"unknown envelope field", `{"target":"t","input":null,"future":true}`, 204, "t", ""},
		{"input array", `{"input":[1]}`, 400, "", ""},
		{"input string", `{"input":"x"}`, 400, "", ""},
		{"input number", `{"input":1}`, 400, "", ""},
		{"target number", `{"target":1}`, 400, "", ""},
		{"not an object", `[1]`, 400, "", ""},
		{"null body", `null`, 400, "", ""},
		{"invalid JSON", `{"target":`, 400, "", ""},
		{"trailing data", `{"target":"a"}{"target":"b"}`, 400, "", ""},
		{"trailing garbage", `{"target":"a"} x`, 400, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got = nil
			resp, body := ts.do(t, "POST", "/v1/actions/run", nil, c.body)
			if c.wantStatus != http.StatusNoContent {
				wantError(t, resp, body, c.wantStatus, "bad_request")
				if got != nil {
					t.Error("handler ran")
				}
				return
			}
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, body %s", resp.StatusCode, body)
			}
			if got.Target != c.wantTarget || string(got.Input) != c.wantInput {
				t.Errorf("target %q input %q, want %q %q", got.Target, got.Input, c.wantTarget, c.wantInput)
			}
			if c.wantInput == "" && got.Input != nil {
				t.Errorf("Input = %#v, want nil", got.Input)
			}
		})
	}
}

func TestActionContentType(t *testing.T) {
	ts := startServer(t, Handler{Actions: map[string]HandlerFunc{
		"run": func(ctx context.Context, r *Request) (any, error) { return nil, nil },
	}}, "")
	for ct, want := range map[string]int{
		"application/json":                  204,
		"application/json; charset=utf-8":   204,
		"text/plain":                        400,
		"application/x-www-form-urlencoded": 400,
	} {
		h := defaultHeaders()
		h.Set("Content-Type", ct)
		resp, body := ts.do(t, "POST", "/v1/actions/run", h, `{}`)
		if resp.StatusCode != want {
			t.Errorf("Content-Type %q: status = %d, want %d (body %s)", ct, resp.StatusCode, want, body)
		}
	}
	// 본문 없음
	resp, body := ts.do(t, "POST", "/v1/actions/run", nil, "")
	wantError(t, resp, body, http.StatusBadRequest, "bad_request")
}

func TestActionBodyLimit(t *testing.T) {
	var gotLen int
	ts := startServer(t, Handler{Actions: map[string]HandlerFunc{
		"run": func(ctx context.Context, r *Request) (any, error) {
			gotLen = len(r.Target)
			return nil, nil
		},
	}}, "")
	envelope := len(`{"target":""}`)
	fits := `{"target":"` + strings.Repeat("a", maxRequestBytes-envelope) + `"}`
	resp, body := ts.do(t, "POST", "/v1/actions/run", nil, fits)
	if resp.StatusCode != http.StatusNoContent || gotLen != maxRequestBytes-envelope {
		t.Fatalf("64 KiB body: status = %d, body %s, target %d bytes", resp.StatusCode, body, gotLen)
	}
	tooLarge := `{"target":"` + strings.Repeat("a", maxRequestBytes-envelope+1) + `"}`
	resp, body = ts.do(t, "POST", "/v1/actions/run", nil, tooLarge)
	wantError(t, resp, body, http.StatusBadRequest, "bad_request")
	if !strings.Contains(body, "64 KiB") {
		t.Errorf("body = %s", body)
	}
}

func TestUnknownAction(t *testing.T) {
	ts := startServer(t, Handler{Actions: map[string]HandlerFunc{
		"run": func(ctx context.Context, r *Request) (any, error) { return nil, nil },
	}}, "")
	resp, body := ts.do(t, "POST", "/v1/actions/walk", nil, `{}`)
	wantError(t, resp, body, http.StatusNotFound, "unknown_action")
}

func TestMethodNotAllowed(t *testing.T) {
	ts := startServer(t, Handler{
		Data: func(ctx context.Context, r *Request) (any, error) { return 1, nil },
		Actions: map[string]HandlerFunc{
			"run": func(ctx context.Context, r *Request) (any, error) { return nil, nil },
		},
	}, "")
	resp, body := ts.do(t, "POST", "/v1/data", nil, `{}`)
	wantError(t, resp, body, http.StatusMethodNotAllowed, "method_not_allowed")
	if a := resp.Header.Get("Allow"); a != "GET" {
		t.Errorf("Allow = %q", a)
	}
	resp, body = ts.do(t, "GET", "/v1/actions/run", nil, "")
	wantError(t, resp, body, http.StatusMethodNotAllowed, "method_not_allowed")
	if a := resp.Header.Get("Allow"); a != "POST" {
		t.Errorf("Allow = %q", a)
	}
}

func TestUnknownPath(t *testing.T) {
	ts := startServer(t, Handler{Data: func(ctx context.Context, r *Request) (any, error) { return 1, nil }}, "")
	for _, p := range []string{"/", "/v2/data", "/v1/data/", "/v1/actions", "/v1/actions/a/b"} {
		resp, body := ts.do(t, "GET", p, nil, "")
		wantError(t, resp, body, http.StatusNotFound, "not_found")
	}
}

func TestErrorMapping(t *testing.T) {
	code32 := "a" + strings.Repeat("b", 31)
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"400", NewError(400, "bad_name", "name is invalid"), 400, "bad_name", "name is invalid"},
		{"404", NewError(404, "no_such_disk", "m"), 404, "no_such_disk", "m"},
		{"409", NewError(409, "already_running", "m"), 409, "already_running", "m"},
		{"422", NewError(422, "x1_2", "m"), 422, "x1_2", "m"},
		{"429", NewError(429, "slow_down", "m"), 429, "slow_down", "m"},
		{"32-char code", NewError(400, code32, "m"), 400, code32, "m"},
		{"wrapped", fmt.Errorf("check: %w", NewError(409, "conflict", "m")), 409, "conflict", "m"},
		{"value literal", &Error{Status: 422, Code: "bad", Message: ""}, 422, "bad", ""},
		// maru가 그대로 넘기지 않는 상태·코드는 500으로 답하고 기록한다
		{"401", NewError(401, "login", "m"), 500, "internal", "internal error"},
		{"403", NewError(403, "forbidden", "m"), 500, "internal", "internal error"},
		{"500", NewError(500, "oops", "m"), 500, "internal", "internal error"},
		{"200", NewError(200, "ok", "m"), 500, "internal", "internal error"},
		{"upper-case code", NewError(400, "Bad", "m"), 500, "internal", "internal error"},
		{"empty code", NewError(400, "", "m"), 500, "internal", "internal error"},
		{"33-char code", NewError(400, code32+"c", "m"), 500, "internal", "internal error"},
		{"code with dash", NewError(400, "a-b", "m"), 500, "internal", "internal error"},
		{"nil *Error", (*Error)(nil), 500, "internal", "internal error"},
		{"host error", &HostError{Status: 409, Code: "config_dirty", Message: "m"}, 500, "internal", "internal error"},
		{"plain", errBoom, 500, "internal", "internal error"},
	}
	byName := map[string]error{}
	for _, c := range cases {
		byName[c.name] = c.err
	}
	ts := startServer(t, Handler{
		Data: func(ctx context.Context, r *Request) (any, error) { return "unused", byName[r.Query.Get("c")] },
		Actions: map[string]HandlerFunc{
			"run": func(ctx context.Context, r *Request) (any, error) { return nil, byName[r.Target] },
		},
	}, "")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check := func(what string, resp *http.Response, body string) {
				t.Helper()
				if resp.StatusCode != c.wantStatus {
					t.Fatalf("%s: status = %d, want %d (body %s)", what, resp.StatusCode, c.wantStatus, body)
				}
				var e map[string]string
				if err := json.Unmarshal([]byte(body), &e); err != nil {
					t.Fatal(err)
				}
				if want := map[string]string{"code": c.wantCode, "message": c.wantMsg}; !reflect.DeepEqual(e, want) {
					t.Errorf("%s: body = %v, want %v", what, e, want)
				}
			}
			resp, body := ts.do(t, "GET", "/v1/data?c="+url.QueryEscape(c.name), nil, "")
			check("data", resp, body)
			envelope, _ := json.Marshal(map[string]string{"target": c.name})
			resp, body = ts.do(t, "POST", "/v1/actions/run", nil, string(envelope))
			check("action", resp, body)
		})
	}
}

func TestInternalErrorLog(t *testing.T) {
	ts := startServer(t, Handler{
		Data: func(ctx context.Context, r *Request) (any, error) { return nil, errBoom },
		Actions: map[string]HandlerFunc{
			"run": func(ctx context.Context, r *Request) (any, error) {
				return nil, NewError(403, "nope", "forbidden")
			},
		},
	}, "")
	resp, body := ts.do(t, "GET", "/v1/data?x=1", nil, "")
	if resp.StatusCode != 500 || body != `{"code":"internal","message":"internal error"}` {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}
	ts.do(t, "POST", "/v1/actions/run", nil, `{"target":"t"}`)
	logs := ts.logs.String()
	for _, want := range []string{`data (user "alice"): ` + errBoom.Error(), `action "run" (user "alice")`,
		"status 403", `"nope"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("log does not contain %q:\n%s", want, logs)
		}
	}
	if strings.Contains(logs, testToken) {
		t.Errorf("log contains the call token:\n%s", logs)
	}
	if strings.Contains(body, "boom") {
		t.Errorf("response leaks the error: %s", body)
	}
}

func TestPanicRecovered(t *testing.T) {
	ts := startServer(t, Handler{
		Data: func(ctx context.Context, r *Request) (any, error) {
			if r.Query.Get("panic") != "" {
				panic("handler bug")
			}
			return "fine", nil
		},
	}, "")
	resp, body := ts.do(t, "GET", "/v1/data?panic=1", nil, "")
	wantError(t, resp, body, http.StatusInternalServerError, "internal")
	if logs := ts.logs.String(); !strings.Contains(logs, "panic: handler bug") || strings.Contains(logs, testToken) {
		t.Errorf("log:\n%s", logs)
	}
	// 같은 서버가 계속 답한다
	resp, body = ts.do(t, "GET", "/v1/data", nil, "")
	if resp.StatusCode != 200 || body != `"fine"` {
		t.Errorf("after panic: status = %d, body %s", resp.StatusCode, body)
	}
}

func TestResponseLimits(t *testing.T) {
	ts := startServer(t, Handler{
		Data: func(ctx context.Context, r *Request) (any, error) {
			switch r.Query.Get("c") {
			case "fits":
				// 따옴표 2바이트를 더해 4MiB가 된다
				return strings.Repeat("a", maxResponseBytes-2), nil
			case "large":
				return strings.Repeat("a", maxResponseBytes-1), nil
			case "unencodable":
				return make(chan int), nil
			}
			return nil, nil
		},
	}, "")
	resp, body := ts.do(t, "GET", "/v1/data?c=fits", nil, "")
	if resp.StatusCode != 200 || len(body) != maxResponseBytes {
		t.Errorf("4 MiB: status = %d, %d bytes", resp.StatusCode, len(body))
	}
	resp, body = ts.do(t, "GET", "/v1/data?c=large", nil, "")
	wantError(t, resp, body, http.StatusInternalServerError, "internal")
	resp, body = ts.do(t, "GET", "/v1/data?c=unencodable", nil, "")
	wantError(t, resp, body, http.StatusInternalServerError, "internal")
	logs := ts.logs.String()
	if !strings.Contains(logs, "larger than the 4 MiB") || !strings.Contains(logs, "encode result") {
		t.Errorf("log:\n%s", logs)
	}
}

func TestDecodeInput(t *testing.T) {
	type in struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	r := &Request{Input: []byte(`{"name":"a","n":2}`)}
	var v in
	if err := r.DecodeInput(&v); err != nil || v != (in{"a", 2}) {
		t.Fatalf("DecodeInput = %v, %+v", err, v)
	}

	// 입력이 없으면 v를 그대로 둔다
	v = in{Name: "keep"}
	if err := (&Request{}).DecodeInput(&v); err != nil || v.Name != "keep" {
		t.Errorf("no input: %v, %+v", err, v)
	}

	for _, input := range []string{`{"name":"a","extra":1}`, `{"n":"x"}`} {
		err := (&Request{Input: []byte(input)}).DecodeInput(&v)
		var e *Error
		if !errors.As(err, &e) || e.Status != 400 || e.Code != "bad_input" || !e.valid() {
			t.Errorf("input %s: err = %v, want 400 bad_input", input, err)
		}
	}

	// 포인터가 아닌 대상은 처리기의 잘못이므로 400으로 바꾸지 않는다
	err := r.DecodeInput(v)
	var e *Error
	if err == nil || errors.As(err, &e) {
		t.Errorf("non-pointer: err = %v", err)
	}
}

func TestHandlerCheck(t *testing.T) {
	noop := func(ctx context.Context, r *Request) (any, error) { return nil, nil }
	for name, h := range map[string]Handler{
		"empty":          {},
		"empty actions":  {Actions: map[string]HandlerFunc{}},
		"nil action":     {Actions: map[string]HandlerFunc{"run": nil}},
		"unnamed action": {Data: noop, Actions: map[string]HandlerFunc{"": noop}},
	} {
		if err := h.check(); err == nil {
			t.Errorf("%s: check passed", name)
		}
		l, err := net.Listen("unix", filepath.Join(shortTempDir(t), "s.sock"))
		if err != nil {
			t.Fatal(err)
		}
		if err := ServeContext(context.Background(), l, h); err == nil {
			t.Errorf("%s: ServeContext returned nil", name)
		}
		// ServeContext가 수신 소켓을 닫았다
		if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
			t.Errorf("%s: listener still open: %v", name, err)
		}
	}
	if err := (Handler{Data: noop}).check(); err != nil {
		t.Error(err)
	}
	if err := (Handler{Actions: map[string]HandlerFunc{"run": noop}}).check(); err != nil {
		t.Error(err)
	}
}

func TestServeClosedListener(t *testing.T) {
	l, err := net.Listen("unix", filepath.Join(shortTempDir(t), "s.sock"))
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	h := Handler{Data: func(ctx context.Context, r *Request) (any, error) { return nil, nil }}
	if err := ServeContext(context.Background(), l, h); err == nil {
		t.Error("ServeContext on a closed listener returned nil")
	}
}

func TestGracefulShutdown(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := Handler{
		Data: func(ctx context.Context, r *Request) (any, error) {
			close(started)
			<-release
			return "done", nil
		},
		ErrorLog: log.New(&syncBuffer{}, "", 0),
	}
	path := filepath.Join(shortTempDir(t), "b.sock")
	l, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, l, h, "", 10*time.Second) }()

	type result struct {
		status int
		body   string
		err    error
	}
	respc := make(chan result, 1)
	go func() {
		req, _ := http.NewRequest("GET", "http://addon/v1/data", nil)
		req.Header = defaultHeaders()
		resp, err := unixClient(path).Do(req)
		if err != nil {
			respc <- result{err: err}
			return
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		respc <- result{status: resp.StatusCode, body: string(data), err: err}
	}()
	<-started
	cancel()

	// 진행 중인 요청이 끝날 때까지 돌아오지 않는다
	select {
	case err := <-done:
		t.Fatalf("serve returned with a request in flight: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	// 새 연결은 받지 않는다(수신 소켓을 닫고 소켓 파일을 지운다)
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.Dial("unix", path)
		if err != nil {
			break
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("still accepting connections after shutdown started")
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(release)
	r := <-respc
	if r.err != nil || r.status != 200 || r.body != `"done"` {
		t.Errorf("in-flight request: %+v", r)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("serve = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket file not removed: %v", err)
	}
}

func TestShutdownTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	h := Handler{
		Data: func(ctx context.Context, r *Request) (any, error) {
			close(started)
			<-release
			return nil, nil
		},
		ErrorLog: log.New(&syncBuffer{}, "", 0),
	}
	path := filepath.Join(shortTempDir(t), "b.sock")
	l, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, l, h, "", 100*time.Millisecond) }()
	go func() {
		req, _ := http.NewRequest("GET", "http://addon/v1/data", nil)
		req.Header = defaultHeaders()
		if resp, err := unixClient(path).Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "requests still running") {
			t.Errorf("serve = %v, want a shutdown timeout error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not give up on the in-flight request")
	}
}
