package server

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/omv/omvfake"
)

func comment(c string) api.ProfileRequest { return api.ProfileRequest{Comment: &c} }

// methods는 가짜 OMV가 받은 호출 중 프로필 저장과 관련된 것의 이름이다(Exec.isRunning은 횟수가 바뀔 수 있어 하나로 줄인다).
func methods(e *env) []string {
	var out []string
	for _, c := range e.fake.Calls() {
		switch c.Service {
		case "UserMgmt", "Config", "Exec":
			name := c.Service + "." + c.Method
			if len(out) > 0 && out[len(out)-1] == name {
				continue
			}
			out = append(out, name)
		}
	}
	return out
}

func TestSetProfileUser(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantCode(t, b.do(http.MethodPut, "/api/profile", comment("x")), http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	rec := b.do(http.MethodPut, "/api/profile", comment("  Backup account  "))
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.ProfileSaveResponse](t, rec); got.Applied {
		t.Error("user save reported applied")
	}
	// 설명만 바꾸고 전자 우편은 그대로 보낸다. 일반 사용자는 적용하지 않아 적용 대기가 남는다.
	u, _ := e.fake.User("user")
	if u.Comment != "Backup account" || u.Email != "user@example.com" {
		t.Errorf("stored %+v", u)
	}
	if got := strings.Join(methods(e), " "); got != "UserMgmt.getUserByContext UserMgmt.setUserByContext" {
		t.Errorf("calls %s", got)
	}
	for _, c := range e.fake.Calls() {
		if c.Service == "UserMgmt" && c.Touch {
			t.Errorf("%s.%s extended the OMV session", c.Service, c.Method)
		}
	}
	if got := decode[api.ProfileResponse](t, b.do(http.MethodGet, "/api/profile", nil)); got.Comment != "Backup account" {
		t.Errorf("profile after save %+v", got)
	}
}

func TestSetProfileAdminApplies(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	loginTOTP(t, b)
	rec := b.do(http.MethodPut, "/api/profile", comment("Admin"))
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.ProfileSaveResponse](t, rec); !got.Applied {
		t.Error("admin save not applied")
	}
	want := "UserMgmt.getUser Config.isDirty UserMgmt.setUserByContext Config.applyChangesBg Exec.isRunning"
	if got := strings.Join(methods(e), " "); got != want {
		t.Errorf("calls %s", got)
	}
	if u, _ := e.fake.User("totp"); u.Comment != "Admin" || u.Email != "totp@example.com" {
		t.Errorf("stored %+v", u)
	}

	// 같은 값이면 저장·적용하지 않는다.
	before := len(e.fake.Calls())
	rec = b.do(http.MethodPut, "/api/profile", comment("Admin"))
	wantStatus(t, rec, http.StatusOK)
	for _, c := range e.fake.Calls()[before:] {
		if c.Method == "setUserByContext" || c.Method == "applyChangesBg" {
			t.Errorf("unchanged save called %s", c.Method)
		}
	}
}

func TestSetProfileAdminRefusesPendingChanges(t *testing.T) {
	e := newEnv(t, envOptions{})
	d := omvfake.DefaultPluginData()
	d.Dirty = []string{"samba"}
	e.fake.SetPluginData(d)
	b := e.browser()
	loginTOTP(t, b)
	wantCode(t, b.do(http.MethodPut, "/api/profile", comment("Admin")), http.StatusConflict, api.ErrConfigDirty)
	if u, _ := e.fake.User("totp"); u.Comment != "" {
		t.Errorf("saved despite pending changes: %+v", u)
	}
}

func TestSetProfileApplyFailure(t *testing.T) {
	e := newEnv(t, envOptions{})
	d := omvfake.DefaultPluginData()
	d.FailApply = true
	e.fake.SetPluginData(d)
	b := e.browser()
	loginTOTP(t, b)
	// 설명은 바뀌었지만 적용이 실패했다.
	wantCode(t, b.do(http.MethodPut, "/api/profile", comment("Admin")), http.StatusBadGateway, api.ErrActionFailed)
	if u, _ := e.fake.User("totp"); u.Comment != "Admin" {
		t.Errorf("stored %+v", u)
	}
}

func TestSetProfileReadOnly(t *testing.T) {
	e := newEnv(t, envOptions{})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantCode(t, admin.do(http.MethodPut, "/api/profile", comment("x")), http.StatusConflict, api.ErrProfileReadOnly)

	e.fake.PutUser(omvfake.User{Name: "user", Password: "user", Email: "user@example.com", ReadOnly: true})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if got := decode[api.ProfileResponse](t, b.do(http.MethodGet, "/api/profile", nil)); got.ReadOnlyReason != api.ProfileReadOnlyLocked {
		t.Errorf("profile %+v", got)
	}
	wantCode(t, b.do(http.MethodPut, "/api/profile", comment("x")), http.StatusConflict, api.ErrProfileReadOnly)
	for _, c := range e.fake.Calls() {
		if c.Method == "setUserByContext" {
			t.Error("read-only account was modified")
		}
	}
}

func TestSetProfileRejectsInvalid(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	for name, body := range map[string]any{
		"missing":  map[string]any{},
		"colon":    comment("a:b"),
		"newline":  comment("line\nbreak"),
		"tab":      comment("a\tb"),
		"too long": comment(strings.Repeat("가", 86)),
	} {
		if rec := b.do(http.MethodPut, "/api/profile", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	b.header.Set("Content-Type", "text/plain")
	wantCode(t, b.do(http.MethodPut, "/api/profile", comment("x")), http.StatusUnsupportedMediaType, api.ErrBadRequest)
	if slices.ContainsFunc(e.fake.Calls(), func(c omvfake.Call) bool { return c.Method == "setUserByContext" }) {
		t.Error("invalid comment reached OMV")
	}
}
