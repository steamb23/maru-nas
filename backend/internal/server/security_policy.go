package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"sync"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/prefs"
	"maru/backend/internal/terminal"
)

// 보안 강화 모드의 기능별 정책(site.json)과 그 적용, 관리자가 보는 사용자별 수단 기록(목록, 초기화).

// policyState는 지금 보안 강화 정책이다. 이 프로세스만 site.json을 쓰므로 메모리에 두고 저장할 때 함께 바꾼다.
type policyState struct {
	mu sync.Mutex
	p  api.SecurityPolicy
}

// validLevel은 정책 단계 값인지다. 빈 값은 off다.
func validLevel(v string) bool {
	return v == "" || v == api.SecurityOff || v == api.SecurityPassword || v == api.SecurityTwoFactor
}

// passkeyLoginDayChoices는 패스키로 로그인 등록의 만료 기간(일)으로 고를 수 있는 값이다(짧은 순).
var passkeyLoginDayChoices = []int{7, 30, 90, 180, 365}

// validDays는 만료 기간 값인지다. 0은 기본값이다.
func validDays(d int) bool { return d == 0 || slices.Contains(passkeyLoginDayChoices, d) }

// normalizePolicy는 빈 값을 off로(만료 기간은 기본값으로) 채운다. 모르는 값(손으로 고친 파일 등)은 가장 엄격한
// 단계와 가장 짧은 기간으로 본다.
func normalizePolicy(p api.SecurityPolicy) api.SecurityPolicy {
	norm := func(v string) string {
		switch {
		case v == "":
			return api.SecurityOff
		case validLevel(v):
			return v
		}
		log.Printf("site settings: unknown security level %q, using %s", v, api.SecurityTwoFactor)
		return api.SecurityTwoFactor
	}
	days := p.PasskeyLoginDays
	switch {
	case days == 0:
		days = api.PasskeyLoginDaysDefault
	case !validDays(days):
		log.Printf("site settings: unknown passkey login period %d, using %d days", days, passkeyLoginDayChoices[0])
		days = passkeyLoginDayChoices[0]
	}
	scope := p.FilesScope
	switch scope {
	case "", api.FilesScopeAll:
		scope = api.FilesScopeAll
	case api.FilesScopeWrites:
	default:
		// 모르는 값은 더 많이 막는 쪽(읽기까지)으로 본다.
		log.Printf("site settings: unknown file explorer policy scope %q, using %s", scope, api.FilesScopeAll)
		scope = api.FilesScopeAll
	}
	return api.SecurityPolicy{Terminal: norm(p.Terminal), Elevation: norm(p.Elevation), Files: norm(p.Files), FilesScope: scope,
		PasskeyLoginDays: days}
}

// validScope는 파일 탐색기 정책의 적용 범위 값인지다. 빈 값은 all이다.
func validScope(v string) bool {
	return v == "" || v == api.FilesScopeAll || v == api.FilesScopeWrites
}

// passkeyLoginDays는 지금 패스키로 로그인 등록의 만료 기간(일)이다.
func (h *handler) passkeyLoginDays() int { return h.securityPolicy().PasskeyLoginDays }

// loadPolicy는 시작할 때 저장된 정책을 읽는다. 읽을 수 없으면 모두 꺼진 정책으로 시작한다.
func (h *handler) loadPolicy() {
	site, err := h.Site.GetSite()
	if err != nil {
		log.Printf("site settings: read security policy: %v", err)
	}
	p := api.SecurityPolicy{}
	if site.Security != nil {
		p = *site.Security
	}
	h.setPolicy(normalizePolicy(p))
}

func (h *handler) setPolicy(p api.SecurityPolicy) {
	h.policy.mu.Lock()
	h.policy.p = p
	h.policy.mu.Unlock()
}

// securityPolicy는 지금 정책이다(모든 값이 채워져 있다).
func (h *handler) securityPolicy() api.SecurityPolicy {
	h.policy.mu.Lock()
	defer h.policy.mu.Unlock()
	return h.policy.p
}

// enforcePolicy는 정책을 올린 뒤 조건을 채우지 못하는 세션의 터미널을 닫고 root 권한 상승을 끝낸다.
// 파일 탐색기는 요청마다 정책을 보므로 따로 닫을 것이 없다(이미 시작한 전송은 끝까지 간다).
func (h *handler) enforcePolicy() {
	p := h.securityPolicy()
	for _, ss := range h.Sessions.List() {
		if h.Terminal != nil && !satisfies(ss, p.Terminal) {
			h.Terminal.CloseSession(ss.ID, terminal.ReasonSecurityRequired)
		}
		if h.Helper != nil && ss.HelperTicket != "" && !satisfies(ss, p.Elevation) {
			go func(ticket string) {
				ctx, cancel := context.WithTimeout(context.Background(), helperCloseTimeout)
				defer cancel()
				h.Helper.Drop(ctx, ticket)
			}(ss.HelperTicket)
		}
	}
}

