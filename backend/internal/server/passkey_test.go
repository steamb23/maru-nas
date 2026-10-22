package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/security"
)

var testRP = &security.RP{ID: "desk.example.lan", Origin: "https://desk.example.lan"}

// fakePasskey는 시험용으로 합성한 패스키(ES256)다. 브라우저가 백엔드에 보내는 JSON을 만든다.
type fakePasskey struct {
	t      *testing.T
	key    *ecdsa.PrivateKey
	id     []byte
	handle []byte
	count  uint32
	// noUV는 등록 응답에는 UV를 켜지만 확인 응답에는 켜지 않는 저장소를 흉내 낸다.
	noUV bool
	// synced는 동기화 저장소(BE, BS 플래그)를 흉내 낸다.
	synced bool
}

func newFakePasskey(t *testing.T) *fakePasskey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	rand.Read(id)
	return &fakePasskey{t: t, key: k, id: id}
}

func (p *fakePasskey) clientData(typ, challenge string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": testRP.Origin})
	return b
}

func (p *fakePasskey) authData(attested bool) []byte {
	h := sha256.Sum256([]byte(testRP.ID))
	flags := byte(0x01)
	if attested || !p.noUV {
		flags |= 0x04
	}
	if p.synced {
		flags |= 0x18
	}
	if attested {
		flags |= 0x40
	}
	b := binary.BigEndian.AppendUint32(append(h[:], flags), p.count)
	if attested {
		b = append(b, make([]byte, 16)...)
		b = binary.BigEndian.AppendUint16(b, uint16(len(p.id)))
		b = append(b, p.id...)
		b = append(b, 0xa0)
	}
	return b
}

// create는 등록 옵션으로 navigator.credentials.create()의 결과를 만든다.
func (p *fakePasskey) create(opts api.PasskeyOptions) *api.PasskeyRegistration {
	if opts.RPID != testRP.ID || opts.User == nil || len(opts.Algorithms) == 0 {
		p.t.Fatalf("registration options = %+v", opts)
	}
	p.handle, _ = security.B64.DecodeString(opts.User.ID)
	spki, _ := x509.MarshalPKIXPublicKey(&p.key.PublicKey)
	return &api.PasskeyRegistration{
		ID:                 security.B64.EncodeToString(p.id),
		ClientDataJSON:     security.B64.EncodeToString(p.clientData("webauthn.create", opts.Challenge)),
		AuthenticatorData:  security.B64.EncodeToString(p.authData(true)),
		PublicKey:          security.B64.EncodeToString(spki),
		PublicKeyAlgorithm: security.AlgES256,
		Transports:         []string{"internal", "bogus"},
	}
}

// get은 확인 옵션으로 navigator.credentials.get()의 결과를 만든다.
func (p *fakePasskey) get(opts api.PasskeyOptions) *api.PasskeyAssertion {
	p.count++
	ad := p.authData(false)
	cd := p.clientData("webauthn.get", opts.Challenge)
	sum := sha256.Sum256(cd)
	h := sha256.Sum256(append(append([]byte{}, ad...), sum[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, p.key, h[:])
	if err != nil {
		p.t.Fatal(err)
	}
	return &api.PasskeyAssertion{
		ID:                security.B64.EncodeToString(p.id),
		ClientDataJSON:    security.B64.EncodeToString(cd),
		AuthenticatorData: security.B64.EncodeToString(ad),
		Signature:         security.B64.EncodeToString(sig),
		UserHandle:        security.B64.EncodeToString(p.handle),
	}
}

func (b *browser) passkeyOptions(purpose string) api.PasskeyOptions {
	b.e.t.Helper()
	rec := b.do(http.MethodPost, "/api/security/passkeys/challenge", api.PasskeyChallengeRequest{Purpose: purpose})
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.PasskeyOptions](b.e.t, rec)
}

// createPasskey는 새 패스키의 등록 값을 보내고 시험 확인 옵션을 받는다(관리 창 필요).
func (b *browser) createPasskey(pk *fakePasskey, name string) api.PasskeyOptions {
	b.e.t.Helper()
	rec := b.do(http.MethodPost, "/api/security/passkeys", api.PasskeyRegisterRequest{Name: name, Credential: pk.create(b.passkeyOptions(api.PasskeyRegister))})
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.PasskeyOptions](b.e.t, rec)
}

