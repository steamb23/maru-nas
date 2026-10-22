package server

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/security"
)

// 패스키(WebAuthn): challenge, 등록, 확인. 확인·등록 값의 검사는 internal/security가 한다.
//
// 등록은 두 단계다. 등록 값(registerPasskey)이 맞으면 저장하지 않고 세션의 대기에 둔 뒤, 그 패스키로 시험 확인
// (confirmPasskey)을 한 번 해서 통과해야 저장한다. 등록할 때만 UV를 켜고 확인할 때는 켜 주지 않는 저장소(최신
// Windows의 동기화 저장소, 잠금이 풀려 있으면 확인을 건너뛰는 비밀번호 관리자 등)가 있어 등록 응답의 플래그만으로는
// 걸러 낼 수 없기 때문이다.

const (
	// challengeTTL은 패스키 challenge의 수명이다.
	challengeTTL = 5 * time.Minute
	// passkeyTimeout은 브라우저에 주는 패스키 확인 제한 시간이다.
	passkeyTimeout = 2 * time.Minute
	// maxPasskeys는 사용자 한 명이 등록할 수 있는 패스키 수다.
	maxPasskeys = 20
	// maxPasskeyName은 패스키 이름의 최대 글자 수다.
	maxPasskeyName = 64
	// rpName은 패스키 등록 창에 보이는 이름이다(브랜드 이름이라 현지화하지 않는다).
	rpName = "Onmaru"
)

// knownTransports는 저장하는 연결 방식이다(WebAuthn AuthenticatorTransport).
var knownTransports = []string{"usb", "nfc", "ble", "smart-card", "hybrid", "internal"}

// ceremony는 진행 중인 패스키 확인·등록이다. 대시보드 세션과 목적에 묶여 한 번만 쓴다.
type ceremony struct {
	challenge []byte
	// handle은 등록할 때 인증기에 넣은 사용자 핸들이다.
	handle []byte
}

func challengeKey(sessionID, purpose string) string { return purpose + ":" + sessionID }

// pendingPasskey는 등록 값은 맞았지만 아직 시험 확인을 하지 않은 패스키다(pendingPasskeys, 대시보드 세션 ID).
type pendingPasskey struct {
	pk     security.Passkey
	handle []byte
	// challenge는 시험 확인에 쓸 challenge다. 확인 challenge(assert)와 따로 둔다.
	challenge []byte
}

// withFlags는 패스키 확인 실패에 인증기 플래그(UP/UV/BE/BS)를 덧붙인다(진단 로그용, 비밀 값 없음).
func withFlags(err error, authData []byte) error {
	if f, ok := security.AuthFlags(authData); ok && err != nil {
		return fmt.Errorf("%w (flags %s)", err, f)
	}
	return err
}

// aaguidLog는 로그에 남길 인증기 종류다. 증명 none에서는 브라우저가 지울 수 있어 로그에만 쓴다.
func aaguidLog(authData []byte) string {
	if g := security.AAGUID(authData); g != "" {
		return g
	}
	return "none"
}

// decodeB64는 브라우저가 보낸 base64url 값을 푼다. 패딩이 있어도 받는다.
func decodeB64(s string) ([]byte, error) {
	return security.B64.DecodeString(strings.TrimRight(s, "="))
}

// canonicalID는 자격 증명 ID를 저장 형태(패딩 없는 base64url)로 바꾼다.
func canonicalID(id string) (string, bool) {
	b, err := decodeB64(id)
	if err != nil || len(b) == 0 {
		return "", false
	}
	return security.B64.EncodeToString(b), true
}

func descriptors(rec security.Record) []api.PasskeyDescriptor {
	out := []api.PasskeyDescriptor{}
	for _, p := range rec.Passkeys {
		out = append(out, api.PasskeyDescriptor{ID: p.ID, Transports: p.Transports})
	}
	return out
}

