package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/prefs"
	"onmaru/backend/internal/security"
)

// passkeyUser는 패스키를 하나 등록한 user의 브라우저와 그 패스키다.
func (e *env) passkeyUser() (*browser, *fakePasskey) {
	e.t.Helper()
	b := e.browser()
	wantStatus(e.t, b.login("user", "user"), http.StatusOK)
	wantStatus(e.t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusOK)
	pk := newFakePasskey(e.t)
	b.enrollPasskey(pk, "key")
	return b, pk
}

// enablePasskeyLogin은 b에서 pk로 패스키로 로그인을 켜는 요청을 보낸다.
func (b *browser) enablePasskeyLogin(pk *fakePasskey, password, replace string) *httptest.ResponseRecorder {
	b.e.t.Helper()
	return b.do(http.MethodPost, "/api/security/passkey-logins", api.PasskeyLoginEnrollRequest{
		Password: password, Passkey: pk.get(b.passkeyOptions(api.PasskeyAssert)), Replace: replace,
	})
}

// mustEnablePasskeyLogin은 패스키로 로그인을 켜고 응답을 돌려준다.
func (b *browser) mustEnablePasskeyLogin(pk *fakePasskey) api.PasskeyLoginEnrollResponse {
	b.e.t.Helper()
	rec := b.enablePasskeyLogin(pk, "user", "")
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.PasskeyLoginEnrollResponse](b.e.t, rec)
}

func TestPasskeyLoginEnroll(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	// 패스키가 없으면 켜기를 권하지 않는다.
	n := e.browser()
	if got := decode[api.LoginResponse](t, n.login("user", "user")); got.PasskeyLoginOffer {
		t.Fatalf("offer without passkeys: %+v", got)
	}
	b, pk := e.passkeyUser()
	if got := decode[api.LoginResponse](t, e.browser().login("user", "user")); !got.PasskeyLoginOffer {
		t.Fatalf("no offer with a passkey: %+v", got)
	}

	// 비밀번호와 등록한 패스키를 함께 확인한다.
	o := e.browser()
	wantStatus(t, o.login("user", "user"), http.StatusOK)
	wantCode(t, o.enablePasskeyLogin(pk, "wrong", ""), http.StatusBadRequest, api.ErrFactorFailed)
	wantCode(t, o.enablePasskeyLogin(newFakePasskey(t), "user", ""), http.StatusBadRequest, api.ErrFactorFailed)
	wantCode(t, o.do(http.MethodPost, "/api/security/passkey-logins", api.PasskeyLoginEnrollRequest{Password: "user"}),
		http.StatusBadRequest, api.ErrBadRequest)
	if st := o.securityStatus(); len(st.PasskeyLogins) != 0 || st.Assurance == api.AssuranceTwoFactor {
		t.Fatalf("enabled after failures: %+v", st)
	}

	got := o.mustEnablePasskeyLogin(pk)
	share, err := security.B64.DecodeString(got.Share)
	if err != nil || len(share) != security.ShareBytes || got.ID == "" {
		t.Fatalf("enroll = %+v", got)
	}
	if want := e.clk.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339); got.ExpiresAt != want {
		t.Errorf("expires %s, want %s", got.ExpiresAt, want)
	}
	st := got.Status
	if st.Assurance != api.AssuranceTwoFactor || len(st.PasskeyLogins) != 1 || st.PasskeyLogins[0].ID != got.ID ||
		st.PasskeyLogins[0].Passkey != st.Passkeys[0].ID || st.PasskeyLogins[0].ExpiresAt != got.ExpiresAt || st.PasskeyLogins[0].LastUsed != "" {
		t.Fatalf("status = %+v", st)
	}
	// 상태 응답에는 키 조각을 넣지 않는다.
	if body := o.do(http.MethodGet, "/api/security", nil).Body.String(); strings.Contains(body, got.Share) {
		t.Errorf("status has the share: %s", body)
	}

	// 다시 켜면(replace) 이 브라우저의 이전 등록은 지운다.
	rec := o.enablePasskeyLogin(pk, "user", got.ID)
	wantStatus(t, rec, http.StatusOK)
	again := decode[api.PasskeyLoginEnrollResponse](t, rec)
	if again.ID == got.ID || again.Share == got.Share || len(again.Status.PasskeyLogins) != 1 || again.Status.PasskeyLogins[0].ID != again.ID {
		t.Fatalf("replace = %+v", again)
	}
	other := b.mustEnablePasskeyLogin(pk)

	// 해제는 관리 창 없이 한다.
	wantStatus(t, o.do(http.MethodDelete, "/api/security/passkey-logins/nope", nil), http.StatusNotFound)
	rec = o.do(http.MethodDelete, "/api/security/passkey-logins/"+again.ID, nil)
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.SecurityStatus](t, rec); len(st.PasskeyLogins) != 1 || st.PasskeyLogins[0].ID != other.ID {
		t.Fatalf("after delete = %+v", st)
	}

	// 패스키를 지우면 그 패스키의 등록도 없어진다.
	wantStatus(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{
		Password: "user", Factor: &api.SecurityVerifyRequest{Method: api.FactorPasskey, Passkey: pk.get(b.passkeyOptions(api.PasskeyAssert))},
	}), http.StatusOK)
	wantStatus(t, b.do(http.MethodDelete, "/api/security/passkeys/"+st.Passkeys[0].ID, nil), http.StatusOK)
	rec2, _ := e.security.GetSecurity("user")
	if len(rec2.PasskeyLogins) != 0 {
		t.Fatalf("logins left after passkey delete: %+v", rec2.PasskeyLogins)
	}

	logs := e.logs.String()
	for _, secret := range []string{got.Share, again.Share, other.Share, got.ID, other.ID} {
		if strings.Contains(logs, secret) {
			t.Errorf("log contains %q:\n%s", secret, logs)
		}
	}
	if !strings.Contains(logs, "security: passkey login enabled") || !strings.Contains(logs, "security: passkey login disabled") {
		t.Errorf("events not logged:\n%s", logs)
	}
}

