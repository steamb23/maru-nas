package server

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/omv/omvfake"
)

func profileReq(email *string, tags ...string) api.ProfileRequest {
	r := api.ProfileRequest{Email: email}
	if tags != nil {
		r.Tags = &tags
	}
	return r
}

func ptr(s string) *string { return &s }

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
	wantCode(t, b.do(http.MethodPut, "/api/profile", profileReq(nil, "x")), http.StatusUnauthorized, api.ErrUnauthorized)
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	// 태그만 보내면 전자 우편은 지금 값을 그대로 보낸다. 태그는 앞뒤 공백·빈 값·중복을 빼고 쉼표로 잇는다.
	rec := b.do(http.MethodPut, "/api/profile", profileReq(nil, " backup ", "nas", "", "nas"))
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.ProfileSaveResponse](t, rec); got.Applied {
		t.Error("user save reported applied")
	}
	u, _ := e.fake.User("user")
	if u.Comment != "backup,nas" || u.Email != "user@example.com" {
		t.Errorf("stored %+v", u)
	}
	// 일반 사용자는 적용하지 않아 적용 대기가 남는다.
	if got := strings.Join(methods(e), " "); got != "UserMgmt.getUserByContext UserMgmt.setUserByContext" {
		t.Errorf("calls %s", got)
	}
	for _, c := range e.fake.Calls() {
		if c.Service == "UserMgmt" && c.Touch {
			t.Errorf("%s.%s extended the OMV session", c.Service, c.Method)
		}
	}

	// 전자 우편만 보내면 태그는 그대로 둔다. 빈 값은 전자 우편을 지운다.
	wantStatus(t, b.do(http.MethodPut, "/api/profile", profileReq(ptr(" me@example.org "))), http.StatusOK)
	if u, _ := e.fake.User("user"); u.Email != "me@example.org" || u.Comment != "backup,nas" {
		t.Errorf("after email %+v", u)
	}
	wantStatus(t, b.do(http.MethodPut, "/api/profile", profileReq(ptr(""))), http.StatusOK)
	got := decode[api.ProfileResponse](t, b.do(http.MethodGet, "/api/profile", nil))
	if got.Email != "" || !slices.Equal(got.Tags, []string{"backup", "nas"}) {
		t.Errorf("profile after save %+v", got)
	}
}

func TestSetProfileKeepsCommentWhenTagsUnchanged(t *testing.T) {
	// 태그가 같으면 OMV가 저장한 설명 칸의 모양(공백 등)을 바꾸지 않는다.
	e := newEnv(t, envOptions{})
	e.fake.PutUser(omvfake.User{Name: "user", Password: "user", Email: "user@example.com", Comment: "a, b"})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	wantStatus(t, b.do(http.MethodPut, "/api/profile", profileReq(nil, "a", "b")), http.StatusOK)
	if slices.ContainsFunc(e.fake.Calls(), func(c omvfake.Call) bool { return c.Method == "setUserByContext" }) {
		t.Error("unchanged tags were saved")
	}
	wantStatus(t, b.do(http.MethodPut, "/api/profile", profileReq(ptr("new@example.com"), "a", "b")), http.StatusOK)
	if u, _ := e.fake.User("user"); u.Comment != "a, b" || u.Email != "new@example.com" {
		t.Errorf("stored %+v", u)
	}
}

func TestSetProfileAdminApplies(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	loginTOTP(t, b)
	rec := b.do(http.MethodPut, "/api/profile", profileReq(ptr("admin@example.com"), "ops"))
	wantStatus(t, rec, http.StatusOK)
	if got := decode[api.ProfileSaveResponse](t, rec); !got.Applied {
		t.Error("admin save not applied")
	}
	want := "UserMgmt.getUser Config.isDirty UserMgmt.setUserByContext Config.applyChangesBg Exec.isRunning"
	if got := strings.Join(methods(e), " "); got != want {
		t.Errorf("calls %s", got)
	}
	if u, _ := e.fake.User("totp"); u.Comment != "ops" || u.Email != "admin@example.com" {
		t.Errorf("stored %+v", u)
	}

	// 같은 값이면 저장·적용하지 않는다.
	before := len(e.fake.Calls())
	wantStatus(t, b.do(http.MethodPut, "/api/profile", profileReq(ptr("admin@example.com"), "ops")), http.StatusOK)
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
	wantCode(t, b.do(http.MethodPut, "/api/profile", profileReq(nil, "ops")), http.StatusConflict, api.ErrConfigDirty)
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
	// 저장은 됐지만 적용이 실패했다.
	wantCode(t, b.do(http.MethodPut, "/api/profile", profileReq(nil, "ops")), http.StatusBadGateway, api.ErrActionFailed)
	if u, _ := e.fake.User("totp"); u.Comment != "ops" {
		t.Errorf("stored %+v", u)
	}
}

func TestSetProfileReadOnly(t *testing.T) {
	e := newEnv(t, envOptions{})
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantCode(t, admin.do(http.MethodPut, "/api/profile", profileReq(nil, "x")), http.StatusConflict, api.ErrProfileReadOnly)

	e.fake.PutUser(omvfake.User{Name: "user", Password: "user", Email: "user@example.com", ReadOnly: true})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	if got := decode[api.ProfileResponse](t, b.do(http.MethodGet, "/api/profile", nil)); got.ReadOnlyReason != api.ProfileReadOnlyLocked {
		t.Errorf("profile %+v", got)
	}
	wantCode(t, b.do(http.MethodPut, "/api/profile", profileReq(ptr("x@example.com"))), http.StatusConflict, api.ErrProfileReadOnly)
	if slices.ContainsFunc(e.fake.Calls(), func(c omvfake.Call) bool { return c.Method == "setUserByContext" }) {
		t.Error("read-only account was modified")
	}
}

func TestSetProfileRejectsInvalid(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	many := make([]string, 33)
	for i := range many {
		many[i] = strings.Repeat("t", i+1)
	}
	for name, body := range map[string]any{
		"nothing":       map[string]any{},
		"tag comma":     profileReq(nil, "a,b"),
		"tag colon":     profileReq(nil, "a:b"),
		"tag newline":   profileReq(nil, "line\nbreak"),
		"tag tab":       profileReq(nil, "a\tb"),
		"tags too long": profileReq(nil, strings.Repeat("가", 86)),
		"too many tags": profileReq(nil, many...),
		"email no at":   profileReq(ptr("not-an-email")),
		"email name":    profileReq(ptr("Me <me@example.com>")),
		"email space":   profileReq(ptr("me @example.com")),
	} {
		if rec := b.do(http.MethodPut, "/api/profile", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	b.header.Set("Content-Type", "text/plain")
	wantCode(t, b.do(http.MethodPut, "/api/profile", profileReq(nil, "x")), http.StatusUnsupportedMediaType, api.ErrBadRequest)
	if slices.ContainsFunc(e.fake.Calls(), func(c omvfake.Call) bool { return c.Method == "setUserByContext" }) {
		t.Error("invalid input reached OMV")
	}
}
