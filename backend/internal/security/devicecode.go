package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// 기기 추가 코드: 수단이 있는 기기가 만들어 주고, 새 기기의 관리 확인이 등록한 수단 대신 받는 1회용 코드다
// (보관과 확인은 server의 deviceCodes). 소문자인 복구 코드와 헷갈리지 않게 대문자 8자(XXXX-XXXX)로 보인다.

// deviceCodeChars는 기기 추가 코드의 글자 수다(base32 8자 = 40비트). 화면에는 4자씩 하이픈으로 나눈다.
// 10분만 쓰고 실패는 securityLimiter가 세므로 이 길이로 충분하다.
const deviceCodeChars = 8

// 대문자 base32에는 0·1·8·9가 없어 O·I와 헷갈리지 않는다.
var deviceCodeEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewDeviceCode는 기기 추가 코드를 새로 만든다. code는 사용자에게 보일 평문(XXXX-XXXX), hash는 보관할 해시다.
func NewDeviceCode() (code, hash string) {
	b := make([]byte, 5)
	rand.Read(b)
	c := deviceCodeEncoding.EncodeToString(b)
	return c[:4] + "-" + c[4:], DeviceCodeHash(c)
}

// normalizeDeviceCode는 입력한 코드를 비교할 형태(대문자, 공백·하이픈 없음)로 바꾼다.
func normalizeDeviceCode(code string) string { return strings.ToUpper(NormalizeCode(code)) }

// DeviceCodeHash는 기기 추가 코드의 보관 형태다(정리한 코드의 SHA-256 hex).
func DeviceCodeHash(code string) string {
	sum := sha256.Sum256([]byte(normalizeDeviceCode(code)))
	return hex.EncodeToString(sum[:])
}

// MatchDeviceCode는 입력한 code가 hash로 보관한 코드인지다. 글자 수가 다르면 비교하지 않는다.
func MatchDeviceCode(hash, code string) bool {
	if len(normalizeDeviceCode(code)) != deviceCodeChars {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hash), []byte(DeviceCodeHash(code))) == 1
}