// passkeyChallenge는 확인(assert) 또는 등록(register, 관리 창 필요)에 쓸 challenge와 옵션을 준다.
func (h *handler) passkeyChallenge(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.PasskeyChallengeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if h.RP == nil {
		writeError(w, http.StatusConflict, api.ErrPasskeyUnavailable, "passkeys are not configured")
		return
	}
	rec, err := h.Security.GetSecurity(s.Username)
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	c := ceremony{challenge: security.NewChallenge()}
	opts := api.PasskeyOptions{RPID: h.RP.ID, TimeoutMs: int(passkeyTimeout / time.Millisecond), Credentials: descriptors(rec)}
	switch req.Purpose {
	case api.PasskeyAssert:
		if len(rec.Passkeys) == 0 {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "no passkeys registered")
			return
		}
	case api.PasskeyRegister:
		if !h.manageOpen(s) {
			writeError(w, http.StatusForbidden, api.ErrManageRequired, "confirm your identity first")
			return
		}
		if len(rec.Passkeys) >= maxPasskeys {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "too many passkeys")
			return
		}
		c.handle = rec.Handle
		if c.handle == nil {
			c.handle = security.NewHandle()
		}
		opts.RPName = rpName
		opts.User = &api.PasskeyUser{ID: security.B64.EncodeToString(c.handle), Name: s.Username, DisplayName: s.Username}
		opts.Algorithms = security.SupportedAlgs
	default:
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown purpose")
		return
	}
	opts.Challenge = security.B64.EncodeToString(c.challenge)
	h.challenges.Put(challengeKey(s.ID, req.Purpose), c)
	writeJSON(w, http.StatusOK, opts)
}

func decodeAssertion(pa *api.PasskeyAssertion) (security.Assertion, error) {
	var a security.Assertion
	var err error
	if pa == nil {
		return a, fmt.Errorf("%w: no assertion", security.ErrVerify)
	}
	for _, f := range []struct {
		dst      *[]byte
		src      string
		optional bool
	}{
		{&a.CredentialID, pa.ID, false},
		{&a.ClientDataJSON, pa.ClientDataJSON, false},
		{&a.AuthenticatorData, pa.AuthenticatorData, false},
		{&a.Signature, pa.Signature, false},
		{&a.UserHandle, pa.UserHandle, true},
	} {
		if f.src == "" && f.optional {
			continue
		}
		if *f.dst, err = decodeB64(f.src); err != nil {
			return a, fmt.Errorf("%w: bad base64", security.ErrVerify)
		}
	}
	return a, nil
}

// verifyPasskey는 이 세션의 확인 challenge로 만든 패스키 서명인지 확인하고 서명 횟수·사용 시각을 고친다.
func (h *handler) verifyPasskey(s auth.Session, pa *api.PasskeyAssertion, now time.Time) error {
	if h.RP == nil {
		return errPasskeyUnavailable
	}
	c, ok := h.challenges.Take(challengeKey(s.ID, api.PasskeyAssert))
	if !ok {
		return fmt.Errorf("%w: no challenge or challenge expired", security.ErrVerify)
	}
	a, err := decodeAssertion(pa)
	if err != nil {
		return err
	}
	return h.checkUserPasskey(s.Username, a, c.challenge, now, nil)
}

// checkUserPasskey는 username의 패스키로 challenge에 한 서명인지(UV 필수) 확인하고 서명 횟수·사용 시각을 고친다.
// also가 있으면 확인이 맞은 뒤 같은 기록 갱신 안에서 부른다(오류를 돌려주면 저장하지 않는다). 패스키로 로그인은 이것으로
// 그 등록의 패스키인지 보고 사용 시각을 함께 고친다.
func (h *handler) checkUserPasskey(username string, a security.Assertion, challenge []byte, now time.Time,
	also func(rec *security.Record, pk *security.Passkey) error) error {
	id := security.B64.EncodeToString(a.CredentialID)
	return h.Security.UpdateSecurity(username, func(rec *security.Record) error {
		pk := rec.Passkey(id)
		if pk == nil {
			return fmt.Errorf("%w: unknown credential", security.ErrVerify)
		}
		if len(a.UserHandle) > 0 && !bytes.Equal(a.UserHandle, rec.Handle) {
			return fmt.Errorf("%w: user handle mismatch", security.ErrVerify)
		}
		count, err := h.RP.VerifyAssertion(*pk, a, challenge)
		if err != nil {
			return withFlags(err, a.AuthenticatorData)
		}
		// 서명 횟수가 늘지 않았으면 복제된 인증기일 수 있다. 횟수를 세지 않는 인증기(동기화 패스키는 늘 0)도 있어
		// 거절하지 않고 기록만 남긴다.
		if (count != 0 || pk.SignCount != 0) && count <= pk.SignCount {
			log.Printf("security: passkey sign count did not increase: user=%q passkey=%q (%d -> %d)", username, pk.Name, pk.SignCount, count)
		}
		pk.SignCount, pk.LastUsed = count, now
		if also != nil {
			return also(rec, pk)
		}
		return nil
	})
}

var (
	errDuplicatePasskey = errors.New("security: passkey already registered")
	errTooManyPasskeys  = errors.New("security: too many passkeys")
	errHandleChanged    = errors.New("security: user handle changed during registration")
)