// enrollPasskey는 등록과 시험 확인을 거쳐 패스키를 저장한다(관리 창 필요).
func (b *browser) enrollPasskey(pk *fakePasskey, name string) api.SecurityChangeResponse {
	b.e.t.Helper()
	rec := b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: pk.get(b.createPasskey(pk, name))})
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.SecurityChangeResponse](b.e.t, rec)
}

func TestPasskeyUnavailableWithoutOrigin(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/challenge", api.PasskeyChallengeRequest{Purpose: api.PasskeyAssert}),
		http.StatusConflict, api.ErrPasskeyUnavailable)
	wantCode(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: &api.PasskeyAssertion{}}),
		http.StatusConflict, api.ErrPasskeyUnavailable)
}

func TestPasskeyRegisterAndVerify(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if st := b.securityStatus(); !st.PasskeysAvailable {
		t.Fatal("passkeys unavailable with an origin")
	}
	// 등록은 관리 창이 필요하다.
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/challenge", api.PasskeyChallengeRequest{Purpose: api.PasskeyRegister}),
		http.StatusForbidden, api.ErrManageRequired)
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/challenge", api.PasskeyChallengeRequest{Purpose: api.PasskeyAssert}),
		http.StatusBadRequest, api.ErrBadRequest)
	wantStatus(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusOK)

	pk := newFakePasskey(t)
	opts := b.passkeyOptions(api.PasskeyRegister)
	if opts.User.Name != "user" || opts.RPName != "Onmaru" || len(opts.Credentials) != 0 {
		t.Fatalf("options = %+v", opts)
	}
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys", api.PasskeyRegisterRequest{Name: " ", Credential: pk.create(opts)}),
		http.StatusBadRequest, api.ErrBadRequest)
	cred := pk.create(opts)
	rec := b.do(http.MethodPost, "/api/security/passkeys", api.PasskeyRegisterRequest{Name: " laptop ", Credential: cred})
	wantStatus(t, rec, http.StatusOK)
	// 등록 값만으로는 저장하지 않고, 새 자격 증명 하나로 시험 확인할 옵션을 준다.
	confirm := decode[api.PasskeyOptions](t, rec)
	if confirm.RPID != testRP.ID || confirm.User != nil || len(confirm.Credentials) != 1 || confirm.Credentials[0].ID != cred.ID ||
		confirm.Challenge == opts.Challenge {
		t.Fatalf("confirm options = %+v", confirm)
	}
	if st := b.securityStatus(); len(st.Passkeys) != 0 || st.Assurance == api.AssuranceTwoFactor {
		t.Fatalf("saved before test verification: %+v", st)
	}
	// challenge는 한 번만 쓴다.
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys", api.PasskeyRegisterRequest{Name: "again", Credential: pk.create(opts)}),
		http.StatusBadRequest, api.ErrFactorFailed)
	rec = b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: pk.get(confirm)})
	wantStatus(t, rec, http.StatusOK)
	ch := decode[api.SecurityChangeResponse](t, rec)
	if len(ch.RecoveryCodes) != security.RecoveryCodeCount || len(ch.Status.Passkeys) != 1 || ch.Status.Passkeys[0].Name != "laptop" ||
		ch.Status.Passkeys[0].BackupEligible || ch.Status.Assurance != api.AssuranceTwoFactor || ch.Status.Methods[0] != api.FactorPasskey {
		t.Fatalf("register = %+v", ch)
	}
	// 시험 확인도 한 번만 쓴다.
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: pk.get(confirm)}),
		http.StatusBadRequest, api.ErrFactorFailed)
	// 같은 자격 증명은 다시 등록하지 않는다. 이미 등록한 것은 excludeCredentials로 알린다.
	opts = b.passkeyOptions(api.PasskeyRegister)
	if len(opts.Credentials) != 1 || opts.Credentials[0].ID != ch.Status.Passkeys[0].ID ||
		len(opts.Credentials[0].Transports) != 1 || opts.Credentials[0].Transports[0] != "internal" {
		t.Fatalf("exclude = %+v", opts.Credentials)
	}
	wantStatus(t, b.do(http.MethodPost, "/api/security/passkeys", api.PasskeyRegisterRequest{Name: "dup", Credential: pk.create(opts)}), http.StatusConflict)

	// 다른 브라우저에서 패스키로 확인한다.
	o := e.browser()
	wantStatus(t, o.login("user", "user"), http.StatusOK)
	opts = o.passkeyOptions(api.PasskeyAssert)
	assertion := pk.get(opts)
	rec = o.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: assertion})
	wantStatus(t, rec, http.StatusOK)
	st := decode[api.SecurityStatus](t, rec)
	if st.Assurance != api.AssuranceTwoFactor || st.Passkeys[0].LastUsed == "" {
		t.Fatalf("verify = %+v", st)
	}
	// 같은 확인 값(쓴 challenge)은 다시 받지 않는다.
	wantCode(t, o.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: assertion}),
		http.StatusBadRequest, api.ErrFactorFailed)
	// 다른 세션의 challenge로 만든 값도 받지 않는다.
	p := e.browser()
	wantStatus(t, p.login("user", "user"), http.StatusOK)
	p.passkeyOptions(api.PasskeyAssert)
	wantCode(t, p.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: pk.get(o.passkeyOptions(api.PasskeyAssert))}),
		http.StatusBadRequest, api.ErrFactorFailed)
	// 다른 사용자의 핸들이면 받지 않는다.
	forged := pk.get(p.passkeyOptions(api.PasskeyAssert))
	forged.UserHandle = security.B64.EncodeToString([]byte("someone-else"))
	wantCode(t, p.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: forged}),
		http.StatusBadRequest, api.ErrFactorFailed)

	// 관리 확인도 패스키로 하고, 지우면 복구 코드도 없어진다.
	q := e.browser()
	wantStatus(t, q.login("user", "user"), http.StatusOK)
	wantStatus(t, q.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{
		Password: "user", Factor: &api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: pk.get(q.passkeyOptions(api.PasskeyAssert))},
	}), http.StatusOK)
	wantStatus(t, q.do(http.MethodDelete, "/api/security/passkeys/nope", nil), http.StatusNotFound)
	rec = q.do(http.MethodDelete, "/api/security/passkeys/"+st.Passkeys[0].ID, nil)
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.SecurityChangeResponse](t, rec).Status; len(st.Passkeys) != 0 || st.RecoveryCodes != 0 {
		t.Fatalf("after delete = %+v", st)
	}
}

