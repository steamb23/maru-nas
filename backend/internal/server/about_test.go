package server

import (
	"net/http"
	"testing"

	"maru/backend/config"
	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

func TestAbout(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	wantCode(t, b.do(http.MethodGet, "/api/about", nil), http.StatusUnauthorized, api.ErrUnauthorized)

	// 일반 사용자도 본다.
	wantStatus(t, b.login("user", "user"), http.StatusOK)
	rec := b.do(http.MethodGet, "/api/about", nil)
	wantStatus(t, rec, http.StatusOK)
	got := decode[api.AboutResponse](t, rec)
	if got.Version != config.Version || got.GoVersion == "" {
		t.Fatalf("about %+v", got)
	}
}

func TestAboutOMVVersion(t *testing.T) {
	plugins := &fakePlugins{
		installed: map[string]bool{},
		omv:       addons.OMVInfo{Version: "8.0.0-1", VersionName: "Synthetic"},
	}
	e := newEnv(t, envOptions{plugins: plugins})

	// OMV 버전은 관리자에게만 준다(OMV System.getInformation과 같은 기준).
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	got := decode[api.AboutResponse](t, admin.do(http.MethodGet, "/api/about", nil))
	if got.OMVVersion != "8.0.0-1" || got.OMVVersionName != "Synthetic" || got.Version != config.Version {
		t.Errorf("admin about %+v", got)
	}

	// 실행 중에 OMV를 업그레이드하면 다음 요청에 반영된다.
	plugins.mu.Lock()
	plugins.omv.Version = "8.1.0-1"
	plugins.mu.Unlock()
	if got := decode[api.AboutResponse](t, admin.do(http.MethodGet, "/api/about", nil)); got.OMVVersion != "8.1.0-1" {
		t.Errorf("after upgrade %+v", got)
	}

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	got = decode[api.AboutResponse](t, user.do(http.MethodGet, "/api/about", nil))
	if got.OMVVersion != "" || got.OMVVersionName != "" {
		t.Errorf("user about %+v", got)
	}
}
