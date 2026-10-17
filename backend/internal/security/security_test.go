package security

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

// rfcSecret은 RFC 6238 부록 B의 공개 시험 비밀("12345678901234567890")의 base32다.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestTOTPVectors(t *testing.T) {
	// RFC 6238 부록 B(SHA-1, 8자리)의 값을 10^6으로 나눈 나머지다.
	for _, c := range []struct {
		unix int64
		code string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		key, _ := decodeTOTPSecret(rfcSecret)
		if got := totpCode(key, c.unix/totpPeriod); got != c.code {
			t.Errorf("T=%d: got %s, want %s", c.unix, got, c.code)
		}
		step, err := VerifyTOTP(rfcSecret, c.code, time.Unix(c.unix, 0), 0)
		if err != nil || step != c.unix/totpPeriod {
			t.Errorf("T=%d: verify step=%d err=%v", c.unix, step, err)
		}
	}
}

func TestVerifyTOTP(t *testing.T) {
	now := time.Unix(1111111111, 0)
	key, _ := decodeTOTPSecret(rfcSecret)
	cur := totpStep(now)

	// 앞뒤 한 단계까지 받고, 공백·하이픈은 무시한다.
	for _, d := range []int64{-1, 0, 1} {
		code := totpCode(key, cur+d)
		if _, err := VerifyTOTP(rfcSecret, code[:3]+" "+code[3:], now, 0); err != nil {
			t.Errorf("skew %d: %v", d, err)
		}
	}
	for _, d := range []int64{-2, 2} {
		if _, err := VerifyTOTP(rfcSecret, totpCode(key, cur+d), now, 0); !errors.Is(err, ErrVerify) {
			t.Errorf("skew %d accepted: %v", d, err)
		}
	}
	// 받은 단계 이하는 다시 받지 않는다.
	step, err := VerifyTOTP(rfcSecret, totpCode(key, cur), now, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyTOTP(rfcSecret, totpCode(key, cur), now, step); err == nil {
		t.Error("reused code accepted")
	}
	if _, err := VerifyTOTP(rfcSecret, totpCode(key, cur-1), now, step); err == nil {
		t.Error("older code accepted after newer one")
	}
	if _, err := VerifyTOTP(rfcSecret, totpCode(key, cur+1), now, step); err != nil {
		t.Errorf("next code: %v", err)
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, err := VerifyTOTP(rfcSecret, bad, now, 0); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := VerifyTOTP("not base32!", "123456", now, 0); err == nil {
		t.Error("bad secret accepted")
	}
}

func TestNewTOTPSecret(t *testing.T) {
	a, b := NewTOTPSecret(), NewTOTPSecret()
	key, err := decodeTOTPSecret(a)
	if err != nil || len(key) != totpSecretBytes || a == b || strings.Contains(a, "=") {
		t.Fatalf("secret %q %q err=%v", a, b, err)
	}
}

func TestTOTPURI(t *testing.T) {
	u, err := url.Parse(TOTPURI(rfcSecret, "alice@nas"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" || u.Path != "/Onmaru:alice@nas" {
		t.Errorf("uri %v", u)
	}
	q := u.Query()
	if q.Get("secret") != rfcSecret || q.Get("issuer") != "Onmaru" || q.Get("digits") != "6" || q.Get("period") != "30" {
		t.Errorf("query %v", q)
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes := NewRecoveryCodes()
	if len(codes) != RecoveryCodeCount || len(hashes) != RecoveryCodeCount {
		t.Fatalf("%d codes, %d hashes", len(codes), len(hashes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 11 || c[5] != '-' || c != strings.ToLower(c) || seen[c] {
			t.Errorf("code %q", c)
		}
		seen[c] = true
	}
	// 대문자·공백으로 입력해도 맞고, 쓴 코드는 다시 쓸 수 없다.
	in := strings.ToUpper(strings.ReplaceAll(codes[3], "-", " "))
	rest, ok := ConsumeRecoveryCode(hashes, in)
	if !ok || len(rest) != RecoveryCodeCount-1 {
		t.Fatalf("consume ok=%v left=%d", ok, len(rest))
	}
	if _, ok := ConsumeRecoveryCode(rest, codes[3]); ok {
		t.Error("used code accepted again")
	}
	if len(hashes) != RecoveryCodeCount {
		t.Error("consume changed the input slice length")
	}
	for _, bad := range []string{"", "abcde", "zzzzz-zzzzz"} {
		if _, ok := ConsumeRecoveryCode(rest, bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestNormalize(t *testing.T) {
	r := Record{Recovery: []string{"h"}, Handle: []byte{1}, Passkeys: []Passkey{}}
	if !Normalize(&r) || r.Recovery != nil || r.Handle != nil || r.Passkeys != nil {
		t.Errorf("record without methods kept %+v", r)
	}
	r = Record{TOTP: &TOTP{Secret: rfcSecret}, Recovery: []string{"h"}, Handle: []byte{1}}
	if Normalize(&r) || len(r.Recovery) != 1 || r.Handle != nil {
		t.Errorf("record with totp %+v", r)
	}
}

func TestEphemeral(t *testing.T) {
	now := time.Unix(1000, 0)
	e := NewEphemeral[string](time.Minute)
	e.Now = func() time.Time { return now }
	e.Put("a", "1")
	if v, ok := e.Peek("a"); !ok || v != "1" {
		t.Fatalf("peek %q %v", v, ok)
	}
	if v, ok := e.Take("a"); !ok || v != "1" {
		t.Fatalf("take %q %v", v, ok)
	}
	if _, ok := e.Take("a"); ok {
		t.Error("taken twice")
	}
	e.Put("b", "2")
	now = now.Add(time.Minute)
	if _, ok := e.Take("b"); ok {
		t.Error("expired value taken")
	}
	for i := range maxEphemeral + 5 {
		e.Put(string(rune('a'+i%26))+strings.Repeat("x", i), "v")
		now = now.Add(time.Millisecond)
	}
	if len(e.m) > maxEphemeral {
		t.Errorf("%d entries", len(e.m))
	}
}
