package server

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	helperclient "maru/backend/internal/helper/client"
	"maru/backend/internal/helper/helperfake"
	"maru/backend/internal/helper/proto"
)

// session은 브라우저의 대시보드 세션이다(저장소에서 도우미 티켓을 보기 위해).
func (b *browser) session() auth.Session {
	b.e.t.Helper()
	ck, ok := b.cookies[SessionCookie]
	if !ok {
		b.e.t.Fatal("no session cookie")
	}
	s, ok := b.e.store.Get(ck.Value)
	if !ok {
		b.e.t.Fatal("session not found")
	}
	return s
}

// waitTicketClosed는 비동기로 폐기한 티켓이 도우미에서 사라질 때까지 기다린다.
func waitTicketClosed(t *testing.T, e *env, ticket string) {
	t.Helper()
	c := helperclient.New(e.helper.Socket)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := c.Status(context.Background(), ticket)
		if proto.CodeOf(err) == proto.CodeTicketInvalid {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ticket still valid: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestHelperDisabled(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("totp", "totp"), http.StatusOK)
	wantStatus(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}), http.StatusOK)
	if st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil)); st.State != api.HelperDisabled {
		t.Fatalf("state = %q", st.State)
	}
	if caps := decode[api.CapabilitiesResponse](t, b.do(http.MethodGet, "/api/capabilities", nil)); caps.Helper {
		t.Fatal("capabilities report a helper")
	}
	wantCode(t, b.do(http.MethodGet, "/api/helper/identity", nil), http.StatusServiceUnavailable, api.ErrHelperUnavailable)
	wantCode(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}), http.StatusServiceUnavailable, api.ErrHelperUnavailable)
}

func TestHelperUserSession(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	wantCode(t, b.do(http.MethodGet, "/api/helper", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	ticket := b.session().HelperTicket
	if ticket == "" {
		t.Fatal("no helper ticket after login")
	}
	st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil))
	if st.State != api.HelperActive || st.CanElevate || st.ElevatedSeconds != 0 {
		t.Fatalf("status = %+v", st)
	}
	if caps := decode[api.CapabilitiesResponse](t, b.do(http.MethodGet, "/api/capabilities", nil)); !caps.Helper {
		t.Fatal("capabilities do not report the helper")
	}
	id := decode[api.HelperIdentityResponse](t, b.do(http.MethodGet, "/api/helper/identity", nil))
	if id.User.UID != 1001 || id.User.User != "user" || id.Root != nil || len(id.User.Groups) == 0 || id.User.Groups[0].Name != "users" {
		t.Fatalf("identity = %+v", id)
	}
	// 관리자가 아니면 상승을 요청할 수 없다.
	wantCode(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "user"}), http.StatusForbidden, api.ErrAdminRequired)

	logs := e.logs.String()
	if strings.Contains(logs, ticket) || strings.Contains(logs, `"user"}`) {
		t.Fatalf("ticket or password in logs:\n%s", logs)
	}
	if !strings.Contains(logs, `login ok: user="user"`) || !strings.Contains(logs, "helper=true") {
		t.Fatalf("login log missing helper flag:\n%s", logs)
	}

	wantStatus(t, b.do(http.MethodPost, "/api/logout", nil), http.StatusNoContent)
	waitTicketClosed(t, e, ticket)
}

func TestHelperElevation(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	loginTOTP(t, b)
	ticket := b.session().HelperTicket
	if ticket == "" {
		t.Fatal("2FA login has no helper ticket")
	}
	st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil))
	if st.State != api.HelperActive || !st.CanElevate {
		t.Fatalf("status = %+v", st)
	}
	wantCode(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{}), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "nope"}), http.StatusBadRequest, api.ErrElevationFailed)

	st = decode[api.HelperStatus](t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}))
	if st.ElevatedSeconds != 300 {
		t.Fatalf("elevated = %+v", st)
	}
	id := decode[api.HelperIdentityResponse](t, b.do(http.MethodGet, "/api/helper/identity", nil))
	if id.Root == nil || id.Root.UID != 0 || id.User.UID != 1002 {
		t.Fatalf("identity = %+v", id)
	}
	e.clk.Add(2 * time.Minute)
	if st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil)); st.ElevatedSeconds != 180 {
		t.Fatalf("remaining = %d", st.ElevatedSeconds)
	}
	st = decode[api.HelperStatus](t, b.do(http.MethodDelete, "/api/helper/elevation", nil))
	if st.ElevatedSeconds != 0 || st.State != api.HelperActive {
		t.Fatalf("after drop = %+v", st)
	}
	if id := decode[api.HelperIdentityResponse](t, b.do(http.MethodGet, "/api/helper/identity", nil)); id.Root != nil {
		t.Fatal("root identity after drop")
	}
	if logs := e.logs.String(); !strings.Contains(logs, `helper elevate ok: user="totp"`) || !strings.Contains(logs, `helper elevate failed: user="totp"`) {
		t.Fatalf("elevation not logged:\n%s", logs)
	}

	// 상승은 세션마다다. 같은 사용자의 다른 브라우저는 상승하지 않는다.
	wantStatus(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}), http.StatusOK)
	other := e.browser()
	loginTOTP(t, other)
	if st := decode[api.HelperStatus](t, other.do(http.MethodGet, "/api/helper", nil)); st.ElevatedSeconds != 0 {
		t.Fatal("elevation leaked to another session")
	}
}

