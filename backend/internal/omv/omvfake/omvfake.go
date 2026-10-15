// Package omvfake는 테스트와 개발용 가짜 OMV rpc.php다.
//
// 실제 비밀번호 없이 로그인 흐름 전체를 검증하기 위해 쓴다. 동작은 OMV 8 소스
// (/var/www/openmediavault/rpc/session.inc, /usr/share/php/openmediavault/session.inc,
// rpc/proxy/json.inc)를 따른다.
//
//   - Session.login: 틀린 비밀번호는 HTTP 400. 2차 인증 계정은 challengeRequired를
//     반환하고, 새 세션 ID를 본문 없이 Set-Cookie로만 준다.
//   - Session.verify: 대기 중 로그인이 없거나 만료되면 401, 실패하면 400, 실패가
//     쌓이면 429.
//   - 인증된 세션은 timeout이 지나면 401("Session expired.")을 반환하고 파기된다.
//     options.updatelastaccess=false면 마지막 접근 시각을 갱신하지 않는다.
//   - 인증이 필요한 서비스(Session 밖)는 세션이 없으면 401.
//   - WebGui.getAutoLogoutSettings는 세션 timeout을 분 단위로 돌려준다(OMV 설정과 같은 단위).
//   - Smart.getList, Compose.getContainers, CpuTemp.get은 합성 값을 돌려준다(plugins.go).
//   - Exec.isRunning은 백그라운드 작업이 끝났는지 알려 준다(exec.go).
//   - 로그인 성공 시 OPENMEDIAVAULT-LOGIN-* 쿠키가 없으면 "새 브라우저 로그인"
//     메일을 보낸 것으로 센다(LoginMails).
//
// 실제 OMV보다 엄격한 점: User-Agent 검사를 Session.logout뿐 아니라 인증된 모든
// 호출에서 한다. 백엔드가 고정 User-Agent를 지키는지 확인하기 위해서다.
//
// 계정과 비밀번호는 모두 가짜 값이다(DefaultUsers).
package omvfake

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie     = "OPENMEDIAVAULT-SESSIONID"
	loginCookiePrefix = "OPENMEDIAVAULT-LOGIN-"
	pendingTTL        = 5 * time.Minute
	// fakeTrace는 실제 OMV처럼 오류 응답에 붙는 trace다. 백엔드가 이것을 브라우저로
	// 넘기지 않는지 테스트에서 확인한다.
	fakeTrace = "#0 /usr/share/php/openmediavault/fake.inc(1): omvfake trace"
)

// User는 가짜 계정이다.
type User struct {
	Name     string
	Password string
	Admin    bool
	// TOTP가 비어 있지 않으면 로그인에 2차 인증이 필요하고, 이 값이 맞는 코드다.
	TOTP string
	// 아래는 UserMgmt RPC가 주는 계정 정보다(usermgmt.go). 모두 합성 값이다.
	Email   string
	Comment string
	Groups  []string
	// System은 OMV 사용자 관리 목록에 없는 시스템 계정인지다(실제 OMV의 기본 admin 계정처럼).
	System bool
	// ReadOnly는 OMV에서 사용자 수정을 막았는지다(disallowusermod).
	ReadOnly bool
}

// DefaultUsers는 개발 서버(cmd/fakeomv)와 테스트가 쓰는 가짜 계정이다.
func DefaultUsers() []User {
	return []User{
		// 실제 OMV의 기본 관리자 계정처럼 시스템 계정이다.
		{Name: "admin", Password: "admin", Admin: true, System: true, Groups: []string{"openmediavault-admin"}},
		{Name: "user", Password: "user", Email: "user@example.com", Comment: "Example user", Groups: []string{"users"}},
		{Name: "totp", Password: "totp", Admin: true, TOTP: "123456", Email: "totp@example.com",
			Groups: []string{"users", "openmediavault-admin"}},
	}
}

// Call은 받은 호출 하나의 기록이다.
type Call struct {
	Service   string
	Method    string
	UserAgent string
	// ForwardedFor는 X-Forwarded-For 헤더 값이다.
	ForwardedFor string
	// SessionID는 X-OpenMediaVault-SessionID 헤더 또는 세션 쿠키로 받은 값이다.
	SessionID string
	// Touch는 이 호출이 마지막 접근 시각을 갱신하도록 요청했는지다.
	Touch bool
	// Params는 받은 RPC 파라미터 원문이다.
	Params json.RawMessage
}

