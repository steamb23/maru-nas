package omv_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"maru/backend/internal/omv"
	"maru/backend/internal/omv/omvfake"
)

const testUA = "maru-test/1"

// clock은 테스트에서 시각을 직접 옮기는 시계다.
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

func setup(t *testing.T, timeout time.Duration) (*omv.Client, *omvfake.Server, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	fake := omvfake.New(omvfake.DefaultUsers(), timeout)
	fake.Now = clk.Now
	ts := httptest.NewServer(fake)
	t.Cleanup(ts.Close)
	return omv.NewClient(ts.URL, testUA), fake, clk
}

var ctx = context.Background()

// probe는 Probe의 오류만 돌려준다.
func probe(c *omv.Client, ctx context.Context, sid string, touch bool) error {
	_, err := c.Probe(ctx, sid, touch)
	return err
}

func TestLoginAuthenticated(t *testing.T) {
	c, fake, _ := setup(t, 5*time.Minute)
	res, err := c.Login(ctx, " admin ", "admin", "192.0.2.10")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != omv.StatusAuthenticated || !res.Admin || res.Username != "admin" || res.SessionID == "" {
		t.Fatalf("unexpected result %+v", res)
	}
	if st, err := c.Probe(ctx, res.SessionID, false); err != nil || !st.TimeoutKnown || st.Timeout != 5*time.Minute {
		t.Fatalf("probe: %+v, %v", st, err)
	}
	for _, call := range fake.Calls() {
		if call.UserAgent != testUA {
			t.Errorf("%s.%s user agent %q", call.Service, call.Method, call.UserAgent)
		}
	}
	if got := fake.Calls()[0].ForwardedFor; got != "192.0.2.10" {
		t.Errorf("X-Forwarded-For %q", got)
	}

	user, err := c.Login(ctx, "user", "user", "")
	if err != nil || user.Admin {
		t.Fatalf("non-admin login: %+v, %v", user, err)
	}
}

// OMV 8.4 이하는 Session.login이 status 없이 authenticated를 준다. 8.0·8.1에는
// WebGui.getAutoLogoutSettings도 없어 세션이 살아 있는지만 알고 timeout은 모른다.
func TestLoginLegacyVersions(t *testing.T) {
	for _, v := range []omvfake.Version{omvfake.Version82, omvfake.Version81, omvfake.Version80} {
		t.Run(v.String(), func(t *testing.T) {
			c, fake, _ := setup(t, 5*time.Minute)
			fake.SetVersion(v)
			res, err := c.Login(ctx, "admin", "admin", "")
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != omv.StatusAuthenticated || !res.Admin || res.Username != "admin" || res.SessionID == "" {
				t.Fatalf("unexpected result %+v", res)
			}
			if user, err := c.Login(ctx, "user", "user", ""); err != nil || user.Admin {
				t.Fatalf("non-admin login: %+v, %v", user, err)
			}
			// 2차 인증이 없는 버전이라 2차 인증 계정도 바로 로그인된다.
			if totp, err := c.Login(ctx, "totp", "totp", ""); err != nil || totp.Status != omv.StatusAuthenticated {
				t.Fatalf("totp login: %+v, %v", totp, err)
			}
			if _, err := c.Login(ctx, "admin", "wrong", ""); omv.HTTPStatus(err) != http.StatusBadRequest {
				t.Fatalf("wrong password err %v", err)
			}
			st, err := c.Probe(ctx, res.SessionID, false)
			if err != nil {
				t.Fatal(err)
			}
			want := omv.SessionState{Timeout: 5 * time.Minute, TimeoutKnown: true}
			if v == omvfake.Version80 || v == omvfake.Version81 {
				want = omv.SessionState{}
			}
			if st != want {
				t.Errorf("probe %+v, want %+v", st, want)
			}
			fake.Expire("admin")
			if err := probe(c, ctx, res.SessionID, false); !omv.IsUnauthorized(err) {
				t.Errorf("expired err %v", err)
			}
		})
	}
}

func TestFakeParseVersion(t *testing.T) {
	for in, want := range map[string]omvfake.Version{
		"8.0": omvfake.Version80, "8.1.1": omvfake.Version81, "8.2": omvfake.Version82,
		"8.4.0": omvfake.Version82, "8.5": omvfake.Version85, "8.5.11": omvfake.Version85, "8.10": omvfake.Version85,
	} {
		if got, err := omvfake.ParseVersion(in); err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "8", "7.4", "9.0", "8.x"} {
		if _, err := omvfake.ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) accepted", in)
		}
	}
}

