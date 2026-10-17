package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	helperclient "onmaru/backend/internal/helper/client"
	"onmaru/backend/internal/helper/helperfake"
	"onmaru/backend/internal/omv"
	"onmaru/backend/internal/omv/omvfake"
	"onmaru/backend/internal/prefs"
	"onmaru/backend/internal/terminal"
	"onmaru/backend/internal/terminal/sshfake"
)

type fakeSource struct {
	resp api.SystemResponse
	err  error
}

func (f fakeSource) System() (api.SystemResponse, error) { return f.resp, f.err }

var static = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("static"))
})

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// env는 백엔드 핸들러 + 실제 omv.Client + 가짜 OMV(omvfake)를 묶은 테스트 환경이다.
type env struct {
	t     *testing.T
	h     http.Handler
	fake  *omvfake.Server
	omvTS *httptest.Server
	clk   *clock
	store *auth.Store
	logs  *bytes.Buffer
	// helper는 envOptions.helper일 때 띄운 가짜 권한 도우미다.
	helper *helperfake.Server
	// sshd, terminal은 envOptions.terminal일 때 띄운 가짜 sshd와 웹 터미널 접속이다.
	sshd     *sshfake.Server
	terminal *terminal.Manager
}

type envOptions struct {
	system     SystemSource
	omvTimeout time.Duration
	failDelay  time.Duration
	interval   time.Duration
	// omvTimeoutOff면 OMV 자동 로그아웃을 끈다(timeout 0).
	omvTimeoutOff bool
	brokenOMV     bool
	omvOverride   OMV
	// wrapOMV가 있으면 실제 클라이언트를 감싼다(호출 기록 등).
	wrapOMV func(OMV) OMV
	addons  *addons.Registry
	plugins PluginDetector
	prefs   prefs.Store
	// workbenchURL은 Config.WorkbenchURL이다.
	workbenchURL string
	// site, images, smartAt, setSmart, runtime은 Config의 같은 이름 필드다.
	site     prefs.SiteStore
	images   prefs.ImageStore
	smartAt  string
	setSmart func(hour, minute int)
	runtime  []api.RuntimeSetting
	// helper면 가짜 권한 도우미를 띄워 연결한다. helperSocket만 주면 그 경로(도우미 없음)에 연결한다.
	helper       bool
	helperSocket string
	// terminal이면 가짜 sshd(설정 sshd)를 띄워 웹 터미널을 연결하고 -terminal을 켠다.
	// terminalOff면 연결은 하되 -terminal을 끈 채로 둔다.
	terminal    bool
	terminalOff bool
	sshd        sshfake.Options
}