func TestPasskeyLoginStepUp(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantStatus(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusOK)
	pk := newFakePasskey(t)
	b.enrollPasskey(pk, "key")
	e.setPolicy(api.SecurityPolicy{Terminal: api.SecurityTwoFactor})

	o := e.browser()
	got := decode[api.LoginResponse](t, o.login("user", "user"))
	if got.StepUp == nil || got.StepUp.Methods[0] != api.FactorPasskey {
		t.Fatalf("login = %+v", got)
	}
	wantStatus(t, o.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{
		Method: api.FactorPasskey, Passkey: pk.get(o.passkeyOptions(api.PasskeyAssert)),
	}), http.StatusOK)
	if s := decode[api.SessionInfo](t, o.do(http.MethodGet, "/api/session", nil)); s.Assurance != api.AssuranceTwoFactor {
		t.Fatalf("session = %+v", s)
	}
}

// 등록할 때만 UV를 켜는 저장소의 패스키는 시험 확인에서 걸러 내고 저장하지 않는다. 값을 맞히려는 시도가 아니므로
// 실패 제한에 세지 않는다.
func TestPasskeyConfirmRejectsMissingUV(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantStatus(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusOK)

	// 실패 제한(5번)보다 많이 거절돼도 잠기지 않는다.
	for range 6 {
		pk := newFakePasskey(t)
		pk.noUV, pk.synced = true, true
		wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: pk.get(b.createPasskey(pk, "synced"))}),
			http.StatusBadRequest, api.ErrPasskeyUVUnsupported)
	}
	if st := b.securityStatus(); len(st.Passkeys) != 0 || st.RecoveryCodes != 0 || st.Assurance == api.AssuranceTwoFactor {
		t.Fatalf("after rejected passkeys = %+v", st)
	}
	// 서명이 틀리면 UV와 상관없이 틀린 값이다.
	pk := newFakePasskey(t)
	pk.noUV = true
	assertion := pk.get(b.createPasskey(pk, "forged"))
	assertion.Signature = security.B64.EncodeToString([]byte("not a signature"))
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: assertion}),
		http.StatusBadRequest, api.ErrFactorFailed)

	// UV를 알리는 저장소는 등록된다. 동기화 저장소면 표시한다.
	good := newFakePasskey(t)
	good.synced = true
	ch := b.enrollPasskey(good, "phone")
	if len(ch.Status.Passkeys) != 1 || !ch.Status.Passkeys[0].BackupEligible || len(ch.RecoveryCodes) != security.RecoveryCodeCount {
		t.Fatalf("register = %+v", ch)
	}
}