// Server는 가짜 rpc.php다. http.Handler를 구현한다.
type Server struct {
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time

	mu       sync.Mutex
	users    map[string]User
	timeout  time.Duration
	sessions map[string]*session
	// verifyFail은 사용자별 2차 인증 실패 기록이다.
	verifyFail map[string]*failure
	calls      []Call
	mails      int
	plugin     PluginData
	// bg는 상태 파일 이름 → 백그라운드 작업이다(exec.go).
	bg      map[string]*bgJob
	bgPolls int
}

type session struct {
	authenticated bool
	username      string
	admin         bool
	userAgent     string
	lastAccess    time.Time
	pending       *pending
}

type pending struct {
	username string
	admin    bool
	expires  time.Time
}

type failure struct {
	attempts int
	last     time.Time
	until    time.Time
}

// New는 users 계정과 세션 timeout으로 가짜 서버를 만든다. timeout이 0이면 만료되지 않는다.
// OMV 설정처럼 분 단위로 준다(getAutoLogoutSettings가 분 단위로 알려 주므로).
func New(users []User, timeout time.Duration) *Server {
	s := &Server{
		Now:        time.Now,
		users:      make(map[string]User),
		timeout:    timeout,
		sessions:   make(map[string]*session),
		verifyFail: make(map[string]*failure),
		plugin:     DefaultPluginData(),
		bg:         make(map[string]*bgJob),
		bgPolls:    DefaultBgPolls,
	}
	for _, u := range users {
		s.users[u.Name] = u
	}
	return s
}

// Expire는 username의 모든 세션을 timeout이 지난 상태로 만든다.
// 다음 인증된 호출이 401("Session expired.")을 받는다.
func (s *Server) Expire(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ss := range s.sessions {
		if ss.authenticated && ss.username == username {
			ss.lastAccess = time.Time{}
		}
	}
}

// SetTimeout은 세션 timeout을 바꾼다. 웹 관리 설정을 바꾼 상황을 흉내 낸다.
func (s *Server) SetTimeout(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = d
}

// Calls는 지금까지 받은 호출 기록의 복사본이다.
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Call(nil), s.calls...)
}

// LoginMails는 "새 브라우저 로그인" 메일을 보냈을 횟수다.
func (s *Server) LoginMails() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mails
}

// ActiveSessions는 인증된 세션 수다.
func (s *Server) ActiveSessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, ss := range s.sessions {
		if ss.authenticated {
			n++
		}
	}
	return n
}

type rpcError struct {
	status  int
	message string
}

func errStatus(status int, message string) *rpcError { return &rpcError{status, message} }

// request는 호출 하나를 처리하는 동안의 상태다.
type request struct {
	w       http.ResponseWriter
	r       *http.Request
	id      string
	sess    *session
	cookies []*http.Cookie
}

