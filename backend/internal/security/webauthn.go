package security

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"onmaru/backend/config"
)

// WebAuthn(패스키)을 CBOR 파서 없이 처리한다.
//   - 등록: 증명(attestation)은 none으로 받는다. 공개 키는 브라우저의 getPublicKey()(SPKI DER)를, 인증기 데이터는
//     getAuthenticatorData()를 쓰므로 attestationObject를 풀지 않는다. 증명이 없으므로 등록 자체는 비밀번호와
//     기존 수단 확인(관리 창)으로 막는다.
//   - 확인: clientDataJSON(type, challenge, origin), 인증기 데이터(rpIdHash, 사용자 확인), 서명을 확인한다.
//     사용자 확인(UV)은 서명까지 맞은 뒤에 본다. UV만 빠진 값(ErrUserNotVerified)은 저장소가 UV를 알려 주지 않는
//     경우라 등록 확인에서 따로 알린다.

// COSE 알고리즘 번호
const (
	AlgES256 = -7
	AlgEdDSA = -8
	AlgRS256 = -257
)

// SupportedAlgs는 등록할 때 브라우저에 알려 주는 알고리즘이다(선호 순).
var SupportedAlgs = []int{AlgES256, AlgEdDSA, AlgRS256}

// 인증기 데이터의 플래그
const (
	flagUP = 0x01 // 사용자 있음
	flagUV = 0x04 // 사용자 확인(PIN, 생체)
	flagBE = 0x08 // 백업 가능(동기화 저장소의 패스키)
	flagBS = 0x10 // 백업됨
	flagAT = 0x40 // 자격 증명 데이터 있음(등록)
)

// ErrUserNotVerified는 서명은 맞지만 인증기가 사용자 확인(UV)을 알리지 않았다는 뜻이다. ErrVerify를 감싼다.
var ErrUserNotVerified = fmt.Errorf("%w: user not verified", ErrVerify)

// Flags는 인증기 데이터의 플래그 바이트다. 진단 로그에 쓴다(비밀 값이 아니다).
type Flags byte

func (f Flags) String() string {
	bit := func(m byte) int {
		if byte(f)&m != 0 {
			return 1
		}
		return 0
	}
	return fmt.Sprintf("UP=%d UV=%d BE=%d BS=%d", bit(flagUP), bit(flagUV), bit(flagBE), bit(flagBS))
}

// AuthFlags는 인증기 데이터의 플래그다. 너무 짧으면 false다.
func AuthFlags(authData []byte) (Flags, bool) {
	if len(authData) < authDataMinLen {
		return 0, false
	}
	return Flags(authData[32]), true
}

