package terminal

import (
	"context"
	"errors"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

const (
	// MaxWindows는 대시보드 세션 하나가 동시에 여는 터미널 창 수다. sshd의 기본 MaxSessions(10)보다 작게 둔다.
	MaxWindows = 4
	// MaxClients는 동시에 두는 SSH 클라이언트(대시보드 세션) 수다.
	MaxClients = 32
	// 창 크기의 범위(문자 칸)
	maxCols = 1000
	maxRows = 500
)

// 창이 닫힌 이유(Window.Wait의 reason). 셸이 끝났으면 빈 값이다.
const (
	// ReasonSessionEnded는 대시보드 세션이 끝났다는 뜻이다(로그아웃, 만료).
	ReasonSessionEnded = "session_ended"
	// ReasonDisabled는 관리자가 웹 터미널을 껐다는 뜻이다.
	ReasonDisabled = "disabled"
	// ReasonConnectionLost는 sshd와의 연결이 끊겼다는 뜻이다(sshd 재시작 등).
	ReasonConnectionLost = "connection_lost"
)

// Open의 오류 코드(Error.Code). 접속 실패 코드(Code*)와 함께 쓴다.
const (
	// CodeTooMany는 이 세션의 창이 MaxWindows개 열려 있다는 뜻이다.
	CodeTooMany = "too_many_windows"
	// CodeBusy는 SSH 클라이언트가 MaxClients개라 더 열 수 없다는 뜻이다.
	CodeBusy = "busy"
)

// Size는 터미널 창의 크기(문자 칸)다.
type Size struct {
	Cols, Rows int
}

func (s Size) clamp() Size {
	return Size{Cols: min(max(s.Cols, 1), maxCols), Rows: min(max(s.Rows, 1), maxRows)}
}

// Manager는 대시보드 세션마다 SSH 클라이언트 하나를 두고, 창마다 그 위에 채널(SSH 세션)을 연다.
// 인증은 세션의 첫 창에서 한 번만 하므로 둘째 창부터는 비밀번호를 다시 묻지 않는다.
// 마지막 창이 닫히거나 대시보드 세션이 끝나면 클라이언트를 닫는다.
type Manager struct {
	Dialer *Dialer

	mu      sync.Mutex
	clients map[string]*client
}

// NewManager는 d로 접속하는 Manager다.
func NewManager(d *Dialer) *Manager {
	return &Manager{Dialer: d, clients: map[string]*client{}}
}

// client는 대시보드 세션 하나의 SSH 클라이언트다.
type client struct {
	key  string
	user string
	// ready는 접속(인증)이 끝나면 닫힌다. 그 뒤 ssh·err 중 하나가 정해진다.
	ready chan struct{}
	ssh   *ssh.Client
	err   error
	// windows는 이 클라이언트를 쓰거나 기다리는 창 수다.
	windows int
	// reason은 클라이언트를 닫은 이유다(Manager가 닫았을 때).
	reason string
	closed bool
	// cancel은 접속 중(비밀번호 입력 대기 포함)인 Dial을 멈춘다.
	cancel context.CancelFunc
}

// Open은 key(대시보드 세션)의 클라이언트에 새 셸 창을 연다. 클라이언트가 없으면 a로 user에 접속한다.
// 같은 세션의 다른 창이 접속하는 중이면 끝날 때까지 기다렸다가 그 클라이언트를 함께 쓴다.
func (m *Manager) Open(ctx context.Context, key, user string, a Auth, size Size) (*Window, error) {
	m.mu.Lock()
	c := m.clients[key]
	if c != nil && c.windows >= MaxWindows {
		m.mu.Unlock()
		return nil, &Error{Code: CodeTooMany}
	}
	if c != nil {
		c.windows++
	}
	m.mu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		if c == nil {
			var err error
			if c, err = m.dial(ctx, key, user, a); err != nil {
				return nil, err
			}
		} else {
			select {
			case <-c.ready:
			case <-ctx.Done():
				m.release(c)
				return nil, &Error{Code: CodeCanceled, Err: ctx.Err()}
			}
			if c.err != nil {
				// 함께 기다리던 창의 접속이 실패했다(취소 포함). 이 창이 직접 접속한다.
				m.release(c)
				c = nil
				continue
			}
		}
		w, err := m.newWindow(c, size)
		if err == nil {
			return w, nil
		}
		// 클라이언트가 이미 끊겼다(sshd 재시작). 닫고 한 번 다시 접속한다.
		m.close(c, ReasonConnectionLost)
		m.release(c)
		c = nil
		if attempt == 1 {
			return nil, &Error{Code: CodeFailed, Err: err}
		}
	}
	return nil, &Error{Code: CodeFailed}
}

