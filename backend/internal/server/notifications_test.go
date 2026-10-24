package server

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/notify"
)

// noticeSource는 정해 둔 알림을 돌려주는 알림 출처다.
type noticeSource struct {
	name    string
	admin   bool
	notices []api.Notice
	calls   int
}

func (s *noticeSource) Name() string { return s.name }
func (s *noticeSource) Admin() bool  { return s.admin }
func (s *noticeSource) Collect(context.Context) ([]api.Notice, error) {
	s.calls++
	return s.notices, nil
}

func sampleNotice(id, rev string) api.Notice {
	return api.Notice{ID: id, Code: api.NoticeSample, Level: api.NoticeWarning, Revision: rev}
}

func noticeIDs(ns []api.Notice) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

// notifyEnv는 관리자 전용 출처 하나와 모두가 보는 출처 하나를 둔 환경이다.
func notifyEnv(t *testing.T) (*env, *noticeSource, *noticeSource) {
	t.Helper()
	admin := &noticeSource{name: "admin", admin: true, notices: []api.Notice{sampleNotice("admin-a", "r1")}}
	all := &noticeSource{name: "all", notices: []api.Notice{sampleNotice("all-a", "r1"), sampleNotice("all-b", "r1")}}
	return newEnv(t, envOptions{notify: notify.New(admin, all)}), admin, all
}

func TestNotificationsRequireSession(t *testing.T) {
	e, _, _ := notifyEnv(t)
	b := e.browser()
	wantStatus(t, b.do(http.MethodGet, "/api/notifications", nil), http.StatusUnauthorized)
	body := api.NotificationPrefsRequest{Notifications: &api.NotificationPrefs{Seen: map[string]string{}}}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/notifications", body), http.StatusUnauthorized)
}

func TestNotificationsByRole(t *testing.T) {
	e, _, _ := notifyEnv(t)
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	got := decode[api.NotificationsResponse](t, admin.do(http.MethodGet, "/api/notifications", nil))
	if ids := noticeIDs(got.Notices); !slices.Equal(ids, []string{"admin-a", "all-a", "all-b"}) {
		t.Errorf("admin notices = %v", ids)
	}
	if got.Seen == nil || len(got.Seen) != 0 {
		t.Errorf("seen = %#v, want empty map", got.Seen)
	}

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	got = decode[api.NotificationsResponse](t, user.do(http.MethodGet, "/api/notifications", nil))
	if ids := noticeIDs(got.Notices); !slices.Equal(ids, []string{"all-a", "all-b"}) {
		t.Errorf("user notices = %v", ids)
	}
}

func TestNotificationsEmptyWithoutSources(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	rec := b.do(http.MethodGet, "/api/notifications", nil)
	wantStatus(t, rec, http.StatusOK)
	if body := strings.TrimSpace(rec.Body.String()); body != `{"notices":[],"seen":{},"tasks":[]}` {
		t.Errorf("body = %s", body)
	}
}

func TestNotificationSeenRoundTrip(t *testing.T) {
	e, admin, _ := notifyEnv(t)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	seen := map[string]string{"admin-a": "r1", "all-a": "r1"}
	body := api.NotificationPrefsRequest{Notifications: &api.NotificationPrefs{Seen: seen}}
	wantStatus(t, b.do(http.MethodPut, "/api/preferences/notifications", body), http.StatusNoContent)

	got := decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil))
	if !maps.Equal(got.Seen, seen) {
		t.Errorf("seen = %v, want %v", got.Seen, seen)
	}
	prefs := decode[api.Preferences](t, b.do(http.MethodGet, "/api/preferences", nil))
	if prefs.Notifications == nil || !maps.Equal(prefs.Notifications.Seen, seen) {
		t.Errorf("preferences.notifications = %+v", prefs.Notifications)
	}

	// 알림이 사라지면 그 읽음 상태는 응답에 담지 않는다. 캐시가 지나야 다시 읽는다.
	admin.notices = nil
	got = decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil))
	if len(got.Notices) != 3 {
		t.Fatalf("cache not used: %v", noticeIDs(got.Notices))
	}
	e.clk.Add(time.Minute)
	got = decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil))
	if ids := noticeIDs(got.Notices); !slices.Equal(ids, []string{"all-a", "all-b"}) {
		t.Fatalf("notices after TTL = %v", ids)
	}
	if !maps.Equal(got.Seen, map[string]string{"all-a": "r1"}) {
		t.Errorf("seen after notice gone = %v", got.Seen)
	}
}

func TestNotificationsCachedAcrossSessions(t *testing.T) {
	e, admin, all := notifyEnv(t)
	for _, u := range []string{"admin", "user"} {
		b := e.browser()
		wantStatus(t, b.login(u, u), http.StatusOK)
		wantStatus(t, b.do(http.MethodGet, "/api/notifications", nil), http.StatusOK)
	}
	if admin.calls != 1 || all.calls != 1 {
		t.Errorf("sources read %d/%d times, want once each", admin.calls, all.calls)
	}
}

func TestNotificationPrefsValidation(t *testing.T) {
	e, _, _ := notifyEnv(t)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	put := func(body any) *httptest.ResponseRecorder {
		return b.do(http.MethodPut, "/api/preferences/notifications", body)
	}
	wantCode(t, put(map[string]any{}), http.StatusBadRequest, api.ErrBadRequest)
	too := map[string]string{}
	for i := range maxSeenEntries + 1 {
		too[fmt.Sprintf("n%d", i)] = "r1"
	}
	for name, seen := range map[string]map[string]string{
		"too many":     too,
		"bad id":       {"Bad ID": "r1"},
		"bad revision": {"admin-a": "r 1"},
		"empty rev":    {"admin-a": ""},
	} {
		body := api.NotificationPrefsRequest{Notifications: &api.NotificationPrefs{Seen: seen}}
		if rec := put(body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
	// 상한까지는 받는다. 비운 목록(null 포함)도 받는다.
	full := map[string]string{}
	for i := range maxSeenEntries {
		full[fmt.Sprintf("n%d", i)] = "r1"
	}
	wantStatus(t, put(api.NotificationPrefsRequest{Notifications: &api.NotificationPrefs{Seen: full}}), http.StatusNoContent)
	wantStatus(t, put(map[string]any{"notifications": map[string]any{"seen": nil}}), http.StatusNoContent)
	wantStatus(t, b.raw(http.MethodPut, "/api/preferences/notifications", "text/plain", []byte(`{"notifications":{"seen":{}}}`)),
		http.StatusUnsupportedMediaType)
}

func TestNotificationsStoreError(t *testing.T) {
	admin := &noticeSource{name: "admin", admin: true, notices: []api.Notice{sampleNotice("admin-a", "r1")}}
	e := newEnv(t, envOptions{notify: notify.New(admin), prefs: failingStore{}})
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	rec := b.do(http.MethodGet, "/api/notifications", nil)
	wantStatus(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "disk on fire") {
		t.Errorf("internal error leaked: %s", rec.Body.String())
	}
}