// AAGUID는 등록한 인증기의 종류(AAGUID)를 UUID 문자열로 준다. 증명 none에서는 브라우저가 0으로 지울 수 있어
// 믿을 수 없으므로 로그에만 쓴다. 없거나 0이면 빈 문자열이다.
func AAGUID(authData []byte) string {
	if len(authData) < authDataMinLen+16 || authData[32]&flagAT == 0 {
		return ""
	}
	g := authData[authDataMinLen : authDataMinLen+16]
	if bytes.Equal(g, make([]byte, 16)) {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", g[0:4], g[4:6], g[6:8], g[8:10], g[10:16])
}

const (
	// ChallengeBytes는 challenge 길이다.
	ChallengeBytes = 32
	// maxCredentialID는 자격 증명 ID의 최대 길이다(WebAuthn 명세의 1023바이트).
	maxCredentialID = 1023
	// minRSABits는 받는 RSA 키의 최소 크기다.
	minRSABits = 2048
)

// RP는 WebAuthn 신뢰 당사자(이 대시보드)다. 공개 주소(-public-origin)에서 정한다.
type RP struct {
	// ID는 RP ID다. 공개 주소의 호스트 이름이다.
	ID string
	// Origin은 브라우저가 clientDataJSON에 넣는 출처(https://호스트[:포트])다.
	Origin string
}

// ParseOrigin은 공개 주소(-public-origin)를 검사해 RP를 만든다. 검사 규칙은 config.ParsePublicOrigin이다
// (https://호스트[:포트]만 받고 IP 주소는 RP ID가 될 수 없으므로 거부한다).
func ParseOrigin(raw string) (RP, error) {
	id, origin, err := config.ParsePublicOrigin(raw)
	if err != nil {
		return RP{}, err
	}
	return RP{ID: id, Origin: origin}, nil
}

// NewChallenge는 새 challenge를 만든다.
func NewChallenge() []byte {
	b := make([]byte, ChallengeBytes)
	rand.Read(b)
	return b
}

// NewHandle은 새 사용자 핸들(Record.Handle)을 만든다.
func NewHandle() []byte {
	b := make([]byte, 16)
	rand.Read(b)
	return b
}

// B64는 WebAuthn이 쓰는 base64url(패딩 없음)이다.
var B64 = base64.RawURLEncoding

// Registration은 navigator.credentials.create()의 결과다(브라우저가 base64url로 보낸 값을 푼 것).
type Registration struct {
	// CredentialID는 rawId다.
	CredentialID      []byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	// PublicKey는 getPublicKey()의 SPKI DER다. 브라우저가 알고리즘을 모르면 비어 있다.
	PublicKey []byte
	// Algorithm은 getPublicKeyAlgorithm()이다.
	Algorithm int
}

// Assertion은 navigator.credentials.get()의 결과다.
type Assertion struct {
	CredentialID      []byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
	UserHandle        []byte
}

func verifyErr(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrVerify}, args...)...)
}

// VerifyRegistration은 등록 결과를 확인하고 저장할 패스키를 만든다(Name, Created는 부르는 쪽이 채운다).
func (rp RP) VerifyRegistration(reg Registration, challenge []byte) (Passkey, error) {
	if err := rp.checkClientData(reg.ClientDataJSON, "webauthn.create", challenge); err != nil {
		return Passkey{}, err
	}
	ad, err := rp.checkAuthData(reg.AuthenticatorData)
	if err != nil {
		return Passkey{}, err
	}
	if ad.flags&flagUV == 0 {
		return Passkey{}, ErrUserNotVerified
	}
	if ad.flags&flagAT == 0 {
		return Passkey{}, verifyErr("no attested credential data")
	}
	// 자격 증명 데이터: aaguid(16) | ID 길이(2) | ID | 공개 키(COSE, 읽지 않음)
	rest := reg.AuthenticatorData[authDataMinLen:]
	if len(rest) < 18 {
		return Passkey{}, verifyErr("short attested credential data")
	}
	n := int(binary.BigEndian.Uint16(rest[16:18]))
	if n == 0 || n > maxCredentialID || len(rest) < 18+n {
		return Passkey{}, verifyErr("bad credential id length")
	}
	if !bytes.Equal(rest[18:18+n], reg.CredentialID) {
		return Passkey{}, verifyErr("credential id mismatch")
	}
	if len(reg.PublicKey) == 0 {
		return Passkey{}, verifyErr("browser did not provide the public key (unsupported algorithm)")
	}
	pub, err := x509.ParsePKIXPublicKey(reg.PublicKey)
	if err != nil {
		return Passkey{}, verifyErr("parse public key: %v", err)
	}
	if err := checkKey(pub, reg.Algorithm); err != nil {
		return Passkey{}, err
	}
	return Passkey{
		ID:             B64.EncodeToString(reg.CredentialID),
		PublicKey:      bytes.Clone(reg.PublicKey),
		Alg:            reg.Algorithm,
		SignCount:      ad.signCount,
		BackupEligible: ad.flags&flagBE != 0,
	}, nil
}