func newEnv(t *testing.T, o envOptions) *env {
	t.Helper()
	clk := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	if o.omvTimeout == 0 && !o.omvTimeoutOff {
		o.omvTimeout = 5 * time.Minute
	}
	if o.failDelay == 0 {
		o.failDelay = -1
	}
	if o.system == nil {
		o.system = fakeSource{resp: api.SystemResponse{UptimeSeconds: 1}}
	}
	fake := omvfake.New(omvfake.DefaultUsers(), o.omvTimeout)
	fake.Now = clk.Now
	ts := httptest.NewServer(fake)
	t.Cleanup(ts.Close)
	if o.brokenOMV {
		ts.Close()
	}

	store := auth.NewStore()
	store.Now = clk.Now
	lim := auth.NewLimiter()
	lim.Now = clk.Now
	var client OMV = omv.NewClient(ts.URL, "onmaru-test/1")
	if o.omvOverride != nil {
		client = o.omvOverride
	}
	if o.wrapOMV != nil {
		client = o.wrapOMV(client)
	}

	logs := &bytes.Buffer{}
	prev := log.Writer()
	log.SetOutput(logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	var helper Helper
	var fakeHelper *helperfake.Server
	if o.helper {
		dir := t.TempDir()
		var err error
		fakeHelper, err = helperfake.Start(filepath.Join(dir, "helper.sock"), filepath.Join(dir, "root"),
			helperfake.DefaultUsers(), helperfake.Options{Now: clk.Now, FailDelay: -1, Logf: func(string, ...any) {}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { fakeHelper.Close() })
		helper = helperclient.New(fakeHelper.Socket)
	} else if o.helperSocket != "" {
		helper = helperclient.New(o.helperSocket)
	}

	var sshd *sshfake.Server
	var manager *terminal.Manager
	if o.terminal {
		dir := t.TempDir()
		opts := o.sshd
		opts.Users = sshfake.DefaultUsers()
		if fakeHelper != nil {
			opts.Home = fakeHelper.Home
		}
		var err error
		if sshd, err = sshfake.Start("127.0.0.1:0", opts); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(sshd.Close)
		if _, err := sshd.WriteHostKey(dir); err != nil {
			t.Fatal(err)
		}
		manager = terminal.NewManager(&terminal.Dialer{Addr: sshd.Addr, HostKeys: filepath.Join(dir, "*.pub"),
			SSHDConfig: filepath.Join(dir, "sshd_config")})
	}

	h := New(Config{
		System:            o.system,
		Static:            static,
		OMV:               client,
		Sessions:          store,
		Limiter:           lim,
		Addons:            o.addons,
		Plugins:           o.plugins,
		Prefs:             o.prefs,
		WorkbenchURL:      o.workbenchURL,
		Site:              o.site,
		Images:            o.images,
		SmartRefreshAt:    o.smartAt,
		SetSmartRefreshAt: o.setSmart,
		Runtime:           o.runtime,
		Helper:            helper,
		Terminal:          manager,
		TerminalEnabled:   o.terminal && !o.terminalOff,
		CheckInterval:     o.interval,
		FailDelay:         o.failDelay,
		Now:               clk.Now,
	})
	return &env{t: t, h: h, fake: fake, omvTS: ts, clk: clk, store: store, logs: logs, helper: fakeHelper,
		sshd: sshd, terminal: manager}
}

// browser는 쿠키를 보관하는 간이 브라우저다.
type browser struct {
	e       *env
	cookies map[string]*http.Cookie
	header  http.Header
}

func (e *env) browser() *browser {
	return &browser{e: e, cookies: map[string]*http.Cookie{}, header: http.Header{}}
}

func (b *browser) do(method, path string, body any) *httptest.ResponseRecorder {
	b.e.t.Helper()
	var rd io.Reader
	if body != nil {
		buf, _ := json.Marshal(body)
		rd = bytes.NewReader(buf)
	}
	req := httptest.NewRequest(method, path, rd)
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}
	return b.send(req)
}

// raw는 JSON이 아닌 본문(올리는 이미지 등)을 contentType으로 보낸다. contentType이 비어 있으면 헤더를 넣지 않는다.
func (b *browser) raw(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	b.e.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return b.send(req)
}

// send는 브라우저 헤더와 쿠키를 붙여 요청을 보내고 응답 쿠키를 보관한다.
func (b *browser) send(req *http.Request) *httptest.ResponseRecorder {
	for k, v := range b.header {
		req.Header[k] = v
	}
	for _, ck := range b.cookies {
		req.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
	}
	rec := httptest.NewRecorder()
	b.e.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 {
			delete(b.cookies, ck.Name)
		} else {
			b.cookies[ck.Name] = ck
		}
	}
	return rec
}

func (b *browser) login(user, pass string) *httptest.ResponseRecorder {
	return b.do(http.MethodPost, "/api/login", api.LoginRequest{Username: user, Password: pass})
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
}

func wantCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) api.ErrorResponse {
	t.Helper()
	wantStatus(t, rec, status)
	got := decode[api.ErrorResponse](t, rec)
	if got.Code != code {
		t.Fatalf("code %q, want %q", got.Code, code)
	}
	return got
}

func TestHealthAndStatic(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.do(http.MethodGet, "/api/health", nil), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/nope", nil), http.StatusNotFound)
	if rec := b.do(http.MethodGet, "/index.html", nil); rec.Body.String() != "static" {
		t.Errorf("static body %q", rec.Body.String())
	}
}

