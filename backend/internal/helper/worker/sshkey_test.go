package worker

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"maru/backend/internal/helper/proto"
)

// testKey는 새 ed25519 공개 키의 authorized_keys 형식("ssh-ed25519 <base64>")이다.
func testKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return proto.SSHKeyType + " " + base64.StdEncoding.EncodeToString(sshBlob(proto.SSHKeyType, pub))
}

func sshBlob(parts ...any) []byte {
	var b []byte
	for _, p := range parts {
		var v []byte
		switch p := p.(type) {
		case string:
			v = []byte(p)
		case []byte:
			v = p
		case ed25519.PublicKey:
			v = p
		}
		b = binary.BigEndian.AppendUint32(b, uint32(len(v)))
		b = append(b, v...)
	}
	return b
}

// sshEnv는 시계를 고정한 worker 환경이다.
func sshEnv(t *testing.T) (*env, *time.Time) {
	e := newEnv(t)
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	e.cfg.Now = func() time.Time { return now }
	return e, &now
}

func (e *env) keysPath() string { return filepath.Join(e.home, ".ssh", "authorized_keys") }

func (e *env) keys() string {
	e.t.Helper()
	b, err := os.ReadFile(e.keysPath())
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

func TestSSHAuthorizeCreatesFile(t *testing.T) {
	e, _ := sshEnv(t)
	key := testKey(t)
	e.ok(nil, proto.ScopeUser, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: key}, nil)
	want := `restrict,pty,from="127.0.0.1,::1",expiry-time="20261017120200Z" ` + key + " maru-ephemeral\n"
	if got := e.keys(); got != want {
		t.Fatalf("authorized_keys = %q, want %q", got, want)
	}
	if m := mode(t, filepath.Join(e.home, ".ssh")); m != 0o700 {
		t.Errorf(".ssh mode = %o, want 700", m)
	}
	if m := mode(t, e.keysPath()); m != 0o600 {
		t.Errorf("authorized_keys mode = %o, want 600", m)
	}
}

func TestSSHAuthorizeKeepsOtherLines(t *testing.T) {
	e, now := sshEnv(t)
	// 사용자 키, 주석, CRLF 줄, 끝에 줄바꿈이 없는 줄은 바이트 그대로 남는다.
	user := "# my keys\r\nssh-ed25519 AAAAuser me@laptop\n\nfrom=\"10.0.0.0/8\" ssh-rsa AAAArsa work"
	e.file(e.keysPath(), user)
	os.Chmod(filepath.Join(e.home, ".ssh"), 0o700)
	k1, k2 := testKey(t), testKey(t)
	e.ok(nil, proto.ScopeUser, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: k1}, nil)
	*now = now.Add(30 * time.Second)
	e.ok(nil, proto.ScopeUser, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: k2}, nil)
	got := e.keys()
	if !strings.HasPrefix(got, user+"\n") {
		t.Fatalf("user lines changed: %q", got)
	}
	if strings.Count(got, SSHKeyMarker) != 2 {
		t.Fatalf("want two maru lines: %q", got)
	}
	// 0644로 있던 파일은 그대로 0644(그룹·다른 사용자 쓰기만 뺀다).
	if m := mode(t, e.keysPath()); m != 0o644 {
		t.Errorf("mode = %o, want 644", m)
	}

	// revoke는 그 키 줄만 지운다.
	e.ok(nil, proto.ScopeUser, proto.ActionSSHRevoke, proto.SSHKeyParams{PublicKey: k1}, nil)
	got = e.keys()
	if strings.Contains(got, strings.Fields(k1)[1]) || !strings.Contains(got, strings.Fields(k2)[1]) {
		t.Fatalf("after revoke k1: %q", got)
	}
	e.ok(nil, proto.ScopeUser, proto.ActionSSHRevoke, proto.SSHKeyParams{PublicKey: k2}, nil)
	if got := e.keys(); got != user+"\n" {
		t.Fatalf("after revoke all = %q, want user lines only", got)
	}
}

func TestSSHKeyRemovesExpiredLines(t *testing.T) {
	e, now := sshEnv(t)
	stale, fresh := testKey(t), testKey(t)
	e.ok(nil, proto.ScopeUser, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: stale}, nil)
	// 만료 시각이 지나면 다른 키를 넣을 때 함께 지운다.
	*now = now.Add(SSHKeyTTL)
	e.ok(nil, proto.ScopeUser, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: fresh}, nil)
	got := e.keys()
	if strings.Contains(got, strings.Fields(stale)[1]) || !strings.Contains(got, strings.Fields(fresh)[1]) {
		t.Fatalf("authorized_keys = %q", got)
	}
	// 만료 시각을 읽을 수 없는 maru 줄도 지운다. 같은 주석이라도 옵션이 다른 줄(사용자가 쓴 줄)은 남긴다.
	broken := `restrict,pty,from="127.0.0.1,::1",expiry-time="soon" ` + testKey(t) + " maru-ephemeral\n"
	mine := "ssh-ed25519 AAAAmine maru-ephemeral\n"
	if err := os.WriteFile(e.keysPath(), []byte(broken+mine), 0o600); err != nil {
		t.Fatal(err)
	}
	e.ok(nil, proto.ScopeUser, proto.ActionSSHRevoke, proto.SSHKeyParams{PublicKey: fresh}, nil)
	if got := e.keys(); got != mine {
		t.Fatalf("authorized_keys = %q, want %q", got, mine)
	}
}

