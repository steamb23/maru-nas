package server

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/omv"
)

const (
	// SessionCookie는 대시보드 세션 쿠키 이름이다.
	SessionCookie = "onmaru_session"
	// PendingCookie는 2차 인증 대기 쿠키 이름이다.
	PendingCookie = "onmaru_pending"
	// DeviceCookie는 신뢰 기기 토큰 쿠키 이름이다(auth.Limiter.Trust). 로그아웃해도 지우지 않는다.
	DeviceCookie = "onmaru_device"
	// maxBodyBytes는 인증 요청 본문의 최대 크기다.
	maxBodyBytes = 4 << 10
)

// 로그인 실패 응답 문구. 사용자 이름이 없는지 비밀번호가 틀렸는지 구분하지 않는다.
const msgInvalidCredentials = "Incorrect username or password."

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var req api.LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "username and password are required")
		return
	}
	ip := clientIP(r)
	done, wait := h.Limiter.Begin(ip, req.Username, deviceToken(r))
	if done == nil {
		log.Printf("login rate limited: user=%q ip=%s", req.Username, ip)
		writeRateLimited(w, wait)
		return
	}

	res, err := h.OMV.Login(r.Context(), req.Username, req.Password, ip)
	if err != nil {
		h.authFailed(w, r, done, err, "login", req.Username, ip)
		return
	}
	done(auth.Success)

	if res.Status == omv.StatusChallengeRequired {
		p := h.Sessions.CreatePending(res.Username, res.SessionID)
		h.setCookie(w, r, PendingCookie, p.ID, int(auth.PendingTTL/time.Second))
		log.Printf("login challenge required: user=%q ip=%s kind=%q", res.Username, ip, res.Challenge.Kind)
		writeJSON(w, http.StatusOK, api.LoginResponse{
			Status:    api.LoginChallengeRequired,
			Challenge: &api.ChallengeInfo{Kind: res.Challenge.Kind, RedirectURL: res.Challenge.RedirectURL},
		})
		return
	}
	h.startSession(w, r, res, ip)
}

func (h *handler) verify(w http.ResponseWriter, r *http.Request) {
	var req api.VerifyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	p, ok := h.pending(r)
	if !ok {
		h.clearCookie(w, r, PendingCookie)
		writeError(w, http.StatusUnauthorized, api.ErrChallengeExpired, "no pending login or the login has expired")
		return
	}
	ip := clientIP(r)
	done, wait := h.Limiter.Begin(ip, p.Username, deviceToken(r))
	if done == nil {
		log.Printf("verify rate limited: user=%q ip=%s", p.Username, ip)
		writeRateLimited(w, wait)
		return
	}

	res, err := h.OMV.Verify(r.Context(), p.OMVSessionID, p.Username, req.Response, ip)
	if err != nil {
		if omv.IsUnauthorized(err) {
			done(auth.Neutral)
			h.Sessions.DeletePending(p.ID)
			h.clearCookie(w, r, PendingCookie)
			writeError(w, http.StatusUnauthorized, api.ErrChallengeExpired, "no pending login or the login has expired")
			return
		}
		h.authFailed(w, r, done, err, "verify", p.Username, ip)
		return
	}
	done(auth.Success)
	h.Sessions.DeletePending(p.ID)
	if res.Status != omv.StatusAuthenticated {
		log.Printf("verify: unexpected status %q for user=%q", res.Status, p.Username)
		writeError(w, http.StatusBadGateway, api.ErrOMVUnavailable, "unexpected response from OpenMediaVault")
		return
	}
	h.startSession(w, r, res, ip)
}

// authFailed는 로그인·2차 인증 실패를 응답한다. 틀린 값이면 FailDelay만큼 기다린다.
func (h *handler) authFailed(w http.ResponseWriter, r *http.Request, done func(auth.Outcome), err error, step, username, ip string) {
	switch omv.HTTPStatus(err) {
	case http.StatusBadRequest:
		done(auth.Failure)
		log.Printf("%s failed: user=%q ip=%s", step, username, ip)
		sleep(r.Context(), h.FailDelay)
		if step == "verify" {
			writeError(w, http.StatusBadRequest, api.ErrChallengeFailed, "challenge verification failed")
		} else {
			writeError(w, http.StatusBadRequest, api.ErrInvalidCredentials, msgInvalidCredentials)
		}
	case http.StatusTooManyRequests:
		// OMV 자체의 2차 인증 실패 제한. 남은 시간은 알려 주지 않는다.
		done(auth.Failure)
		log.Printf("%s rate limited by omv: user=%q ip=%s", step, username, ip)
		writeError(w, http.StatusTooManyRequests, api.ErrRateLimited, "too many failed attempts, try again later")
	default:
		done(auth.Neutral)
		log.Printf("%s: omv error: %v", step, err)
		writeError(w, http.StatusBadGateway, api.ErrOMVUnavailable, "cannot reach OpenMediaVault")
	}
}

