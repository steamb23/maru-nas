package server

import (
	"encoding/base64"
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
	if got := decode[api.LoginResponse](t, n.login("user", "user")); got.PasskeyLoginOffer != nil {
		t.Fatalf("offer without passkeys: %+v", got)
	}
	b, pk := e.passkeyUser()
	// 권유에는 지금 만료 기간을 담는다.
	if got := decode[api.LoginResponse](t, e.browser().login("user", "user")); got.PasskeyLoginOffer == nil ||
		got.PasskeyLoginOffer.Days != api.PasskeyLoginDaysDefault {
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
	if got.PasskeyLoginOffer != nil {
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
	// 로그인 권유도 지금 기간을 알린다.
	if got := decode[api.LoginResponse](t, b.login("user", "user")); got.PasskeyLoginOffer == nil || got.PasskeyLoginOffer.Days != 7 {
		t.Fatalf("offer = %+v", got.PasskeyLoginOffer)
	}
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

// passkeyLoginOptions는 브라우저 항목(사용자 이름, 등록 ID)으로 패스키로 로그인을 시작한다.
func (b *browser) passkeyLoginOptions(user, enrollment string) *httptest.ResponseRecorder {
	b.e.t.Helper()
	return b.do(http.MethodPost, "/api/login/passkey/options", api.PasskeyLoginOptionsRequest{Username: user, Enrollment: enrollment})
}

// passkeyLoginUnlock은 options를 받아 pk로 확인한 결과를 보낸다.
func (b *browser) passkeyLoginUnlock(pk *fakePasskey, user, enrollment string) *httptest.ResponseRecorder {
	b.e.t.Helper()
	rec := b.passkeyLoginOptions(user, enrollment)
	wantStatus(b.e.t, rec, http.StatusOK)
	return b.do(http.MethodPost, "/api/login/passkey", api.PasskeyLoginRequest{Passkey: pk.get(decode[api.PasskeyOptions](b.e.t, rec))})
}

// mustPasskeyLoginUnlock은 options부터 확인까지 보내고 풀림 응답(share, 다음 share)을 돌려준다.
func (b *browser) mustPasskeyLoginUnlock(pk *fakePasskey, user, enrollment string) api.PasskeyLoginUnlockResponse {
	b.e.t.Helper()
	rec := b.passkeyLoginUnlock(pk, user, enrollment)
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.PasskeyLoginUnlockResponse](b.e.t, rec)
}

// passkeyLoginFinish는 항목을 다음 share용으로 바꾸지 못한 브라우저의 finish다(rotated 없음).
func (b *browser) passkeyLoginFinish(password string) *httptest.ResponseRecorder {
	b.e.t.Helper()
	return b.do(http.MethodPost, "/api/login/passkey/finish", api.PasskeyLoginFinishRequest{Password: password})
}

// passkeyLoginRotate는 항목을 다음 share용으로 바꿔 저장한 브라우저의 finish다.
func (b *browser) passkeyLoginRotate(password string) *httptest.ResponseRecorder {
	b.e.t.Helper()
	return b.do(http.MethodPost, "/api/login/passkey/finish", api.PasskeyLoginFinishRequest{Password: password, Rotated: true})
}

// storedPasskeyLogin은 저장된 user의 등록 id다. 없으면 테스트를 멈춘다.
func (e *env) storedPasskeyLogin(user, id string) security.PasskeyLogin {
	e.t.Helper()
	rec, err := e.security.GetSecurity(user)
	if err != nil {
		e.t.Fatal(err)
	}
	l := rec.PasskeyLogin(id)
	if l == nil {
		e.t.Fatalf("passkey login %s of %s is gone", id, user)
	}
	return *l
}

func TestPasskeyLoginFlow(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b, pk := e.passkeyUser()
	en := b.mustEnablePasskeyLogin(pk)

	n := e.browser()
	// 없는 사용자와 없는 등록은 같은 응답이다.
	unknownUser := wantCode(t, n.passkeyLoginOptions("nobody", en.ID), http.StatusBadRequest, api.ErrPasskeyLoginUnknown)
	unknownLogin := wantCode(t, n.passkeyLoginOptions("user", "bm9wZQ"), http.StatusBadRequest, api.ErrPasskeyLoginUnknown)
	if unknownUser != unknownLogin {
		t.Errorf("unknown responses differ: %+v %+v", unknownUser, unknownLogin)
	}
	wantCode(t, n.passkeyLoginOptions("", ""), http.StatusBadRequest, api.ErrPasskeyLoginUnknown)
	// options 없이는 확인을 받지 않는다.
	wantCode(t, n.do(http.MethodPost, "/api/login/passkey", api.PasskeyLoginRequest{Passkey: &api.PasskeyAssertion{}}),
		http.StatusBadRequest, api.ErrChallengeExpired)

	rec := n.passkeyLoginOptions("user", en.ID)
	wantStatus(t, rec, http.StatusOK)
	opts := decode[api.PasskeyOptions](t, rec)
	if opts.RPID != testRP.ID || len(opts.Credentials) != 1 || opts.Credentials[0].ID != b.securityStatus().Passkeys[0].ID {
		t.Fatalf("options = %+v", opts)
	}
	if strings.Contains(rec.Body.String(), en.Share) || n.cookies[PasskeyLoginCookie] == nil {
		t.Fatalf("options body %s, cookies %v", rec.Body.String(), n.cookies)
	}
	assertion := pk.get(opts)
	rec = n.do(http.MethodPost, "/api/login/passkey", api.PasskeyLoginRequest{Passkey: assertion})
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.PasskeyLoginUnlockResponse](t, rec); got.Share != en.Share {
		t.Fatalf("share %q, want the enrolled share", got.Share)
	}
	// 확인 값과 풀림은 한 번만 쓴다.
	wantCode(t, n.do(http.MethodPost, "/api/login/passkey", api.PasskeyLoginRequest{Passkey: assertion}), http.StatusBadRequest, api.ErrChallengeExpired)

	rec = n.passkeyLoginFinish("user")
	wantStatus(t, rec, http.StatusOK)
	got := decode[api.LoginResponse](t, rec)
	if got.Status != api.LoginAuthenticated || got.Session.Assurance != api.AssuranceTwoFactor || got.StepUp != nil {
		t.Fatalf("login = %+v", got)
	}
	if n.cookies[PasskeyLoginCookie] != nil || n.cookies[SessionCookie] == nil {
		t.Fatalf("cookies after login: %v", n.cookies)
	}
	if s := decode[api.SessionInfo](t, n.do(http.MethodGet, "/api/session", nil)); s.Username != "user" || s.Assurance != api.AssuranceTwoFactor {
		t.Fatalf("session = %+v", s)
	}
	if st := n.securityStatus(); st.PasskeyLogins[0].LastUsed == "" || st.Passkeys[0].LastUsed == "" {
		t.Fatalf("last used not recorded: %+v", st)
	}
	wantCode(t, n.passkeyLoginFinish("user"), http.StatusBadRequest, api.ErrChallengeExpired)

	// 등록한 패스키가 아니면 share를 주지 않는다.
	o := e.browser()
	wantCode(t, o.passkeyLoginUnlock(newFakePasskey(t), "user", en.ID), http.StatusBadRequest, api.ErrFactorFailed)
	wantCode(t, o.passkeyLoginFinish("user"), http.StatusBadRequest, api.ErrChallengeExpired)

	logs := e.logs.String()
	for _, secret := range []string{en.Share, en.ID} {
		if strings.Contains(logs, secret) {
			t.Errorf("log contains %q:\n%s", secret, logs)
		}
	}
	if !strings.Contains(logs, `security: passkey login: user="user"`) || !strings.Contains(logs, "assurance=twoFactor") {
		t.Errorf("passkey login not logged:\n%s", logs)
	}
}

func TestPasskeyLoginRotation(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b, pk := e.passkeyUser()
	en := b.mustEnablePasskeyLogin(pk)
	b64 := security.B64.EncodeToString
	if l := e.storedPasskeyLogin("user", en.ID); l.NextShare != nil {
		t.Fatalf("next share before any login: %+v", l)
	}

	// finish 없이 두 번 풀어도 다음 share는 같다(확정 전에는 바꾸지 않는다).
	n := e.browser()
	first := n.mustPasskeyLoginUnlock(pk, "user", en.ID)
	if first.Share != en.Share || first.NextShare == "" || first.NextShare == first.Share {
		t.Fatalf("unlock = %+v, enrolled share %q", first, en.Share)
	}
	if next, err := security.B64.DecodeString(first.NextShare); err != nil || len(next) != security.ShareBytes {
		t.Fatalf("next share %q: %v", first.NextShare, err)
	}
	if again := n.mustPasskeyLoginUnlock(pk, "user", en.ID); again != first {
		t.Fatalf("second unlock = %+v, want %+v", again, first)
	}
	if l := e.storedPasskeyLogin("user", en.ID); b64(l.Share) != en.Share || b64(l.NextShare) != first.NextShare {
		t.Fatalf("stored after unlock: share %q next %q", b64(l.Share), b64(l.NextShare))
	}

	// 항목을 바꾸지 못한 브라우저(rotated 없음)의 로그인은 확정하지 않는다.
	wantStatus(t, n.passkeyLoginFinish("user"), http.StatusOK)
	if l := e.storedPasskeyLogin("user", en.ID); b64(l.Share) != en.Share || b64(l.NextShare) != first.NextShare {
		t.Fatalf("confirmed without rotated: share %q next %q", b64(l.Share), b64(l.NextShare))
	}

	// rotated면 다음 share를 지금 share로 확정하고 대기를 지운다. 만료 시각은 그대로다.
	o := e.browser()
	if got := o.mustPasskeyLoginUnlock(pk, "user", en.ID); got != first {
		t.Fatalf("unlock after unrotated login = %+v, want %+v", got, first)
	}
	wantStatus(t, o.passkeyLoginRotate("user"), http.StatusOK)
	l := e.storedPasskeyLogin("user", en.ID)
	if b64(l.Share) != first.NextShare || l.NextShare != nil {
		t.Fatalf("stored after rotation: share %q next %q", b64(l.Share), b64(l.NextShare))
	}
	if st := o.securityStatus(); len(st.PasskeyLogins) != 1 || st.PasskeyLogins[0].ExpiresAt != en.ExpiresAt {
		t.Fatalf("status after rotation = %+v, enrolled expiry %s", st.PasskeyLogins, en.ExpiresAt)
	}

	// 다음 풀림은 확정한 share와 새 다음 share를 준다.
	after := e.browser().mustPasskeyLoginUnlock(pk, "user", en.ID)
	if after.Share != first.NextShare || after.NextShare == "" || after.NextShare == first.NextShare ||
		after.NextShare == en.Share || after.NextShare == after.Share {
		t.Fatalf("unlock after rotation = %+v (previous next %q, enrolled %q)", after, first.NextShare, en.Share)
	}

	// 로그에는 회전 여부만 남기고 share 값은 남기지 않는다(base64url, 기록 파일의 base64 모두).
	logs := e.logs.String()
	for _, v := range []string{en.Share, first.NextShare, after.NextShare} {
		raw, _ := security.B64.DecodeString(v)
		for _, enc := range []string{v, base64.StdEncoding.EncodeToString(raw)} {
			if strings.Contains(logs, enc) {
				t.Errorf("log contains a share %q:\n%s", enc, logs)
			}
		}
	}
	if !strings.Contains(logs, "rotated=false") || !strings.Contains(logs, "rotated=true") {
		t.Errorf("rotation not logged:\n%s", logs)
	}
}

func TestPasskeyLoginStaleAndDiscard(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b, pk := e.passkeyUser()
	first := b.mustEnablePasskeyLogin(pk)
	second := b.mustEnablePasskeyLogin(pk)

	// 풀어 낸 비밀번호가 맞지 않으면(비밀번호를 바꿈) 항목을 바꿨어도(rotated) 확정하지 않고 그 등록을 지운다.
	n := e.browser()
	wantStatus(t, n.passkeyLoginUnlock(pk, "user", first.ID), http.StatusOK)
	wantCode(t, n.passkeyLoginRotate("old-password"), http.StatusBadRequest, api.ErrPasskeyLoginStale)
	wantCode(t, n.passkeyLoginOptions("user", first.ID), http.StatusBadRequest, api.ErrPasskeyLoginUnknown)
	if rec, _ := e.security.GetSecurity("user"); rec.PasskeyLogin(first.ID) != nil || len(rec.PasskeyLogins) != 1 {
		t.Fatalf("stale login kept: %+v", rec.PasskeyLogins)
	}
	if strings.Contains(e.logs.String(), "old-password") || strings.Contains(e.logs.String(), "rotated=true") {
		t.Errorf("password or rotation logged:\n%s", e.logs.String())
	}

	// 브라우저가 풀지 못하면 그 등록을 지운다. 확인을 마친 뒤에만 할 수 있다.
	wantStatus(t, n.passkeyLoginOptions("user", second.ID), http.StatusOK)
	wantCode(t, n.do(http.MethodPost, "/api/login/passkey/finish", api.PasskeyLoginFinishRequest{Discard: true}),
		http.StatusBadRequest, api.ErrChallengeExpired)
	wantStatus(t, n.passkeyLoginUnlock(pk, "user", second.ID), http.StatusOK)
	wantStatus(t, n.do(http.MethodPost, "/api/login/passkey/finish", api.PasskeyLoginFinishRequest{Discard: true}), http.StatusNoContent)
	if rec, _ := e.security.GetSecurity("user"); len(rec.PasskeyLogins) != 0 {
		t.Fatalf("logins left: %+v", rec.PasskeyLogins)
	}
	// 두 share 모두로 풀리지 않는 항목은 회전 전에 복사된 사본일 수 있다고 남긴다.
	if logs := e.logs.String(); !strings.Contains(logs, "could not decrypt with either share, possibly a copy made before rotation") {
		t.Errorf("discard not logged:\n%s", logs)
	}
}

func TestPasskeyLoginExpiresAtLogin(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b, pk := e.passkeyUser()
	en := b.mustEnablePasskeyLogin(pk)
	day := 24 * time.Hour

	// options를 받은 뒤 만료되면 share를 주지 않고 지운다.
	n := e.browser()
	e.clk.Add(30*day - time.Second)
	rec := n.passkeyLoginOptions("user", en.ID)
	wantStatus(t, rec, http.StatusOK)
	opts := decode[api.PasskeyOptions](t, rec)
	e.clk.Add(2 * time.Second)
	wantCode(t, n.do(http.MethodPost, "/api/login/passkey", api.PasskeyLoginRequest{Passkey: pk.get(opts)}),
		http.StatusBadRequest, api.ErrPasskeyLoginExpired)
	wantCode(t, n.passkeyLoginOptions("user", en.ID), http.StatusBadRequest, api.ErrPasskeyLoginUnknown)

	// options에서 만료를 알게 되어도 지운다.
	b = e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	en = b.mustEnablePasskeyLogin(pk)
	e.clk.Add(31 * day)
	wantCode(t, n.passkeyLoginOptions("user", en.ID), http.StatusBadRequest, api.ErrPasskeyLoginExpired)
	wantCode(t, n.passkeyLoginOptions("user", en.ID), http.StatusBadRequest, api.ErrPasskeyLoginUnknown)
	if !strings.Contains(e.logs.String(), "security: passkey login removed (expired)") {
		t.Errorf("expiry not logged:\n%s", e.logs.String())
	}
}

// OMV 2차 인증이 있는 계정은 패스키로 로그인해도 그 단계를 거친다.
func TestPasskeyLoginOMVChallenge(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b := e.strongAdmin()
	wantStatus(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "totp"}), http.StatusOK)
	pk := newFakePasskey(t)
	b.enrollPasskey(pk, "key")
	rec := b.enablePasskeyLogin(pk, "totp", "")
	wantStatus(t, rec, http.StatusOK)
	en := decode[api.PasskeyLoginEnrollResponse](t, rec)

	n := e.browser()
	unlocked := n.mustPasskeyLoginUnlock(pk, "totp", en.ID)
	rec = n.passkeyLoginRotate("totp")
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.LoginResponse](t, rec); got.Status != api.LoginChallengeRequired || n.cookies[PendingCookie] == nil {
		t.Fatalf("login = %+v", got)
	}
	// OMV가 비밀번호를 받았으므로 2차 인증을 마치기 전이라도 회전을 확정한다.
	if l := e.storedPasskeyLogin("totp", en.ID); security.B64.EncodeToString(l.Share) != unlocked.NextShare || l.NextShare != nil {
		t.Fatalf("not rotated at finish: %+v", l)
	}
	rec = n.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"})
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.LoginResponse](t, rec); got.Session == nil || got.Session.Assurance != api.AssuranceTwoFactor {
		t.Fatalf("verify = %+v", got)
	}
}

