package terminal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"onmaru/backend/config"
	"onmaru/backend/internal/helper/proto"
)

const (
	// DefaultHostKeys는 sshd 호스트 공개 키 파일의 glob 패턴이다(0644라 누구나 읽는다).
	DefaultHostKeys = config.DefaultSSHHostKeys
	// AddrAuto는 sshd_config의 Port로 루프백에 붙는다는 뜻이다.
	AddrAuto = config.DefaultSSHAddr
	// handshakeTimeout은 인증(비밀번호 입력 포함)까지 걸릴 수 있는 시간이다. sshd의 LoginGraceTime(OMV 120초)보다 길게 둔다.
	handshakeTimeout = 3 * time.Minute
	// keyTimeout은 도우미에 임시 키를 넣거나 지우는 요청의 시간 제한이다.
	keyTimeout = 10 * time.Second
	// askTries는 비밀번호·keyboard-interactive를 한 접속에서 다시 묻는 횟수다(잘못 입력했을 때).
	askTries = 3
)

// 접속 실패 코드(Error.Code). 프론트가 안내 문구를 고른다.
const (
	// CodeUnavailable은 sshd에 연결하지 못했다는 뜻이다(SSH 서비스가 꺼져 있거나 포트가 다름).
	CodeUnavailable = "ssh_unavailable"
	// CodeHostKey는 호스트 키를 읽지 못했거나 서버 키가 그중에 없다는 뜻이다.
	CodeHostKey = "ssh_host_key"
	// CodeAuthFailed는 sshd가 모든 인증을 거절했다는 뜻이다. Error의 힌트로 이유를 좁힌다.
	CodeAuthFailed = "ssh_auth_failed"
	// CodeCanceled는 사용자가 비밀번호 입력을 취소했거나 창을 닫았다는 뜻이다.
	CodeCanceled = "canceled"
	// CodeFailed는 그 밖의 SSH 오류다.
	CodeFailed = "ssh_failed"
	// KeyRejected는 Error.KeyCode의 값으로, 도우미가 임시 키를 넣었는데 sshd가 받지 않았다는 뜻이다
	// (AllowUsers·DenyGroups·Match 블록 등). AllowGroups에 속하지 않을 때는 NotAllowedGroups가 붙는다.
	KeyRejected = "key_rejected"
)

// Error는 접속 실패다. 세부(주소, 원래 오류)는 Err에 두고 브라우저에는 코드와 힌트만 준다.
type Error struct {
	Code string
	// KeyCode는 도우미 임시 키를 쓰지 못한 이유(도우미 오류 코드)다. 도우미를 쓰지 않았으면 빈 값이다.
	KeyCode string
	// NotAllowedGroups는 사용자가 sshd의 AllowGroups 중 어느 그룹에도 없을 때 그 그룹 목록이다.
	NotAllowedGroups []string
	// Asked는 비밀번호·keyboard-interactive 답을 보냈는지다(실패 제한에 센다).
	Asked bool
	Err   error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return "terminal: " + e.Code + ": " + e.Err.Error()
	}
	return "terminal: " + e.Code
}

func (e *Error) Unwrap() error { return e.Err }

// ErrCanceled는 Ask가 돌려주는 취소 오류다.
var ErrCanceled = errors.New("terminal: canceled")

// Prompt는 sshd가 묻는 질문 하나다.
type Prompt struct {
	Text string `json:"text"`
	Echo bool   `json:"echo"`
}

// 질문 종류(Challenge.Kind)
const (
	// ChallengePassword는 password 방식이다. 질문 문구는 프론트가 현지화한다.
	ChallengePassword = "password"
	// ChallengeKeyboard는 keyboard-interactive 방식이다. 문구는 sshd(PAM)가 준 그대로다.
	ChallengeKeyboard = "keyboard-interactive"
)

// Challenge는 브라우저에 보여 줄 질문 묶음이다.
type Challenge struct {
	Kind        string
	Instruction string
	Prompts     []Prompt
}

