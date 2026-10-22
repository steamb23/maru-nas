package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/prefs"
	"maru/backend/internal/security"
	"maru/backend/internal/terminal"
)

// strongAdmin은 OMV 2차 인증을 마친 관리자(totp, 보안 강화 모드에서도 twoFactor)의 브라우저다.
func (e *env) strongAdmin() *browser {
	e.t.Helper()
	b := e.browser()
	wantStatus(e.t, b.login("totp", "totp"), http.StatusOK)
	rec := b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"})
	wantStatus(e.t, rec, http.StatusOK)
	if got := decode[api.LoginResponse](e.t, rec); got.Session.Assurance != api.AssuranceTwoFactor || got.StepUp != nil {
		e.t.Fatalf("omv 2fa login = %+v", got)
	}
	return b
}

// setPolicy는 2차 인증을 마친 관리자로 정책을 바꾼다.
func (e *env) setPolicy(p api.SecurityPolicy) {
	e.t.Helper()
	wantStatus(e.t, e.strongAdmin().do(http.MethodPut, "/api/site/security", api.SecurityPolicyRequest{Security: &p}), http.StatusNoContent)
}

func (b *browser) securityStatus() api.SecurityStatus {
	b.e.t.Helper()
	rec := b.do(http.MethodGet, "/api/security", nil)
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.SecurityStatus](b.e.t, rec)
}

// totpCode는 지금(가짜 시계) 인증 앱이 보여 줄 코드다.
func (e *env) totpCode(secret string) string {
	e.t.Helper()
	code, err := security.TOTPCode(secret, e.clk.Now())
	if err != nil {
		e.t.Fatal(err)
	}
	return code
}

// enrollTOTP는 b의 사용자에게 인증 앱을 등록하고 비밀 키와 복구 코드를 돌려준다(수단이 없을 때).
func (b *browser) enrollTOTP(password string) (string, []string) {
	b.e.t.Helper()
	wantStatus(b.e.t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: password}), http.StatusOK)
	setup := decode[api.TOTPSetupResponse](b.e.t, b.do(http.MethodPost, "/api/security/totp/setup", struct{}{}))
	rec := b.do(http.MethodPost, "/api/security/totp", api.TOTPConfirmRequest{Code: b.e.totpCode(setup.Secret)})
	wantStatus(b.e.t, rec, http.StatusOK)
	return setup.Secret, decode[api.SecurityChangeResponse](b.e.t, rec).RecoveryCodes
}

func TestSecurityStatusDefault(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	rec := b.login("user", "user")
	wantStatus(t, rec, http.StatusOK)
	got := decode[api.LoginResponse](t, rec)
	if got.Session.Assurance != api.AssuranceLogin || got.StepUp != nil {
		t.Fatalf("login = %+v", got)
	}
	st := b.securityStatus()
	if st.Assurance != api.AssuranceLogin || st.PasskeysAvailable || len(st.Passkeys) != 0 || st.TOTP != nil ||
		len(st.Methods) != 0 || st.Policy.Terminal != api.SecurityOff || st.Policy.Elevation != api.SecurityOff || st.ManageSeconds != 0 {
		t.Fatalf("status = %+v", st)
	}
}

func TestSecurityPasswordConfirm(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	logins := func() int {
		n := 0
		for _, c := range e.fake.Calls() {
			if c.Service == "Session" && c.Method == "login" {
				n++
			}
		}
		return n
	}
	before := logins()
	wantCode(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "wrong"}),
		http.StatusBadRequest, api.ErrFactorFailed)
	rec := b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "user"})
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.SecurityStatus](t, rec); st.Assurance != api.AssurancePassword {
		t.Fatalf("assurance = %q", st.Assurance)
	}
	if logins() != before+2 {
		t.Errorf("omv logins = %d, want %d", logins(), before+2)
	}
	// 확인에 쓴 OMV 세션은 바로 끝내고, 대시보드 세션은 그대로다.
	logouts := func() int {
		n := 0
		for _, c := range e.fake.Calls() {
			if c.Service == "Session" && c.Method == "logout" {
				n++
			}
		}
		return n
	}
	if logouts() != 1 {
		t.Errorf("omv logouts = %d, want 1", logouts())
	}
	if s := decode[api.SessionInfo](t, b.do(http.MethodGet, "/api/session", nil)); s.Assurance != api.AssurancePassword {
		t.Errorf("session = %+v", s)
	}
	if strings.Contains(e.logs.String(), "wrong") {
		t.Errorf("password logged:\n%s", e.logs.String())
	}
	// OMV 2차 인증 계정도 비밀번호만 확인한다(challengeRequired는 비밀번호가 맞았다는 뜻).
	a := e.strongAdmin()
	wantStatus(t, a.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "totp"}), http.StatusOK)
	if logouts() != 2 {
		t.Errorf("pending omv session not logged out: %d logouts", logouts())
	}
}