func (h *handler) startSession(w http.ResponseWriter, r *http.Request, res omv.LoginResult, ip string) {
	s := h.Sessions.Create(res.Username, res.Admin, res.SessionID)
	// 로그인이 OMV 세션의 마지막 접근 시각을 갱신했으므로 만료 시각 계산에 쓸 timeout만 읽는다.
	if st, err := h.OMV.Probe(r.Context(), s.OMVSessionID, false); err != nil {
		log.Printf("login: read auto logout timeout: %v", err)
	} else {
		s, _ = h.Sessions.Update(s.ID, func(ss *auth.Session) { applyState(ss, st) })
	}
	// 브라우저가 기존 세션 쿠키를 갖고 있었다면 그 세션은 버린다.
	if old, err := r.Cookie(SessionCookie); err == nil {
		h.Sessions.Delete(old.Value)
	}
	h.setCookie(w, r, SessionCookie, s.ID, 0)
	h.clearCookie(w, r, PendingCookie)
	// 인증을 끝까지 마친 브라우저를 신뢰 기기로 등록한다. 이후 이 사용자 이름의 잠금을 받지 않는다.
	h.setCookie(w, r, DeviceCookie, h.Limiter.Trust(res.Username, deviceToken(r)), int(auth.DeviceTTL/time.Second))
	// 실행 중에 플러그인을 설치하거나 지웠을 수 있으므로 로그인할 때 다시 확인한다.
	h.Plugins.Refresh()
	log.Printf("login ok: user=%q ip=%s admin=%t", s.Username, ip, s.Admin)
	info := h.sessionInfo(s)
	writeJSON(w, http.StatusOK, api.LoginResponse{Status: api.LoginAuthenticated, Session: &info})
}

// applyState는 Probe로 읽은 timeout을 세션에 반영한다. 읽지 못했으면 이전 값을 둔다.
func applyState(ss *auth.Session, st omv.SessionState) {
	if st.TimeoutKnown {
		ss.Timeout, ss.TimeoutKnown = st.Timeout, true
	}
}

// sessionInfo는 세션의 응답 형태다. 만료까지 남은 시간을 함께 담는다.
func (h *handler) sessionInfo(s auth.Session) api.SessionInfo {
	info := api.SessionInfo{Username: s.Username, Admin: s.Admin}
	if exp, ok := s.ExpiresAt(); ok {
		secs := max(0, int(exp.Sub(h.Now())/time.Second))
		info.ExpiresInSeconds = &secs
	}
	return info
}

func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	if ck, err := r.Cookie(SessionCookie); err == nil {
		if s, ok := h.Sessions.Delete(ck.Value); ok {
			// 로컬 세션은 OMV 응답과 상관없이 지운다.
			if err := h.OMV.Logout(r.Context(), s.OMVSessionID); err != nil && !omv.IsUnauthorized(err) {
				log.Printf("logout: omv error: %v", err)
			}
			log.Printf("logout: user=%q ip=%s", s.Username, clientIP(r))
		}
	}
	h.clearCookie(w, r, SessionCookie)
	h.clearCookie(w, r, PendingCookie)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) session(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	writeJSON(w, http.StatusOK, h.sessionInfo(s))
}

// activity는 사용자가 화면을 조작했다는 신호를 받아 OMV 세션의 마지막 접근 시각을 갱신하고,
// 새 만료 시각을 담은 세션 정보를 반환한다. 주기 요청(/api/system 등)은 OMV 세션을 연장하지
// 않으므로, 사용자 입력이 있을 때만 이 요청으로 연장한다.
func (h *handler) activity(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !requireJSON(w, r) {
		return
	}
	now := h.Now()
	st, err := h.OMV.Probe(r.Context(), s.OMVSessionID, true)
	switch {
	case omv.IsUnauthorized(err):
		h.expire(w, r, s)
		return
	case err != nil:
		log.Printf("activity: omv error: %v", err)
	default:
		s, _ = h.Sessions.Update(s.ID, func(ss *auth.Session) {
			ss.LastTouch, ss.LastOMVCheck = now, now
			applyState(ss, st)
		})
	}
	writeJSON(w, http.StatusOK, h.sessionInfo(s))
}

