package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"maru/backend/internal/api"
	"maru/backend/internal/terminal/sshfake"
)

// wsTerm은 테스트용 터미널 WebSocket 클라이언트다.
type wsTerm struct {
	t      *testing.T
	conn   *websocket.Conn
	ctx    context.Context
	output strings.Builder
}

// dialTerminal은 b의 쿠키로 /api/terminal에 WebSocket을 연다. header는 더할 헤더(Origin 등)다.
func (b *browser) dialTerminal(ts *httptest.Server, header http.Header) (*wsTerm, *http.Response, error) {
	b.e.t.Helper()
	h := http.Header{}
	for k, v := range header {
		h[k] = v
	}
	var cookies []string
	for _, ck := range b.cookies {
		cookies = append(cookies, ck.Name+"="+ck.Value)
	}
	h.Set("Cookie", strings.Join(cookies, "; "))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	b.e.t.Cleanup(cancel)
	conn, resp, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/terminal?cols=80&rows=24",
		&websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		return nil, resp, err
	}
	b.e.t.Cleanup(func() { conn.CloseNow() })
	return &wsTerm{t: b.e.t, conn: conn, ctx: ctx}, resp, nil
}

func (e *env) server() *httptest.Server {
	ts := httptest.NewServer(e.h)
	e.t.Cleanup(ts.Close)
	return ts
}

// next는 다음 제어 메시지를 읽는다. 그사이 온 출력은 output에 모은다.
func (w *wsTerm) next() api.TerminalServerMessage {
	w.t.Helper()
	for {
		typ, data, err := w.conn.Read(w.ctx)
		if err != nil {
			w.t.Fatalf("read: %v (output %q)", err, w.output.String())
		}
		if typ == websocket.MessageBinary {
			w.output.Write(data)
			continue
		}
		var msg api.TerminalServerMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			w.t.Fatal(err)
		}
		return msg
	}
}

// expect는 다음 제어 메시지가 typ(status면 state)인지 확인한다.
func (w *wsTerm) expect(typ, state string) api.TerminalServerMessage {
	w.t.Helper()
	msg := w.next()
	if msg.Type != typ || msg.State != state {
		w.t.Fatalf("message = %+v, want %s %s", msg, typ, state)
	}
	return msg
}

// until은 출력에 want가 나올 때까지 읽는다.
func (w *wsTerm) until(want string) {
	w.t.Helper()
	for !strings.Contains(w.output.String(), want) {
		typ, data, err := w.conn.Read(w.ctx)
		if err != nil {
			w.t.Fatalf("read: %v (output %q, want %q)", err, w.output.String(), want)
		}
		if typ != websocket.MessageBinary {
			w.t.Fatalf("unexpected control message %s while waiting for %q", data, want)
		}
		w.output.Write(data)
	}
}

func (w *wsTerm) input(s string) {
	w.t.Helper()
	if err := w.conn.Write(w.ctx, websocket.MessageBinary, []byte(s)); err != nil {
		w.t.Fatal(err)
	}
}

func (w *wsTerm) control(msg api.TerminalClientMessage) {
	w.t.Helper()
	b, _ := json.Marshal(msg)
	if err := w.conn.Write(w.ctx, websocket.MessageText, b); err != nil {
		w.t.Fatal(err)
	}
}

func TestTerminalShellWithHelperKey(t *testing.T) {
	// 이 NAS처럼 공개 키만 받는 sshd. 도우미 임시 키로 비밀번호를 다시 묻지 않는다.
	e := newEnv(t, envOptions{helper: true, terminal: true, sshd: sshfake.Options{NoPassword: true, NoKeyboardInteractive: true}})
	ts := e.server()
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	w, _, err := b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.expect("status", "connecting")
	w.expect("status", "connected")
	w.until("$ ")
	w.input("whoami\r")
	w.until("user\r\n")
	w.control(api.TerminalClientMessage{Type: "resize", Cols: 100, Rows: 30})
	// 가짜 sshd는 window-change와 입력을 다른 고루틴에서 처리하므로 반영될 때까지 다시 묻는다.
	for i := 0; !strings.Contains(w.output.String(), "100x30"); i++ {
		if i == 100 {
			t.Fatalf("size never changed: %q", w.output.String())
		}
		w.output.Reset()
		w.input("size\r")
		w.until("$ ")
	}
	w.input("exit 2\r")
	msg := w.expect("status", "closed")
	if msg.Reason != "exit" || msg.ExitCode == nil || *msg.ExitCode != 2 {
		t.Fatalf("closed = %+v", msg)
	}
	if strings.Contains(e.logs.String(), "ssh-ed25519") {
		t.Fatalf("public key logged:\n%s", e.logs.String())
	}
}

func TestTerminalPasswordPrompt(t *testing.T) {
	e := newEnv(t, envOptions{terminal: true, sshd: sshfake.Options{NoKeyboardInteractive: true}})
	ts := e.server()
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)

	// 취소하면 접속을 그만둔다.
	w, _, err := b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.expect("status", "connecting")
	p := w.expect("prompt", "")
	if p.Kind != "password" || len(p.Prompts) != 1 || p.Prompts[0].Echo {
		t.Fatalf("prompt = %+v", p)
	}
	w.control(api.TerminalClientMessage{Type: "answer", Answers: []string{"wrong"}})
	w.expect("prompt", "")
	w.control(api.TerminalClientMessage{Type: "cancel"})
	if msg := w.expect("status", "closed"); msg.Reason != "canceled" {
		t.Fatalf("closed = %+v", msg)
	}

	w, _, err = b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.expect("status", "connecting")
	w.expect("prompt", "")
	w.control(api.TerminalClientMessage{Type: "answer", Answers: []string{"user"}})
	w.expect("status", "connected")
	w.until("$ ")
	if strings.Contains(e.logs.String(), "wrong") {
		t.Fatalf("password logged:\n%s", e.logs.String())
	}

	// 같은 대시보드 세션의 둘째 창은 다시 묻지 않는다.
	w2, _, err := b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w2.expect("status", "connecting")
	w2.expect("status", "connected")
	if e.sshd.Conns() != 1 {
		t.Fatalf("sshd connections = %d, want 1", e.sshd.Conns())
	}
}