func TestSecurityVerifyTouchesSession(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	// 로그인 직후 단계에서 수단을 찾느라 4분이 지났다. 확인하면 OMV 세션도 연장한다.
	e.clk.Add(4 * time.Minute)
	wantStatus(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "user"}), http.StatusOK)
	e.clk.Add(4 * time.Minute)
	if s := decode[api.SessionInfo](t, b.do(http.MethodGet, "/api/session", nil)); s.ExpiresInSeconds == nil || *s.ExpiresInSeconds != 60 {
		t.Fatalf("session = %+v", s)
	}
}

func TestSecurityPasswordConfirmRateLimit(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	for range auth.FreeFailures + 1 {
		wantCode(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "x"}),
			http.StatusBadRequest, api.ErrFactorFailed)
	}
	wantCode(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "user"}),
		http.StatusTooManyRequests, api.ErrRateLimited)
	// 같은 비밀번호이므로 로그인 제한도 함께 걸린다.
	wantCode(t, e.browser().login("user", "user"), http.StatusTooManyRequests, api.ErrRateLimited)
}

func TestSecurityTOTPEnrollAndVerify(t *testing.T) {
	// 관리 창이 닫힐 때까지 OMV 세션이 만료되지 않게 timeout을 늘린다.
	e := newEnv(t, envOptions{omvTimeout: time.Hour})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)

	// 관리 창 없이는 바꿀 수 없다.
	wantCode(t, b.do(http.MethodPost, "/api/security/totp/setup", struct{}{}), http.StatusForbidden, api.ErrManageRequired)
	wantCode(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "wrong"}),
		http.StatusBadRequest, api.ErrFactorFailed)
	rec := b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"})
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.SecurityStatus](t, rec); st.ManageSeconds != int(manageWindow/time.Second) || st.Assurance != api.AssurancePassword {
		t.Fatalf("manage = %+v", st)
	}
	setup := decode[api.TOTPSetupResponse](t, b.do(http.MethodPost, "/api/security/totp/setup", struct{}{}))
	if setup.Secret == "" || !strings.HasPrefix(setup.URI, "otpauth://totp/Maru%20NAS:user?") {
		t.Fatalf("setup = %+v", setup)
	}
	wantCode(t, b.do(http.MethodPost, "/api/security/totp", api.TOTPConfirmRequest{Code: "000000"}), http.StatusBadRequest, api.ErrFactorFailed)
	rec = b.do(http.MethodPost, "/api/security/totp", api.TOTPConfirmRequest{Code: e.totpCode(setup.Secret)})
	wantStatus(t, rec, http.StatusOK)
	ch := decode[api.SecurityChangeResponse](t, rec)
	if len(ch.RecoveryCodes) != security.RecoveryCodeCount || ch.Status.TOTP == nil || ch.Status.Assurance != api.AssuranceTwoFactor ||
		ch.Status.RecoveryCodes != security.RecoveryCodeCount || !slices.Equal(ch.Status.Methods, []string{api.FactorTOTP, api.FactorRecovery}) {
		t.Fatalf("confirm = %+v", ch)
	}
	if strings.Contains(e.logs.String(), setup.Secret) {
		t.Errorf("secret logged:\n%s", e.logs.String())
	}

	// 다른 브라우저(새 세션)에서 코드로 확인한다. 같은 코드는 다시 쓸 수 없다.
	e.clk.Add(30 * time.Second)
	o := e.browser()
	wantStatus(t, o.login("user", "user"), http.StatusOK)
	code := e.totpCode(setup.Secret)
	rec = o.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorTOTP, Code: code})
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.SecurityStatus](t, rec); st.Assurance != api.AssuranceTwoFactor {
		t.Fatalf("assurance = %q", st.Assurance)
	}
	p := e.browser()
	wantStatus(t, p.login("user", "user"), http.StatusOK)
	wantCode(t, p.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorTOTP, Code: code}),
		http.StatusBadRequest, api.ErrFactorFailed)

	// 복구 코드는 한 번만 쓴다.
	wantStatus(t, p.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorRecovery, Code: ch.RecoveryCodes[0]}), http.StatusOK)
	if st := p.securityStatus(); st.RecoveryCodes != security.RecoveryCodeCount-1 || st.Assurance != api.AssuranceTwoFactor {
		t.Fatalf("after recovery = %+v", st)
	}
	q := e.browser()
	wantStatus(t, q.login("user", "user"), http.StatusOK)
	wantCode(t, q.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorRecovery, Code: ch.RecoveryCodes[0]}),
		http.StatusBadRequest, api.ErrFactorFailed)

	// 수단이 있으면 관리 창은 비밀번호와 그 수단을 함께 받는다.
	wantCode(t, q.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}), http.StatusBadRequest, api.ErrBadRequest)
	e.clk.Add(30 * time.Second)
	wantStatus(t, q.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{
		Password: "user", Factor: &api.SecurityVerifyRequest{Method: api.FactorTOTP, Code: e.totpCode(setup.Secret)},
	}), http.StatusOK)
	rec = q.do(http.MethodPost, "/api/security/recovery-codes", struct{}{})
	wantStatus(t, rec, http.StatusOK)
	if again := decode[api.SecurityChangeResponse](t, rec); len(again.RecoveryCodes) != security.RecoveryCodeCount ||
		again.Status.RecoveryCodes != security.RecoveryCodeCount || again.RecoveryCodes[1] == ch.RecoveryCodes[1] {
		t.Fatalf("regenerate = %+v", again)
	}
	// 관리 창은 5분 뒤 닫힌다.
	e.clk.Add(manageWindow)
	wantCode(t, q.do(http.MethodDelete, "/api/security/totp", nil), http.StatusForbidden, api.ErrManageRequired)
}

