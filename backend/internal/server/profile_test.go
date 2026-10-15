package server

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/omv/omvfake"
)

// loginTOTP는 2차 인증이 필요한 가짜 관리자(일반 계정)로 로그인한다.
func loginTOTP(t *testing.T, b *browser) {
	t.Helper()
	wantStatus(t, b.login("totp", "totp"), http.StatusOK)
	wantStatus(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}), http.StatusOK)
}

// profileCalls는 가짜 OMV가 받은 UserMgmt 호출이다.
func profileCalls(e *env) []omvfake.Call {
	var out []omvfake.Call
	for _, c := range e.fake.Calls() {
		if c.Service == "UserMgmt" {
			out = append(out, c)
		}
	}
	return out
}

func TestProfileUser(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantCode(t, b.do(http.MethodGet, "/api/profile", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	rec := b.do(http.MethodGet, "/api/profile", nil)
	wantStatus(t, rec, http.StatusOK)
	got := decode[api.ProfileResponse](t, rec)
	want := api.ProfileResponse{Username: "user", Email: "user@example.com", Comment: "Example user"}
	if got.Username != want.Username || got.Email != want.Email || got.Comment != want.Comment ||
		got.Groups != nil || got.SystemAccount || got.ReadOnlyReason != "" {
		t.Fatalf("profile %+v", got)
	}
	// SSH 키 등 화면에 쓰지 않는 값은 넘기지 않는다.
	for _, field := range []string{"sshpubkeys", "uid", "dir", "shell"} {
		if strings.Contains(rec.Body.String(), field) {
			t.Errorf("response leaks %s: %s", field, rec.Body.String())
		}
	}
	// 일반 사용자는 자기 정보만 주는 getUserByContext로 읽고, OMV 세션을 연장하지 않는다.
	calls := profileCalls(e)
	if len(calls) != 1 || calls[0].Method != "getUserByContext" || calls[0].Touch {
		t.Errorf("calls %+v", calls)
	}
}

func TestProfileAdmin(t *testing.T) {
	e := newEnv(t, envOptions{})
	// 기본 admin은 실제 OMV처럼 시스템 계정이다.
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	got := decode[api.ProfileResponse](t, admin.do(http.MethodGet, "/api/profile", nil))
	if !got.SystemAccount || got.ReadOnlyReason != api.ProfileReadOnlySystem || !slices.Equal(got.Groups, []string{"openmediavault-admin"}) {
		t.Fatalf("admin profile %+v", got)
	}
	calls := profileCalls(e)
	if len(calls) != 1 || calls[0].Method != "getUser" || string(calls[0].Params) != `{"name":"admin"}` || calls[0].Touch {
		t.Errorf("calls %+v", calls)
	}

	// 일반 계정인 관리자는 편집할 수 있다.
	b := e.browser()
	loginTOTP(t, b)
	got = decode[api.ProfileResponse](t, b.do(http.MethodGet, "/api/profile", nil))
	if got.Username != "totp" || got.Email != "totp@example.com" || got.SystemAccount || got.ReadOnlyReason != "" ||
		!slices.Equal(got.Groups, []string{"users", "openmediavault-admin"}) {
		t.Errorf("totp profile %+v", got)
	}
}

func TestProfileOMVErrors(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	// OMV 세션이 만료되면 대시보드 세션도 지운다.
	e.fake.Expire("user")
	e.clk.Add(2 * time.Minute)
	wantCode(t, b.do(http.MethodGet, "/api/profile", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	if _, ok := b.cookies[SessionCookie]; ok {
		t.Error("session cookie not cleared")
	}
}