func TestSSHRevokeWithoutFile(t *testing.T) {
	e, _ := sshEnv(t)
	e.ok(nil, proto.ScopeUser, proto.ActionSSHRevoke, proto.SSHKeyParams{PublicKey: testKey(t)}, nil)
	if _, err := os.Stat(filepath.Join(e.home, ".ssh")); !os.IsNotExist(err) {
		t.Fatalf("revoke created .ssh: %v", err)
	}
	// 바꿀 줄이 없으면 파일을 다시 쓰지 않는다.
	e.file(e.keysPath(), "ssh-ed25519 AAAAuser me\n")
	os.Chmod(filepath.Join(e.home, ".ssh"), 0o700)
	before, _ := os.Stat(e.keysPath())
	e.ok(nil, proto.ScopeUser, proto.ActionSSHRevoke, proto.SSHKeyParams{PublicKey: testKey(t)}, nil)
	after, _ := os.Stat(e.keysPath())
	if !os.SameFile(before, after) {
		t.Fatal("unchanged file was rewritten")
	}
}

func TestSSHAuthorizeStrictModes(t *testing.T) {
	e, _ := sshEnv(t)
	key := proto.SSHKeyParams{PublicKey: testKey(t)}
	// 그룹이 쓸 수 있는 홈은 고치지 않고 거절한다.
	if err := os.Chmod(e.home, 0o775); err != nil {
		t.Fatal(err)
	}
	e.fail(proto.CodeSSHStrictModes, proto.ActionSSHAuthorize, key, nil)
	if _, err := os.Stat(filepath.Join(e.home, ".ssh")); !os.IsNotExist(err) {
		t.Fatal(".ssh created despite strict modes failure")
	}
	// ~/.ssh와 파일은 사용자 소유면 그룹·다른 사용자 쓰기를 뺀다.
	os.Chmod(e.home, 0o755)
	e.file(e.keysPath(), "ssh-ed25519 AAAAuser me\n")
	os.Chmod(filepath.Join(e.home, ".ssh"), 0o777)
	os.Chmod(e.keysPath(), 0o666)
	e.ok(nil, proto.ScopeUser, proto.ActionSSHAuthorize, key, nil)
	if m := mode(t, filepath.Join(e.home, ".ssh")); m != 0o755 {
		t.Errorf(".ssh mode = %o, want 755", m)
	}
	if m := mode(t, e.keysPath()); m != 0o644 {
		t.Errorf("authorized_keys mode = %o, want 644", m)
	}
}

func TestSSHAuthorizeRefusesSymlink(t *testing.T) {
	e, _ := sshEnv(t)
	other := filepath.Join(e.home, "keys")
	e.file(other, "ssh-ed25519 AAAAuser me\n")
	os.MkdirAll(filepath.Join(e.home, ".ssh"), 0o700)
	if err := os.Symlink(other, e.keysPath()); err != nil {
		t.Fatal(err)
	}
	e.fail(proto.CodeNotRegular, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: testKey(t)}, nil)
	if b, _ := os.ReadFile(other); string(b) != "ssh-ed25519 AAAAuser me\n" {
		t.Fatalf("symlink target changed: %q", b)
	}
}

func TestSSHAuthorizeNoHome(t *testing.T) {
	e, _ := sshEnv(t)
	key := proto.SSHKeyParams{PublicKey: testKey(t)}
	for _, home := range []string{"/", "", filepath.Join(e.home, "missing")} {
		e.job.Home = home
		e.fail(proto.CodeNoHome, proto.ActionSSHAuthorize, key, nil)
	}
	e.job.Home = filepath.Join(e.home, "file")
	e.file(e.job.Home, "x")
	e.fail(proto.CodeNoHome, proto.ActionSSHAuthorize, key, nil)
}

func TestSSHAuthorizeRejectsBadKeys(t *testing.T) {
	e, _ := sshEnv(t)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	b64 := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
	good := b64(sshBlob(proto.SSHKeyType, pub))
	bad := []string{
		"",
		proto.SSHKeyType,
		proto.SSHKeyType + " " + good + " comment",
		`command="sh" ` + proto.SSHKeyType + " " + good,
		"ssh-rsa " + good,
		proto.SSHKeyType + " " + b64(sshBlob("ssh-rsa", pub)),
		proto.SSHKeyType + " " + b64(sshBlob(proto.SSHKeyType, []byte(pub[:31]))),
		proto.SSHKeyType + " " + b64(append(sshBlob(proto.SSHKeyType, pub), 0)),
		proto.SSHKeyType + " " + good + "\nssh-ed25519 " + good,
		proto.SSHKeyType + " not-base64!",
	}
	for _, k := range bad {
		e.fail(proto.CodeBadRequest, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: k}, nil)
	}
	if _, err := os.Stat(filepath.Join(e.home, ".ssh")); !os.IsNotExist(err) {
		t.Fatal("bad key created .ssh")
	}
}

func TestSSHAuthorizeConcurrent(t *testing.T) {
	e, _ := sshEnv(t)
	const n = 8
	keys := make([]string, n)
	for i := range keys {
		keys[i] = testKey(t)
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, k := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _, err := e.run(proto.ScopeUser, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: k}, nil)
			if err != nil || !resp.OK {
				errs <- fmt.Errorf("authorize: %q %v", resp.Code, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	got := e.keys()
	for _, k := range keys {
		if !strings.Contains(got, strings.Fields(k)[1]) {
			t.Errorf("key lost in concurrent authorize: %q", got)
		}
	}
}
