package security

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
)

// fakeAuthenticator는 시험용으로 합성한 인증기다. 실제 기기 출력은 쓰지 않는다.
type fakeAuthenticator struct {
	alg    int
	signer crypto.Signer
	credID []byte
	count  uint32
}

func newFakeAuthenticator(t *testing.T, alg int) *fakeAuthenticator {
	t.Helper()
	var s crypto.Signer
	var err error
	switch alg {
	case AlgES256:
		s, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case AlgEdDSA:
		_, s, err = ed25519.GenerateKey(rand.Reader)
	case AlgRS256:
		s, err = rsa.GenerateKey(rand.Reader, 2048)
	}
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	rand.Read(id)
	return &fakeAuthenticator{alg: alg, signer: s, credID: id}
}

func (a *fakeAuthenticator) spki(t *testing.T) []byte {
	b, err := x509.MarshalPKIXPublicKey(a.signer.Public())
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func clientDataJSON(typ string, challenge []byte, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": B64.EncodeToString(challenge), "origin": origin, "crossOrigin": false})
	return b
}

func authDataBytes(rpID string, flags byte, count uint32, credID []byte) []byte {
	h := sha256.Sum256([]byte(rpID))
	b := append(h[:], flags)
	b = binary.BigEndian.AppendUint32(b, count)
	if credID != nil {
		b = append(b, make([]byte, 16)...) // aaguid
		b = binary.BigEndian.AppendUint16(b, uint16(len(credID)))
		b = append(b, credID...)
		b = append(b, 0xa1, 0x01, 0x02) // COSE 키 자리(읽지 않는다)
	}
	return b
}

func (a *fakeAuthenticator) register(t *testing.T, rp RP, challenge []byte) Registration {
	return Registration{
		CredentialID:      a.credID,
		ClientDataJSON:    clientDataJSON("webauthn.create", challenge, rp.Origin),
		AuthenticatorData: authDataBytes(rp.ID, flagUP|flagUV|flagAT, a.count, a.credID),
		PublicKey:         a.spki(t),
		Algorithm:         a.alg,
	}
}

func (a *fakeAuthenticator) sign(t *testing.T, authData, cdj []byte) []byte {
	t.Helper()
	cd := sha256.Sum256(cdj)
	msg := append(append([]byte{}, authData...), cd[:]...)
	var sig []byte
	var err error
	switch a.alg {
	case AlgEdDSA:
		sig, err = a.signer.Sign(rand.Reader, msg, crypto.Hash(0))
	default:
		h := sha256.Sum256(msg)
		sig, err = a.signer.Sign(rand.Reader, h[:], crypto.SHA256)
	}
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func (a *fakeAuthenticator) assert(t *testing.T, rp RP, challenge []byte) Assertion {
	a.count++
	ad := authDataBytes(rp.ID, flagUP|flagUV, a.count, nil)
	cdj := clientDataJSON("webauthn.get", challenge, rp.Origin)
	return Assertion{CredentialID: a.credID, ClientDataJSON: cdj, AuthenticatorData: ad, Signature: a.sign(t, ad, cdj)}
}

var testRP = RP{ID: "desk.example.lan", Origin: "https://desk.example.lan"}

func TestWebAuthnAlgorithms(t *testing.T) {
	for _, alg := range SupportedAlgs {
		a := newFakeAuthenticator(t, alg)
		ch := NewChallenge()
		pk, err := testRP.VerifyRegistration(a.register(t, testRP, ch), ch)
		if err != nil {
			t.Fatalf("alg %d register: %v", alg, err)
		}
		if pk.ID != B64.EncodeToString(a.credID) || pk.Alg != alg {
			t.Errorf("alg %d passkey %+v", alg, pk)
		}
		ch = NewChallenge()
		count, err := testRP.VerifyAssertion(pk, a.assert(t, testRP, ch), ch)
		if err != nil || count != 1 {
			t.Fatalf("alg %d assert count=%d err=%v", alg, count, err)
		}
	}
}

func TestWebAuthnRegistrationRejects(t *testing.T) {
	a := newFakeAuthenticator(t, AlgES256)
	ch := NewChallenge()
	for name, mutate := range map[string]func(*Registration){
		"wrong challenge": func(r *Registration) {
			r.ClientDataJSON = clientDataJSON("webauthn.create", NewChallenge(), testRP.Origin)
		},
		"wrong type": func(r *Registration) { r.ClientDataJSON = clientDataJSON("webauthn.get", ch, testRP.Origin) },
		"wrong origin": func(r *Registration) {
			r.ClientDataJSON = clientDataJSON("webauthn.create", ch, "https://evil.example")
		},
		"wrong rp id": func(r *Registration) {
			r.AuthenticatorData = authDataBytes("evil.example", flagUP|flagUV|flagAT, 0, a.credID)
		},
		"no user verify":    func(r *Registration) { r.AuthenticatorData = authDataBytes(testRP.ID, flagUP|flagAT, 0, a.credID) },
		"no attested data":  func(r *Registration) { r.AuthenticatorData = authDataBytes(testRP.ID, flagUP|flagUV, 0, nil) },
		"credential id":     func(r *Registration) { r.CredentialID = []byte("other") },
		"no public key":     func(r *Registration) { r.PublicKey = nil },
		"algorithm":         func(r *Registration) { r.Algorithm = AlgEdDSA },
		"unsupported alg":   func(r *Registration) { r.Algorithm = -35 },
		"short auth data":   func(r *Registration) { r.AuthenticatorData = r.AuthenticatorData[:20] },
		"client data json":  func(r *Registration) { r.ClientDataJSON = []byte("{") },
		"garbage key":       func(r *Registration) { r.PublicKey = []byte{1, 2, 3} },
		"cross origin true": func(r *Registration) { r.ClientDataJSON = crossOrigin(ch) },
	} {
		reg := a.register(t, testRP, ch)
		mutate(&reg)
		if _, err := testRP.VerifyRegistration(reg, ch); !errors.Is(err, ErrVerify) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	// 작은 RSA 키는 받지 않는다.
	small, _ := rsa.GenerateKey(rand.Reader, 1024)
	weak := &fakeAuthenticator{alg: AlgRS256, signer: small, credID: []byte("weak")}
	if _, err := testRP.VerifyRegistration(weak.register(t, testRP, ch), ch); err == nil {
		t.Error("1024-bit rsa accepted")
	}
}

func crossOrigin(ch []byte) []byte {
	b, _ := json.Marshal(map[string]any{"type": "webauthn.create", "challenge": B64.EncodeToString(ch), "origin": testRP.Origin, "crossOrigin": true})
	return b
}

func TestWebAuthnAssertionRejects(t *testing.T) {
	a := newFakeAuthenticator(t, AlgES256)
	ch := NewChallenge()
	pk, err := testRP.VerifyRegistration(a.register(t, testRP, ch), ch)
	if err != nil {
		t.Fatal(err)
	}
	other := newFakeAuthenticator(t, AlgES256)
	for name, mutate := range map[string]func(*Assertion){
		"wrong challenge": func(as *Assertion) { as.ClientDataJSON = clientDataJSON("webauthn.get", NewChallenge(), testRP.Origin) },
		"wrong type":      func(as *Assertion) { as.ClientDataJSON = clientDataJSON("webauthn.create", ch, testRP.Origin) },
		"wrong origin": func(as *Assertion) {
			as.ClientDataJSON = clientDataJSON("webauthn.get", ch, "https://desk.example.lan:8443")
		},
		"no user verify": func(as *Assertion) {
			as.AuthenticatorData = authDataBytes(testRP.ID, flagUP, 9, nil)
			as.Signature = a.sign(t, as.AuthenticatorData, as.ClientDataJSON)
		},
		"no user present": func(as *Assertion) {
			as.AuthenticatorData = authDataBytes(testRP.ID, flagUV, 9, nil)
			as.Signature = a.sign(t, as.AuthenticatorData, as.ClientDataJSON)
		},
		"wrong rp id": func(as *Assertion) {
			as.AuthenticatorData = authDataBytes("example.lan", flagUP|flagUV, 9, nil)
			as.Signature = a.sign(t, as.AuthenticatorData, as.ClientDataJSON)
		},
		"other key":       func(as *Assertion) { as.Signature = other.sign(t, as.AuthenticatorData, as.ClientDataJSON) },
		"tampered data":   func(as *Assertion) { as.AuthenticatorData[36] ^= 1 },
		"empty signature": func(as *Assertion) { as.Signature = nil },
	} {
		as := a.assert(t, testRP, ch)
		mutate(&as)
		if _, err := testRP.VerifyAssertion(pk, as, ch); !errors.Is(err, ErrVerify) {
			t.Errorf("%s: err=%v", name, err)
		}
	}
	// 저장한 알고리즘과 키가 맞지 않으면 받지 않는다.
	bad := pk
	bad.Alg = AlgRS256
	if _, err := testRP.VerifyAssertion(bad, a.assert(t, testRP, ch), ch); err == nil {
		t.Error("mismatched stored algorithm accepted")
	}
}

func TestParseOrigin(t *testing.T) {
	for raw, want := range map[string]RP{
		"https://desk.example.lan":        {ID: "desk.example.lan", Origin: "https://desk.example.lan"},
		"https://Desk.Example.LAN/":       {ID: "desk.example.lan", Origin: "https://desk.example.lan"},
		"https://desk.example.lan:8443":   {ID: "desk.example.lan", Origin: "https://desk.example.lan:8443"},
		"https://desk.example.lan:443/":   {ID: "desk.example.lan", Origin: "https://desk.example.lan"},
		"https://localhost:5174":          {ID: "localhost", Origin: "https://localhost:5174"},
		"https://xn--9n2bp8q.example.lan": {ID: "xn--9n2bp8q.example.lan", Origin: "https://xn--9n2bp8q.example.lan"},
	} {
		got, err := ParseOrigin(raw)
		if err != nil || got != want {
			t.Errorf("%q: %+v %v", raw, got, err)
		}
	}
	for _, raw := range []string{
		"", "desk.example.lan", "http://desk.example.lan", "https://", "https://192.168.0.10", "https://[::1]:443",
		"https://desk.example.lan/desk/", "https://desk.example.lan?x=1", "https://desk.example.lan#a",
		"https://user@desk.example.lan", "https://desk.example.lan.", "https:desk.example.lan", "https://desk.example.lan?",
		"https://desk.example.lan:99999", "https://desk.example.lan:0",
	} {
		if _, err := ParseOrigin(raw); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
}
