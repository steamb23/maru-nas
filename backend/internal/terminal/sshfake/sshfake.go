// Package sshfake는 개발·테스트용 가짜 sshd다.
//
// x/crypto/ssh 서버를 프로세스 안에서 띄우고 가짜 계정(omvfake.DefaultUsers와 같은 이름·비밀번호)으로
// password·keyboard-interactive·publickey 인증을 받는다. publickey는 가짜 도우미(helperfake)가 가짜 홈에
// 쓴 ~/.ssh/authorized_keys를 읽어 확인하고 expiry-time·from 옵션도 해석하므로, 도우미 임시 키 흐름을
// 처음부터 끝까지 시험할 수 있다. 셸은 몇 가지 명령만 아는 장난감 셸이다(실제 명령을 실행하지 않는다).
//
// 운영 바이너리(cmd/maru, cmd/maru-helper)에서 import하지 않는다.
package sshfake

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
)

// User는 가짜 계정이다. OTP가 있으면 keyboard-interactive에서 비밀번호 다음에 인증 코드를 묻는다.
type User struct {
	Name     string
	Password string
	OTP      string
}

// DefaultUsers는 omvfake.DefaultUsers와 같은 계정이다. totp는 OMV처럼 2차 인증 코드 123456을 묻는다.
func DefaultUsers() []User {
	return []User{
		{Name: "admin", Password: "admin"},
		{Name: "user", Password: "user"},
		{Name: "totp", Password: "totp", OTP: "123456"},
	}
}

// Options는 가짜 sshd 설정이다.
type Options struct {
	Users []User
	// Home은 사용자의 홈 디렉터리다(publickey 확인용). nil이면 publickey 인증은 늘 실패한다.
	Home func(user string) string
	// 인증 방법을 끈다(실제 OMV의 PasswordAuthentication no 등을 흉내 낸다).
	NoPublicKey, NoPassword, NoKeyboardInteractive bool
	// Now는 현재 시각이다(expiry-time 확인).
	Now  func() time.Time
	Logf func(format string, args ...any)
}

// Server는 실행 중인 가짜 sshd다.
type Server struct {
	// Addr는 수신 주소다.
	Addr string

	opts    Options
	hostKey ssh.Signer
	l       net.Listener
	wg      sync.WaitGroup

	mu    sync.Mutex
	conns map[*ssh.ServerConn]bool
}

// Start는 addr(예: 127.0.0.1:0)에서 가짜 sshd를 띄운다.
func Start(addr string, opts Options) (*Server, error) {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := &Server{Addr: l.Addr().String(), opts: opts, hostKey: signer, l: l, conns: map[*ssh.ServerConn]bool{}}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

// HostKey는 호스트 공개 키의 authorized_keys 형식(".pub" 파일 내용)이다.
func (s *Server) HostKey() []byte { return ssh.MarshalAuthorizedKey(s.hostKey.PublicKey()) }

// WriteHostKey는 호스트 공개 키를 path(".pub" 파일)에 쓴다. maru의 -ssh-host-keys가 이 파일을 가리키게 한다.
func (s *Server) WriteHostKey(path string) error {
	return os.WriteFile(path, s.HostKey(), 0o644)
}

// ParseMethods는 "publickey,password,keyboard-interactive" 같은 목록에서 끈 방법을 Options에 넣는다.
// 빈 값이면 모두 켠다.
func ParseMethods(list string, o *Options) error {
	if list == "" {
		return nil
	}
	o.NoPublicKey, o.NoPassword, o.NoKeyboardInteractive = true, true, true
	for _, m := range strings.Split(list, ",") {
		switch strings.TrimSpace(m) {
		case "publickey":
			o.NoPublicKey = false
		case "password":
			o.NoPassword = false
		case "keyboard-interactive":
			o.NoKeyboardInteractive = false
		default:
			return fmt.Errorf("unknown ssh auth method %q", m)
		}
	}
	return nil
}

// Close는 수신을 멈추고 열린 연결을 모두 끊는다.
func (s *Server) Close() {
	s.l.Close()
	s.mu.Lock()
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// DropConns는 열린 연결을 모두 끊는다(sshd 재시작을 흉내 낸다). 수신은 계속한다.
func (s *Server) DropConns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		c.Close()
	}
}