func TestRequiresLogin(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantCode(t, b.do(http.MethodGet, "/api/system", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	wantCode(t, b.do(http.MethodGet, "/api/session", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	b.cookies[SessionCookie] = &http.Cookie{Name: SessionCookie, Value: "forged"}
	wantCode(t, b.do(http.MethodGet, "/api/system", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	if _, ok := b.cookies[SessionCookie]; ok {
		t.Error("invalid session cookie not cleared")
	}
}

func TestLoginAndSession(t *testing.T) {
	want := api.SystemResponse{
		CPU:           api.CPUInfo{UsagePercent: 12.5},
		Memory:        api.MemoryInfo{TotalBytes: 100, UsedBytes: 40, AvailableBytes: 60},
		UptimeSeconds: 99,
	}
	e := newEnv(t, envOptions{system: fakeSource{resp: want}})
	b := e.browser()
	rec := b.login("admin", "admin")
	wantStatus(t, rec, http.StatusOK)
	got := decode[api.LoginResponse](t, rec)
	if got.Status != api.LoginAuthenticated || got.Session == nil || got.Session.Username != "admin" || !got.Session.Admin {
		t.Fatalf("login response %+v", got)
	}

	ck := b.cookies[SessionCookie]
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" || ck.Secure {
		t.Fatalf("session cookie %+v", ck)
	}

	// 새로고침·새 탭: 같은 쿠키로 세션을 복원한다.
	rec = b.do(http.MethodGet, "/api/session", nil)
	wantStatus(t, rec, http.StatusOK)
	if s := decode[api.SessionInfo](t, rec); s.Username != "admin" || !s.Admin {
		t.Errorf("session %+v", s)
	}
	rec = b.do(http.MethodGet, "/api/system", nil)
	wantStatus(t, rec, http.StatusOK)
	if s := decode[api.SystemResponse](t, rec); s != want {
		t.Errorf("system %+v", s)
	}

	u := e.browser()
	wantStatus(t, u.login("user", "user"), http.StatusOK)
	if s := decode[api.SessionInfo](t, u.do(http.MethodGet, "/api/session", nil)); s.Admin {
		t.Error("non-admin reported as admin")
	}
}

func TestSecureCookieBehindHTTPS(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	b.header.Set("X-Forwarded-Proto", "https")
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	if ck := b.cookies[SessionCookie]; ck == nil || !ck.Secure {
		t.Fatalf("cookie not secure behind https: %+v", ck)
	}
}

func TestWrongPassword(t *testing.T) {
	e := newEnv(t, envOptions{failDelay: 50 * time.Millisecond})
	b := e.browser()
	start := time.Now()
	rec := b.login("admin", "wrong")
	if d := time.Since(start); d < 50*time.Millisecond {
		t.Errorf("failure answered after %v, want delay", d)
	}
	got := wantCode(t, rec, http.StatusBadRequest, api.ErrInvalidCredentials)
	if strings.Contains(rec.Body.String(), "trace") || strings.Contains(got.Error, ".inc") {
		t.Errorf("body leaks omv trace: %s", rec.Body.String())
	}
	if _, ok := b.cookies[SessionCookie]; ok {
		t.Error("session cookie set on failure")
	}
	// 없는 사용자도 같은 응답이다.
	wantCode(t, b.login("nobody", "x"), http.StatusBadRequest, api.ErrInvalidCredentials)
	wantCode(t, b.login("", "x"), http.StatusBadRequest, api.ErrBadRequest)
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	for range auth.FreeFailures + 1 {
		wantCode(t, b.login("admin", "wrong"), http.StatusBadRequest, api.ErrInvalidCredentials)
	}
	// 잠긴 동안에는 맞는 비밀번호도 OMV에 보내지 않는다.
	before := len(e.fake.Calls())
	rec := b.login("admin", "admin")
	got := wantCode(t, rec, http.StatusTooManyRequests, api.ErrRateLimited)
	if rec.Header().Get("Retry-After") != "30" || got.RetryAfterSeconds != 30 {
		t.Errorf("retry after %q / %d", rec.Header().Get("Retry-After"), got.RetryAfterSeconds)
	}
	if len(e.fake.Calls()) != before {
		t.Error("locked login reached omv")
	}
	e.clk.Add(auth.BaseLockout)
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
}

func TestTrustedDeviceSkipsUserLock(t *testing.T) {
	e := newEnv(t, envOptions{})
	from := func(ip string) *browser {
		b := e.browser()
		b.header.Set("X-Forwarded-For", ip)
		return b
	}
	owner := from("192.0.2.10")
	wantStatus(t, owner.login("admin", "admin"), http.StatusOK)
	dev, ok := owner.cookies[DeviceCookie]
	if !ok || !dev.HttpOnly || dev.SameSite != http.SameSiteStrictMode || dev.MaxAge != int(auth.DeviceTTL/time.Second) {
		t.Fatalf("device cookie %+v", dev)
	}
	// 로그아웃해도 신뢰 기기 쿠키는 남는다.
	wantStatus(t, owner.do(http.MethodPost, "/api/logout", struct{}{}), http.StatusNoContent)
	if _, ok := owner.cookies[DeviceCookie]; !ok {
		t.Fatal("device cookie cleared on logout")
	}

	attacker := from("198.51.100.7")
	for range auth.FreeFailures + 1 {
		wantCode(t, attacker.login("admin", "wrong"), http.StatusBadRequest, api.ErrInvalidCredentials)
	}
	wantCode(t, from("192.0.2.11").login("admin", "admin"), http.StatusTooManyRequests, api.ErrRateLimited)
	// 신뢰 기기는 잠긴 사용자 이름으로도 로그인할 수 있고, 새 토큰을 받는다.
	wantStatus(t, owner.login("admin", "admin"), http.StatusOK)
	if owner.cookies[DeviceCookie].Value == dev.Value {
		t.Error("device token not rotated on login")
	}
}

func TestDeviceCookieAfterChallenge(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("totp", "totp"), http.StatusOK)
	if _, ok := b.cookies[DeviceCookie]; ok {
		t.Fatal("device cookie issued before the challenge was answered")
	}
	wantStatus(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}), http.StatusOK)
	if _, ok := b.cookies[DeviceCookie]; !ok {
		t.Error("device cookie not issued after the challenge")
	}
}

func TestRequireJSON(t *testing.T) {
	e := newEnv(t, envOptions{})
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("username=admin&password=admin"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	wantStatus(t, rec, http.StatusUnsupportedMediaType)
	wantStatus(t, e.browser().do(http.MethodGet, "/api/login", nil), http.StatusNotFound)
}

func TestLogout(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	stolen := b.cookies[SessionCookie].Value

	wantStatus(t, b.do(http.MethodPost, "/api/logout", struct{}{}), http.StatusNoContent)
	if _, ok := b.cookies[SessionCookie]; ok {
		t.Error("session cookie not cleared")
	}
	wantCode(t, b.do(http.MethodGet, "/api/system", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	// 지운 세션 ID를 다시 보내도 401이다.
	b.cookies[SessionCookie] = &http.Cookie{Name: SessionCookie, Value: stolen}
	wantCode(t, b.do(http.MethodGet, "/api/system", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	if n := e.fake.ActiveSessions(); n != 0 {
		t.Errorf("omv sessions left %d", n)
	}
	// 로그인하지 않은 상태의 로그아웃도 성공한다.
	wantStatus(t, e.browser().do(http.MethodPost, "/api/logout", struct{}{}), http.StatusNoContent)
}

func TestOMVSessionExpiry(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	e.fake.Expire("admin")

	// 확인 주기 안에서는 OMV에 묻지 않는다.
	wantStatus(t, b.do(http.MethodGet, "/api/system", nil), http.StatusOK)
	e.clk.Add(60 * time.Second)
	wantCode(t, b.do(http.MethodGet, "/api/system", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	if e.store.Len() != 0 {
		t.Error("dashboard session not deleted")
	}
	if _, ok := b.cookies[SessionCookie]; ok {
		t.Error("session cookie not cleared")
	}
}

func TestPollingDoesNotExtendButActivityDoes(t *testing.T) {
	e := newEnv(t, envOptions{omvTimeout: 5 * time.Minute})
	poll := e.browser()
	active := e.browser()
	wantStatus(t, poll.login("admin", "admin"), http.StatusOK)
	wantStatus(t, active.login("user", "user"), http.StatusOK)

	expiredAt := time.Duration(0)
	for i := 1; i <= 10; i++ {
		e.clk.Add(time.Minute)
		wantStatus(t, active.do(http.MethodPost, "/api/session/activity", struct{}{}), http.StatusOK)
		wantStatus(t, active.do(http.MethodGet, "/api/system", nil), http.StatusOK)
		if expiredAt == 0 && poll.do(http.MethodGet, "/api/system", nil).Code == http.StatusUnauthorized {
			expiredAt = time.Duration(i) * time.Minute
		}
	}
	// 주기 요청만 있는 세션은 OMV timeout(5분)이 지나면 만료된다.
	if expiredAt != 6*time.Minute {
		t.Errorf("polling-only session expired at %v, want 6m", expiredAt)
	}
}

func TestChallengeFlow(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	rec := b.login("totp", "totp")
	wantStatus(t, rec, http.StatusOK)
	got := decode[api.LoginResponse](t, rec)
	if got.Status != api.LoginChallengeRequired || got.Challenge == nil || got.Challenge.Kind != "totp" || got.Session != nil {
		t.Fatalf("login response %+v", got)
	}
	if _, ok := b.cookies[SessionCookie]; ok {
		t.Fatal("session cookie set before verify")
	}
	if b.cookies[PendingCookie] == nil {
		t.Fatal("pending cookie not set")
	}
	wantStatus(t, b.do(http.MethodGet, "/api/session", nil), http.StatusUnauthorized)

	wantCode(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "000000"}),
		http.StatusBadRequest, api.ErrChallengeFailed)
	rec = b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"})
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.LoginResponse](t, rec); got.Session == nil || got.Session.Username != "totp" || !got.Session.Admin {
		t.Fatalf("verify response %+v", got)
	}
	if _, ok := b.cookies[PendingCookie]; ok {
		t.Error("pending cookie not cleared")
	}
	wantStatus(t, b.do(http.MethodGet, "/api/system", nil), http.StatusOK)
	// 대기는 한 번만 쓸 수 있다.
	b.cookies[PendingCookie] = &http.Cookie{Name: PendingCookie, Value: "replayed"}
	wantCode(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}),
		http.StatusUnauthorized, api.ErrChallengeExpired)
}

func TestChallengeExpires(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("totp", "totp"), http.StatusOK)
	e.clk.Add(auth.PendingTTL + time.Second)
	wantCode(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}),
		http.StatusUnauthorized, api.ErrChallengeExpired)
	wantCode(t, e.browser().do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "1"}),
		http.StatusUnauthorized, api.ErrChallengeExpired)
}

