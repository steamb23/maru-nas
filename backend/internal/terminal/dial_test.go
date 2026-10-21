package terminal

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	helperclient "onmaru/backend/internal/helper/client"
	"onmaru/backend/internal/helper/helperfake"
	"onmaru/backend/internal/terminal/sshfake"
)

// testEnv는 가짜 도우미와, 그 가짜 홈의 authorized_keys를 보는 가짜 sshd다.
type testEnv struct {
	t      *testing.T
	helper *helperfake.Server
	hc     *helperclient.Client
	sshd   *sshfake.Server
	dialer *Dialer
}

func newTestEnv(t *testing.T, opts sshfake.Options) *testEnv {
	t.Helper()
	// helperfake는 자기 uid를 클라이언트 uid로 쓰는데 도우미는 root 클라이언트를 거부한다(클라우드 세션은 root로 돈다).
	if os.Getuid() == 0 {
		t.Skip("the helper rejects a root client; run as a normal user")
	}
	dir := t.TempDir()
	h, err := helperfake.Start(filepath.Join(dir, "helper.sock"), filepath.Join(dir, "root"), helperfake.DefaultUsers(),
		helperfake.Options{FailDelay: -1, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	opts.Users = sshfake.DefaultUsers()
	opts.Home = h.Home
	s, err := sshfake.Start("127.0.0.1:0", opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.WriteHostKey(filepath.Join(dir, "ssh_host_ed25519_key.pub")); err != nil {
		t.Fatal(err)
	}
	return &testEnv{t: t, helper: h, hc: helperclient.New(h.Socket), sshd: s,
		dialer: &Dialer{Addr: s.Addr, HostKeys: filepath.Join(dir, "*.pub"), SSHDConfig: filepath.Join(dir, "none")}}
}

// keys는 user의 도우미 티켓으로 임시 키를 넣고 지우는 KeyAuthorizer다.
func (e *testEnv) keys(user string) KeyAuthorizer {
	e.t.Helper()
	res, err := e.hc.Open(context.Background(), user, user, false)
	if err != nil {
		e.t.Fatal(err)
	}
	return ticketKeys{c: e.hc, ticket: res.Ticket}
}

type ticketKeys struct {
	c      *helperclient.Client
	ticket string
}

func (k ticketKeys) SSHAuthorize(ctx context.Context, pub string) error {
	return k.c.SSHAuthorize(ctx, k.ticket, pub)
}

func (k ticketKeys) SSHRevoke(ctx context.Context, pub string) error {
	return k.c.SSHRevoke(ctx, k.ticket, pub)
}

// answers는 질문마다 정해진 답을 돌려주는 Ask와 받은 질문 기록이다.
type answers struct {
	mu   sync.Mutex
	got  []Challenge
	next func(c Challenge) ([]string, error)
}

func (a *answers) ask(_ context.Context, c Challenge) ([]string, error) {
	a.mu.Lock()
	a.got = append(a.got, c)
	a.mu.Unlock()
	return a.next(c)
}

func (a *answers) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.got)
}

func fixed(vals ...string) func(Challenge) ([]string, error) {
	i := 0
	return func(c Challenge) ([]string, error) {
		out := vals[i : i+len(c.Prompts)]
		i += len(c.Prompts)
		return out, nil
	}
}

func dialErr(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("err = %v, want *Error", err)
	}
	return e
}

func (e *testEnv) authorizedKeys(user string) string {
	b, _ := os.ReadFile(filepath.Join(e.helper.Home(user), ".ssh", "authorized_keys"))
	return string(b)
}

func TestDialWithHelperKey(t *testing.T) {
	// 실제 이 NAS처럼 공개 키만 받는 sshd.
	e := newTestEnv(t, sshfake.Options{NoPassword: true, NoKeyboardInteractive: true})
	ask := &answers{next: fixed()}
	c, err := e.dialer.Dial(context.Background(), "user", Auth{Keys: e.keys("user"), Ask: ask.ask})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if ask.count() != 0 {
		t.Fatal("asked for a password despite the helper key")
	}
	// 인증이 끝나면 임시 키 줄을 지운다.
	if keys := e.authorizedKeys("user"); strings.Contains(keys, "onmaru-ephemeral") {
		t.Fatalf("temporary key left: %q", keys)
	}
}