func TestHelperElevationNotAllowed(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	// 가짜 admin은 OMV 관리자지만 sudo 그룹이 아니다.
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	if st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil)); st.State != api.HelperActive || st.CanElevate {
		t.Fatalf("status = %+v", st)
	}
	wantCode(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "admin"}), http.StatusForbidden, api.ErrElevationNotAllowed)
}

func TestHelperElevationRateLimited(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	loginTOTP(t, b)
	for range 3 {
		wantCode(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "nope"}), http.StatusBadRequest, api.ErrElevationFailed)
	}
	rec := b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"})
	got := wantCode(t, rec, http.StatusTooManyRequests, api.ErrRateLimited)
	if got.RetryAfterSeconds != 30 || rec.Header().Get("Retry-After") != "30" {
		t.Fatalf("retry after = %d %q", got.RetryAfterSeconds, rec.Header().Get("Retry-After"))
	}
}

func TestHelperPendingAbandoned(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	wantStatus(t, b.login("totp", "totp"), http.StatusOK)
	p, ok := e.store.GetPending(b.cookies[PendingCookie].Value)
	if !ok || p.HelperTicket == "" {
		t.Fatalf("pending = %+v %v", p, ok)
	}
	// 2차 인증을 마치기 전의 티켓은 쓸 수 없다.
	if _, err := helperclient.New(e.helper.Socket).Identity(context.Background(), p.HelperTicket, proto.ScopeUser); proto.CodeOf(err) != proto.CodeTicketInvalid {
		t.Fatalf("pending ticket ran: %v", err)
	}
	e.clk.Add(auth.PendingTTL + time.Second)
	wantCode(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}), http.StatusUnauthorized, api.ErrChallengeExpired)
	waitTicketClosed(t, e, p.HelperTicket)
}

func TestHelperReauthAfterRestart(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	// 도우미가 다시 시작하면 티켓이 모두 사라진다.
	socket, root := e.helper.Socket, e.helper.Root
	e.helper.Close()
	restarted, err := helperfake.Start(socket, root, helperfake.DefaultUsers(), helperfake.Options{FailDelay: -1, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	e.helper = restarted
	wantCode(t, b.do(http.MethodGet, "/api/helper/identity", nil), http.StatusConflict, api.ErrHelperReauth)
	if st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil)); st.State != api.HelperReauth {
		t.Fatalf("state = %q", st.State)
	}
	// 다시 로그인하면 새 티켓을 받는다.
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil)); st.State != api.HelperActive {
		t.Fatalf("state after re-login = %q", st.State)
	}
}

func TestHelperUnreachable(t *testing.T) {
	e := newEnv(t, envOptions{helperSocket: filepath.Join(t.TempDir(), "missing.sock")})
	b := e.browser()
	// 도우미에 닿지 못해도 로그인은 된다.
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if st := decode[api.HelperStatus](t, b.do(http.MethodGet, "/api/helper", nil)); st.State != api.HelperNone {
		t.Fatalf("state = %q", st.State)
	}
	wantCode(t, b.do(http.MethodGet, "/api/helper/identity", nil), http.StatusConflict, api.ErrHelperReauth)
	if !strings.Contains(e.logs.String(), `helper: no ticket for user="user": unavailable`) {
		t.Fatalf("missing log:\n%s", e.logs.String())
	}
}

func TestHelperOldSessionTicketClosedOnRelogin(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	first := b.session().HelperTicket
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if b.session().HelperTicket == first {
		t.Fatal("ticket reused")
	}
	waitTicketClosed(t, e, first)
}