func TestOMVUnavailable(t *testing.T) {
	e := newEnv(t, envOptions{brokenOMV: true})
	wantCode(t, e.browser().login("admin", "admin"), http.StatusBadGateway, api.ErrOMVUnavailable)
}

func TestOMVDownKeepsSession(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	e.omvTS.Close()
	e.clk.Add(2 * time.Minute)
	// OMV에 연결할 수 없어도 /proc 현황은 계속 보인다.
	wantStatus(t, b.do(http.MethodGet, "/api/system", nil), http.StatusOK)
	wantStatus(t, b.do(http.MethodPost, "/api/session/activity", struct{}{}), http.StatusOK)
}

func TestSystemError(t *testing.T) {
	e := newEnv(t, envOptions{system: fakeSource{err: errors.New("boom")}})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/system", nil), http.StatusInternalServerError)
}

func TestForwardedFor(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	// 앞쪽 항목은 브라우저가 지어낼 수 있으므로 프록시가 붙인 마지막 항목을 쓴다.
	b.header.Set("X-Forwarded-For", "203.0.113.99, 192.0.2.7")
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	calls := e.fake.Calls()
	if calls[0].ForwardedFor != "192.0.2.7" {
		t.Errorf("forwarded for %q", calls[0].ForwardedFor)
	}
}