// issueDeviceCode는 b(관리 창이 열린 세션)로 기기 추가 코드를 만든다.
func (b *browser) issueDeviceCode() api.DeviceCodeResponse {
	b.e.t.Helper()
	rec := b.do(http.MethodPost, "/api/security/device-code", struct{}{})
	wantStatus(b.e.t, rec, http.StatusOK)
	return decode[api.DeviceCodeResponse](b.e.t, rec)
}

func (b *browser) manage(req api.SecurityManageRequest) *httptest.ResponseRecorder {
	b.e.t.Helper()
	return b.do(http.MethodPost, "/api/security/manage", req)
}

func TestSecurityDeviceCode(t *testing.T) {
	// 관리 창과 코드가 닫힐 때까지 OMV 세션이 만료되지 않게 timeout을 늘린다.
	e := newEnv(t, envOptions{omvTimeout: time.Hour})
	old := e.browser()
	wantStatus(t, old.login("user", "user"), http.StatusOK)

	// 관리 창이 열린 세션만, 수단을 등록한 사용자만 만든다.
	wantCode(t, old.do(http.MethodPost, "/api/security/device-code", struct{}{}), http.StatusForbidden, api.ErrManageRequired)
	wantStatus(t, old.manage(api.SecurityManageRequest{Password: "user"}), http.StatusOK)
	wantCode(t, old.do(http.MethodPost, "/api/security/device-code", struct{}{}), http.StatusBadRequest, api.ErrBadRequest)
	old.enrollTOTP("user")
	first := old.issueDeviceCode()
	if len(first.Code) != 9 || first.Code[4] != '-' || first.ExpiresInSeconds != int(deviceCodeTTL/time.Second) {
		t.Fatalf("device code = %+v", first)
	}
	// 다시 만들면 이전 코드는 쓸 수 없다.
	code := old.issueDeviceCode().Code

	nb := e.browser()
	wantStatus(t, nb.login("user", "user"), http.StatusOK)
	wantCode(t, nb.manage(api.SecurityManageRequest{Password: "user", DeviceCode: first.Code}), http.StatusBadRequest, api.ErrFactorFailed)
	wantCode(t, nb.manage(api.SecurityManageRequest{
		Password: "user", DeviceCode: code, Factor: &api.SecurityVerifyRequest{Method: api.FactorTOTP, Code: "000000"},
	}), http.StatusBadRequest, api.ErrBadRequest)
	// 비밀번호가 틀리면 코드를 보지 않고, 틀린 코드는 맞는 코드를 지우지 않는다.
	wantCode(t, nb.manage(api.SecurityManageRequest{Password: "wrong", DeviceCode: code}), http.StatusBadRequest, api.ErrFactorFailed)
	wantCode(t, nb.manage(api.SecurityManageRequest{Password: "user", DeviceCode: "AAAA-AAAA"}), http.StatusBadRequest, api.ErrFactorFailed)
	// 새 기기는 비밀번호와 코드로 관리 창을 연다. 이 세션에서 수단을 확인한 것은 아니므로 password까지만 오른다.
	rec := nb.manage(api.SecurityManageRequest{Password: "user", DeviceCode: strings.ToLower(code)})
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.SecurityStatus](t, rec); st.ManageSeconds != int(manageWindow/time.Second) || st.Assurance != api.AssurancePassword ||
		slices.Contains(st.Methods, "deviceCode") {
		t.Fatalf("manage with device code = %+v", st)
	}

	// 코드는 한 번만 쓴다.
	again := e.browser()
	wantStatus(t, again.login("user", "user"), http.StatusOK)
	wantCode(t, again.manage(api.SecurityManageRequest{Password: "user", DeviceCode: code}), http.StatusBadRequest, api.ErrFactorFailed)
	// 2차 수단이 아니므로 세션 중 확인에는 쓸 수 없다.
	wantCode(t, again.do(http.MethodPost, "/api/security/verify", map[string]string{"method": "deviceCode", "code": code}),
		http.StatusBadRequest, api.ErrBadRequest)
	// 10분이 지나면 쓸 수 없다.
	late := old.issueDeviceCode()
	e.clk.Add(deviceCodeTTL)
	wantCode(t, again.manage(api.SecurityManageRequest{Password: "user", DeviceCode: late.Code}), http.StatusBadRequest, api.ErrFactorFailed)

	logs := e.logs.String()
	for _, c := range []string{first.Code, code, late.Code} {
		if strings.Contains(logs, c) || strings.Contains(logs, strings.ReplaceAll(c, "-", "")) {
			t.Errorf("device code %s logged:\n%s", c, logs)
		}
	}
	for _, want := range []string{`security: device code issued: user="user"`, `security: device code used: user="user"`, `security: device code failed: user="user"`} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing log %q:\n%s", want, logs)
		}
	}
}

