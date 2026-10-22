package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP(RFC 6238)는 인증 앱이 가장 널리 받는 설정(SHA-1, 6자리, 30초)만 쓴다.
const (
	totpPeriod = 30
	totpDigits = 6
	// totpModulus는 10^totpDigits다.
	totpModulus = 1_000_000
	// totpSkew는 앞뒤로 받아 주는 시간 단계 수다(시계 차이, 입력하는 동안 바뀐 코드).
	totpSkew = 1
	// totpSecretBytes는 비밀 키 길이다(RFC 4226이 권하는 160비트).
	totpSecretBytes = 20
	// TOTPIssuer는 인증 앱에 보이는 발급자 이름이다(브랜드 이름이라 현지화하지 않는다).
	TOTPIssuer = "Maru NAS"
)

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret은 새 비밀 키를 base32(패딩 없음)로 만든다.
func NewTOTPSecret() string {
	b := make([]byte, totpSecretBytes)
	rand.Read(b)
	return totpEncoding.EncodeToString(b)
}

// TOTPURI는 인증 앱에 넣는 otpauth:// 주소다(QR 코드의 내용). account는 앱에 보이는 계정 이름이다.
func TOTPURI(secret, account string) string {
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", TOTPIssuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(totpPeriod))
	label := url.PathEscape(TOTPIssuer + ":" + account)
	// 발급자 이름의 공백은 쿼리에서도 %20으로 쓴다. Key URI 형식은 RFC 3986 인코딩이고, +를 그대로 보이는 인증 앱이 있다.
	// Encode는 + 문자 자체를 %2B로 바꾸므로 남은 +는 모두 공백이다.
	return "otpauth://totp/" + label + "?" + strings.ReplaceAll(q.Encode(), "+", "%20")
}

// totpStep은 t가 속한 시간 단계다.
func totpStep(t time.Time) int64 { return t.Unix() / totpPeriod }

// totpCode는 step의 코드다(RFC 4226 HOTP의 동적 잘라내기).
func totpCode(key []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", totpDigits, v%totpModulus)
}

func decodeTOTPSecret(secret string) ([]byte, error) {
	return totpEncoding.DecodeString(strings.ToUpper(strings.TrimRight(secret, "=")))
}

// TOTPCode는 t의 코드다. 개발용 가짜 인증 앱과 테스트가 쓴다.
func TOTPCode(secret string, t time.Time) (string, error) {
	key, err := decodeTOTPSecret(secret)
	if err != nil || len(key) == 0 {
		return "", fmt.Errorf("%w: invalid totp secret", ErrVerify)
	}
	return totpCode(key, totpStep(t)), nil
}

// NormalizeCode는 사용자가 입력한 코드의 공백·하이픈을 뺀다.
func NormalizeCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, code)
}

// VerifyTOTP는 code가 now 앞뒤 시간 단계의 코드인지 확인한다. lastStep(마지막으로 받은 단계) 이하의 단계는
// 받지 않으므로 같은 코드를 두 번 쓸 수 없다. 맞으면 받은 단계를 돌려준다(다음 lastStep).
func VerifyTOTP(secret, code string, now time.Time, lastStep int64) (int64, error) {
	key, err := decodeTOTPSecret(secret)
	if err != nil || len(key) == 0 {
		return 0, fmt.Errorf("%w: invalid totp secret", ErrVerify)
	}
	code = NormalizeCode(code)
	if len(code) != totpDigits {
		return 0, fmt.Errorf("%w: totp code length", ErrVerify)
	}
	cur := totpStep(now)
	for d := -totpSkew; d <= totpSkew; d++ {
		step := cur + int64(d)
		if subtle.ConstantTimeCompare([]byte(totpCode(key, step)), []byte(code)) == 1 {
			if step <= lastStep {
				return 0, fmt.Errorf("%w: totp code reused", ErrVerify)
			}
			return step, nil
		}
	}
	return 0, fmt.Errorf("%w: totp code mismatch", ErrVerify)
}