func TestNoSecretsInLogs(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	b.login("admin", "wrong-password-xyz")
	b.login("admin", "admin")
	dashID := b.cookies[SessionCookie].Value
	b.do(http.MethodPost, "/api/logout", struct{}{})
	c := e.browser()
	c.login("totp", "totp")
	pendingID := c.cookies[PendingCookie].Value
	c.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "999999"})
	c.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"})

	logs := e.logs.String()
	secrets := []string{"wrong-password-xyz", "999999", "123456", dashID, pendingID}
	for _, call := range e.fake.Calls() {
		if call.SessionID != "" {
			secrets = append(secrets, call.SessionID)
		}
	}
	for _, s := range secrets {
		if strings.Contains(logs, s) {
			t.Errorf("log contains secret %q:\n%s", s, logs)
		}
	}
	if !strings.Contains(logs, "login ok") || !strings.Contains(logs, "login failed") {
		t.Errorf("expected auth events in log:\n%s", logs)
	}
}

func expiresIn(t *testing.T, rec *httptest.ResponseRecorder) int {
	t.Helper()
	wantStatus(t, rec, http.StatusOK)
	s := decode[api.SessionInfo](t, rec)
	if s.ExpiresInSeconds == nil {
		t.Fatalf("no expiresInSeconds in %s", rec.Body.String())
	}
	return *s.ExpiresInSeconds
}