func TestPasskeyConfirmPending(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP, omvTimeout: time.Hour})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	// 시험 확인도 관리 창이 필요하다.
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: &api.PasskeyAssertion{}}),
		http.StatusForbidden, api.ErrManageRequired)
	wantStatus(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusOK)

	// 시험 확인하지 않은 채 대기가 끝나면 저장하지 않는다.
	pk := newFakePasskey(t)
	confirm := b.createPasskey(pk, "late")
	e.clk.Add(challengeTTL + time.Second)
	wantStatus(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusOK)
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: pk.get(confirm)}),
		http.StatusBadRequest, api.ErrFactorFailed)
	if st := b.securityStatus(); len(st.Passkeys) != 0 {
		t.Fatalf("expired registration saved: %+v", st)
	}

	// 대기는 그 세션의 것이다. 같은 사용자의 다른 세션도 쓸 수 없다.
	pk = newFakePasskey(t)
	confirm = b.createPasskey(pk, "mine")
	assertion := pk.get(confirm)
	o := e.browser()
	wantStatus(t, o.login("user", "user"), http.StatusOK)
	wantStatus(t, o.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusOK)
	wantCode(t, o.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: assertion}),
		http.StatusBadRequest, api.ErrFactorFailed)
	if st := o.securityStatus(); len(st.Passkeys) != 0 || st.Assurance == api.AssuranceTwoFactor {
		t.Fatalf("other session used the pending passkey: %+v", st)
	}
	// 다른 자격 증명으로 한 확인은 받지 않고, 대기도 끝난다.
	other := newFakePasskey(t)
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: other.get(confirm)}),
		http.StatusBadRequest, api.ErrFactorFailed)
	wantCode(t, b.do(http.MethodPost, "/api/security/passkeys/confirm", api.PasskeyConfirmRequest{Passkey: assertion}),
		http.StatusBadRequest, api.ErrFactorFailed)

	// 처음부터 다시 하면 등록된다.
	if ch := b.enrollPasskey(pk, "mine"); len(ch.Status.Passkeys) != 1 || ch.Status.Passkeys[0].Name != "mine" {
		t.Fatalf("register = %+v", ch)
	}
}