func TestPasskeyLoginEnrollWithoutOrigin(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	got := decode[api.LoginResponse](t, b.login("user", "user"))
	if got.PasskeyLoginOffer {
		t.Fatal("offer without an origin")
	}
	wantCode(t, b.do(http.MethodPost, "/api/security/passkey-logins", api.PasskeyLoginEnrollRequest{Password: "user", Passkey: &api.PasskeyAssertion{}}),
		http.StatusConflict, api.ErrPasskeyUnavailable)
}

func TestPasskeyLoginExpiry(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b, pk := e.passkeyUser()
	first := b.mustEnablePasskeyLogin(pk)

	// 기간을 줄이면 기존 등록에도 바로 적용된다.
	e.setPolicy(api.SecurityPolicy{PasskeyLoginDays: 7})
	e.clk.Add(8 * 24 * time.Hour)
	b = e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if st := b.securityStatus(); len(st.PasskeyLogins) != 0 || st.Policy.PasskeyLoginDays != 7 {
		t.Fatalf("expired login listed: %+v", st)
	}
	// 새 등록은 지금 기간으로 만료되고, 만든 김에 만료된 등록을 지운다.
	second := b.mustEnablePasskeyLogin(pk)
	if want := e.clk.Now().Add(7 * 24 * time.Hour).UTC().Format(time.RFC3339); second.ExpiresAt != want {
		t.Errorf("expires %s, want %s", second.ExpiresAt, want)
	}
	if rec, _ := e.security.GetSecurity("user"); len(rec.PasskeyLogins) != 1 || rec.PasskeyLogins[0].ID != second.ID || rec.PasskeyLogin(first.ID) != nil {
		t.Fatalf("stored = %+v", rec.PasskeyLogins)
	}
	// 늘려도 기존 등록은 등록할 때의 기간으로 만료된다.
	e.setPolicy(api.SecurityPolicy{PasskeyLoginDays: 365})
	if st := b.securityStatus(); len(st.PasskeyLogins) != 1 || st.PasskeyLogins[0].ExpiresAt != second.ExpiresAt {
		t.Fatalf("lengthened = %+v", st.PasskeyLogins)
	}
}

func TestPasskeyLoginDaysPolicy(t *testing.T) {
	site := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{site: site})
	a := e.browser()
	wantStatus(t, a.login("admin", "admin"), http.StatusOK)
	put := func(p api.SecurityPolicy) *httptest.ResponseRecorder {
		return a.do(http.MethodPut, "/api/site/security", api.SecurityPolicyRequest{Security: &p})
	}
	if p := decode[api.SecurityPolicyResponse](t, a.do(http.MethodGet, "/api/site/security", nil)).Policy; p.PasskeyLoginDays != api.PasskeyLoginDaysDefault {
		t.Fatalf("default = %+v", p)
	}
	wantCode(t, put(api.SecurityPolicy{PasskeyLoginDays: 45}), http.StatusBadRequest, api.ErrBadRequest)
	// 기간은 비밀번호를 다시 확인한 세션만 바꾼다. 기본값을 그대로 보내는 것은 바꾸는 것이 아니다.
	if got := wantCode(t, put(api.SecurityPolicy{PasskeyLoginDays: 365}), http.StatusForbidden, api.ErrSecurityRequired); got.Required != api.SecurityPassword {
		t.Fatalf("required = %q", got.Required)
	}
	wantStatus(t, put(api.SecurityPolicy{PasskeyLoginDays: api.PasskeyLoginDaysDefault}), http.StatusNoContent)
	wantStatus(t, a.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "admin"}), http.StatusOK)
	wantStatus(t, put(api.SecurityPolicy{PasskeyLoginDays: 90}), http.StatusNoContent)
	if got, _ := site.GetSite(); got.Security == nil || got.Security.PasskeyLoginDays != 90 {
		t.Fatalf("stored = %+v", got.Security)
	}
	// 손으로 고친 모르는 값은 가장 짧은 기간으로 본다.
	site.UpdateSite(func(v *api.SiteSettings) { v.Security.PasskeyLoginDays = 1000 })
	e2 := newEnv(t, envOptions{site: site})
	b := e2.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if p := b.securityStatus().Policy; p.PasskeyLoginDays != 7 {
		t.Fatalf("policy after restart = %+v", p)
	}
	// 기본값으로 돌리고 모두 끄면 site.json에서 뺀다.
	e2.setPolicy(api.SecurityPolicy{})
	if got, _ := site.GetSite(); got.Security != nil {
		t.Fatalf("stored = %+v", got.Security)
	}
}
