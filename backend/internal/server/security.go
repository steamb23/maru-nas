package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/omv"
	"maru/backend/internal/security"
)

// 보안 강화 모드: 세션 보증 수준과 그것을 올리는 확인(비밀번호 재확인, 2차 인증). 기능별 정책(site.json)과 정책 적용은
// security_policy.go, 수단 등록·삭제는 security_manage.go, 패스키는 passkey.go. 확인은 이 백엔드가 하고 권한 도우미에
// 넣지 않는다.

const (
	// manageWindow는 비밀번호와 등록한 수단을 다시 확인한 뒤 수단을 바꿀 수 있는 시간이다.
	manageWindow = 5 * time.Minute
	// maxSecurityBodyBytes는 확인·등록 요청 본문의 최대 크기다(패스키 값이 들어간다).
	maxSecurityBodyBytes = 64 << 10
)

var (
	// errNoFactor는 그 수단을 등록하지 않았다는 뜻이다. 틀린 값과 같이 다룬다.
	errNoFactor = errors.New("security: factor not registered")
	// errPasskeyUnavailable은 공개 주소가 없어 패스키를 쓸 수 없다는 뜻이다.
	errPasskeyUnavailable = errors.New("security: passkeys unavailable")
)

// required는 정책 단계가 요구하는 보증 수준이다.
func required(level string) auth.Assurance {
	switch level {
	case api.SecurityPassword:
		return auth.AssurancePassword
	case api.SecurityTwoFactor:
		return auth.AssuranceTwoFactor
	}
	return auth.AssuranceLogin
}

// assuranceName은 보증 수준의 API 값이다.
func assuranceName(a auth.Assurance) string {
	switch a {
	case auth.AssurancePassword:
		return api.AssurancePassword
	case auth.AssuranceTwoFactor:
		return api.AssuranceTwoFactor
	}
	return api.AssuranceLogin
}

// levelName은 보증 수준을 요구하는 정책 단계 이름이다(ErrorResponse.Required).
func levelName(a auth.Assurance) string {
	switch a {
	case auth.AssurancePassword:
		return api.SecurityPassword
	case auth.AssuranceTwoFactor:
		return api.SecurityTwoFactor
	}
	return api.SecurityOff
}

// satisfies는 s가 정책 단계 level을 만족하는지다.
func satisfies(s auth.Session, level string) bool { return s.Assurance >= required(level) }

// writeSecurityRequired는 level 단계의 확인을 먼저 거치라는 403이다.
func writeSecurityRequired(w http.ResponseWriter, level string) {
	writeJSON(w, http.StatusForbidden, api.ErrorResponse{
		Error:    "security verification required",
		Code:     api.ErrSecurityRequired,
		Required: level,
	})
}

// writeSecurityUnavailable은 수단 기록을 읽거나 쓰지 못했다는 500이다.
func writeSecurityUnavailable(w http.ResponseWriter, s auth.Session, err error) {
	log.Printf("security: user=%q: record: %v", s.Username, err)
	writeError(w, http.StatusInternalServerError, api.ErrSecurityUnavailable, "cannot read security settings")
}

// factorMethods는 기록에서 지금 확인에 쓸 수 있는 2차 수단이다(패스키는 공개 주소가 있을 때만).
func (h *handler) factorMethods(rec security.Record) []string {
	out := []string{}
	if len(rec.Passkeys) > 0 && h.RP != nil {
		out = append(out, api.FactorPasskey)
	}
	if rec.TOTP != nil {
		out = append(out, api.FactorTOTP)
	}
	if len(rec.Recovery) > 0 && rec.HasMethods() {
		out = append(out, api.FactorRecovery)
	}
	return out
}

