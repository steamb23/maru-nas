package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

const (
	// RecoveryCodeCount는 한 번에 만드는 복구 코드 수다.
	RecoveryCodeCount = 10
	// recoveryCodeChars는 복구 코드 하나의 글자 수다(base32 10자 = 50비트). 화면에는 5자씩 하이픈으로 나눈다.
	recoveryCodeChars = 10
)

// 복구 코드는 소문자로 보여 준다(0·1·8·9가 없어 O·l과 헷갈리지 않는다).
var recoveryEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// NewRecoveryCodes는 복구 코드를 새로 만든다. codes는 사용자에게 한 번만 보여 줄 평문(xxxxx-xxxxx),
// hashes는 저장할 해시다. 코드가 충분히 길고 무작위라 느린 해시를 쓰지 않는다.
func NewRecoveryCodes() (codes, hashes []string) {
	for range RecoveryCodeCount {
		b := make([]byte, 7)
		rand.Read(b)
		c := recoveryEncoding.EncodeToString(b)[:recoveryCodeChars]
		codes = append(codes, c[:5]+"-"+c[5:])
		hashes = append(hashes, RecoveryHash(c))
	}
	return codes, hashes
}

// normalizeRecovery는 입력한 복구 코드를 비교할 형태(소문자, 공백·하이픈 없음)로 바꾼다.
func normalizeRecovery(code string) string { return strings.ToLower(NormalizeCode(code)) }

// RecoveryHash는 복구 코드의 저장 형태다(정리한 코드의 SHA-256 hex).
func RecoveryHash(code string) string {
	sum := sha256.Sum256([]byte(normalizeRecovery(code)))
	return hex.EncodeToString(sum[:])
}

// ConsumeRecoveryCode는 code가 hashes 중 하나면 그 해시를 뺀 목록을 돌려준다. 쓴 코드는 다시 쓸 수 없다.
func ConsumeRecoveryCode(hashes []string, code string) ([]string, bool) {
	if len(normalizeRecovery(code)) != recoveryCodeChars {
		return hashes, false
	}
	want := []byte(RecoveryHash(code))
	hit := -1
	for i, h := range hashes {
		// 맞는 코드를 찾아도 끝까지 비교해 걸리는 시간이 자리에 따라 달라지지 않게 한다.
		if subtle.ConstantTimeCompare([]byte(h), want) == 1 && hit < 0 {
			hit = i
		}
	}
	if hit < 0 {
		return hashes, false
	}
	rest := make([]string, 0, len(hashes)-1)
	rest = append(rest, hashes[:hit]...)
	return append(rest, hashes[hit+1:]...), true
}