func TestSecurityDeviceCodeClearedByReset(t *testing.T) {
	e := newEnv(t, envOptions{omvTimeout: time.Hour})
	u := e.browser()
	wantStatus(t, u.login("user", "user"), http.StatusOK)
	u.enrollTOTP("user")
	code := u.issueDeviceCode().Code
	wantStatus(t, e.strongAdmin().do(http.MethodDelete, "/api/security/users/user", nil), http.StatusNoContent)

	// 초기화 뒤 다시 등록해도 초기화 전에 만든 코드는 쓸 수 없다.
	wantStatus(t, u.login("user", "user"), http.StatusOK)
	u.enrollTOTP("user")
	nb := e.browser()
	wantStatus(t, nb.login("user", "user"), http.StatusOK)
	wantCode(t, nb.manage(api.SecurityManageRequest{Password: "user", DeviceCode: code}), http.StatusBadRequest, api.ErrFactorFailed)
}

func TestSecurityDeleteLastMethodClearsRecovery(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	b.enrollTOTP("user")
	rec := b.do(http.MethodDelete, "/api/security/totp", nil)
	wantStatus(t, rec, http.StatusOK)
	if st := decode[api.SecurityChangeResponse](t, rec).Status; st.TOTP != nil || st.RecoveryCodes != 0 || len(st.Methods) != 0 {
		t.Fatalf("after delete = %+v", st)
	}
	wantCode(t, b.do(http.MethodPost, "/api/security/recovery-codes", struct{}{}), http.StatusBadRequest, api.ErrBadRequest)
}