// VerifyAssertion은 pk로 만든 서명인지 확인하고 인증기의 서명 횟수를 돌려준다. 자격 증명 ID가 pk와 같은지,
// userHandle이 맞는지는 부르는 쪽이 확인한다. 서명까지 맞고 UV만 없으면 ErrUserNotVerified다(플래그는 서명한
// 인증기 데이터 안에 있어 바꿀 수 없다).
func (rp RP) VerifyAssertion(pk Passkey, a Assertion, challenge []byte) (uint32, error) {
	if err := rp.checkClientData(a.ClientDataJSON, "webauthn.get", challenge); err != nil {
		return 0, err
	}
	ad, err := rp.checkAuthData(a.AuthenticatorData)
	if err != nil {
		return 0, err
	}
	pub, err := x509.ParsePKIXPublicKey(pk.PublicKey)
	if err != nil {
		return 0, verifyErr("stored public key: %v", err)
	}
	if err := checkKey(pub, pk.Alg); err != nil {
		return 0, err
	}
	cd := sha256.Sum256(a.ClientDataJSON)
	msg := append(bytes.Clone(a.AuthenticatorData), cd[:]...)
	if !verifySignature(pub, pk.Alg, msg, a.Signature) {
		return 0, verifyErr("bad signature")
	}
	if ad.flags&flagUV == 0 {
		return 0, ErrUserNotVerified
	}
	return ad.signCount, nil
}

type clientData struct {
	Type        string `json:"type"`
	Challenge   string `json:"challenge"`
	Origin      string `json:"origin"`
	CrossOrigin bool   `json:"crossOrigin"`
}

func (rp RP) checkClientData(raw []byte, typ string, challenge []byte) error {
	var cd clientData
	if err := json.Unmarshal(raw, &cd); err != nil {
		return verifyErr("client data is not JSON")
	}
	if cd.Type != typ {
		return verifyErr("client data type %q", cd.Type)
	}
	got, err := B64.DecodeString(cd.Challenge)
	if err != nil || len(challenge) == 0 || subtle.ConstantTimeCompare(got, challenge) != 1 {
		return verifyErr("challenge mismatch")
	}
	if cd.Origin != rp.Origin {
		return verifyErr("origin %q", cd.Origin)
	}
	if cd.CrossOrigin {
		return verifyErr("cross-origin request")
	}
	return nil
}

// authDataMinLen은 rpIdHash(32) + flags(1) + signCount(4)다.
const authDataMinLen = 37

type authData struct {
	flags     byte
	signCount uint32
}

// checkAuthData는 인증기 데이터의 rpIdHash와 사용자 있음(UP)을 확인한다. UV(필수)는 부르는 쪽이 본다.
func (rp RP) checkAuthData(b []byte) (authData, error) {
	if len(b) < authDataMinLen {
		return authData{}, verifyErr("short authenticator data")
	}
	want := sha256.Sum256([]byte(rp.ID))
	if subtle.ConstantTimeCompare(b[:32], want[:]) != 1 {
		return authData{}, verifyErr("rp id hash mismatch")
	}
	ad := authData{flags: b[32], signCount: binary.BigEndian.Uint32(b[33:37])}
	if ad.flags&flagUP == 0 {
		return authData{}, verifyErr("user not present")
	}
	return ad, nil
}

// checkKey는 공개 키가 알고리즘과 맞는지 확인한다.
func checkKey(pub any, alg int) error {
	switch k := pub.(type) {
	case *ecdsa.PublicKey:
		if alg == AlgES256 && k.Curve == elliptic.P256() {
			return nil
		}
	case ed25519.PublicKey:
		if alg == AlgEdDSA {
			return nil
		}
	case *rsa.PublicKey:
		if alg == AlgRS256 && k.N.BitLen() >= minRSABits {
			return nil
		}
	}
	return verifyErr("unsupported key for algorithm %d", alg)
}

func verifySignature(pub any, alg int, msg, sig []byte) bool {
	switch alg {
	case AlgES256:
		h := sha256.Sum256(msg)
		return ecdsa.VerifyASN1(pub.(*ecdsa.PublicKey), h[:], sig)
	case AlgEdDSA:
		return ed25519.Verify(pub.(ed25519.PublicKey), msg, sig)
	case AlgRS256:
		h := sha256.Sum256(msg)
		return rsa.VerifyPKCS1v15(pub.(*rsa.PublicKey), crypto.SHA256, h[:], sig) == nil
	}
	return false
}