func TestTerminalAuthFailedHints(t *testing.T) {
	// 도우미가 없고 sshd가 공개 키만 받으면 들어갈 방법이 없다.
	e := newEnv(t, envOptions{terminal: true, sshd: sshfake.Options{NoPassword: true, NoKeyboardInteractive: true}})
	ts := e.server()
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	w, _, err := b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.expect("status", "connecting")
	if msg := w.expect("status", "closed"); msg.Reason != "ssh_auth_failed" {
		t.Fatalf("closed = %+v", msg)
	}
}

func TestTerminalRateLimit(t *testing.T) {
	e := newEnv(t, envOptions{terminal: true, sshd: sshfake.Options{NoKeyboardInteractive: true}})
	ts := e.server()
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	for i := 0; ; i++ {
		w, _, err := b.dialTerminal(ts, nil)
		if err != nil {
			t.Fatal(err)
		}
		w.expect("status", "connecting")
		msg := w.next()
		if msg.Type == "status" && msg.Reason == "rate_limited" {
			if i != 5 || msg.RetryAfterSeconds <= 0 {
				t.Fatalf("rate limited after %d failures: %+v", i, msg)
			}
			return
		}
		// 한 접속에서 세 번 틀리면 실패 한 번이다.
		for range 3 {
			if msg.Type != "prompt" {
				t.Fatalf("message = %+v, want prompt", msg)
			}
			w.control(api.TerminalClientMessage{Type: "answer", Answers: []string{"wrong"}})
			msg = w.next()
		}
		if msg.Reason != "ssh_auth_failed" {
			t.Fatalf("closed = %+v", msg)
		}
		if i > 6 {
			t.Fatal("never rate limited")
		}
	}
}

func TestTerminalClosedWhenSessionEnds(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, terminal: true})
	ts := e.server()
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	w, _, err := b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.expect("status", "connecting")
	w.expect("status", "connected")
	wantStatus(t, b.do(http.MethodPost, "/api/logout", nil), http.StatusNoContent)
	if msg := w.expect("status", "closed"); msg.Reason != "session_ended" {
		t.Fatalf("closed = %+v", msg)
	}
	if n := e.terminal.Clients(); n != 0 {
		t.Fatalf("clients = %d after logout", n)
	}
}

func TestTerminalSetting(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, terminal: true, terminalOff: true})
	ts := e.server()
	admin, user := e.browser(), e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, user.login("user", "user"), http.StatusOK)

	// 기본값(-terminal 끔)이면 앱을 숨기고 연결도 거절한다.
	if decode[api.CapabilitiesResponse](t, user.do(http.MethodGet, "/api/capabilities", nil)).Terminal {
		t.Fatal("terminal advertised while disabled")
	}
	rec := user.do(http.MethodGet, "/api/terminal", nil)
	wantStatus(t, rec, http.StatusForbidden)
	if code := decode[api.ErrorResponse](t, rec).Code; code != api.ErrTerminalDisabled {
		t.Fatalf("code = %q", code)
	}

	on, off, bad := "true", "false", "yes"
	wantStatus(t, user.do(http.MethodPut, "/api/site/server", api.ServerSettingsRequest{Server: &api.ServerOverrides{Terminal: &on}}),
		http.StatusForbidden)
	wantStatus(t, admin.do(http.MethodPut, "/api/site/server", api.ServerSettingsRequest{Server: &api.ServerOverrides{Terminal: &bad}}),
		http.StatusBadRequest)
	wantStatus(t, admin.do(http.MethodPut, "/api/site/server", api.ServerSettingsRequest{Server: &api.ServerOverrides{Terminal: &on}}),
		http.StatusNoContent)
	got := decode[api.ServerSettings](t, admin.do(http.MethodGet, "/api/site/server", nil)).Terminal
	if got != (api.OverridableSetting{Value: "true", Base: "false", Overridden: true}) {
		t.Fatalf("terminal setting = %+v", got)
	}
	if !decode[api.CapabilitiesResponse](t, user.do(http.MethodGet, "/api/capabilities", nil)).Terminal {
		t.Fatal("terminal not advertised after enabling")
	}
	// 열린 터미널은 끄는 순간 닫힌다.
	w, _, err := user.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.expect("status", "connecting")
	w.expect("status", "connected")
	wantStatus(t, admin.do(http.MethodPut, "/api/site/server", api.ServerSettingsRequest{Server: &api.ServerOverrides{Terminal: &off}}),
		http.StatusNoContent)
	if msg := w.expect("status", "closed"); msg.Reason != "disabled" {
		t.Fatalf("closed = %+v", msg)
	}
}

func TestTerminalRejectsCrossOrigin(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, terminal: true})
	ts := e.server()
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	_, resp, err := b.dialTerminal(ts, http.Header{"Origin": {"http://evil.example"}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin dial: err=%v resp=%v", err, resp)
	}
	// 세션이 없으면 401이다.
	_, resp, err = e.browser().dialTerminal(ts, nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous dial: err=%v resp=%v", err, resp)
	}
}