// 옛 형식 응답을 그대로 해석한다. 본문은 OMV 8.3.1 응답의 형식이고 세션 ID는 합성 값이다.
func TestLoginLegacyResponse(t *testing.T) {
	var body string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer ts.Close()
	c := omv.NewClient(ts.URL, testUA)

	body = `{"response":{"authenticated":true,"username":"admin","permissions":{"role":"admin"},"sessionid":"0123456789abcdef"},"error":null}`
	res, err := c.Login(ctx, "admin", "x", "")
	if err != nil || res.Status != omv.StatusAuthenticated || !res.Admin || res.SessionID != "0123456789abcdef" {
		t.Fatalf("admin: %+v, %v", res, err)
	}
	body = `{"response":{"authenticated":true,"username":"user","permissions":{"role":"user"},"sessionid":"0123456789abcdef"},"error":null}`
	if res, err := c.Login(ctx, "user", "x", ""); err != nil || res.Admin {
		t.Fatalf("user: %+v, %v", res, err)
	}
	// 옛 OMV는 실패를 HTTP 400으로 알리므로 false나 빈 세션 ID는 예상 밖의 응답이다.
	for _, bad := range []string{
		`{"response":{"authenticated":false,"username":"admin","permissions":{"role":"admin"},"sessionid":"0123456789abcdef"},"error":null}`,
		`{"response":{"authenticated":true,"username":"admin","permissions":{"role":"admin"},"sessionid":""},"error":null}`,
	} {
		body = bad
		if _, err := c.Login(ctx, "admin", "x", ""); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestLoginWrongPassword(t *testing.T) {
	c, _, _ := setup(t, 5*time.Minute)
	_, err := c.Login(ctx, "admin", "wrong", "")
	if omv.HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "trace") || strings.Contains(err.Error(), ".inc") {
		t.Errorf("error leaks trace: %v", err)
	}
}

func TestChallengeFlow(t *testing.T) {
	c, _, _ := setup(t, 5*time.Minute)
	res, err := c.Login(ctx, "totp", "totp", "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != omv.StatusChallengeRequired || res.Challenge == nil || res.Challenge.Kind != "totp" {
		t.Fatalf("unexpected result %+v", res)
	}
	// 대기 중 세션 ID는 본문이 아닌 Set-Cookie로만 온다.
	if res.SessionID == "" {
		t.Fatal("pending session id not taken from Set-Cookie")
	}
	if _, err := c.Verify(ctx, res.SessionID, "totp", "000000", ""); omv.HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("wrong code err %v", err)
	}
	done, err := c.Verify(ctx, res.SessionID, "totp", "123456", "")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != omv.StatusAuthenticated || !done.Admin || done.SessionID == "" || done.SessionID == res.SessionID {
		t.Fatalf("unexpected verify result %+v", done)
	}
	if err := probe(c, ctx, done.SessionID, false); err != nil {
		t.Errorf("probe after verify: %v", err)
	}
}

func TestVerifyWithoutPending(t *testing.T) {
	c, _, _ := setup(t, 5*time.Minute)
	if _, err := c.Verify(ctx, "nope", "totp", "123456", ""); !omv.IsUnauthorized(err) {
		t.Fatalf("err %v", err)
	}
}

func TestVerifyRateLimited(t *testing.T) {
	c, _, _ := setup(t, 5*time.Minute)
	res, err := c.Login(ctx, "totp", "totp", "")
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for range 3 {
		_, last = c.Verify(ctx, res.SessionID, "totp", "000000", "")
	}
	if omv.HTTPStatus(last) != http.StatusTooManyRequests {
		t.Fatalf("third failure err %v", last)
	}
}

func TestProbeTouch(t *testing.T) {
	c, _, clk := setup(t, 5*time.Minute)
	a, _ := c.Login(ctx, "admin", "admin", "")
	b, _ := c.Login(ctx, "user", "user", "")

	clk.Add(4 * time.Minute)
	if err := probe(c, ctx, a.SessionID, false); err != nil {
		t.Fatal(err)
	}
	if err := probe(c, ctx, b.SessionID, true); err != nil {
		t.Fatal(err)
	}
	clk.Add(2 * time.Minute)
	// touch=false는 마지막 접근 시각을 늘리지 않으므로 6분째에 만료된다.
	if err := probe(c, ctx, a.SessionID, false); !omv.IsUnauthorized(err) {
		t.Errorf("untouched session err %v", err)
	}
	if err := probe(c, ctx, b.SessionID, false); err != nil {
		t.Errorf("touched session err %v", err)
	}
}