// stepUpOffer는 로그인 직후 권할 2차 인증이다. 2차 인증을 요구하는 기능이 있고 사용자가 수단을 등록했을 때만 있다.
func (h *handler) stepUpOffer(s auth.Session) *api.StepUpInfo {
	if s.Assurance >= auth.AssuranceTwoFactor {
		return nil
	}
	p := h.securityPolicy()
	if p.Terminal != api.SecurityTwoFactor && p.Elevation != api.SecurityTwoFactor && p.Files != api.SecurityTwoFactor {
		return nil
	}
	rec, err := h.Security.GetSecurity(s.Username)
	if err != nil {
		log.Printf("security: user=%q: record: %v", s.Username, err)
		return nil
	}
	if m := h.factorMethods(rec); len(m) > 0 {
		return &api.StepUpInfo{Methods: m}
	}
	return nil
}

// manageOpen은 s의 관리 창이 열려 있는지다.
func (h *handler) manageOpen(s auth.Session) bool { return s.ManageUntil.After(h.Now()) }

// securityStatus는 s의 사용자 자신의 보안 상태다.
func (h *handler) securityStatus(s auth.Session) (api.SecurityStatus, error) {
	rec, err := h.Security.GetSecurity(s.Username)
	if err != nil {
		return api.SecurityStatus{}, err
	}
	st := api.SecurityStatus{
		Assurance:         assuranceName(s.Assurance),
		PasskeysAvailable: h.RP != nil,
		Passkeys:          []api.PasskeyInfo{},
		RecoveryCodes:     len(rec.Recovery),
		Methods:           h.factorMethods(rec),
		Policy:            h.securityPolicy(),
		PasskeyLogins:     h.passkeyLoginInfos(rec),
	}
	if left := s.ManageUntil.Sub(h.Now()); left > 0 {
		st.ManageSeconds = retrySeconds(left)
	}
	for _, p := range rec.Passkeys {
		info := api.PasskeyInfo{ID: p.ID, Name: p.Name, Created: p.Created.UTC().Format(time.RFC3339), BackupEligible: p.BackupEligible}
		if !p.LastUsed.IsZero() {
			info.LastUsed = p.LastUsed.UTC().Format(time.RFC3339)
		}
		st.Passkeys = append(st.Passkeys, info)
	}
	if rec.TOTP != nil {
		st.TOTP = &api.TOTPInfo{Created: rec.TOTP.Created.UTC().Format(time.RFC3339)}
	}
	return st, nil
}