// checkNewPasskey는 rec에 pk를 더할 수 있는지 본다(중복, 개수 상한, 그사이 바뀐 사용자 핸들).
func checkNewPasskey(rec security.Record, pk security.Passkey, handle []byte) error {
	switch {
	case rec.Passkey(pk.ID) != nil:
		return errDuplicatePasskey
	case len(rec.Passkeys) >= maxPasskeys:
		return errTooManyPasskeys
	case rec.Handle != nil && !bytes.Equal(rec.Handle, handle):
		return errHandleChanged
	}
	return nil
}

func isNewPasskeyConflict(err error) bool {
	return errors.Is(err, errDuplicatePasskey) || errors.Is(err, errTooManyPasskeys) || errors.Is(err, errHandleChanged)
}

// registerPasskey는 새 패스키의 등록 값을 확인한다(관리 창 필요). 저장하지 않고 이 세션의 대기에 둔 뒤, 그 패스키
// 하나만 허용하는 시험 확인 옵션을 준다. 저장은 confirmPasskey가 한다.
func (h *handler) registerPasskey(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.PasskeyRegisterRequest
	if !decodeJSONLimit(w, r, &req, maxSecurityBodyBytes) {
		return
	}
	if h.RP == nil {
		writeError(w, http.StatusConflict, api.ErrPasskeyUnavailable, "passkeys are not configured")
		return
	}
	if !h.manageOpen(s) {
		writeError(w, http.StatusForbidden, api.ErrManageRequired, "confirm your identity first")
		return
	}
	name := strings.TrimSpace(req.Name)
	cr := req.Credential
	if name == "" || utf8.RuneCountInString(name) > maxPasskeyName || !utf8.ValidString(name) || cr == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid passkey")
		return
	}
	c, ok := h.challenges.Take(challengeKey(s.ID, api.PasskeyRegister))
	if !ok {
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "registration expired, try again")
		return
	}
	var reg security.Registration
	var err error
	reg.Algorithm = cr.PublicKeyAlgorithm
	for _, f := range []struct {
		dst *[]byte
		src string
	}{
		{&reg.CredentialID, cr.ID}, {&reg.ClientDataJSON, cr.ClientDataJSON},
		{&reg.AuthenticatorData, cr.AuthenticatorData}, {&reg.PublicKey, cr.PublicKey},
	} {
		if *f.dst, err = decodeB64(f.src); err != nil {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid passkey")
			return
		}
	}
	ip := clientIP(r)
	pk, err := h.RP.VerifyRegistration(reg, c.challenge)
	if err != nil {
		log.Printf("security: passkey registration failed: user=%q ip=%s aaguid=%s: %v", s.Username, ip, aaguidLog(reg.AuthenticatorData),
			withFlags(err, reg.AuthenticatorData))
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "passkey registration failed")
		return
	}
	pk.Name, pk.Created = name, h.Now()
	for _, t := range cr.Transports {
		if slices.Contains(knownTransports, t) && !slices.Contains(pk.Transports, t) {
			pk.Transports = append(pk.Transports, t)
		}
	}
	// 시험 확인을 요청하기 전에 미리 걸러 낸다. 저장할 때 다시 본다.
	rec, err := h.Security.GetSecurity(s.Username)
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	if err := checkNewPasskey(rec, pk, c.handle); err != nil {
		writeError(w, http.StatusConflict, api.ErrBadRequest, "cannot register this passkey")
		return
	}
	p := pendingPasskey{pk: pk, handle: c.handle, challenge: security.NewChallenge()}
	h.pendingPasskeys.Put(s.ID, p)
	f, _ := security.AuthFlags(reg.AuthenticatorData)
	log.Printf("security: passkey created, awaiting test verification: user=%q ip=%s name=%q aaguid=%s (flags %s)", s.Username, ip, name,
		aaguidLog(reg.AuthenticatorData), f)
	writeJSON(w, http.StatusOK, api.PasskeyOptions{
		Challenge:   security.B64.EncodeToString(p.challenge),
		RPID:        h.RP.ID,
		TimeoutMs:   int(passkeyTimeout / time.Millisecond),
		Credentials: []api.PasskeyDescriptor{{ID: pk.ID, Transports: pk.Transports}},
	})
}