func TestSecurityLoginStepUp(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	b.enrollTOTP("user")

	// 2차 인증을 요구하는 기능이 없으면 묻지 않는다.
	if got := decode[api.LoginResponse](t, e.browser().login("user", "user")); got.StepUp != nil {
		t.Fatalf("step-up offered without policy: %+v", got.StepUp)
	}
	e.setPolicy(api.SecurityPolicy{Terminal: api.SecurityTwoFactor})
	got := decode[api.LoginResponse](t, e.browser().login("user", "user"))
	if got.StepUp == nil || !slices.Equal(got.StepUp.Methods, []string{api.FactorTOTP, api.FactorRecovery}) || got.Session.Assurance != api.AssuranceLogin {
		t.Fatalf("login = %+v", got)
	}
	// 세션 복원에는 넣지 않는다.
	if body := b.do(http.MethodGet, "/api/session", nil).Body.String(); strings.Contains(body, "stepUp") {
		t.Errorf("session info has step-up: %s", body)
	}
	// 수단이 없는 사용자에게는 묻지 않는다.
	if got := decode[api.LoginResponse](t, e.browser().login("admin", "admin")); got.StepUp != nil {
		t.Fatalf("step-up offered to a user without methods: %+v", got.StepUp)
	}
}

func TestSecurityPolicyGuard(t *testing.T) {
	e := newEnv(t, envOptions{})
	u := e.browser()
	wantStatus(t, u.login("user", "user"), http.StatusOK)
	wantCode(t, u.do(http.MethodPut, "/api/site/security", api.SecurityPolicyRequest{Security: &api.SecurityPolicy{}}),
		http.StatusForbidden, api.ErrAdminRequired)

	a := e.browser()
	wantStatus(t, a.login("admin", "admin"), http.StatusOK)
	put := func(b *browser, p api.SecurityPolicy) *httptest.ResponseRecorder {
		return b.do(http.MethodPut, "/api/site/security", api.SecurityPolicyRequest{Security: &p})
	}
	// 로그인만 한 관리자 세션은 정책을 올릴 수 없다.
	if got := wantCode(t, put(a, api.SecurityPolicy{Terminal: api.SecurityPassword}), http.StatusForbidden, api.ErrSecurityRequired); got.Required != api.SecurityPassword {
		t.Fatalf("required = %q", got.Required)
	}
	wantCode(t, put(a, api.SecurityPolicy{Terminal: "nope"}), http.StatusBadRequest, api.ErrBadRequest)
	wantStatus(t, a.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "admin"}), http.StatusOK)
	wantStatus(t, put(a, api.SecurityPolicy{Terminal: api.SecurityPassword}), http.StatusNoContent)
	if got := wantCode(t, put(a, api.SecurityPolicy{Terminal: api.SecurityTwoFactor}), http.StatusForbidden, api.ErrSecurityRequired); got.Required != api.SecurityTwoFactor {
		t.Fatalf("required = %q", got.Required)
	}

	// 빼앗은 관리자 세션(로그인만 함)은 정책을 내리지도 못한다. 바꾸지 않는 기능은 따지지 않는다.
	x := e.browser()
	wantStatus(t, x.login("admin", "admin"), http.StatusOK)
	if got := wantCode(t, put(x, api.SecurityPolicy{}), http.StatusForbidden, api.ErrSecurityRequired); got.Required != api.SecurityPassword {
		t.Fatalf("required = %q", got.Required)
	}
	wantStatus(t, put(a, api.SecurityPolicy{Terminal: api.SecurityPassword}), http.StatusNoContent)

	info := decode[api.SecurityPolicyResponse](t, a.do(http.MethodGet, "/api/site/security", nil))
	if info.Policy.Terminal != api.SecurityPassword || info.Policy.Elevation != api.SecurityOff || info.AutoLogoutMinutes == nil ||
		*info.AutoLogoutMinutes != 5 || info.PasskeyRPID != "" {
		t.Fatalf("policy info = %+v", info)
	}
	// 공개 정보에는 넣지 않는다.
	if body := e.browser().do(http.MethodGet, "/api/site", nil).Body.String(); strings.Contains(body, "security") {
		t.Errorf("public site info has policy: %s", body)
	}
}