type rpcBody struct {
	Service string          `json:"service"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Options map[string]any  `json:"options"`
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rpc.php" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var body rpcBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil ||
		body.Service == "" || body.Method == "" {
		writeError(w, nil, errStatus(http.StatusInternalServerError, "Failed to get the RPC parameters."))
		return
	}
	touch := true
	if v, ok := body.Options["updatelastaccess"].(bool); ok {
		touch = v
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	req := &request{w: w, r: r}
	s.start(req)
	s.calls = append(s.calls, Call{
		Service:      body.Service,
		Method:       body.Method,
		UserAgent:    r.UserAgent(),
		ForwardedFor: r.Header.Get("X-Forwarded-For"),
		SessionID:    req.id,
		Touch:        touch,
		Params:       body.Params,
	})

	result, rerr := s.handle(req, body, touch)
	// 로그인 실패 등으로 생긴 빈 세션은 남기지 않는다(개발 서버가 오래 떠 있어도 쌓이지 않게).
	if !req.sess.authenticated && req.sess.pending == nil {
		delete(s.sessions, req.id)
	}
	if rerr != nil {
		writeError(w, req.cookies, rerr)
		return
	}
	for _, ck := range req.cookies {
		http.SetCookie(w, ck)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"response": result, "error": nil})
}

func (s *Server) handle(req *request, body rpcBody, touch bool) (any, *rpcError) {
	// json.inc: 인증된 세션은 서비스와 상관없이 timeout을 먼저 검사한다.
	if req.sess.authenticated {
		if s.timedOut(req.sess) {
			s.destroy(req)
			return nil, errStatus(http.StatusUnauthorized, "Session expired.")
		}
		if req.sess.userAgent != req.r.UserAgent() {
			s.destroy(req)
			return nil, errStatus(http.StatusUnauthorized, "Invalid User-Agent.")
		}
		if touch {
			req.sess.lastAccess = s.Now()
		}
	}

	if body.Service == "Session" {
		switch body.Method {
		case "login":
			return s.login(req, body.Params)
		case "verify":
			return s.verify(req, body.Params)
		case "logout":
			return s.logout(req)
		}
		return nil, errStatus(http.StatusInternalServerError, "The method '"+body.Method+"' does not exist for the RPC service 'Session'.")
	}

	// Session 밖의 서비스는 엔진으로 넘어가므로 인증이 필요하다.
	if !req.sess.authenticated {
		s.destroy(req)
		return nil, errStatus(http.StatusUnauthorized, "Session not authenticated.")
	}
	if body.Service == "WebGui" && body.Method == "getAutoLogoutSettings" {
		return map[string]any{"timeout": int(s.timeout / time.Minute)}, nil
	}
	if body.Service == "UserMgmt" {
		if res, rerr, ok := s.handleUserMgmt(req.sess, body.Method, body.Params); ok {
			return res, rerr
		}
	}
	if body.Service == "Exec" && body.Method == "isRunning" {
		return s.execIsRunning(req.sess, body.Params)
	}
	if res, rerr, ok := s.handlePlugin(req.sess, body.Service, body.Method, body.Params); ok {
		return res, rerr
	}
	return nil, errStatus(http.StatusNotFound, "RPC service '"+body.Service+"' not found.")
}

// start는 PHP session_start처럼 헤더나 쿠키의 세션 ID로 세션을 찾고, 없으면 새로 만든다.
func (s *Server) start(req *request) {
	id := req.r.Header.Get("X-OpenMediaVault-SessionID")
	if id == "" {
		if ck, err := req.r.Cookie(sessionCookie); err == nil {
			id = ck.Value
		}
	}
	if ss, ok := s.sessions[id]; ok && id != "" {
		req.id, req.sess = id, ss
		return
	}
	req.id, req.sess = newID(), &session{}
	s.sessions[req.id] = req.sess
	req.setSessionCookie()
}

// regenerate는 session_regenerate_id처럼 세션 데이터를 새 ID로 옮긴다.
func (s *Server) regenerate(req *request) {
	delete(s.sessions, req.id)
	req.id = newID()
	s.sessions[req.id] = req.sess
	req.setSessionCookie()
}

func (s *Server) destroy(req *request) {
	delete(s.sessions, req.id)
	req.sess = &session{}
	req.cookies = append(removeCookie(req.cookies, sessionCookie),
		&http.Cookie{Name: sessionCookie, Value: "deleted", Path: "/", MaxAge: -1})
}

func (req *request) setSessionCookie() {
	req.cookies = append(removeCookie(req.cookies, sessionCookie),
		&http.Cookie{Name: sessionCookie, Value: req.id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (s *Server) timedOut(ss *session) bool {
	return s.timeout > 0 && s.Now().Sub(ss.lastAccess) > s.timeout
}

func (s *Server) login(req *request, raw json.RawMessage) (any, *rpcError) {
	var p struct {
		Username *string `json:"username"`
		Password *string `json:"password"`
	}
	if json.Unmarshal(raw, &p) != nil || p.Username == nil || p.Password == nil {
		return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters.")
	}
	name := strings.TrimSpace(*p.Username)
	if f := s.verifyFail[name]; f != nil && s.Now().Before(f.until) {
		return nil, errStatus(http.StatusTooManyRequests, "Too many failed verification attempts. Please try again later.")
	}
	u, ok := s.users[name]
	if !ok || u.Password != *p.Password {
		return nil, errStatus(http.StatusBadRequest, "Incorrect username or password.")
	}

	if u.TOTP != "" {
		if req.sess.authenticated && req.sess.username != name {
			return nil, errStatus(http.StatusBadRequest, "Another user is already authenticated.")
		}
		s.regenerate(req)
		req.sess.pending = &pending{username: name, admin: u.Admin, expires: s.Now().Add(pendingTTL)}
		return map[string]any{
			"username":  name,
			"status":    "challengeRequired",
			"challenge": map[string]any{"kind": "totp"},
		}, nil
	}

	id, rerr := s.initialize(req, name, u.Admin)
	if rerr != nil {
		return nil, rerr
	}
	return map[string]any{
		"username":    name,
		"status":      "authenticated",
		"permissions": permissions(u.Admin),
		"sessionid":   id,
	}, nil
}

func (s *Server) verify(req *request, raw json.RawMessage) (any, *rpcError) {
	pd := req.sess.pending
	if pd == nil || s.Now().After(pd.expires) {
		req.sess.pending = nil
		return nil, errStatus(http.StatusUnauthorized, "No pending login or the login has expired.")
	}
	now := s.Now()
	f := s.verifyFail[pd.username]
	if f != nil && now.Before(f.until) {
		return nil, errStatus(http.StatusTooManyRequests, "Too many failed verification attempts. Please try again later.")
	}
	var p struct {
		ChallengeResponse any `json:"challengeresponse"`
	}
	json.Unmarshal(raw, &p)
	if code, _ := p.ChallengeResponse.(string); code == "" || code != s.users[pd.username].TOTP {
		// session.inc: 15분 안의 실패를 세고, 3번째부터 5초에서 두 배씩 늘려 막는다.
		if f == nil || now.Sub(f.last) > 15*time.Minute {
			f = &failure{}
			s.verifyFail[pd.username] = f
		}
		f.attempts++
		f.last = now
		if f.attempts > 2 {
			f.until = now.Add(min(300*time.Second, 5*time.Second<<min(f.attempts-3, 10)))
			return nil, errStatus(http.StatusTooManyRequests, "Too many failed verification attempts. Please try again later.")
		}
		return nil, errStatus(http.StatusBadRequest, "Challenge verification failed.")
	}
	delete(s.verifyFail, pd.username)

	id, rerr := s.initialize(req, pd.username, pd.admin)
	if rerr != nil {
		return nil, rerr
	}
	return map[string]any{
		"status":      "authenticated",
		"username":    pd.username,
		"permissions": permissions(pd.admin),
		"sessionid":   id,
	}, nil
}

func (s *Server) logout(req *request) (any, *rpcError) {
	if !req.sess.authenticated {
		s.destroy(req)
		return nil, errStatus(http.StatusUnauthorized, "Session not authenticated.")
	}
	s.destroy(req)
	return nil, nil
}

// initialize는 Session::initialize와 handleSuccessfulLogin을 흉내 낸다.
func (s *Server) initialize(req *request, name string, admin bool) (string, *rpcError) {
	if req.sess.authenticated {
		if req.sess.username != name {
			return "", errStatus(http.StatusBadRequest, "Another user is already authenticated.")
		}
		req.sess.pending = nil
		return req.id, nil
	}
	s.regenerate(req)
	*req.sess = session{
		authenticated: true,
		username:      name,
		admin:         admin,
		userAgent:     req.r.UserAgent(),
		lastAccess:    s.Now(),
	}

	key := loginCookiePrefix + hex.EncodeToString([]byte(name))
	if _, err := req.r.Cookie(key); err != nil {
		s.mails++
		req.cookies = append(req.cookies, &http.Cookie{
			Name: key, Value: "Fear is the mind-killer.", MaxAge: 60 * 60 * 24 * 60,
			HttpOnly: true, SameSite: http.SameSiteStrictMode,
		})
	}
	return req.id, nil
}

func permissions(admin bool) map[string]string {
	if admin {
		return map[string]string{"role": "admin"}
	}
	return map[string]string{"role": "user"}
}

func writeError(w http.ResponseWriter, cookies []*http.Cookie, e *rpcError) {
	for _, ck := range cookies {
		http.SetCookie(w, ck)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.status)
	json.NewEncoder(w).Encode(map[string]any{
		"response": nil,
		"error":    map[string]any{"code": 0, "message": e.message, "trace": fakeTrace},
	})
}

func removeCookie(cookies []*http.Cookie, name string) []*http.Cookie {
	out := cookies[:0]
	for _, ck := range cookies {
		if ck.Name != name {
			out = append(out, ck)
		}
	}
	return out
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