// checkPending은 대기 중인 패스키 p로 한 시험 확인인지 검사하고 서명 횟수를 돌려준다.
func (h *handler) checkPending(p pendingPasskey, a security.Assertion) (uint32, error) {
	if security.B64.EncodeToString(a.CredentialID) != p.pk.ID {
		return 0, fmt.Errorf("%w: not the new credential", security.ErrVerify)
	}
	if len(a.UserHandle) > 0 && !bytes.Equal(a.UserHandle, p.handle) {
		return 0, fmt.Errorf("%w: user handle mismatch", security.ErrVerify)
	}
	count, err := h.RP.VerifyAssertion(p.pk, a, p.challenge)
	return count, withFlags(err, a.AuthenticatorData)
}

// confirmPasskey는 대기 중인 새 패스키로 한 시험 확인을 검사하고, 통과하면 저장한다(관리 창 필요). 첫 수단이면
// 복구 코드를 만들어 한 번만 보여 준다. 서명은 맞는데 UV만 없으면 passkey_uv_unsupported로 거절하고 실패로 세지
// 않는다(값을 맞히려는 시도가 아니라 저장소의 동작이다). 대기는 한 번만 쓴다.
func (h *handler) confirmPasskey(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.PasskeyConfirmRequest
	if !decodeJSONLimit(w, r, &req, maxSecurityBodyBytes) {
		return
	}
	if h.RP == nil {
		writeError(w, http.StatusConflict, api.ErrPasskeyUnavailable, "passkeys are not configured")
		return
	}
	if !h.manageOpen(s) {
		writeError(w, http.StatusForbidden, api.ErrManageRequired, "confirm your identity first")
		return
	}
	a, err := decodeAssertion(req.Passkey)
	if err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid passkey")
		return
	}
	ip := clientIP(r)
	done, wait := h.securityLimiter.Begin(ip, s.Username, "")
	if done == nil {
		log.Printf("security: passkey test verification rate limited: user=%q ip=%s", s.Username, ip)
		writeRateLimited(w, wait)
		return
	}
	p, ok := h.pendingPasskeys.Take(s.ID)
	if !ok {
		done(auth.Neutral)
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "registration expired, try again")
		return
	}
	count, err := h.checkPending(p, a)
	switch {
	case errors.Is(err, security.ErrUserNotVerified):
		done(auth.Neutral)
		log.Printf("security: passkey not registered, provider does not report user verification: user=%q ip=%s name=%q: %v",
			s.Username, ip, p.pk.Name, err)
		writeError(w, http.StatusBadRequest, api.ErrPasskeyUVUnsupported, "this passkey provider does not report user verification")
		return
	case err != nil:
		done(auth.Failure)
		log.Printf("security: passkey test verification failed: user=%q ip=%s name=%q: %v", s.Username, ip, p.pk.Name, err)
		sleep(r.Context(), h.FailDelay)
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "passkey verification failed")
		return
	}
	done(auth.Success)
	pk := p.pk
	pk.SignCount = count
	var codes []string
	err = h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		if err := checkNewPasskey(*rec, pk, p.handle); err != nil {
			return err
		}
		codes = addFirstMethod(rec)
		rec.Handle = p.handle
		rec.Passkeys = append(rec.Passkeys, pk)
		return nil
	})
	switch {
	case isNewPasskeyConflict(err):
		writeError(w, http.StatusConflict, api.ErrBadRequest, "cannot register this passkey")
		return
	case err != nil:
		writeSecurityUnavailable(w, s, err)
		return
	}
	log.Printf("security: passkey registered: user=%q ip=%s name=%q synced=%t", s.Username, ip, pk.Name, pk.BackupEligible)
	// 방금 이 패스키를 가지고 있음을 증명했다.
	h.writeSecurityChange(w, h.raise(s, auth.AssuranceTwoFactor), codes)
}

// deletePasskey는 패스키 하나를 지운다(관리 창 필요).
func (h *handler) deletePasskey(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !h.manageOpen(s) {
		writeError(w, http.StatusForbidden, api.ErrManageRequired, "confirm your identity first")
		return
	}
	id, ok := canonicalID(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "", "not found")
		return
	}
	var name string
	err := h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		i := slices.IndexFunc(rec.Passkeys, func(p security.Passkey) bool { return p.ID == id })
		if i < 0 {
			return errNoFactor
		}
		name = rec.Passkeys[i].Name
		rec.Passkeys = slices.Delete(rec.Passkeys, i, i+1)
		return nil
	})
	switch {
	case errors.Is(err, errNoFactor):
		writeError(w, http.StatusNotFound, "", "not found")
		return
	case err != nil:
		writeSecurityUnavailable(w, s, err)
		return
	}
	log.Printf("security: passkey deleted: user=%q ip=%s name=%q", s.Username, clientIP(r), name)
	h.writeSecurityChange(w, s, nil)
}