// dial은 key의 클라이언트를 만들고 접속한다. 이미 다른 창이 만들었으면 그것을 기다린다.
func (m *Manager) dial(ctx context.Context, key, user string, a Auth) (*client, error) {
	m.mu.Lock()
	if c := m.clients[key]; c != nil {
		// 그사이 다른 창이 먼저 만들었다.
		if c.windows >= MaxWindows {
			m.mu.Unlock()
			return nil, &Error{Code: CodeTooMany}
		}
		c.windows++
		m.mu.Unlock()
		select {
		case <-c.ready:
		case <-ctx.Done():
			m.release(c)
			return nil, &Error{Code: CodeCanceled, Err: ctx.Err()}
		}
		if c.err != nil {
			m.release(c)
			return nil, c.err
		}
		return c, nil
	}
	if len(m.clients) >= MaxClients {
		m.mu.Unlock()
		return nil, &Error{Code: CodeBusy}
	}
	dctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c := &client{key: key, user: user, ready: make(chan struct{}), windows: 1, cancel: cancel}
	m.clients[key] = c
	m.mu.Unlock()

	sc, err := m.Dialer.Dial(dctx, user, a)
	m.mu.Lock()
	c.cancel = nil
	switch {
	case err != nil:
		c.err = err
		m.forgetLocked(c)
	case c.closed:
		// 접속하는 사이에 세션이 끝났다(Dial이 취소되기 전에 인증을 마쳤다).
		sc.Close()
		c.err = &Error{Code: CodeCanceled, Err: errors.New(c.reason)}
		err = c.err
	default:
		c.ssh = sc
	}
	close(c.ready)
	m.mu.Unlock()
	if err != nil {
		m.release(c)
		return nil, err
	}
	// sshd와의 연결이 끊기면 클라이언트를 잊는다(다음 창은 다시 접속한다).
	go func() {
		sc.Wait()
		m.mu.Lock()
		if c.reason == "" {
			c.reason = ReasonConnectionLost
		}
		m.forgetLocked(c)
		m.mu.Unlock()
	}()
	return c, nil
}

// forgetLocked는 c가 아직 key의 클라이언트면 목록에서 뺀다.
func (m *Manager) forgetLocked(c *client) {
	if m.clients[c.key] == c {
		delete(m.clients, c.key)
	}
}

// release는 창 하나가 c를 그만 쓴다. 마지막 창이면 클라이언트를 닫는다.
func (m *Manager) release(c *client) {
	m.mu.Lock()
	c.windows--
	last := c.windows <= 0
	if last {
		m.forgetLocked(c)
	}
	sc := c.ssh
	m.mu.Unlock()
	if last && sc != nil {
		sc.Close()
	}
}

// close는 c를 reason으로 닫는다. 열린 창은 셸이 끊겨 Wait가 돌아온다.
func (m *Manager) close(c *client, reason string) {
	m.mu.Lock()
	if c.reason == "" {
		c.reason = reason
	}
	c.closed = true
	m.forgetLocked(c)
	sc, cancel := c.ssh, c.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if sc != nil {
		sc.Close()
	}
}

// CloseSession은 key(대시보드 세션)의 클라이언트를 닫는다. 세션이 끝날 때(auth.Store.OnRemove) 부른다.
func (m *Manager) CloseSession(key string) {
	m.mu.Lock()
	c := m.clients[key]
	m.mu.Unlock()
	if c != nil {
		m.close(c, ReasonSessionEnded)
	}
}

// CloseAll은 모든 클라이언트를 reason으로 닫는다(관리자가 웹 터미널을 껐을 때).
func (m *Manager) CloseAll(reason string) {
	m.mu.Lock()
	all := make([]*client, 0, len(m.clients))
	for _, c := range m.clients {
		all = append(all, c)
	}
	m.mu.Unlock()
	for _, c := range all {
		m.close(c, reason)
	}
}

// Clients는 지금 둔 SSH 클라이언트 수다(테스트용).
func (m *Manager) Clients() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.clients)
}

// Window는 창 하나의 셸(SSH 세션 채널)이다.
type Window struct {
	m       *Manager
	c       *client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	once    sync.Once
}

func (m *Manager) newWindow(c *client, size Size) (*Window, error) {
	s, err := c.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	w := &Window{m: m, c: c, session: s}
	if w.stdin, err = s.StdinPipe(); err != nil {
		s.Close()
		return nil, err
	}
	if w.stdout, err = s.StdoutPipe(); err != nil {
		s.Close()
		return nil, err
	}
	size = size.clamp()
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.IUTF8: 1, ssh.TTY_OP_ISPEED: 38400, ssh.TTY_OP_OSPEED: 38400}
	if err := s.RequestPty("xterm-256color", size.Rows, size.Cols, modes); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.Shell(); err != nil {
		s.Close()
		return nil, err
	}
	return w, nil
}

// Read는 셸의 출력이다(PTY라 표준 오류도 함께 온다).
func (w *Window) Read(p []byte) (int, error) { return w.stdout.Read(p) }

// Write는 셸의 입력이다.
func (w *Window) Write(p []byte) (int, error) { return w.stdin.Write(p) }

// Resize는 창 크기를 sshd에 알린다.
func (w *Window) Resize(size Size) error {
	size = size.clamp()
	return w.session.WindowChange(size.Rows, size.Cols)
}

// Wait는 셸이 끝날 때까지 기다린다. 셸이 끝났으면 종료 코드와 빈 reason, 클라이언트가 닫혔으면
// 닫힌 이유(Reason*)를 돌려준다.
func (w *Window) Wait() (exitCode int, reason string) {
	err := w.session.Wait()
	var exit *ssh.ExitError
	if errors.As(err, &exit) {
		return exit.ExitStatus(), ""
	}
	if err == nil {
		return 0, ""
	}
	w.m.mu.Lock()
	reason = w.c.reason
	w.m.mu.Unlock()
	if reason == "" {
		reason = ReasonConnectionLost
	}
	return -1, reason
}

// Close는 창을 닫는다. 마지막 창이면 클라이언트도 닫는다. 여러 번 불러도 된다.
func (w *Window) Close() {
	w.once.Do(func() {
		w.session.Close()
		w.m.release(w.c)
	})
}