// Conns는 열린 연결 수다(테스트용).
func (s *Server) Conns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

func (s *Server) config() *ssh.ServerConfig {
	cfg := &ssh.ServerConfig{}
	if !s.opts.NoPassword {
		cfg.PasswordCallback = func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if u, ok := s.user(c.User()); ok && u.OTP == "" && u.Password == string(pw) {
				return nil, nil
			}
			return nil, errors.New("wrong password")
		}
	}
	if !s.opts.NoKeyboardInteractive {
		cfg.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, ask ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			u, ok := s.user(c.User())
			ans, err := ask("", "", []string{"Password: "}, []bool{false})
			if err != nil {
				return nil, err
			}
			if !ok || ans[0] != u.Password {
				return nil, errors.New("wrong password")
			}
			if u.OTP != "" {
				ans, err := ask("", "Two-factor authentication", []string{"Verification code: "}, []bool{true})
				if err != nil {
					return nil, err
				}
				if ans[0] != u.OTP {
					return nil, errors.New("wrong code")
				}
			}
			return nil, nil
		}
	}
	if !s.opts.NoPublicKey {
		cfg.PublicKeyCallback = s.publicKey
	}
	cfg.AddHostKey(s.hostKey)
	return cfg
}

func (s *Server) user(name string) (User, bool) {
	i := slices.IndexFunc(s.opts.Users, func(u User) bool { return u.Name == name })
	if i < 0 {
		return User{}, false
	}
	return s.opts.Users[i], true
}

// publicKey는 사용자의 authorized_keys에서 key를 찾는다. from과 expiry-time 옵션만 해석한다.
func (s *Server) publicKey(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
	if _, ok := s.user(c.User()); !ok || s.opts.Home == nil {
		return nil, errors.New("unknown user")
	}
	b, err := os.ReadFile(filepath.Join(s.opts.Home(c.User()), ".ssh", "authorized_keys"))
	if err != nil {
		return nil, err
	}
	remote, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	want := key.Marshal()
	for len(b) > 0 {
		k, _, options, rest, err := ssh.ParseAuthorizedKey(b)
		if err != nil {
			break
		}
		b = rest
		if !bytes.Equal(k.Marshal(), want) {
			continue
		}
		if s.optionsAllow(options, remote) {
			return nil, nil
		}
	}
	return nil, errors.New("key not authorized")
}

func (s *Server) optionsAllow(options []string, remote string) bool {
	for _, o := range options {
		name, value, _ := strings.Cut(o, "=")
		value = strings.Trim(value, `"`)
		switch name {
		case "from":
			if !slices.Contains(strings.Split(value, ","), remote) {
				return false
			}
		case "expiry-time":
			t, err := time.Parse("20060102150405", strings.TrimSuffix(value, "Z"))
			if err != nil || !s.opts.Now().Before(t) {
				return false
			}
		}
	}
	return true
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		nc, err := s.l.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(nc)
		}()
	}
}

func (s *Server) handle(nc net.Conn) {
	conn, chans, reqs, err := ssh.NewServerConn(nc, s.config())
	if err != nil {
		s.opts.Logf("sshfake: handshake: %v", err)
		nc.Close()
		return
	}
	s.opts.Logf("sshfake: user=%q authenticated", conn.User())
	s.mu.Lock()
	s.conns[conn] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()
	go ssh.DiscardRequests(reqs)
	var wg sync.WaitGroup
	for nch := range chans {
		if nch.ChannelType() != "session" {
			nch.Reject(ssh.UnknownChannelType, "only session channels")
			continue
		}
		ch, chReqs, err := nch.Accept()
		if err != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.session(conn.User(), ch, chReqs)
		}()
	}
	wg.Wait()
}