func TestExpireAndLogout(t *testing.T) {
	c, fake, _ := setup(t, 5*time.Minute)
	a, _ := c.Login(ctx, "admin", "admin", "")
	fake.Expire("admin")
	if err := probe(c, ctx, a.SessionID, false); !omv.IsUnauthorized(err) {
		t.Fatalf("expired err %v", err)
	}

	b, _ := c.Login(ctx, "admin", "admin", "")
	if err := c.Logout(ctx, b.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := probe(c, ctx, b.SessionID, false); !omv.IsUnauthorized(err) {
		t.Errorf("after logout err %v", err)
	}
	if n := fake.ActiveSessions(); n != 0 {
		t.Errorf("active sessions %d", n)
	}
}

func TestProbeTimeout(t *testing.T) {
	c, fake, _ := setup(t, 5*time.Minute)
	a, _ := c.Login(ctx, "admin", "admin", "")
	fake.SetTimeout(time.Minute)
	if st, err := c.Probe(ctx, a.SessionID, false); err != nil || st != (omv.SessionState{Timeout: time.Minute, TimeoutKnown: true}) {
		t.Fatalf("probe: %+v, %v", st, err)
	}
	fake.SetTimeout(0)
	if st, err := c.Probe(ctx, a.SessionID, false); err != nil || st != (omv.SessionState{TimeoutKnown: true}) {
		t.Fatalf("probe with auto logout off: %+v, %v", st, err)
	}
}

func TestLoginCookieKept(t *testing.T) {
	c, fake, _ := setup(t, 5*time.Minute)
	for range 3 {
		if _, err := c.Login(ctx, "admin", "admin", ""); err != nil {
			t.Fatal(err)
		}
	}
	if n := fake.LoginMails(); n != 1 {
		t.Errorf("login mails %d, want 1", n)
	}
}

func TestErrors(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("X-OpenMediaVault-SessionID") {
		case "html":
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte("<html>bad gateway</html>"))
		case "role":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"response":null,"error":{"code":0,"message":"Invalid context role.","trace":"x"}}`))
		default:
			w.Write([]byte(`{"response":{"status":"weird"},"error":null}`))
		}
	}))
	defer ts.Close()
	c := omv.NewClient(ts.URL, testUA)

	if err := probe(c, ctx, "html", false); omv.HTTPStatus(err) != http.StatusBadGateway {
		t.Errorf("html err %v", err)
	}
	// 인증 검사를 통과한 뒤의 RPC 오류는 세션이 살아 있다는 뜻이다. 시간은 모른다.
	if st, err := c.Probe(ctx, "role", false); err != nil || st.TimeoutKnown {
		t.Errorf("role: %+v, %v", st, err)
	}
	if _, err := c.Login(ctx, "admin", "admin", ""); err == nil {
		t.Error("unexpected login response accepted")
	}

	ts.Close()
	err := probe(c, ctx, "x", false)
	if err == nil || omv.HTTPStatus(err) != 0 || omv.IsUnauthorized(err) {
		t.Errorf("network err %v", err)
	}
}

// 마감 시각이 있는 ctx는 HTTP 클라이언트의 기본 제한 시간보다 오래 기다린다(SMART 조회가 디스크를 깨우는 경우).
func TestCallDeadlineOverridesClientTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"response": {"ok": true}, "error": null}`))
	}))
	t.Cleanup(slow.Close)
	c := omv.NewClient(slow.URL, testUA)
	c.HTTP.Timeout = 50 * time.Millisecond
	req := omv.Request{SessionID: "s", Service: "Smart", Method: "getList"}

	if _, err := c.Call(context.Background(), req); err == nil {
		t.Error("want client timeout without a deadline")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, req); err != nil {
		t.Errorf("call with deadline: %v", err)
	}
	if c.HTTP.Timeout != 50*time.Millisecond {
		t.Error("client timeout changed")
	}
}