func TestPasskeyLoginRateLimit(t *testing.T) {
	e := newEnv(t, envOptions{rp: testRP})
	b, pk := e.passkeyUser()
	en := b.mustEnablePasskeyLogin(pk)
	n := e.browser()
	forged := func() *httptest.ResponseRecorder {
		rec := n.passkeyLoginOptions("user", en.ID)
		wantStatus(t, rec, http.StatusOK)
		a := pk.get(decode[api.PasskeyOptions](t, rec))
		a.Signature = security.B64.EncodeToString([]byte("not a signature"))
		return n.do(http.MethodPost, "/api/login/passkey", api.PasskeyLoginRequest{Passkey: a})
	}
	for range 5 {
		wantCode(t, forged(), http.StatusBadRequest, api.ErrFactorFailed)
	}
	wantCode(t, forged(), http.StatusTooManyRequests, api.ErrRateLimited)
	// 비밀번호 로그인은 따로 센다.
	wantStatus(t, e.browser().login("user", "user"), http.StatusOK)
}

func TestPasskeyLoginWithoutOrigin(t *testing.T) {
	e := newEnv(t, envOptions{})
	n := e.browser()
	wantCode(t, n.passkeyLoginOptions("user", "x"), http.StatusConflict, api.ErrPasskeyUnavailable)
	wantCode(t, n.do(http.MethodPost, "/api/login/passkey", api.PasskeyLoginRequest{}), http.StatusConflict, api.ErrPasskeyUnavailable)
}
