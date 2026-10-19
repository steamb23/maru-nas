package server

import (
	"errors"
	"log"
	"net/http"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/security"
)

// 인증 수단 관리(관리 창 필요): TOTP 설정·삭제, 복구 코드 다시 만들기, 기기 추가 코드. 패스키는 passkey.go.

const (
	// totpSetupTTL은 설정 중인 TOTP 비밀을 기다리는 시간이다.
	totpSetupTTL = 10 * time.Minute
	// deviceCodeTTL은 기기 추가 코드를 쓸 수 있는 시간이다.
	deviceCodeTTL = 10 * time.Minute
)

// addFirstMethod는 rec에 첫 수단을 넣기 직전에 부른다. 첫 수단이면 복구 코드를 새로 만들어 평문을 돌려준다
// (한 번만 보여 준다). 이미 수단이 있으면 아무것도 하지 않는다.
func addFirstMethod(rec *security.Record) []string {
	if rec.HasMethods() {
		return nil
	}
	codes, hashes := security.NewRecoveryCodes()
	rec.Recovery = hashes
	return codes
}

// writeSecurityChange는 수단을 바꾼 뒤의 상태와(있으면) 새 복구 코드를 응답한다.
func (h *handler) writeSecurityChange(w http.ResponseWriter, s auth.Session, codes []string) {
	st, err := h.securityStatus(s)
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, api.SecurityChangeResponse{Status: st, RecoveryCodes: codes})
}

// requireManage는 관리 창이 열려 있어야 true다. 아니면 403 manage_required를 쓴다.
func (h *handler) requireManage(w http.ResponseWriter, s auth.Session) bool {
	if h.manageOpen(s) {
		return true
	}
	writeError(w, http.StatusForbidden, api.ErrManageRequired, "confirm your identity first")
	return false
}

// totpAccount는 인증 앱에 보이는 계정 이름이다. 여러 NAS를 쓰는 사용자를 위해 호스트 이름을 붙인다.
func (h *handler) totpAccount(user string) string {
	if h.Hostname == "" {
		return user
	}
	return user + "@" + h.Hostname
}

// setupTOTP는 새 TOTP 비밀을 만든다. 확인(confirmTOTP)할 때까지 이 세션의 메모리에만 둔다. 이미 등록한
// 인증 앱이 있으면 확인할 때 바꾼다(휴대전화를 바꾼 경우).
func (h *handler) setupTOTP(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !requireJSON(w, r) || !h.requireManage(w, s) {
		return
	}
	secret := security.NewTOTPSecret()
	h.totpSetup.Put(s.ID, secret)
	writeJSON(w, http.StatusOK, api.TOTPSetupResponse{Secret: secret, URI: security.TOTPURI(secret, h.totpAccount(s.Username))})
}

// confirmTOTP는 인증 앱이 보여 준 코드로 설정 중인 TOTP를 확인해 저장한다. 첫 수단이면 복구 코드를 만든다.
func (h *handler) confirmTOTP(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.TOTPConfirmRequest
	if !decodeJSON(w, r, &req) || !h.requireManage(w, s) {
		return
	}
	secret, ok := h.totpSetup.Peek(s.ID)
	if !ok {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "no authenticator setup in progress")
		return
	}
	ip := clientIP(r)
	done, wait := h.securityLimiter.Begin(ip, s.Username, "")
	if done == nil {
		writeRateLimited(w, wait)
		return
	}
	now := h.Now()
	step, err := security.VerifyTOTP(secret, req.Code, now, 0)
	if err != nil {
		done(auth.Failure)
		log.Printf("security: totp setup code failed: user=%q ip=%s", s.Username, ip)
		sleep(r.Context(), h.FailDelay)
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "verification failed")
		return
	}
	done(auth.Success)
	h.totpSetup.Delete(s.ID)
	var codes []string
	if err := h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		codes = addFirstMethod(rec)
		rec.TOTP = &security.TOTP{Secret: secret, Created: now, LastStep: step}
		return nil
	}); err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	log.Printf("security: totp registered: user=%q ip=%s", s.Username, ip)
	// 방금 이 인증 앱을 가지고 있음을 증명했다.
	h.writeSecurityChange(w, h.raise(s, auth.AssuranceTwoFactor), codes)
}

