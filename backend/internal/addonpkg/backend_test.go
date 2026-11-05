package addonpkg

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/api"
)

// shortDir는 유닉스 소켓 경로 길이 한도(108바이트) 안에 드는 임시 디렉터리다(t.TempDir는 테스트 이름이 들어가 길다).
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mab")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// fakeBackend는 유닉스 소켓에서 받는 가짜 패키지 백엔드다.
func fakeBackend(t *testing.T, path string, h http.Handler) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
}

func TestTranslate(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		data      bool
		want      int
		code      string
		addonCode string
		message   string
		out       string
	}{
		{name: "json", status: 200, body: ` {"a":1} `, want: 200, out: `{"a":1}`},
		{name: "invalid json", status: 200, body: `{"a":`, want: 502, code: api.ErrAddonError},
		{name: "empty 200", status: 200, body: ``, want: 502, code: api.ErrAddonError},
		{name: "data 204", status: 204, data: true, want: 200, out: `null`},
		{name: "action 204", status: 204, want: 204},
		{name: "conflict", status: 409, body: `{"code":"already_on","message":"it is on"}`, want: 409,
			code: api.ErrAddonError, addonCode: "already_on", message: "it is on"},
		{name: "bad code dropped", status: 422, body: `{"code":"Bad-Code","message":"x"}`, want: 422,
			code: api.ErrAddonError, message: "x"},
		{name: "long code dropped", status: 400, body: `{"code":"` + strings.Repeat("a", 33) + `"}`, want: 400,
			code: api.ErrAddonError, message: defaultBackendMessage},
		{name: "control chars", status: 404, body: `{"code":"gone","message":"a\u0000b\nc\u001b[31m"}`, want: 404,
			code: api.ErrAddonError, addonCode: "gone", message: "abc[31m"},
		{name: "bidi controls", status: 409, body: `{"message":"a\u202eb\u2066c\u200fd"}`, want: 409,
			code: api.ErrAddonError, message: "abcd"},
		{name: "empty object", status: 404, body: `{}`, want: 404, code: api.ErrAddonError, message: defaultBackendMessage},
		{name: "not json", status: 429, body: `slow down`, want: 502, code: api.ErrAddonError},
		{name: "empty 4xx", status: 404, body: ``, want: 502, code: api.ErrAddonError},
		{name: "4xx null", status: 400, body: `null`, want: 502, code: api.ErrAddonError},
		{name: "4xx array", status: 422, body: `[{"code":"x"}]`, want: 502, code: api.ErrAddonError},
		{name: "fake 401", status: 401, body: `{"code":"unauthorized","message":"x"}`, want: 502, code: api.ErrAddonError},
		{name: "403", status: 403, body: `{}`, want: 502, code: api.ErrAddonError},
		{name: "500", status: 500, body: `{"code":"boom"}`, want: 502, code: api.ErrAddonError},
		{name: "redirect", status: 302, want: 502, code: api.ErrAddonError},
		{name: "too large", status: 200, body: `"` + strings.Repeat("x", MaxBackendResponse) + `"`, want: 502, code: api.ErrAddonError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Translate(c.status, []byte(c.body), c.data)
			if r.Status != c.want || r.Code != c.code || r.AddonCode != c.addonCode {
				t.Fatalf("reply = %+v", r)
			}
			if c.message != "" && r.Message != c.message {
				t.Errorf("message = %q, want %q", r.Message, c.message)
			}
			if c.out != "" && string(r.Body) != c.out {
				t.Errorf("body = %s, want %s", r.Body, c.out)
			}
			if r.Code == api.ErrAddonError && r.Message == "" {
				t.Error("empty message")
			}
		})
	}
	long := Translate(400, []byte(`{"message":"`+strings.Repeat("가", 400)+`"}`), false)
	if n := len([]rune(long.Message)); n != maxBackendMessage {
		t.Errorf("message length = %d", n)
	}
}

func TestValidBackendQuery(t *testing.T) {
	for q, want := range map[string]bool{
		"":                               true,
		"a=1&b=%20x":                     true,
		strings.Repeat("a", 2048):        true,
		strings.Repeat("a", 2049):        false,
		"a=1 HTTP/1.1\r\nHost: evil":     false,
		"a=\x7f":                         false,
		"a=\xea\xb0\x80":                 false,
		"refresh=1&processes=1&x=%EA%B0": true,
	} {
		if got := ValidBackendQuery(q); got != want {
			t.Errorf("ValidBackendQuery(%q) = %v", q, got)
		}
	}
}

func TestValidActionInput(t *testing.T) {
	for in, want := range map[string]bool{
		"":          true,
		"null":      true,
		` {"a":1} `: true,
		`[1]`:       false,
		`"x"`:       false,
		`{"a":`:     false,
		`{"a":"` + strings.Repeat("x", MaxActionInput) + `"}`: false,
	} {
		if got := ValidActionInput(json.RawMessage(in)); got != want {
			t.Errorf("ValidActionInput(%.20q) = %v", in, got)
		}
	}
}