// KeyAuthorizer는 사용자의 authorized_keys에 짧은 수명의 키를 넣고 지운다. 권한 도우미 클라이언트를
// 그 세션의 티켓으로 감싼 것이다.
type KeyAuthorizer interface {
	SSHAuthorize(ctx context.Context, publicKey string) error
	SSHRevoke(ctx context.Context, publicKey string) error
}

// Auth는 접속 한 번에 쓰는 인증 수단이다.
type Auth struct {
	// Keys가 있으면 도우미 임시 키를 먼저 쓴다.
	Keys KeyAuthorizer
	// Ask는 비밀번호·keyboard-interactive 질문을 브라우저에 묻는다. 답의 수는 질문의 수와 같아야 한다.
	// ErrCanceled(또는 다른 오류)를 돌려주면 접속을 그만둔다. nil이면 묻지 않는다.
	Ask func(ctx context.Context, c Challenge) ([]string, error)
}

// Dialer는 sshd에 접속한다.
type Dialer struct {
	// Addr는 sshd 주소(host:port)다. 비우거나 AddrAuto면 SSHDConfig의 Port로 127.0.0.1에 붙는다.
	Addr string
	// SSHDConfig는 sshd 설정 파일 경로다. 기본 DefaultSSHDConfig.
	SSHDConfig string
	// HostKeys는 호스트 공개 키 파일의 glob 패턴이다. 기본 DefaultHostKeys.
	HostKeys string
	// UserGroups는 사용자가 속한 그룹 이름이다(실패 안내용). 기본은 os/user.
	UserGroups func(name string) ([]string, error)
	// Logf는 키 정리 실패 같은 운영 로그다. 기본 아무것도 하지 않는다.
	Logf func(format string, args ...any)
}

// CheckAddr는 -ssh-addr 값이 AddrAuto이거나 루프백 IP 주소:포트인지 확인한다. sshd에 보내는
// 비밀번호와 터미널 내용이 네트워크로 나가지 않게 루프백만 받는다.
func CheckAddr(addr string) error {
	if addr == "" || addr == AddrAuto {
		return nil
	}
	ap, err := netip.ParseAddrPort(addr)
	if err != nil {
		return fmt.Errorf("ssh address %q: want host:port with an IP address", addr)
	}
	if !ap.Addr().IsLoopback() || ap.Port() == 0 {
		return fmt.Errorf("ssh address %q: only loopback addresses are allowed", addr)
	}
	return nil
}

func (d *Dialer) logf(format string, args ...any) {
	if d.Logf != nil {
		d.Logf(format, args...)
	}
}

// readConfig는 sshd 설정을 읽는다. 읽지 못하면 빈 설정(포트 22)이다.
func (d *Dialer) readConfig() SSHDConfig {
	path := d.SSHDConfig
	if path == "" {
		path = DefaultSSHDConfig
	}
	f, err := os.Open(path)
	if err != nil {
		return SSHDConfig{}
	}
	defer f.Close()
	c, _ := ParseSSHDConfig(f)
	return c
}

