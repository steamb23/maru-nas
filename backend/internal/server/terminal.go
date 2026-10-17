package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/terminal"
)

const (
	// terminalReadLimit는 브라우저가 보내는 메시지 하나의 최대 크기다. 붙여 넣기는 프론트가 나눠 보낸다.
	terminalReadLimit = 64 << 10
	// terminalPing은 프록시가 유휴 연결을 끊지 않게 보내는 ping 주기다.
	terminalPing = 30 * time.Second
	// terminalDrain은 셸이 끝난 뒤 남은 출력을 보낼 때까지 기다리는 시간이다.
	terminalDrain = 2 * time.Second
	// 처음 창 크기를 받지 못했을 때의 크기
	defaultCols, defaultRows = 80, 24
)

// errTerminalRateLimited는 비밀번호를 묻기 전에 실패 제한에 걸렸다는 뜻이다.
var errTerminalRateLimited = errors.New("terminal: rate limited")

// terminalEnabled는 웹 터미널을 쓸 수 있는지다(웹에서 덮어쓴 값, 없으면 -terminal).
func (h *handler) terminalEnabled() bool {
	if h.Terminal == nil {
		return false
	}
	h.over.mu.Lock()
	defer h.over.mu.Unlock()
	if h.over.terminal != nil {
		return *h.over.terminal == "true"
	}
	return h.TerminalEnabled
}

// ticketKeys는 세션의 도우미 티켓으로 임시 SSH 키를 넣고 지운다(terminal.KeyAuthorizer).
type ticketKeys struct {
	helper Helper
	ticket string
}

func (k ticketKeys) SSHAuthorize(ctx context.Context, pub string) error {
	return k.helper.SSHAuthorize(ctx, k.ticket, pub)
}

func (k ticketKeys) SSHRevoke(ctx context.Context, pub string) error {
	return k.helper.SSHRevoke(ctx, k.ticket, pub)
}

// terminal은 웹 터미널 WebSocket이다. 대시보드 세션 사용자로 sshd(루프백)에 접속해 셸 하나를 중계한다.
// Origin은 coder/websocket의 기본 검사(Host와 같아야 함)를 따른다. 세션 쿠키가 SameSite=Strict라
// 다른 사이트의 페이지는 애초에 세션을 싣지 못한다.
func (h *handler) terminal(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !h.terminalEnabled() {
		writeError(w, http.StatusForbidden, api.ErrTerminalDisabled, "web terminal is disabled")
		return
	}
	size := terminal.Size{Cols: queryInt(r, "cols", defaultCols), Rows: queryInt(r, "rows", defaultRows)}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		// Accept가 오류 응답을 이미 썼다(Origin 불일치 등).
		log.Printf("terminal: user=%q ip=%s: accept: %v", s.Username, clientIP(r), err)
		return
	}
	conn.SetReadLimit(terminalReadLimit)
	t := &termConn{h: h, s: s, ip: clientIP(r), conn: conn, size: size, answers: make(chan []string, 1)}
	t.run(r.Context())
}

func queryInt(r *http.Request, key string, def int) int {
	if v, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil && v > 0 {
		return v
	}
	return def
}

// termConn은 WebSocket 하나(터미널 창 하나)다.
type termConn struct {
	h    *handler
	s    auth.Session
	ip   string
	conn *websocket.Conn

	mu      sync.Mutex
	size    terminal.Size
	window  *terminal.Window
	answers chan []string
	// prompting은 답을 기다리는 질문이 있는지다. 그때만 answer를 받는다.
	prompting bool
	// limited는 실패 제한 때문에 묻지 않고 그만둔 경우의 남은 시간이다.
	limited time.Duration
}