func TestDialPasswordFallback(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{NoKeyboardInteractive: true})
	ask := &answers{next: fixed("wrong", "user")}
	// 도우미가 없으면 비밀번호를 묻고, 틀리면 다시 묻는다.
	c, err := e.dialer.Dial(context.Background(), "user", Auth{Ask: ask.ask})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if ask.count() != 2 || ask.got[0].Kind != ChallengePassword || len(ask.got[0].Prompts) != 1 || ask.got[0].Prompts[0].Echo {
		t.Fatalf("challenges = %+v", ask.got)
	}
}

func TestDialHelperKeyRefusedFallsBack(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{NoKeyboardInteractive: true})
	// 그룹이 쓸 수 있는 홈은 sshd가 authorized_keys를 무시하므로 도우미가 거절한다.
	if err := os.Chmod(e.helper.Home("user"), 0o775); err != nil {
		t.Fatal(err)
	}
	ask := &answers{next: fixed("user")}
	c, err := e.dialer.Dial(context.Background(), "user", Auth{Keys: e.keys("user"), Ask: ask.ask})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if ask.count() != 1 {
		t.Fatalf("asked %d times, want 1", ask.count())
	}

	// 비밀번호도 받지 않는 sshd면 실패하고 도우미가 거절한 이유를 알려 준다.
	e2 := newTestEnv(t, sshfake.Options{NoPassword: true, NoKeyboardInteractive: true})
	os.Chmod(e2.helper.Home("user"), 0o775)
	_, err = e2.dialer.Dial(context.Background(), "user", Auth{Keys: e2.keys("user"), Ask: ask.ask})
	de := dialErr(t, err)
	if de.Code != CodeAuthFailed || de.KeyCode != "ssh_strict_modes" || de.Asked {
		t.Fatalf("error = %+v", de)
	}
}

func TestDialKeyboardInteractive(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{NoPublicKey: true})
	ask := &answers{next: fixed("totp", "123456")}
	c, err := e.dialer.Dial(context.Background(), "totp", Auth{Ask: ask.ask})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if ask.count() != 2 || ask.got[0].Kind != ChallengeKeyboard || ask.got[1].Prompts[0].Text != "Verification code: " ||
		!ask.got[1].Prompts[0].Echo {
		t.Fatalf("challenges = %+v", ask.got)
	}
}

func TestDialCanceled(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{})
	ask := &answers{next: func(Challenge) ([]string, error) { return nil, ErrCanceled }}
	_, err := e.dialer.Dial(context.Background(), "user", Auth{Ask: ask.ask})
	if de := dialErr(t, err); de.Code != CodeCanceled {
		t.Fatalf("error = %+v", de)
	}
	// keyboard-interactive에서 취소하면 password로 넘어가 다시 묻지 않는다.
	if ask.count() != 1 {
		t.Fatalf("asked %d times after cancel", ask.count())
	}

	// 묻는 중에 ctx가 끝나면(창을 닫음) 취소다.
	ctx, cancel := context.WithCancel(context.Background())
	ask = &answers{next: func(Challenge) ([]string, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	_, err = e.dialer.Dial(ctx, "user", Auth{Ask: ask.ask})
	if de := dialErr(t, err); de.Code != CodeCanceled {
		t.Fatalf("error = %+v", de)
	}
}

func TestDialAuthFailedHints(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{NoPassword: true, NoKeyboardInteractive: true})
	conf := filepath.Join(t.TempDir(), "sshd_config")
	os.WriteFile(conf, []byte("AllowGroups root _ssh\n"), 0o644)
	e.dialer.SSHDConfig = conf
	e.dialer.UserGroups = func(string) ([]string, error) { return []string{"users"}, nil }
	_, err := e.dialer.Dial(context.Background(), "user", Auth{})
	de := dialErr(t, err)
	if de.Code != CodeAuthFailed || !slices.Equal(de.NotAllowedGroups, []string{"root", "_ssh"}) {
		t.Fatalf("error = %+v", de)
	}
	// 그룹에 속하면 그룹 힌트를 주지 않는다.
	e.dialer.UserGroups = func(string) ([]string, error) { return []string{"users", "_ssh"}, nil }
	_, err = e.dialer.Dial(context.Background(), "user", Auth{})
	if de := dialErr(t, err); de.NotAllowedGroups != nil {
		t.Fatalf("error = %+v", de)
	}
}

