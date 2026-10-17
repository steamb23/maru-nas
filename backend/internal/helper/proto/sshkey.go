package proto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"strings"
)

// SSHKeyType은 ssh.authorize가 받는 유일한 키 종류다.
const SSHKeyType = "ssh-ed25519"

// maxSSHPublicKey는 SSHKeyParams.PublicKey의 최대 길이다. ed25519 공개 키 줄은 80바이트 남짓이다.
const maxSSHPublicKey = 256

// ParseSSHPublicKey는 "ssh-ed25519 <base64>" 형식의 공개 키를 읽어 와이어 형식(blob)을 돌려준다.
// 주석·옵션·다른 키 종류는 받지 않는다. 도우미에 SSH 라이브러리를 들이지 않으려고 직접 읽는다:
// blob은 string("ssh-ed25519") + string(32바이트 키)이고 string은 4바이트 big-endian 길이 + 내용이다.
func ParseSSHPublicKey(s string) ([]byte, error) {
	if len(s) > maxSSHPublicKey {
		return nil, NewError(CodeBadRequest)
	}
	typ, b64, ok := strings.Cut(s, " ")
	if !ok || typ != SSHKeyType || b64 == "" || strings.ContainsAny(b64, " \t\r\n") {
		return nil, NewError(CodeBadRequest)
	}
	blob, err := base64.StdEncoding.Strict().DecodeString(b64)
	if err != nil {
		return nil, NewError(CodeBadRequest)
	}
	rest := blob
	name, rest, ok := sshString(rest)
	if !ok || string(name) != SSHKeyType {
		return nil, NewError(CodeBadRequest)
	}
	key, rest, ok := sshString(rest)
	if !ok || len(key) != 32 || len(rest) != 0 {
		return nil, NewError(CodeBadRequest)
	}
	return blob, nil
}

// SSHFingerprint는 blob의 OpenSSH 형식 지문("SHA256:<base64, 패딩 없음>")이다. 감사 로그에 키 대신 남긴다.
func SSHFingerprint(blob []byte) string {
	sum := sha256.Sum256(blob)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// SameSSHKey는 authorized_keys 형식의 두 키 base64가 같은 blob인지다.
func SameSSHKey(b64 string, blob []byte) bool {
	got, err := base64.StdEncoding.DecodeString(b64)
	return err == nil && bytes.Equal(got, blob)
}

func sshString(b []byte) (s, rest []byte, ok bool) {
	if len(b) < 4 {
		return nil, nil, false
	}
	n := binary.BigEndian.Uint32(b)
	if uint64(n) > uint64(len(b)-4) {
		return nil, nil, false
	}
	return b[4 : 4+n], b[4+n:], true
}