// securityPolicyInfo는 지금 정책과 함께, 정책을 고를 때 알아야 할 것(OMV 자동 로그아웃, 패스키)을 준다.
func (h *handler) securityPolicyInfo(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	resp := api.SecurityPolicyResponse{Policy: h.securityPolicy()}
	if s.TimeoutKnown {
		m := int(s.Timeout / time.Minute)
		resp.AutoLogoutMinutes = &m
	}
	if h.RP != nil {
		resp.PasskeyRPID = h.RP.ID
	}
	writeJSON(w, http.StatusOK, resp)
}

// setSecurityPolicy는 정책 전체를 바꾼다. 바꾸는 기능마다 이전·새 단계 중 높은 쪽을 이 세션이 거쳤어야 한다
// (빼앗은 관리자 세션이 정책을 내리고 기능을 여는 것을 막는다).
func (h *handler) setSecurityPolicy(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.SecurityPolicyRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Security == nil || !validLevel(req.Security.Terminal) || !validLevel(req.Security.Elevation) ||
		!validLevel(req.Security.Files) || !validScope(req.Security.FilesScope) || !validDays(req.Security.PasskeyLoginDays) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid security policy")
		return
	}
	next := normalizePolicy(*req.Security)
	cur := h.securityPolicy()
	need := auth.AssuranceLogin
	for _, pair := range [][2]string{{cur.Terminal, next.Terminal}, {cur.Elevation, next.Elevation}, {cur.Files, next.Files}} {
		if pair[0] != pair[1] {
			need = max(need, required(pair[0]), required(pair[1]))
		}
	}
	// 파일 탐색기의 적용 범위를 바꾸려면 그 단계를 거쳤어야 한다(쓰기에만 적용하면 목록·내려받기가 열린다).
	if cur.FilesScope != next.FilesScope {
		need = max(need, required(cur.Files), required(next.Files))
	}
	// 만료 기간은 비밀번호를 다시 확인한 세션만 바꾼다(빼앗은 세션이 기간을 늘리지 못하게).
	if cur.PasskeyLoginDays != next.PasskeyLoginDays {
		need = max(need, auth.AssurancePassword)
	}
	if s.Assurance < need {
		writeSecurityRequired(w, levelName(need))
		return
	}
	if !h.updateSite(w, func(v *api.SiteSettings) {
		if next.Terminal == api.SecurityOff && next.Elevation == api.SecurityOff && next.Files == api.SecurityOff &&
			next.FilesScope == api.FilesScopeAll && next.PasskeyLoginDays == api.PasskeyLoginDaysDefault {
			v.Security = nil
		} else {
			p := next
			v.Security = &p
		}
	}) {
		return
	}
	h.setPolicy(next)
	log.Printf("security: policy changed: by=%q ip=%s terminal=%s elevation=%s files=%s files_scope=%s passkey_login_days=%d",
		s.Username, clientIP(r), next.Terminal, next.Elevation, next.Files, next.FilesScope, next.PasskeyLoginDays)
	h.enforcePolicy()
	w.WriteHeader(http.StatusNoContent)
}

// securityUsers는 인증 수단 기록이 있는 사용자 목록이다(관리자).
func (h *handler) securityUsers(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	names, err := h.Security.ListSecurity()
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	out := []api.SecurityUserInfo{}
	for _, name := range names {
		rec, err := h.Security.GetSecurity(name)
		if errors.Is(err, prefs.ErrSecurityRecord) {
			out = append(out, api.SecurityUserInfo{Username: name, Unreadable: true})
			continue
		}
		if err != nil {
			writeSecurityUnavailable(w, s, err)
			return
		}
		out = append(out, api.SecurityUserInfo{
			Username: name, Passkeys: len(rec.Passkeys), TOTP: rec.TOTP != nil, RecoveryCodes: len(rec.Recovery),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// resetSecurityUser는 사용자의 인증 수단을 모두 지우고 그 사용자의 대시보드 세션을 끝낸다(관리자, 2차 인증을 거친
// 세션). 이미 강화된 세션이 초기화 뒤에도 남지 않게 한다. 손상된 기록도 지운다.
func (h *handler) resetSecurityUser(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !satisfies(s, api.SecurityTwoFactor) {
		writeSecurityRequired(w, api.SecurityTwoFactor)
		return
	}
	name := r.PathValue("name")
	found, err := h.Security.DeleteSecurity(name)
	if err != nil {
		writeSecurityUnavailable(w, s, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "", "not found")
		return
	}
	// 초기화 전에 만든 기기 추가 코드로 다시 등록한 수단을 관리하지 못하게 지운다.
	h.deviceCodes.Delete(name)
	n := h.Sessions.DeleteUser(name)
	log.Printf("security: methods reset: user=%q by=%q ip=%s sessions_ended=%d", name, s.Username, clientIP(r), n)
	w.WriteHeader(http.StatusNoContent)
}