// requireSession은 대시보드 세션이 있어야 next를 부른다. 없으면 401을 반환한다.
// 마지막 확인에서 CheckInterval이 지났거나 예상 만료 시각이 지났으면 OMV 세션이 살아
// 있는지 확인한다(마지막 접근 시각은 갱신하지 않음). OMV가 401을 반환하면 대시보드 세션도
// 지운다. OMV에 연결할 수 없으면 세션을 유지한 채 통과시켜 /proc 현황은 계속 보이게 한다.
func (h *handler) requireSession(next func(http.ResponseWriter, *http.Request, auth.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie(SessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, api.ErrUnauthorized, "not logged in")
			return
		}
		s, ok := h.Sessions.Get(ck.Value)
		if !ok {
			h.clearCookie(w, r, SessionCookie)
			writeError(w, http.StatusUnauthorized, api.ErrUnauthorized, "not logged in")
			return
		}
		now := h.Now()
		exp, known := s.ExpiresAt()
		if now.Sub(s.LastOMVCheck) >= h.CheckInterval || (known && !now.Before(exp)) {
			st, err := h.OMV.Probe(r.Context(), s.OMVSessionID, false)
			switch {
			case omv.IsUnauthorized(err):
				h.expire(w, r, s)
				return
			case err != nil:
				log.Printf("session check: omv error: %v", err)
			default:
				s, _ = h.Sessions.Update(s.ID, func(ss *auth.Session) {
					ss.LastOMVCheck = now
					applyState(ss, st)
				})
			}
		}
		next(w, r, s)
	}
}

// expire는 OMV 세션이 만료된 대시보드 세션을 지우고 401을 반환한다.
func (h *handler) expire(w http.ResponseWriter, r *http.Request, s auth.Session) {
	h.Sessions.Delete(s.ID)
	h.clearCookie(w, r, SessionCookie)
	log.Printf("omv session expired: user=%q", s.Username)
	writeError(w, http.StatusUnauthorized, api.ErrUnauthorized, "session expired")
}

// deviceToken은 브라우저가 보낸 신뢰 기기 토큰이다. 없으면 빈 문자열이다.
func deviceToken(r *http.Request) string {
	if ck, err := r.Cookie(DeviceCookie); err == nil {
		return ck.Value
	}
	return ""
}

func (h *handler) pending(r *http.Request) (auth.Pending, bool) {
	ck, err := r.Cookie(PendingCookie)
	if err != nil {
		return auth.Pending{}, false
	}
	return h.Sessions.GetPending(ck.Value)
}

// setCookie는 HttpOnly, SameSite=Strict 쿠키를 설정한다. maxAge가 0이면 세션 쿠키다.
// Secure는 HTTPS로 들어온 요청에만 붙인다. 개발 서버(Vite, 평문 HTTP)에서는
// 브라우저가 Secure 쿠키를 저장하지 않기 때문이다. 백엔드는 루프백에만 바인딩하므로
// X-Forwarded-Proto는 앞단 프록시(Caddy)가 보낸 값이다.
func (h *handler) setCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     h.CookiePath,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *handler) clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	if _, err := r.Cookie(name); err != nil {
		return
	}
	h.setCookie(w, r, name, "", -1)
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// clientIP는 브라우저 IP다. 앞단 프록시가 붙인 X-Forwarded-For의 마지막 항목을 쓴다.
// 앞쪽 항목은 브라우저가 임의로 보낼 수 있으므로 믿지 않는다.
// 알 수 없으면 빈 문자열이다(Unix 소켓으로 들어온 요청은 RemoteAddr가 IP가 아니다).
// 빈 값끼리는 로그인 시도 제한에서 같은 IP로 취급된다.
func clientIP(r *http.Request) string {
	if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
		parts := strings.Split(xff[len(xff)-1], ",")
		if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
			return ip.String()
		}
	}
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

// requireJSON은 본문이 있는 요청(POST, PUT)의 Content-Type이 application/json인지 확인한다.
// SameSite=Strict 쿠키와 함께 쓰는 CSRF 보조 장치다(다른 출처의 폼은 이 형식으로 보낼 수 없다).
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, api.ErrBadRequest, "content type must be application/json")
		return false
	}
	return true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONLimit(w, r, v, maxBodyBytes)
}

// decodeJSONLimit은 limit 바이트까지의 JSON 본문을 v로 읽는다. 실패하면 오류 응답을 쓰고 false를 반환한다.
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	if !requireJSON(w, r) {
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid request body")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, api.ErrorResponse{Error: msg, Code: code})
}

func writeRateLimited(w http.ResponseWriter, wait time.Duration) {
	secs := int(math.Ceil(wait.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSON(w, http.StatusTooManyRequests, api.ErrorResponse{
		Error:             "too many failed attempts, try again later",
		Code:              api.ErrRateLimited,
		RetryAfterSeconds: secs,
	})
}

// sleep은 d만큼 기다린다. 요청이 취소되면 바로 돌아온다.
func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}