func TestDialHelperKeyRejectedBySSHD(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{})
	// sshd가 도우미가 쓴 authorized_keys를 보지 않는다(AllowUsers·Match 등으로 키를 받지 않는 경우를 흉내 낸다).
	empty := t.TempDir()
	s, err := sshfake.Start("127.0.0.1:0", sshfake.Options{Users: sshfake.DefaultUsers(),
		Home: func(string) string { return empty }, NoPassword: true, NoKeyboardInteractive: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	dir := t.TempDir()
	s.WriteHostKey(filepath.Join(dir, "ssh_host_ed25519_key.pub"))
	e.dialer.Addr, e.dialer.HostKeys = s.Addr, filepath.Join(dir, "*.pub")
	_, err = e.dialer.Dial(context.Background(), "user", Auth{Keys: e.keys("user")})
	if de := dialErr(t, err); de.Code != CodeAuthFailed || de.KeyCode != KeyRejected {
		t.Fatalf("error = %+v", de)
	}
	// 넣었던 키 줄은 지운다.
	if keys := e.authorizedKeys("user"); strings.Contains(keys, "onmaru-ephemeral") {
		t.Fatalf("temporary key left: %q", keys)
	}
}

func TestDialHostKeyMismatch(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{})
	other, err := sshfake.Start("127.0.0.1:0", sshfake.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	dir := t.TempDir()
	other.WriteHostKey(filepath.Join(dir, "ssh_host_ed25519_key.pub"))
	e.dialer.HostKeys = filepath.Join(dir, "*.pub")
	ask := &answers{next: fixed("user")}
	_, err = e.dialer.Dial(context.Background(), "user", Auth{Ask: ask.ask})
	if de := dialErr(t, err); de.Code != CodeHostKey {
		t.Fatalf("error = %+v", de)
	}
	if ask.count() != 0 {
		t.Fatal("asked for a password before verifying the host key")
	}
	e.dialer.HostKeys = filepath.Join(t.TempDir(), "*.pub")
	_, err = e.dialer.Dial(context.Background(), "user", Auth{Ask: ask.ask})
	if de := dialErr(t, err); de.Code != CodeHostKey {
		t.Fatalf("no host keys: %+v", de)
	}
}

func TestDialUnavailable(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{})
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	e.dialer.Addr = addr
	_, err := e.dialer.Dial(context.Background(), "user", Auth{})
	if de := dialErr(t, err); de.Code != CodeUnavailable {
		t.Fatalf("error = %+v", de)
	}
}

func TestDialerAddress(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "sshd_config")
	os.WriteFile(conf, []byte("Port 2222\n"), 0o644)
	d := &Dialer{Addr: AddrAuto, SSHDConfig: conf}
	if got := d.address(d.readConfig()); got != "127.0.0.1:2222" {
		t.Fatalf("address = %q", got)
	}
	d.SSHDConfig = filepath.Join(t.TempDir(), "missing")
	if got := d.address(d.readConfig()); got != "127.0.0.1:22" {
		t.Fatalf("address without config = %q", got)
	}
	d.Addr = "[::1]:2200"
	if got := d.address(d.readConfig()); got != "[::1]:2200" {
		t.Fatalf("explicit address = %q", got)
	}
}

// readUntil은 r에서 want가 나올 때까지 읽는다.
func readUntil(t *testing.T, r interface{ Read([]byte) (int, error) }, want string) string {
	t.Helper()
	var out strings.Builder
	buf := make([]byte, 1024)
	done := make(chan struct{})
	timer := time.AfterFunc(5*time.Second, func() { close(done) })
	defer timer.Stop()
	for !strings.Contains(out.String(), want) {
		select {
		case <-done:
			t.Fatalf("timed out waiting for %q, got %q", want, out.String())
		default:
		}
		n, err := r.Read(buf)
		out.Write(buf[:n])
		if err != nil {
			t.Fatalf("read: %v (got %q, want %q)", err, out.String(), want)
		}
	}
	return out.String()
}