// address는 접속할 sshd 주소다.
func (d *Dialer) address(c SSHDConfig) string {
	if d.Addr != "" && d.Addr != AddrAuto {
		return d.Addr
	}
	port := c.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

// hostKeys는 호스트 공개 키를 읽는다.
func (d *Dialer) hostKeys() ([]ssh.PublicKey, error) {
	pattern := d.HostKeys
	if pattern == "" {
		pattern = DefaultHostKeys
	}
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var keys []ssh.PublicKey
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		k, _, _, _, err := ssh.ParseAuthorizedKey(b)
		if err != nil {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no host keys readable at %s", pattern)
	}
	return keys, nil
}

var errHostKey = errors.New("host key mismatch")

// hostKeyConfig는 읽은 키 중 하나와 같은 서버 키만 받는 콜백과, 그 키 종류로 제한한 알고리즘 목록이다.
// ssh.FixedHostKey는 키를 하나만 받으므로 직접 비교한다.
func hostKeyConfig(keys []ssh.PublicKey) (ssh.HostKeyCallback, []string) {
	var algos []string
	for _, k := range keys {
		switch k.Type() {
		case ssh.KeyAlgoRSA:
			// OpenSSH 10은 SHA-1 서명(ssh-rsa)을 받지 않는다.
			algos = append(algos, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256)
		default:
			algos = append(algos, k.Type())
		}
	}
	// ed25519를 앞에 둔다(서버는 클라이언트 목록의 앞쪽을 고른다).
	slices.SortStableFunc(algos, func(a, b string) int {
		return boolInt(b == ssh.KeyAlgoED25519) - boolInt(a == ssh.KeyAlgoED25519)
	})
	cb := func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got := key.Marshal()
		for _, k := range keys {
			if bytes.Equal(k.Marshal(), got) {
				return nil
			}
		}
		return errHostKey
	}
	return cb, slices.Compact(algos)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Dial은 user로 sshd에 접속해 인증까지 마친 클라이언트를 돌려준다. 실패하면 *Error다.
func (d *Dialer) Dial(ctx context.Context, name string, a Auth) (*ssh.Client, error) {
	conf := d.readConfig()
	addr := d.address(conf)
	keys, err := d.hostKeys()
	if err != nil {
		return nil, &Error{Code: CodeHostKey, Err: err}
	}
	hostKeyCallback, algos := hostKeyConfig(keys)

	st := &authState{auth: a, ctx: ctx}
	cfg := &ssh.ClientConfig{
		User:              name,
		HostKeyCallback:   hostKeyCallback,
		HostKeyAlgorithms: algos,
		Auth:              st.methods(),
	}

	var dialer net.Dialer
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, err := dialer.DialContext(dctx, "tcp", addr)
	cancel()
	if err != nil {
		return nil, &Error{Code: CodeUnavailable, Err: err}
	}
	// 인증이 끝날 때까지만 마감 시각을 두고, ctx가 끝나면(창을 닫음) 바로 끊는다.
	conn.SetDeadline(time.Now().Add(handshakeTimeout))
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	stopped := stop()
	st.revoke(d)
	if err != nil {
		conn.Close()
		return nil, d.classify(name, conf, st, err, !stopped)
	}
	conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// classify는 핸드셰이크 오류를 코드로 바꾼다.
func (d *Dialer) classify(name string, conf SSHDConfig, st *authState, err error, ctxDone bool) *Error {
	st.mu.Lock()
	defer st.mu.Unlock()
	e := &Error{Code: CodeFailed, KeyCode: st.keyCode, Asked: st.asked, Err: err}
	switch {
	case st.canceled || ctxDone:
		e.Code = CodeCanceled
	case errors.Is(err, errHostKey):
		e.Code = CodeHostKey
	case strings.Contains(err.Error(), "unable to authenticate"):
		e.Code = CodeAuthFailed
		e.NotAllowedGroups = d.notAllowed(name, conf.AllowGroups)
		if st.authorized && e.KeyCode == "" {
			e.KeyCode = KeyRejected
		}
	}
	return e
}

// notAllowed는 사용자가 groups 중 어느 그룹에도 속하지 않으면 groups를 돌려준다. 확인할 수 없으면 nil이다.
// sshd는 AllowUsers·DenyGroups·Match에 따라서도 거절하므로 접속을 막는 데는 쓰지 않고 안내에만 쓴다.
func (d *Dialer) notAllowed(name string, groups []string) []string {
	if len(groups) == 0 {
		return nil
	}
	lookup := d.UserGroups
	if lookup == nil {
		lookup = userGroups
	}
	mine, err := lookup(name)
	if err != nil {
		return nil
	}
	for _, g := range groups {
		if slices.Contains(mine, g) {
			return nil
		}
	}
	return slices.Clone(groups)
}

// userGroups는 사용자의 기본 그룹과 보조 그룹 이름이다.
func userGroups(name string) ([]string, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return nil, err
	}
	gids, err := u.GroupIds()
	if err != nil {
		return nil, err
	}
	if !slices.Contains(gids, u.Gid) {
		gids = append(gids, u.Gid)
	}
	var names []string
	for _, id := range gids {
		if g, err := user.LookupGroupId(id); err == nil {
			names = append(names, g.Name)
		}
	}
	return names, nil
}

// authState는 접속 한 번의 인증 진행 상태다. x/crypto의 콜백은 핸드셰이크 고루틴에서 불린다.
type authState struct {
	auth Auth
	ctx  context.Context

	mu         sync.Mutex
	publicKey  string
	authorized bool
	keyCode    string
	asked      bool
	canceled   bool
}

// methods는 시도할 인증 방법이다. 서버가 허용하는 것만 이 순서로 시도된다.
func (st *authState) methods() []ssh.AuthMethod {
	var out []ssh.AuthMethod
	if st.auth.Keys != nil {
		out = append(out, ssh.PublicKeysCallback(st.signers))
	}
	if st.auth.Ask != nil {
		out = append(out,
			ssh.RetryableAuthMethod(ssh.KeyboardInteractive(st.keyboard), askTries),
			ssh.RetryableAuthMethod(ssh.PasswordCallback(st.password), askTries))
	}
	return out
}

// signers는 메모리에서 만든 일회용 ed25519 키를 도우미로 authorized_keys에 넣고 돌려준다. 넣지 못하면
// 오류 대신 빈 목록을 돌려줘 다음 방법(비밀번호)으로 넘어가게 한다.
func (st *authState) signers() ([]ssh.Signer, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, nil
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, nil
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	ctx, cancel := context.WithTimeout(st.ctx, keyTimeout)
	defer cancel()
	err = st.auth.Keys.SSHAuthorize(ctx, line)
	st.mu.Lock()
	defer st.mu.Unlock()
	if err != nil {
		st.keyCode = codeOf(err)
		return nil, nil
	}
	st.publicKey, st.authorized = line, true
	return []ssh.Signer{signer}, nil
}

// revoke는 넣은 임시 키를 지운다. 인증이 끝나면 키가 필요 없으므로 성공·실패와 관계없이 지운다.
// 지우지 못해도 키 줄은 2분 뒤 만료되고 다음 연산이 정리한다.
func (st *authState) revoke(d *Dialer) {
	st.mu.Lock()
	line, ok := st.publicKey, st.authorized
	st.mu.Unlock()
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(st.ctx), keyTimeout)
	defer cancel()
	if err := st.auth.Keys.SSHRevoke(ctx, line); err != nil {
		d.logf("terminal: revoke temporary ssh key: %v", err)
	}
}