func TestSecurityPolicyPersists(t *testing.T) {
	site := prefs.NewMemoryStore()
	e := newEnv(t, envOptions{site: site})
	e.setPolicy(api.SecurityPolicy{Elevation: api.SecurityTwoFactor})
	got, _ := site.GetSite()
	if got.Security == nil || got.Security.Elevation != api.SecurityTwoFactor {
		t.Fatalf("stored = %+v", got.Security)
	}
	// 다시 시작해도 저장된 정책을 쓴다. 모르는 값은 가장 엄격한 단계로 본다.
	site.UpdateSite(func(v *api.SiteSettings) { v.Security.Terminal = "bogus" })
	e2 := newEnv(t, envOptions{site: site})
	b := e2.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if p := b.securityStatus().Policy; p.Terminal != api.SecurityTwoFactor || p.Elevation != api.SecurityTwoFactor {
		t.Fatalf("policy after restart = %+v", p)
	}
	// 모두 끄면 site.json에서 뺀다.
	e2.setPolicy(api.SecurityPolicy{})
	if got, _ := site.GetSite(); got.Security != nil {
		t.Fatalf("stored = %+v", got.Security)
	}
}

func TestSecurityTerminalRequired(t *testing.T) {
	e := newEnv(t, envOptions{terminal: true})
	ts := e.server()
	e.setPolicy(api.SecurityPolicy{Terminal: api.SecurityPassword})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	w, _, err := b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg := w.expect("status", "closed"); msg.Reason != terminal.ReasonSecurityRequired || msg.Required != api.SecurityPassword {
		t.Fatalf("closed = %+v", msg)
	}
	if e.sshd.Conns() != 0 {
		t.Fatalf("sshd connections = %d", e.sshd.Conns())
	}
	// 확인한 뒤에는 접속한다.
	wantStatus(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "user"}), http.StatusOK)
	w, _, err = b.dialTerminal(ts, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.expect("status", "connecting")
	w.expect("prompt", "")
	w.control(api.TerminalClientMessage{Type: "answer", Answers: []string{"user"}})
	w.expect("status", "connected")
	w.until("$ ")

	// 정책을 올리면 조건을 채우지 못하는 세션의 터미널을 닫는다.
	e.setPolicy(api.SecurityPolicy{Terminal: api.SecurityTwoFactor})
	if msg := w.expect("status", "closed"); msg.Reason != terminal.ReasonSecurityRequired {
		t.Fatalf("closed = %+v", msg)
	}
}

func TestSecurityElevationRequired(t *testing.T) {
	e := newEnv(t, envOptions{helper: true})
	e.setPolicy(api.SecurityPolicy{Elevation: api.SecurityTwoFactor})
	b := e.browser()
	wantStatus(t, b.login("totp", "totp"), http.StatusOK)
	wantStatus(t, b.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}), http.StatusOK)
	wantStatus(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}), http.StatusOK)

	// 2차 인증을 거치지 않은 관리자는 비밀번호를 묻기 전에 막는다.
	a := e.browser()
	wantStatus(t, a.login("admin", "admin"), http.StatusOK)
	if got := wantCode(t, a.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "admin"}),
		http.StatusForbidden, api.ErrSecurityRequired); got.Required != api.SecurityTwoFactor {
		t.Fatalf("required = %q", got.Required)
	}
}

