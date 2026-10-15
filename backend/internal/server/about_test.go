package server

import (
	"net/http"
	"testing"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/buildinfo"
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
	if got.Version != buildinfo.Version || got.GoVersion == "" {
		t.Fatalf("about %+v", got)
	}
}