// deleteTOTP는 등록한 인증 앱을 지운다.
func (h *handler) deleteTOTP(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !h.requireManage(w, s) {
		return
	}
	err := h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		if rec.TOTP == nil {
			return errNoFactor
		}
		rec.TOTP = nil
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
	log.Printf("security: totp deleted: user=%q ip=%s", s.Username, clientIP(r))
	h.writeSecurityChange(w, s, nil)
}

// regenerateRecovery는 복구 코드를 새로 만든다. 이전 코드는 모두 쓸 수 없게 된다.
func (h *handler) regenerateRecovery(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !requireJSON(w, r) || !h.requireManage(w, s) {
		return
	}
	var codes []string
	err := h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		if !rec.HasMethods() {
			return errNoFactor
		}
		var hashes []string
		codes, hashes = security.NewRecoveryCodes()
		rec.Recovery = hashes
		return nil
	})
	switch {
	case errors.Is(err, errNoFactor):
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "register a passkey or an authenticator app first")
		return
	case err != nil:
		writeSecurityUnavailable(w, s, err)
		return
	}
	log.Printf("security: recovery codes regenerated: user=%q ip=%s", s.Username, clientIP(r))
	h.writeSecurityChange(w, s, codes)
}

// createDeviceCode는 기기 추가 코드를 만든다. 등록한 패스키가 없는 새 기기의 관리 확인이 등록한 수단 대신 받는다.
// 사용자마다 하나이고 다시 만들면 이전 코드는 버린다. 평문은 이 응답에만 담고 해시만 메모리에 둔다.
func (h *handler) createDeviceCode(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !requireJSON(w, r) || !h.requireManage(w, s) {
		return
	}
	rec, err := h.Security.GetSecurity(s.Username)
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	// 수단이 없으면 새 기기의 관리 확인도 비밀번호만 받으므로 코드가 필요 없다.
	if !rec.HasMethods() {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "register a passkey or an authenticator app first")
		return
	}
	code, hash := security.NewDeviceCode()
	h.deviceCodes.Put(s.Username, hash)
	log.Printf("security: device code issued: user=%q ip=%s", s.Username, clientIP(r))
	writeJSON(w, http.StatusOK, api.DeviceCodeResponse{Code: code, ExpiresInSeconds: int(deviceCodeTTL / time.Second)})
}

// checkDeviceCode는 관리 확인에서 등록한 수단 대신 받은 기기 추가 코드를 확인한다. 맞을 때만 코드를 지운다
// (비밀번호를 가진 쪽이 틀린 값을 보내 정상 사용자의 코드를 없애지 못하게). 실패하면 응답을 쓰고 false다.
func (h *handler) checkDeviceCode(w http.ResponseWriter, r *http.Request, s auth.Session, code string) bool {
	ip := clientIP(r)
	done, wait := h.securityLimiter.Begin(ip, s.Username, "")
	if done == nil {
		log.Printf("security: verify rate limited: user=%q ip=%s", s.Username, ip)
		writeRateLimited(w, wait)
		return false
	}
	match := func(hash string) bool { return security.MatchDeviceCode(hash, code) }
	if _, ok := h.deviceCodes.TakeIf(s.Username, match); !ok {
		done(auth.Failure)
		log.Printf("security: device code failed: user=%q ip=%s", s.Username, ip)
		sleep(r.Context(), h.FailDelay)
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "verification failed")
		return false
	}
	done(auth.Success)
	log.Printf("security: device code used: user=%q ip=%s", s.Username, ip)
	return true
}