// writeSecurityStatus는 s(고친 뒤의 세션)의 보안 상태를 응답한다.
func (h *handler) writeSecurityStatus(w http.ResponseWriter, s auth.Session) {
	st, err := h.securityStatus(s)
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (h *handler) securityInfo(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	h.writeSecurityStatus(w, s)
}

// touchSession은 확인을 마친 세션의 OMV 마지막 접근 시각을 갱신한다(활동 신호와 같다). 확인 값을 입력한 것은
// 사용자 활동이고, 로그인 직후 단계에서는 활동 신호가 돌지 않아 수단을 찾는 동안 세션이 만료에 가까워진다.
// OMV에 닿지 않으면 그대로 둔다.
func (h *handler) touchSession(ctx context.Context, s auth.Session) auth.Session {
	now := h.Now()
	st, err := h.OMV.Probe(ctx, s.OMVSessionID, true)
	if err != nil {
		if !omv.IsUnauthorized(err) {
			log.Printf("security: touch session: omv error: %v", err)
		}
		return s
	}
	if upd, ok := h.Sessions.Update(s.ID, func(ss *auth.Session) {
		ss.LastTouch, ss.LastOMVCheck = now, now
		applyState(ss, st)
	}); ok {
		return upd
	}
	return s
}

// raise는 세션의 보증 수준을 올린 결과다. 그사이 세션이 지워졌으면 s를 그대로 돌려준다.
func (h *handler) raise(s auth.Session, a auth.Assurance) auth.Session {
	if upd, ok := h.Sessions.Update(s.ID, func(ss *auth.Session) { ss.Raise(a) }); ok {
		return upd
	}
	return s
}

// verifySecurity는 세션 중 확인이다. 비밀번호를 다시 확인하면 password, 2차 수단을 확인하면 twoFactor로 올린다.
// 로그인 직후 권하는 2차 인증(LoginResponse.StepUp)도 이 요청으로 한다.
func (h *handler) verifySecurity(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.SecurityVerifyRequest
	if !decodeJSONLimit(w, r, &req, maxSecurityBodyBytes) {
		return
	}
	level := auth.AssuranceTwoFactor
	switch req.Method {
	case api.FactorPassword:
		if !h.confirmPassword(w, r, s, req.Password) {
			return
		}
		level = auth.AssurancePassword
	case api.FactorTOTP, api.FactorRecovery, api.FactorPasskey:
		if !h.checkFactor(w, r, s, req) {
			return
		}
	default:
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown method")
		return
	}
	upd := h.touchSession(r.Context(), h.raise(s, level))
	log.Printf("security: verified: user=%q ip=%s method=%s assurance=%s", s.Username, clientIP(r), req.Method, assuranceName(upd.Assurance))
	h.writeSecurityStatus(w, upd)
}

// manageSecurity는 비밀번호와(등록한 수단이 있으면) 그 수단을 다시 확인해 이 세션에 관리 창을 연다. 세션을 빼앗은
// 쪽이 자기 수단을 등록하거나 남의 수단을 지우지 못하게 한다. 등록한 수단 대신 다른 기기에서 만든 기기 추가 코드를
// 받을 수 있다(security_manage.go의 createDeviceCode). 이때는 이 세션에서 수단을 확인한 것이 아니므로 보증 수준을
// password까지만 올린다.
func (h *handler) manageSecurity(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.SecurityManageRequest
	if !decodeJSONLimit(w, r, &req, maxSecurityBodyBytes) {
		return
	}
	if req.Factor != nil && req.DeviceCode != "" {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "send either a factor or a device code")
		return
	}
	rec, err := h.Security.GetSecurity(s.Username)
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	needFactor := rec.HasMethods()
	useCode := needFactor && req.DeviceCode != ""
	if needFactor && !useCode && (req.Factor == nil || req.Factor.Method == api.FactorPassword) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "a registered factor is required")
		return
	}
	if !h.confirmPassword(w, r, s, req.Password) {
		return
	}
	level := auth.AssurancePassword
	switch {
	case useCode:
		if !h.checkDeviceCode(w, r, s, req.DeviceCode) {
			return
		}
	case needFactor:
		if !h.checkFactor(w, r, s, *req.Factor) {
			return
		}
		level = auth.AssuranceTwoFactor
	}
	until := h.Now().Add(manageWindow)
	upd, ok := h.Sessions.Update(s.ID, func(ss *auth.Session) {
		ss.ManageUntil = until
		ss.Raise(level)
	})
	if !ok {
		upd = s
	}
	upd = h.touchSession(r.Context(), upd)
	log.Printf("security: manage window opened: user=%q ip=%s", s.Username, clientIP(r))
	h.writeSecurityStatus(w, upd)
}

