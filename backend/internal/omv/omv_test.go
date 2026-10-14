package omv_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"onmaru/backend/internal/omv"
	"onmaru/backend/internal/omv/omvfake"
)

const testUA = "onmaru-test/1"

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