// session은 채널 하나다. pty-req·window-change·shell만 받는다.
func (s *Server) session(user string, ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	sh := &shell{user: user, ch: ch, cols: 80, rows: 24}
	started := false
	for req := range reqs {
		switch req.Type {
		case "pty-req":
			var p struct {
				Term          string
				Cols, Rows    uint32
				Width, Height uint32
				Modes         string
			}
			if ssh.Unmarshal(req.Payload, &p) != nil {
				req.Reply(false, nil)
				continue
			}
			sh.setSize(int(p.Cols), int(p.Rows))
			req.Reply(true, nil)
		case "window-change":
			var p struct{ Cols, Rows, Width, Height uint32 }
			if ssh.Unmarshal(req.Payload, &p) == nil {
				sh.setSize(int(p.Cols), int(p.Rows))
			}
		case "shell":
			if started {
				req.Reply(false, nil)
				continue
			}
			started = true
			req.Reply(true, nil)
			go func() {
				code := sh.run()
				ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
				ch.Close()
			}()
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// shell은 장난감 셸이다. PTY처럼 입력을 되돌려 보여 주고(echo) 줄 단위로 명령을 읽는다.
type shell struct {
	user string
	ch   ssh.Channel

	mu         sync.Mutex
	cols, rows int
}

func (sh *shell) setSize(cols, rows int) {
	sh.mu.Lock()
	sh.cols, sh.rows = cols, rows
	sh.mu.Unlock()
}

func (sh *shell) size() (int, int) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.cols, sh.rows
}

func (sh *shell) out(s string) { sh.ch.Write([]byte(strings.ReplaceAll(s, "\n", "\r\n"))) }

func (sh *shell) prompt() {
	sh.out("\x1b[1;32m" + sh.user + "@maru-fake\x1b[0m:\x1b[1;34m~\x1b[0m$ ")
}

// run은 셸을 실행하고 종료 코드를 돌려준다.
func (sh *shell) run() int {
	sh.out("Fake shell for maru development. Type \"help\".\n")
	sh.prompt()
	var line []rune
	buf := make([]byte, 1024)
	var pending []byte
	for {
		n, err := sh.ch.Read(buf)
		if err != nil {
			return 0
		}
		pending = append(pending, buf[:n]...)
		for len(pending) > 0 {
			if !utf8.FullRune(pending) {
				break
			}
			r, size := utf8.DecodeRune(pending)
			pending = pending[size:]
			switch r {
			case '\r', '\n':
				sh.out("\n")
				if code, exit := sh.exec(string(line)); exit {
					return code
				}
				line = line[:0]
				sh.prompt()
			case 0x7f, 0x08:
				if len(line) > 0 {
					line = line[:len(line)-1]
					sh.out("\b \b")
				}
			case 0x03:
				sh.out("^C\n")
				line = line[:0]
				sh.prompt()
			case 0x04:
				if len(line) == 0 {
					sh.out("logout\n")
					return 0
				}
			case 0x1b:
				// 화살표 키 같은 이스케이프 열은 무시한다.
				pending = skipEscape(pending)
			default:
				if r >= 0x20 {
					line = append(line, r)
					sh.out(string(r))
				}
			}
		}
	}
}

func skipEscape(b []byte) []byte {
	if len(b) > 0 && (b[0] == '[' || b[0] == 'O') {
		for i := 1; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				return b[i+1:]
			}
		}
		return nil
	}
	return b
}

// exec는 명령 한 줄을 실행한다. exit면 종료 코드와 true다.
func (sh *shell) exec(line string) (int, bool) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return 0, false
	}
	switch f[0] {
	case "help":
		sh.out("commands: help, whoami, size, colors, echo <text>, clear, exit [code]\n")
	case "whoami":
		sh.out(sh.user + "\n")
	case "size":
		cols, rows := sh.size()
		sh.out(fmt.Sprintf("%dx%d\n", cols, rows))
	case "colors":
		var b strings.Builder
		for i := 0; i < 16; i++ {
			fmt.Fprintf(&b, "\x1b[48;5;%dm  ", i)
		}
		b.WriteString("\x1b[0m\n")
		sh.out(b.String())
	case "echo":
		sh.out(strings.Join(f[1:], " ") + "\n")
	case "clear":
		sh.out("\x1b[H\x1b[2J")
	case "exit", "logout":
		code := 0
		if len(f) > 1 {
			code, _ = strconv.Atoi(f[1])
		}
		return code, true
	default:
		sh.out(f[0] + ": command not found\n")
	}
	return 0, false
}