// confirmPassword는 비밀번호를 OMV에 다시 확인한다. 새 OMV 세션을 받아 바로 끝내므로 도우미 없이도, OMV가 받는
// 모든 계정에서 된다. 같은 비밀번호이므로 로그인 실패 제한(h.Limiter)을 함께 쓴다. 실패하면 응답을 쓰고 false다.
func (h *handler) confirmPassword(w http.ResponseWriter, r *http.Request, s auth.Session, password string) bool {
	if password == "" {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "password is required")
		return false
	}
	ip := clientIP(r)
	done, wait := h.Limiter.Begin(ip, s.Username, deviceToken(r))
	if done == nil {
		log.Printf("security: password confirm rate limited: user=%q ip=%s", s.Username, ip)
		writeRateLimited(w, wait)
		return false
	}
	res, err := h.OMV.Login(r.Context(), s.Username, password, ip)
	if err != nil {
		switch omv.HTTPStatus(err) {
		case http.StatusBadRequest:
			done(auth.Failure)
			log.Printf("security: password confirm failed: user=%q ip=%s", s.Username, ip)
			sleep(r.Context(), h.FailDelay)
			writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "incorrect password")
		case http.StatusTooManyRequests:
			done(auth.Failure)
			writeError(w, http.StatusTooManyRequests, api.ErrRateLimited, "too many failed attempts, try again later")
		default:
			done(auth.Neutral)
			log.Printf("security: password confirm: omv error: %v", err)
			writeError(w, http.StatusBadGateway, api.ErrOMVUnavailable, "cannot reach OpenMediaVault")
		}
		return false
	}
	done(auth.Success)
	// 확인에만 쓴 OMV 세션은 바로 끝낸다. 2차 인증 대기(challengeRequired)도 비밀번호는 맞았다는 뜻이다.
	if err := h.OMV.Logout(r.Context(), res.SessionID); err != nil && !omv.IsUnauthorized(err) {
		log.Printf("security: password confirm: omv logout: %v", err)
	}
	return true
}

// checkFactor는 2차 수단(TOTP, 복구 코드, 패스키)을 확인한다. 실패하면 응답을 쓰고 false다.
func (h *handler) checkFactor(w http.ResponseWriter, r *http.Request, s auth.Session, req api.SecurityVerifyRequest) bool {
	ip := clientIP(r)
	switch req.Method {
	case api.FactorTOTP, api.FactorRecovery, api.FactorPasskey:
	default:
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "unknown method")
		return false
	}
	done, wait := h.securityLimiter.Begin(ip, s.Username, "")
	if done == nil {
		log.Printf("security: verify rate limited: user=%q ip=%s", s.Username, ip)
		writeRateLimited(w, wait)
		return false
	}
	err := h.verifyFactor(s, req)
	switch {
	case err == nil:
		done(auth.Success)
		return true
	case errors.Is(err, errPasskeyUnavailable):
		done(auth.Neutral)
		writeError(w, http.StatusConflict, api.ErrPasskeyUnavailable, "passkeys are not configured")
	case errors.Is(err, security.ErrVerify), errors.Is(err, errNoFactor):
		done(auth.Failure)
		// 감싼 문구는 이유만 담는다(비밀 값 없음).
		log.Printf("security: verify failed: user=%q ip=%s method=%s: %v", s.Username, ip, req.Method, err)
		sleep(r.Context(), h.FailDelay)
		writeError(w, http.StatusBadRequest, api.ErrFactorFailed, "verification failed")
	default:
		done(auth.Neutral)
		writeSecurityUnavailable(w, s, err)
	}
	return false
}

// verifyFactor는 2차 수단 하나를 확인하고 기록을 고친다(TOTP의 마지막 단계, 쓴 복구 코드, 패스키 사용 시각).
func (h *handler) verifyFactor(s auth.Session, req api.SecurityVerifyRequest) error {
	now := h.Now()
	switch req.Method {
	case api.FactorTOTP:
		return h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
			if rec.TOTP == nil {
				return errNoFactor
			}
			step, err := security.VerifyTOTP(rec.TOTP.Secret, req.Code, now, rec.TOTP.LastStep)
			if err != nil {
				return err
			}
			rec.TOTP.LastStep = step
			return nil
		})
	case api.FactorRecovery:
		return h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
			if !rec.HasMethods() {
				return errNoFactor
			}
			rest, ok := security.ConsumeRecoveryCode(rec.Recovery, req.Code)
			if !ok {
				return security.ErrVerify
			}
			rec.Recovery = rest
			log.Printf("security: recovery code used: user=%q left=%d", s.Username, len(rest))
			return nil
		})
	case api.FactorPasskey:
		return h.verifyPasskey(s, req.Passkey, now)
	}
	return errNoFactor
}