func TestSecurityAdminReset(t *testing.T) {
	dir := t.TempDir()
	fs, err := prefs.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, envOptions{security: fs})
	u := e.browser()
	wantStatus(t, u.login("user", "user"), http.StatusOK)
	u.enrollTOTP("user")
	os.WriteFile(filepath.Join(dir, "security", "broken.json"), []byte("{"), 0o600)

	a := e.browser()
	wantStatus(t, a.login("admin", "admin"), http.StatusOK)
	users := decode[[]api.SecurityUserInfo](t, a.do(http.MethodGet, "/api/security/users", nil))
	if len(users) != 2 || users[0] != (api.SecurityUserInfo{Username: "broken", Unreadable: true}) ||
		users[1] != (api.SecurityUserInfo{Username: "user", TOTP: true, RecoveryCodes: security.RecoveryCodeCount}) {
		t.Fatalf("users = %+v", users)
	}
	wantCode(t, u.do(http.MethodGet, "/api/security/users", nil), http.StatusForbidden, api.ErrAdminRequired)
	// 2차 인증을 거친 관리자만 초기화한다.
	if got := wantCode(t, a.do(http.MethodDelete, "/api/security/users/user", nil), http.StatusForbidden, api.ErrSecurityRequired); got.Required != api.SecurityTwoFactor {
		t.Fatalf("required = %q", got.Required)
	}
	s := e.strongAdmin()
	wantStatus(t, s.do(http.MethodDelete, "/api/security/users/user", nil), http.StatusNoContent)
	wantStatus(t, s.do(http.MethodDelete, "/api/security/users/broken", nil), http.StatusNoContent)
	wantStatus(t, s.do(http.MethodDelete, "/api/security/users/user", nil), http.StatusNotFound)
	// 그 사용자의 세션은 끝나고, 다시 로그인하면 수단이 없다.
	wantCode(t, u.do(http.MethodGet, "/api/session", nil), http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, u.login("user", "user"), http.StatusOK)
	if st := u.securityStatus(); st.TOTP != nil || st.RecoveryCodes != 0 {
		t.Fatalf("after reset = %+v", st)
	}
	if !strings.Contains(e.logs.String(), `security: methods reset: user="user" by="totp"`) {
		t.Errorf("reset not logged:\n%s", e.logs.String())
	}
}

func TestSecurityCorruptRecordFailsClosed(t *testing.T) {
	dir := t.TempDir()
	fs, _ := prefs.NewFileStore(dir)
	os.MkdirAll(filepath.Join(dir, "security"), 0o700)
	os.WriteFile(filepath.Join(dir, "security", "user.json"), []byte("{"), 0o600)
	e := newEnv(t, envOptions{security: fs})
	e.setPolicy(api.SecurityPolicy{Terminal: api.SecurityTwoFactor})
	b := e.browser()
	if got := decode[api.LoginResponse](t, b.login("user", "user")); got.Status != api.LoginAuthenticated || got.StepUp != nil {
		t.Fatalf("login = %+v", got)
	}
	wantCode(t, b.do(http.MethodGet, "/api/security", nil), http.StatusInternalServerError, api.ErrSecurityUnavailable)
	wantCode(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorTOTP, Code: "123456"}),
		http.StatusInternalServerError, api.ErrSecurityUnavailable)
	wantCode(t, b.do(http.MethodPost, "/api/security/manage", api.SecurityManageRequest{Password: "user"}),
		http.StatusInternalServerError, api.ErrSecurityUnavailable)
	if s := decode[api.SessionInfo](t, b.do(http.MethodGet, "/api/session", nil)); s.Assurance != api.AssuranceLogin {
		t.Fatalf("session = %+v", s)
	}
}

func TestSecurityFactorRateLimit(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	secret, _ := b.enrollTOTP("user")
	o := e.browser()
	wantStatus(t, o.login("user", "user"), http.StatusOK)
	// 등록에 쓴 코드의 다음 단계로 넘어간 뒤 틀린다.
	e.clk.Add(30 * time.Second)
	for range auth.FreeFailures + 1 {
		wantCode(t, o.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorTOTP, Code: "000000"}),
			http.StatusBadRequest, api.ErrFactorFailed)
	}
	wantCode(t, o.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorTOTP, Code: e.totpCode(secret)}),
		http.StatusTooManyRequests, api.ErrRateLimited)
}
