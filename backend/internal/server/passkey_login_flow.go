package server

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/omv"
	"onmaru/backend/internal/security"
)

// 패스키로 로그인의 로그인 단계(인증 없음). 브라우저 항목의 사용자 이름·등록 ID로 그 패스키 하나만 허용하는 확인을
// 받고(options), 서명이 맞으면 share를 주고(unlock), 브라우저가 풀어 낸 비밀번호로 OMV에 로그인한다(finish).
// 단계 사이는 의식 쿠키로 잇는다. 등록·해제는 passkey_login.go.

const (
	// PasskeyLoginCookie는 패스키로 로그인 단계를 잇는 쿠키 이름이다(options부터 finish까지).
	PasskeyLoginCookie = "onmaru_passkey_login"
	// passkeyUnlockTTL은 share를 내준 뒤 비밀번호를 받을 때까지 기다리는 시간이다.
	passkeyUnlockTTL = 2 * time.Minute
)

var errPasskeyLoginExpired = errors.New("security: passkey login enrollment expired")

// loginCeremony는 options로 시작한 패스키로 로그인의 확인이다(loginCeremonies, 의식 쿠키 값).
type loginCeremony struct {
	username   string
	enrollment string
	credential string
	challenge  []byte
}

// passkeyUnlock은 확인을 마치고 share를 내준 패스키로 로그인이다(passkeyUnlocks, 의식 쿠키 값).
type passkeyUnlock struct {
	username   string
	enrollment string
}

func (h *handler) loginCeremonyID(r *http.Request) string {
	if ck, err := r.Cookie(PasskeyLoginCookie); err == nil {
		return ck.Value
	}
	return ""
}

// writePasskeyLoginUnknown은 등록이 없다는 응답이다. 없는 사용자와 없는 등록을 구분하지 않는다.
func writePasskeyLoginUnknown(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, api.ErrPasskeyLoginUnknown, "passkey login is not enabled")
}

// dropPasskeyLogin은 쓸 수 없게 된 등록(만료, 비밀번호 변경, 브라우저가 풀지 못함)을 지운다.
func (h *handler) dropPasskeyLogin(username, id, reason, ip string) {
	err := h.Security.UpdateSecurity(username, func(rec *security.Record) error {
		if !rec.DeletePasskeyLogin(id) {
			return errNoPasskeyLogin
		}
		return nil
	})
	if err != nil && !errors.Is(err, errNoPasskeyLogin) {
		log.Printf("security: user=%q: drop passkey login: %v", username, err)
		return
	}
	log.Printf("security: passkey login removed (%s): user=%q ip=%s login=%s", reason, username, ip, loginTag(id))
}

// passkeyLoginOptions는 브라우저 항목의 등록으로 그 패스키 하나만 허용하는 확인 옵션을 주고 의식 쿠키를 연다.
// 유효한 등록 ID가 있어야 상태가 생기므로 이 요청으로 저장소를 채울 수 없다.
func (h *handler) passkeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	var req api.PasskeyLoginOptionsRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if h.RP == nil {
		writeError(w, http.StatusConflict, api.ErrPasskeyUnavailable, "passkeys are not configured")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Enrollment == "" {
		writePasskeyLoginUnknown(w)
		return
	}
	rec, err := h.Security.GetSecurity(username)
	if err != nil {
		log.Printf("security: user=%q: record: %v", username, err)
		writePasskeyLoginUnknown(w)
		return
	}
	l := rec.PasskeyLogin(req.Enrollment)
	var pk *security.Passkey
	if l != nil {
		pk = rec.Passkey(l.Credential)
	}
	if pk == nil {
		writePasskeyLoginUnknown(w)
		return
	}
	ip := clientIP(r)
	if l.Expired(h.Now(), h.passkeyLoginDays()) {
		h.dropPasskeyLogin(username, l.ID, "expired", ip)
		writeError(w, http.StatusBadRequest, api.ErrPasskeyLoginExpired, "passkey login has expired")
		return
	}
	c := loginCeremony{username: username, enrollment: l.ID, credential: pk.ID, challenge: security.NewChallenge()}
	id := security.B64.EncodeToString(security.NewChallenge())
	h.loginCeremonies.Put(id, c)
	h.setCookie(w, r, PasskeyLoginCookie, id, int(challengeTTL/time.Second))
	writeJSON(w, http.StatusOK, api.PasskeyOptions{
		Challenge:   security.B64.EncodeToString(c.challenge),
		RPID:        h.RP.ID,
		TimeoutMs:   int(passkeyTimeout / time.Millisecond),
		Credentials: []api.PasskeyDescriptor{{ID: pk.ID, Transports: pk.Transports}},
	})
}

