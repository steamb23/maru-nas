package proto

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// 직접 읽는 파서와 지문이 OpenSSH 형식(x/crypto/ssh)과 같은지 확인한다. 도우미 실행 파일은 x/crypto를
// 링크하지 않고 테스트에서만 쓴다.
func TestParseSSHPublicKeyMatchesOpenSSH(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	blob, err := ParseSSHPublicKey(line)
	if err != nil {
		t.Fatalf("parse %q: %v", line, err)
	}
	if string(blob) != string(key.Marshal()) {
		t.Fatal("blob differs from ssh.PublicKey.Marshal")
	}
	if got, want := SSHFingerprint(blob), ssh.FingerprintSHA256(key); got != want {
		t.Fatalf("fingerprint = %s, want %s", got, want)
	}
	if !SameSSHKey(strings.Fields(line)[1], blob) || SameSSHKey("AAAA", blob) {
		t.Fatal("SameSSHKey mismatch")
	}
	if _, err := ParseSSHPublicKey(strings.Repeat("A", 300)); CodeOf(err) != CodeBadRequest {
		t.Fatalf("long key: %v", err)
	}
}
