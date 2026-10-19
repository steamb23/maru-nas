package security

import (
	"bytes"
	"errors"
	"net/url"
	"slices"
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

func TestDeviceCode(t *testing.T) {
	code, hash := NewDeviceCode()
	if len(code) != 9 || code[4] != '-' || code != strings.ToUpper(code) {
		t.Fatalf("code %q", code)
	}
	if other, _ := NewDeviceCode(); other == code {
		t.Errorf("same code twice: %q", code)
	}
	// 소문자·공백·하이픈 없이 입력해도 맞는다.
	for _, in := range []string{code, strings.ToLower(code), strings.ReplaceAll(code, "-", ""), strings.ReplaceAll(code, "-", " ")} {
		if !MatchDeviceCode(hash, in) {
			t.Errorf("%q rejected", in)
		}
	}
	for _, bad := range []string{"", code[:4], code + "A", "AAAA-AAAA"} {
		if MatchDeviceCode(hash, bad) {
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

func TestNormalizeDropsOrphanLogins(t *testing.T) {
	r := Record{
		Passkeys:      []Passkey{{ID: "a"}},
		Handle:        []byte{1},
		PasskeyLogins: []PasskeyLogin{{ID: "x", Credential: "a"}, {ID: "y", Credential: "gone"}},
	}
	if Normalize(&r) || len(r.PasskeyLogins) != 1 || r.PasskeyLogins[0].ID != "x" {
		t.Errorf("orphan login kept: %+v", r.PasskeyLogins)
	}
	// 패스키를 모두 지우면 등록도 모두 없어진다.
	r.Passkeys = nil
	if !Normalize(&r) || r.PasskeyLogins != nil {
		t.Errorf("logins without passkeys kept: %+v", r.PasskeyLogins)
	}
}

func TestPasskeyLoginExpiry(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	day := 24 * time.Hour
	l := NewPasskeyLogin("cred", now, 30)
	if len(l.Share) != ShareBytes || l.Credential != "cred" {
		t.Fatalf("new login %+v", l)
	}
	if id, err := B64.DecodeString(l.ID); err != nil || len(id) != LoginIDBytes {
		t.Fatalf("id %q: %v", l.ID, err)
	}
	if other := NewPasskeyLogin("cred", now, 30); other.ID == l.ID || string(other.Share) == string(l.Share) {
		t.Error("ids or shares repeat")
	}
	// 지금 기간이 같으면 등록할 때 정한 시각이다. 경계 시각부터 만료다.
	if got := l.Expiry(30); !got.Equal(now.Add(30 * day)) {
		t.Errorf("expiry %v", got)
	}
	if l.Expired(now.Add(30*day-time.Second), 30) || !l.Expired(now.Add(30*day), 30) {
		t.Error("expiry boundary")
	}
	// 기간을 줄이면 기존 등록에도 바로 적용된다.
	if !l.Expired(now.Add(8*day), 7) {
		t.Error("shortened period not applied")
	}
	// 늘려도 기존 등록은 등록할 때의 시각에 만료된다.
	if got := l.Expiry(365); !got.Equal(now.Add(30 * day)) {
		t.Errorf("lengthened period extended an old login: %v", got)
	}
	// 만료 시각이 없는 기록(손으로 고친 파일)은 만료로 본다.
	if !(PasskeyLogin{Created: now}).Expired(now, 30) {
		t.Error("login without expiry accepted")
	}

	r := Record{PasskeyLogins: []PasskeyLogin{l, NewPasskeyLogin("cred", now.Add(-31*day), 30)}}
	if n := r.PruneExpiredLogins(now, 30); n != 1 || len(r.PasskeyLogins) != 1 || r.PasskeyLogins[0].ID != l.ID {
		t.Errorf("prune removed %d, left %+v", n, r.PasskeyLogins)
	}
	if r.PasskeyLogin(l.ID) == nil || !r.DeletePasskeyLogin(l.ID) || r.DeletePasskeyLogin(l.ID) || r.PasskeyLogin(l.ID) != nil {
		t.Error("lookup or delete")
	}
}

func TestCloneLogins(t *testing.T) {
	r := Record{PasskeyLogins: []PasskeyLogin{{ID: "x", Share: []byte{1, 2}, NextShare: []byte{3, 4}}}}
	c := r.Clone()
	c.PasskeyLogins[0].Share[0] = 9
	c.PasskeyLogins[0].NextShare[0] = 9
	c.PasskeyLogins[0].ID = "y"
	if l := r.PasskeyLogins[0]; l.Share[0] != 1 || l.NextShare[0] != 3 || l.ID != "x" {
		t.Errorf("clone shares memory: %+v", l)
	}
	// 기다리는 조각이 없으면 복사본에도 없다.
	if c := (Record{PasskeyLogins: []PasskeyLogin{{ID: "x"}}}).Clone(); c.PasskeyLogins[0].NextShare != nil {
		t.Errorf("clone made a next share: %+v", c.PasskeyLogins[0])
	}
}

func TestPasskeyLoginRotation(t *testing.T) {
	now := time.Unix(1000, 0)
	l := NewPasskeyLogin("cred", now, 30)
	first, expires := slices.Clone(l.Share), l.ExpiresAt
	if l.NextShare != nil || l.Rotate() || !bytes.Equal(l.Share, first) {
		t.Fatalf("rotated without a pending share: %+v", l)
	}
	// 기다리는 조각은 확정할 때까지 그대로다.
	l.PrepareRotation()
	next := slices.Clone(l.NextShare)
	if len(next) != ShareBytes || bytes.Equal(next, first) {
		t.Fatalf("next share = %x", next)
	}
	l.PrepareRotation()
	if !bytes.Equal(l.NextShare, next) {
		t.Fatal("pending share changed before rotation")
	}
	if !l.Rotate() || !bytes.Equal(l.Share, next) || l.NextShare != nil || !l.ExpiresAt.Equal(expires) {
		t.Fatalf("after rotate: %+v", l)
	}
	l.PrepareRotation()
	if bytes.Equal(l.NextShare, next) || len(l.NextShare) != ShareBytes {
		t.Fatalf("next share reused after rotation: %x", l.NextShare)
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
	// TakeIf는 조건이 맞을 때만 꺼내 지운다.
	e.Put("c", "3")
	if _, ok := e.TakeIf("c", func(v string) bool { return v == "x" }); ok {
		t.Error("taken although the condition failed")
	}
	if v, ok := e.TakeIf("c", func(v string) bool { return v == "3" }); !ok || v != "3" {
		t.Fatalf("take if %q %v", v, ok)
	}
	if _, ok := e.Peek("c"); ok {
		t.Error("value left after take if")
	}
	e.Put("b", "2")
	now = now.Add(time.Minute)
	if _, ok := e.Take("b"); ok {
		t.Error("expired value taken")
	}
	e.Put("d", "4")
	now = now.Add(time.Minute)
	if _, ok := e.TakeIf("d", func(string) bool { return true }); ok {
		t.Error("expired value taken by take if")
	}
	for i := range maxEphemeral + 5 {
		e.Put(string(rune('a'+i%26))+strings.Repeat("x", i), "v")
		now = now.Add(time.Millisecond)
	}
	if len(e.m) > maxEphemeral {
		t.Errorf("%d entries", len(e.m))
	}
}