// ask는 질문을 브라우저에 묻는다. 한 번 취소하면 다음 방법에서도 다시 묻지 않는다
// (x/crypto는 방법 하나가 오류를 내도 남은 방법을 계속 시도한다).
func (st *authState) ask(c Challenge) ([]string, error) {
	st.mu.Lock()
	if st.canceled {
		st.mu.Unlock()
		return nil, ErrCanceled
	}
	st.asked = true
	st.mu.Unlock()
	answers, err := st.auth.Ask(st.ctx, c)
	if err == nil && len(answers) != len(c.Prompts) {
		err = errors.New("terminal: answer count mismatch")
	}
	if err != nil {
		st.mu.Lock()
		st.canceled = true
		st.mu.Unlock()
		return nil, err
	}
	return answers, nil
}

func (st *authState) keyboard(_, instruction string, questions []string, echos []bool) ([]string, error) {
	// 질문 없는 라운드(안내만 보냄)에는 묻지 않고 빈 답을 보낸다.
	if len(questions) == 0 {
		return []string{}, nil
	}
	c := Challenge{Kind: ChallengeKeyboard, Instruction: instruction}
	for i, q := range questions {
		c.Prompts = append(c.Prompts, Prompt{Text: q, Echo: i < len(echos) && echos[i]})
	}
	return st.ask(c)
}

func (st *authState) password() (string, error) {
	answers, err := st.ask(Challenge{Kind: ChallengePassword, Prompts: []Prompt{{}}})
	if err != nil {
		return "", err
	}
	return answers[0], nil
}

// codeOf는 도우미 오류의 코드다. 코드가 없는 오류(도우미에 연결하지 못함)는 helper_unavailable이다.
func codeOf(err error) string {
	if c := proto.CodeOf(err); c != "" {
		return c
	}
	return "helper_unavailable"
}