// passkeyLoginUnlock은 options의 확인으로 만든 패스키 서명(UV 필수)인지 보고, 맞으면 그 등록의 share를 준다. 실패는
// 2차 수단과 같은 제한(securityLimiter, IP·사용자 이름)으로 센다.
func (h *handler) passkeyLoginUnlock(w http.ResponseWriter, r *http.Request) {
	var req api.PasskeyLoginRequest
	if !decodeJSONLimit(w, r, &req, maxSecurityBodyBytes) {
		return
	}
	if h.RP == nil {
		writeError(w, http.StatusConflict, api.ErrPasskeyUnavailable, "passkeys are not configured")
		return
	}
	id := h.loginCeremonyID(r)
	c, ok := h.loginCeremonies.Take(id)
	if !ok {
		// 쿠키는 그대로 둔다. 같은 확인을 두 번 보낸 경우 그 쿠키에 풀림 상태가 있을 수 있다.
		writeError(w, http.StatusBadRequest, api.ErrChallengeExpired, "no pending passkey login or it has expired")
		return
	}
	a, err := decodeAssertion(req.Passkey)
	if err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid passkey")
		return
	}
	ip := clientIP(r)
	done, wait := h.securityLimiter.Begin(ip, c.username, "")
	if done == nil {
		log.Printf("security: passkey login rate limited: user=%q ip=%s", c.username, ip)
		writeRateLimited(w, wait)
		return
	}
	now, days := h.Now(), h.passkeyLoginDays()
	var share []byte
	err = security.ErrVerify
	if security.B64.EncodeToString(a.CredentialID) == c.credential {
		err = h.checkUserPasskey(c.username, a, c.challenge, now, func(rec *security.Record, pk *security.Passkey) error {
			l := rec.PasskeyLogin(c.enrollment)
			switch {
			case l == nil || l.Credential != pk.ID:
				return errNoPasskeyLogin
			case l.Expired(now, days):
				return errPasskeyLoginExpired
			}
			share = bytes.Clone(l.Share)
			return nil
		})
	}
	switch {
	case err == nil:
		done(auth.Success)
	case errors.Is(err, errNoPasskeyLogin):
		done(auth.Neutral)
		h.clearCookie(w, r, PasskeyLoginCookie)
		writePasskeyLoginUnknown(w)
		return
	case errors.Is(err, errPasskeyLoginExpired):
		done(auth.Neutral)
		h.dropPasskeyLogin(c.username, c.enrollment, "expired", ip)
		h.clearCookie(w, r, PasskeyLoginCookie)
		writeError(w, http.StatusBadRequest, api.ErrPasskeyLoginExpired, "passkey login has expired")
		return
	case errors.Is(err, security.ErrVerify):
		done(auth.Failure)
		log.Printf("security: passkey login failed: user=%q ip=%s login=%s: %v", c.username, ip, loginTag(c.enrollment), err)
		sleep(r.Context(), h.FailDelay)
		h.clearCookie(w, r, PasskeyLoginCookie)
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "passkey verification failed")
		return
	default:
		done(auth.Neutral)
		log.Printf("security: user=%q: record: %v", c.username, err)
		writeError(w, http.StatusInternalServerError, api.ErrSecurityUnavailable, "cannot read security settings")
		return
	}
	h.passkeyUnlocks.Put(id, passkeyUnlock{username: c.username, enrollment: c.enrollment})
	h.setCookie(w, r, PasskeyLoginCookie, id, int(passkeyUnlockTTL/time.Second))
	log.Printf("security: passkey login verified: user=%q ip=%s login=%s", c.username, ip, loginTag(c.enrollment))
	writeJSON(w, http.StatusOK, api.PasskeyLoginUnlockResponse{Share: security.B64.EncodeToString(share)})
}

// passkeyLoginFinish는 브라우저가 풀어 낸 비밀번호로 OMV에 로그인하고 twoFactor 세션을 만든다. 이 비밀번호는 브라우저가
// 보낸 값이라 복호화 결과인지 알 수 없으므로 일반 로그인과 같은 제한(h.Limiter)으로 센다. OMV가 비밀번호를 받지 않으면
// 비밀번호를 바꾼 것이므로 그 등록을 지운다. Discard면 브라우저가 풀지 못한 등록을 지운다.
func (h *handler) passkeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req api.PasskeyLoginFinishRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	u, ok := h.passkeyUnlocks.Take(h.loginCeremonyID(r))
	h.clearCookie(w, r, PasskeyLoginCookie)
	if !ok {
		writeError(w, http.StatusBadRequest, api.ErrChallengeExpired, "no pending passkey login or it has expired")
		return
	}
	ip := clientIP(r)
	if req.Discard {
		h.dropPasskeyLogin(u.username, u.enrollment, "browser could not decrypt", ip)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "password is required")
		return
	}
	done, wait := h.Limiter.Begin(ip, u.username, deviceToken(r))
	if done == nil {
		log.Printf("passkey login rate limited: user=%q ip=%s", u.username, ip)
		writeRateLimited(w, wait)
		return
	}
	res, err := h.OMV.Login(r.Context(), u.username, req.Password, ip)
	if err != nil {
		if omv.HTTPStatus(err) == http.StatusBadRequest {
			done(auth.Failure)
			h.dropPasskeyLogin(u.username, u.enrollment, "password changed", ip)
			sleep(r.Context(), h.FailDelay)
			writeError(w, http.StatusBadRequest, api.ErrPasskeyLoginStale, "the saved password no longer works")
			return
		}
		h.authFailed(w, r, done, err, "passkey login", u.username, ip)
		return
	}
	done(auth.Success)
	now := h.Now()
	if err := h.Security.UpdateSecurity(u.username, func(rec *security.Record) error {
		if l := rec.PasskeyLogin(u.enrollment); l != nil {
			l.LastUsed = now
		}
		return nil
	}); err != nil {
		log.Printf("security: user=%q: record: %v", u.username, err)
	}
	log.Printf("security: passkey login: user=%q ip=%s login=%s", u.username, ip, loginTag(u.enrollment))
	h.completeLogin(w, r, res, req.Password, ip, auth.AssuranceTwoFactor)
}