func TestBackendClientRequests(t *testing.T) {
	dir := shortDir(t)
	type seen struct {
		method, path, query, host, user, admin, lang, call, ctype, body string
	}
	got := make(chan seen, 4)
	fakeBackend(t, filepath.Join(dir, "hello.sock"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Host, r.Header.Get("X-Maru-User"), r.Header.Get("X-Maru-Admin"),
			r.Header.Get("X-Maru-Lang"), r.Header.Get("X-Maru-Call"), r.Header.Get("Content-Type"), string(b)}
		w.Header().Set("Set-Cookie", "evil=1")
		switch r.URL.Path {
		case "/v1/data":
			w.Write([]byte(`{"ok":true}`))
		case "/v1/actions/redirect":
			http.Redirect(w, r, "/v1/data", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	c := NewBackendClient(func(id string) string { return filepath.Join(dir, id+".sock") })
	call := BackendCall{User: "admin", Admin: true, Lang: "ko", Token: "tok"}

	r := c.Data(context.Background(), "hello", "a=1&b=%20", call)
	if r.Status != 200 || string(r.Body) != `{"ok":true}` {
		t.Fatalf("data = %+v", r)
	}
	s := <-got
	if s.method != "GET" || s.path != "/v1/data" || s.query != "a=1&b=%20" || s.host != "addon" || s.user != "admin" ||
		s.admin != "1" || s.lang != "ko" || s.call != "tok" {
		t.Errorf("data request = %+v", s)
	}

	call.Admin = false
	r = c.Action(context.Background(), "hello", "start", "web", json.RawMessage(`{"n":1}`), call)
	if r.Status != 204 {
		t.Fatalf("action = %+v", r)
	}
	s = <-got
	if s.method != "POST" || s.path != "/v1/actions/start" || s.admin != "0" || s.ctype != "application/json" ||
		s.body != `{"target":"web","input":{"n":1}}` {
		t.Errorf("action request = %+v", s)
	}
	if r = c.Action(context.Background(), "hello", "start", "", nil, call); r.Status != 204 {
		t.Fatalf("action without input = %+v", r)
	}
	if s = <-got; s.body != `{"target":"","input":null}` {
		t.Errorf("action body without input = %s", s.body)
	}
	// 리디렉션은 따라가지 않는다(3xx는 502).
	if r = c.Action(context.Background(), "hello", "redirect", "", nil, call); r.Status != 502 || r.Code != api.ErrAddonError {
		t.Errorf("redirect = %+v", r)
	}
	<-got
}

func TestBackendClientFailures(t *testing.T) {
	dir := shortDir(t)
	release := make(chan struct{})
	fakeBackend(t, filepath.Join(dir, "slow.sock"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 본문을 다 읽어야 서버가 연결 끊김을 보고 요청 문맥을 끝낸다.
		io.Copy(io.Discard, r.Body)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	fakeBackend(t, filepath.Join(dir, "drop.sock"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 응답 없이 연결을 끊는다.
		conn, _, err := http.NewResponseController(w).Hijack()
		if err == nil {
			conn.Close()
		}
	}))
	// 정리는 거꾸로 돈다. 서버를 닫기 전에 처리기를 풀어 준다.
	t.Cleanup(func() { close(release) })
	c := NewBackendClient(func(id string) string { return filepath.Join(dir, id+".sock") })

	// 소켓이 없으면 연결 실패(503)다.
	if r := c.Data(context.Background(), "missing", "", BackendCall{}); r.Status != 503 || r.Code != api.ErrAddonBackendUnavailable {
		t.Errorf("missing socket = %+v", r)
	}
	// 시간 초과: 데이터는 504 addon_backend_unavailable, 조작은 504 action_timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if r := c.Data(ctx, "slow", "", BackendCall{}); r.Status != 504 || r.Code != api.ErrAddonBackendUnavailable {
		t.Errorf("slow data = %+v", r)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if r := c.Action(ctx2, "slow", "go", "", nil, BackendCall{}); r.Status != 504 || r.Code != api.ErrActionTimeout {
		t.Errorf("slow action = %+v", r)
	}
	// 연결한 뒤 응답 없이 끊으면 502다.
	if r := c.Data(context.Background(), "drop", "", BackendCall{}); r.Status != 502 || r.Code != api.ErrAddonError {
		t.Errorf("dropped = %+v", r)
	}

	// Retain은 남길 id의 Transport만 둔다.
	c.Retain(map[string]bool{"slow": true})
	c.mu.Lock()
	n := len(c.transports)
	c.mu.Unlock()
	if n != 1 {
		t.Errorf("transports after Retain = %d", n)
	}
}

func TestBackendPaths(t *testing.T) {
	dev := DevBackendPaths("/state")
	if dev.Backend("hello") != "/state/addon-dev/hello.sock" || dev.Host("hello") != "/state/addon-host/hello.sock" ||
		dev.HostMode != 0o600 || dev.HostDirMode != 0 {
		t.Errorf("dev paths = %s %s %v", dev.Backend("hello"), dev.Host("hello"), dev.HostMode)
	}
	prod := InstalledBackendPaths()
	if prod.Backend("hello") != "/run/maru-addon/hello.sock" || prod.Host("hello") != "/run/maru-addon-host/hello/host.sock" ||
		prod.HostMode != 0o666 || prod.HostDirMode != 0o755 {
		t.Errorf("installed paths = %s %s", prod.Backend("hello"), prod.Host("hello"))
	}
}