func TestSessionExpiry(t *testing.T) {
	e := newEnv(t, envOptions{omvTimeout: 5 * time.Minute})
	b := e.browser()
	rec := b.login("admin", "admin")
	wantStatus(t, rec, http.StatusOK)
	if s := decode[api.LoginResponse](t, rec).Session; s.ExpiresInSeconds == nil || *s.ExpiresInSeconds != 300 {
		t.Fatalf("login expiresInSeconds %+v", s)
	}
	// 주기 요청과 세션 확인은 만료 시각을 늦추지 않는다.
	e.clk.Add(2 * time.Minute)
	wantStatus(t, b.do(http.MethodGet, "/api/system", nil), http.StatusOK)
	if got := expiresIn(t, b.do(http.MethodGet, "/api/session", nil)); got != 180 {
		t.Errorf("after 2m expiresInSeconds %d, want 180", got)
	}
	// 활동 신호는 만료 시각을 늘린다.
	if got := expiresIn(t, b.do(http.MethodPost, "/api/session/activity", struct{}{})); got != 300 {
		t.Errorf("after activity expiresInSeconds %d, want 300", got)
	}
}

func TestExpiredEstimateForcesCheck(t *testing.T) {
	// 확인 주기를 길게 둬도 예상 만료 시각이 지나면 바로 OMV에 확인한다.
	e := newEnv(t, envOptions{omvTimeout: 5 * time.Minute, interval: time.Hour})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	e.clk.Add(5*time.Minute + time.Second)
	wantCode(t, b.do(http.MethodGet, "/api/session", nil), http.StatusUnauthorized, api.ErrUnauthorized)
}

func TestTimeoutChangeIsPickedUp(t *testing.T) {
	e := newEnv(t, envOptions{omvTimeout: 5 * time.Minute})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	e.fake.SetTimeout(10 * time.Minute)
	if got := expiresIn(t, b.do(http.MethodPost, "/api/session/activity", struct{}{})); got != 600 {
		t.Errorf("expiresInSeconds %d, want 600", got)
	}
}

// OMV 8.2 ~ 8.4는 Session.login이 옛 형식(authenticated)이고 2차 인증이 없다.
func TestLegacyOMVLogin(t *testing.T) {
	e := newEnv(t, envOptions{omvTimeout: 5 * time.Minute})
	e.fake.SetVersion(omvfake.Version82)
	rec := e.browser().login("admin", "admin")
	wantStatus(t, rec, http.StatusOK)
	res := decode[api.LoginResponse](t, rec)
	if res.Status != api.LoginAuthenticated || res.Session == nil || !res.Session.Admin ||
		res.Session.ExpiresInSeconds == nil || *res.Session.ExpiresInSeconds != 300 {
		t.Fatalf("legacy login %s", rec.Body.String())
	}
	// 2차 인증 계정도 바로 로그인된다.
	rec = e.browser().login("totp", "totp")
	wantStatus(t, rec, http.StatusOK)
	if res := decode[api.LoginResponse](t, rec); res.Status != api.LoginAuthenticated {
		t.Fatalf("totp login %s", rec.Body.String())
	}
	wantCode(t, e.browser().login("user", "wrong"), http.StatusBadRequest, api.ErrInvalidCredentials)
}

// OMV 8.0 ~ 8.1에는 WebGui.getAutoLogoutSettings가 없다. 남은 시간 없이 로그인하고,
// OMV 세션 만료는 확인 주기마다 OMV에 물어 안다.
func TestOMVWithoutAutoLogoutSettings(t *testing.T) {
	e := newEnv(t, envOptions{omvTimeout: 5 * time.Minute, interval: time.Minute})
	e.fake.SetVersion(omvfake.Version80)
	b := e.browser()
	rec := b.login("user", "user")
	wantStatus(t, rec, http.StatusOK)
	if s := decode[api.LoginResponse](t, rec).Session; s == nil || s.ExpiresInSeconds != nil {
		t.Fatalf("login session %s", rec.Body.String())
	}
	wantStatus(t, b.do(http.MethodGet, "/api/session", nil), http.StatusOK)
	e.fake.Expire("user")
	e.clk.Add(2 * time.Minute)
	wantCode(t, b.do(http.MethodGet, "/api/session", nil), http.StatusUnauthorized, api.ErrUnauthorized)
}

func TestAutoLogoutOff(t *testing.T) {
	e := newEnv(t, envOptions{omvTimeoutOff: true})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	rec := b.do(http.MethodGet, "/api/session", nil)
	wantStatus(t, rec, http.StatusOK)
	if s := decode[api.SessionInfo](t, rec); s.ExpiresInSeconds != nil {
		t.Errorf("expiresInSeconds %d with auto logout off", *s.ExpiresInSeconds)
	}
}