func (t *termConn) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer t.conn.CloseNow()
	// 읽기는 고루틴 하나가 맡는다(ping의 pong도 여기서 처리된다). 브라우저가 끊으면 ctx를 끝낸다.
	go func() {
		defer cancel()
		t.readLoop(ctx)
	}()
	go t.pingLoop(ctx)

	start := time.Now()
	t.send(ctx, api.TerminalServerMessage{Type: "status", State: "connecting"})
	w, err := t.open(ctx)
	if err != nil {
		t.fail(ctx, err)
		return
	}
	defer w.Close()
	t.mu.Lock()
	t.window = w
	// 접속하는 동안 바뀐 크기를 반영한다.
	size := t.size
	t.mu.Unlock()
	w.Resize(size)
	log.Printf("terminal: user=%q ip=%s: connected", t.s.Username, t.ip)
	t.send(ctx, api.TerminalServerMessage{Type: "status", State: "connected"})

	copied := make(chan struct{})
	go func() {
		defer close(copied)
		buf := make([]byte, 32<<10)
		for {
			n, err := w.Read(buf)
			if n > 0 {
				if t.conn.Write(ctx, websocket.MessageBinary, buf[:n]) != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	waited := make(chan struct{})
	var code int
	var reason string
	go func() {
		code, reason = w.Wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-ctx.Done():
		// 브라우저가 창을 닫았다.
		w.Close()
		<-waited
		log.Printf("terminal: user=%q ip=%s: closed by browser after %s", t.s.Username, t.ip, time.Since(start).Round(time.Second))
		return
	}
	// 셸이 끝난 뒤 남은 출력을 먼저 보낸다.
	select {
	case <-copied:
	case <-time.After(terminalDrain):
	}
	msg := api.TerminalServerMessage{Type: "status", State: "closed"}
	if reason == "" {
		msg.Reason, msg.ExitCode = "exit", &code
	} else {
		msg.Reason = reason
	}
	log.Printf("terminal: user=%q ip=%s: closed (%s) after %s", t.s.Username, t.ip, msg.Reason, time.Since(start).Round(time.Second))
	t.send(ctx, msg)
	t.conn.Close(websocket.StatusNormalClosure, "")
}

// open은 세션의 SSH 클라이언트에 셸 창을 연다. 비밀번호를 물어야 하면 실패 제한을 거친다.
func (t *termConn) open(ctx context.Context) (*terminal.Window, error) {
	var done func(auth.Outcome)
	a := terminal.Auth{Ask: func(ctx context.Context, c terminal.Challenge) ([]string, error) {
		if done == nil {
			d, retry := t.h.terminalLimiter.Begin(t.ip, t.s.Username, "")
			if d == nil {
				t.mu.Lock()
				t.limited = retry
				t.mu.Unlock()
				return nil, errTerminalRateLimited
			}
			done = d
		}
		return t.ask(ctx, c)
	}}
	if t.h.Helper != nil && t.s.HelperTicket != "" && !t.s.HelperLost {
		a.Keys = ticketKeys{helper: t.h.Helper, ticket: t.s.HelperTicket}
	}
	t.mu.Lock()
	size := t.size
	t.mu.Unlock()
	w, err := t.h.Terminal.Open(ctx, t.s.ID, t.s.Username, a, size)
	if done != nil {
		// Ask는 접속(Dial) 고루틴 안에서만 불리고 Open이 돌아오면 끝났으므로 done을 그대로 읽어도 된다.
		var de *terminal.Error
		switch {
		case err == nil:
			done(auth.Success)
		case errors.As(err, &de) && de.Code == terminal.CodeAuthFailed && de.Asked:
			done(auth.Failure)
		default:
			done(auth.Neutral)
		}
	}
	return w, err
}

// ask는 질문을 브라우저에 보내고 답을 기다린다.
func (t *termConn) ask(ctx context.Context, c terminal.Challenge) ([]string, error) {
	msg := api.TerminalServerMessage{Type: "prompt", Kind: c.Kind, Instruction: c.Instruction}
	for _, p := range c.Prompts {
		msg.Prompts = append(msg.Prompts, api.TerminalPrompt{Text: p.Text, Echo: p.Echo})
	}
	t.mu.Lock()
	t.prompting = true
	t.mu.Unlock()
	// 앞 질문에 늦게 온 답이 남아 있으면 버린다.
	select {
	case <-t.answers:
	default:
	}
	defer func() {
		t.mu.Lock()
		t.prompting = false
		t.mu.Unlock()
	}()
	if err := t.send(ctx, msg); err != nil {
		return nil, terminal.ErrCanceled
	}
	select {
	case ans := <-t.answers:
		if ans == nil {
			return nil, terminal.ErrCanceled
		}
		return ans, nil
	case <-ctx.Done():
		return nil, terminal.ErrCanceled
	}
}

// fail은 접속 실패를 알리고 닫는다.
func (t *termConn) fail(ctx context.Context, err error) {
	msg := api.TerminalServerMessage{Type: "status", State: "closed", Reason: terminal.CodeFailed}
	var de *terminal.Error
	if errors.As(err, &de) {
		msg.Reason, msg.KeyCode, msg.NotAllowedGroups = de.Code, de.KeyCode, de.NotAllowedGroups
	}
	t.mu.Lock()
	limited := t.limited
	t.mu.Unlock()
	switch {
	case limited > 0:
		msg.Reason, msg.RetryAfterSeconds = api.ErrRateLimited, retrySeconds(limited)
	case msg.Reason == terminal.CodeCanceled && !t.h.terminalEnabled():
		msg.Reason = terminal.ReasonDisabled
	case msg.Reason == terminal.CodeCanceled && !t.sessionAlive():
		msg.Reason = terminal.ReasonSessionEnded
	}
	if msg.Reason == terminal.CodeCanceled {
		log.Printf("terminal: user=%q ip=%s: canceled", t.s.Username, t.ip)
	} else {
		log.Printf("terminal: user=%q ip=%s: failed: %v", t.s.Username, t.ip, err)
	}
	t.send(ctx, msg)
	t.conn.Close(websocket.StatusNormalClosure, "")
}

func (t *termConn) sessionAlive() bool {
	_, ok := t.h.Sessions.Get(t.s.ID)
	return ok
}

func retrySeconds(d time.Duration) int {
	return int((d + time.Second - 1) / time.Second)
}

func (t *termConn) send(ctx context.Context, msg api.TerminalServerMessage) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return t.conn.Write(ctx, websocket.MessageText, b)
}

// readLoop는 브라우저의 입력과 제어 메시지를 처리한다.
func (t *termConn) readLoop(ctx context.Context) {
	for {
		typ, data, err := t.conn.Read(ctx)
		if err != nil {
			return
		}
		if typ == websocket.MessageBinary {
			t.mu.Lock()
			w := t.window
			t.mu.Unlock()
			// 셸이 열리기 전의 입력(비밀번호는 answer로 온다)은 버린다.
			if w != nil {
				if _, err := w.Write(data); err != nil && !errors.Is(err, io.EOF) {
					return
				}
			}
			continue
		}
		var msg api.TerminalClientMessage
		if json.Unmarshal(data, &msg) != nil {
			continue
		}
		switch msg.Type {
		case "resize":
			t.mu.Lock()
			t.size = terminal.Size{Cols: msg.Cols, Rows: msg.Rows}
			w := t.window
			t.mu.Unlock()
			if w != nil {
				w.Resize(terminal.Size{Cols: msg.Cols, Rows: msg.Rows})
			}
		case "answer", "cancel":
			t.mu.Lock()
			prompting := t.prompting
			t.mu.Unlock()
			if !prompting {
				continue
			}
			var ans []string
			if msg.Type == "answer" {
				ans = msg.Answers
				if ans == nil {
					ans = []string{}
				}
			}
			select {
			case t.answers <- ans:
			default:
			}
		}
	}
}

func (t *termConn) pingLoop(ctx context.Context) {
	tick := time.NewTicker(terminalPing)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			pctx, cancel := context.WithTimeout(ctx, terminalPing)
			err := t.conn.Ping(pctx)
			cancel()
			if err != nil {
				t.conn.CloseNow()
				return
			}
		}
	}
}
