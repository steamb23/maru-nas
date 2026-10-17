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

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/security"
)

// 패스키(WebAuthn): challenge, 등록, 확인. 확인·등록 값의 검사는 internal/security가 한다.

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
	id := security.B64.EncodeToString(a.CredentialID)
	return h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		pk := rec.Passkey(id)
		if pk == nil {
			return fmt.Errorf("%w: unknown credential", security.ErrVerify)
		}
		if len(a.UserHandle) > 0 && !bytes.Equal(a.UserHandle, rec.Handle) {
			return fmt.Errorf("%w: user handle mismatch", security.ErrVerify)
		}
		count, err := h.RP.VerifyAssertion(*pk, a, c.challenge)
		if err != nil {
			return err
		}
		// 서명 횟수가 늘지 않았으면 복제된 인증기일 수 있다. 횟수를 세지 않는 인증기(동기화 패스키는 늘 0)도 있어
		// 거절하지 않고 기록만 남긴다.
		if (count != 0 || pk.SignCount != 0) && count <= pk.SignCount {
			log.Printf("security: passkey sign count did not increase: user=%q passkey=%q (%d -> %d)", s.Username, pk.Name, pk.SignCount, count)
		}
		pk.SignCount, pk.LastUsed = count, now
		return nil
	})
}

var (
	errDuplicatePasskey = errors.New("security: passkey already registered")
	errTooManyPasskeys  = errors.New("security: too many passkeys")
	errHandleChanged    = errors.New("security: user handle changed during registration")
)

// registerPasskey는 새 패스키를 등록한다(관리 창 필요). 첫 수단이면 복구 코드를 만들어 한 번만 보여 준다.
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
	pk, err := h.RP.VerifyRegistration(reg, c.challenge)
	if err != nil {
		log.Printf("security: passkey registration failed: user=%q ip=%s: %v", s.Username, clientIP(r), err)
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "passkey registration failed")
		return
	}
	now := h.Now()
	pk.Name, pk.Created = name, now
	for _, t := range cr.Transports {
		if slices.Contains(knownTransports, t) && !slices.Contains(pk.Transports, t) {
			pk.Transports = append(pk.Transports, t)
		}
	}
	var codes []string
	err = h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		switch {
		case rec.Passkey(pk.ID) != nil:
			return errDuplicatePasskey
		case len(rec.Passkeys) >= maxPasskeys:
			return errTooManyPasskeys
		case rec.Handle != nil && !bytes.Equal(rec.Handle, c.handle):
			return errHandleChanged
		}
		codes = addFirstMethod(rec)
		rec.Handle = c.handle
		rec.Passkeys = append(rec.Passkeys, pk)
		return nil
	})
	switch {
	case errors.Is(err, errDuplicatePasskey), errors.Is(err, errTooManyPasskeys), errors.Is(err, errHandleChanged):
		writeError(w, http.StatusConflict, api.ErrBadRequest, "cannot register this passkey")
		return
	case err != nil:
		writeSecurityUnavailable(w, s, err)
		return
	}
	log.Printf("security: passkey registered: user=%q ip=%s name=%q", s.Username, clientIP(r), name)
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
