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

// 패스키로 로그인(분할 키). 비밀번호 암호문은 브라우저에만 있고, 서버는 키의 나머지 조각(share)만 둔다. 키는 패스키
// PRF 결과와 share를 함께 써야 만들어지고, share는 패스키 서명을 확인한 뒤에만 내주므로 복호화마다 이 서버를
// 거친다(로그·시도 제한·폐기). 설계는 .claude/rules/security.md의 "패스키로 로그인".
//
// 이 파일은 등록(켜기)·해제와 등록 목록이다. 로그인 단계는 passkey_login_flow.go.

var (
	errTooManyPasskeyLogins = errors.New("security: too many passkey login enrollments")
	errNoPasskeyLogin       = errors.New("security: passkey login not enrolled")
)

// maxPasskeyLogins는 사용자 한 명의 패스키로 로그인 등록(브라우저) 수다.
const maxPasskeyLogins = 20

// loginTag는 로그에 남길 등록 ID의 앞부분이다. 등록 ID는 로그인 단계를 시작하는 값이라 전부 남기지 않는다.
func loginTag(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}

// passkeyLoginInfos는 기록의 만료되지 않은 등록 목록이다(지금 만료 기간을 적용한다).
func (h *handler) passkeyLoginInfos(rec security.Record) []api.PasskeyLoginInfo {
	out := []api.PasskeyLoginInfo{}
	now, days := h.Now(), h.passkeyLoginDays()
	for _, l := range rec.PasskeyLogins {
		if l.Expired(now, days) {
			continue
		}
		info := api.PasskeyLoginInfo{
			ID: l.ID, Passkey: l.Credential,
			Created:   l.Created.UTC().Format(time.RFC3339),
			ExpiresAt: l.Expiry(days).UTC().Format(time.RFC3339),
		}
		if !l.LastUsed.IsZero() {
			info.LastUsed = l.LastUsed.UTC().Format(time.RFC3339)
		}
		out = append(out, info)
	}
	return out
}

// passkeyLoginOffer는 로그인 응답의 PasskeyLoginOffer다. 공개 주소가 있고 사용자가 패스키를 등록했으면 true다.
// 이 브라우저에 등록이 있는지는 브라우저가 본다.
func (h *handler) passkeyLoginOffer(s auth.Session) bool {
	if h.RP == nil {
		return false
	}
	rec, err := h.Security.GetSecurity(s.Username)
	if err != nil {
		log.Printf("security: user=%q: record: %v", s.Username, err)
		return false
	}
	return len(rec.Passkeys) > 0
}

// enrollPasskeyLogin은 이 브라우저의 패스키로 로그인을 켠다. 비밀번호와 그 패스키를 함께 확인한 뒤 서버가 등록 ID와
// share를 만들어 이 응답에만 담는다. 브라우저는 같은 확인의 PRF 결과와 share로 비밀번호를 암호화해 둔다.
// 비밀번호와 등록한 수단을 함께 확인하므로 관리 확인과 같은 수준이고, 세션은 twoFactor로 오른다.
func (h *handler) enrollPasskeyLogin(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.PasskeyLoginEnrollRequest
	if !decodeJSONLimit(w, r, &req, maxSecurityBodyBytes) {
		return
	}
	if h.RP == nil {
		writeError(w, http.StatusConflict, api.ErrPasskeyUnavailable, "passkeys are not configured")
		return
	}
	if req.Passkey == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "a passkey is required")
		return
	}
	cred, ok := canonicalID(req.Passkey.ID)
	if !ok {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid passkey")
		return
	}
	if !h.confirmPassword(w, r, s, req.Password) {
		return
	}
	if !h.checkFactor(w, r, s, api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: req.Passkey}) {
		return
	}
	now, days := h.Now(), h.passkeyLoginDays()
	var pl security.PasskeyLogin
	var name string
	err := h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		pk := rec.Passkey(cred)
		if pk == nil {
			// 확인한 사이에 지웠다.
			return errNoFactor
		}
		name = pk.Name
		rec.PruneExpiredLogins(now, days)
		if req.Replace != "" {
			rec.DeletePasskeyLogin(req.Replace)
		}
		if len(rec.PasskeyLogins) >= maxPasskeyLogins {
			return errTooManyPasskeyLogins
		}
		pl = security.NewPasskeyLogin(cred, now, days)
		rec.PasskeyLogins = append(rec.PasskeyLogins, pl)
		return nil
	})
	switch {
	case errors.Is(err, errNoFactor), errors.Is(err, errTooManyPasskeyLogins):
		writeError(w, http.StatusConflict, api.ErrBadRequest, "cannot enable passkey login")
		return
	case err != nil:
		writeSecurityUnavailable(w, s, err)
		return
	}
	upd := h.touchSession(r.Context(), h.raise(s, auth.AssuranceTwoFactor))
	log.Printf("security: passkey login enabled: user=%q ip=%s passkey=%q login=%s days=%d", s.Username, clientIP(r), name, loginTag(pl.ID), days)
	st, err := h.securityStatus(upd)
	if err != nil {
		writeSecurityUnavailable(w, upd, err)
		return
	}
	writeJSON(w, http.StatusOK, api.PasskeyLoginEnrollResponse{
		ID:        pl.ID,
		Share:     security.B64.EncodeToString(pl.Share),
		ExpiresAt: pl.Expiry(days).UTC().Format(time.RFC3339),
		Status:    st,
	})
}

// deletePasskeyLogin은 패스키로 로그인 등록 하나를 해제한다. 관리 창은 요구하지 않는다. 로그인 방법을 줄이기만 하고
// 인증 수단을 지우지 않기 때문이다(세션을 빼앗은 쪽이 해도 비밀번호로 다시 켜면 된다).
func (h *handler) deletePasskeyLogin(w http.ResponseWriter, r *http.Request, s auth.Session) {
	id := r.PathValue("id")
	err := h.Security.UpdateSecurity(s.Username, func(rec *security.Record) error {
		if !rec.DeletePasskeyLogin(id) {
			return errNoPasskeyLogin
		}
		return nil
	})
	switch {
	case errors.Is(err, errNoPasskeyLogin):
		writeError(w, http.StatusNotFound, "", "not found")
		return
	case err != nil:
		writeSecurityUnavailable(w, s, err)
		return
	}
	log.Printf("security: passkey login disabled: user=%q ip=%s login=%s", s.Username, clientIP(r), loginTag(id))
	h.writeSecurityStatus(w, s)
}
