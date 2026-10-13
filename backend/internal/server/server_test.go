package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"omv-desktop-dashboard/backend/internal/api"
)

type fakeSource struct {
	resp api.SystemResponse
	err  error
}

func (f fakeSource) System() (api.SystemResponse, error) { return f.resp, f.err }

var static = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.Write([]byte("static"))
})

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealth(t *testing.T) {
	rec := get(t, New(fakeSource{}, static), "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got api.HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || got.Status != "ok" {
		t.Errorf("body %q, err %v", rec.Body.String(), err)
	}
}

func TestSystem(t *testing.T) {
	want := api.SystemResponse{
		CPU:           api.CPUInfo{UsagePercent: 12.5},
		Memory:        api.MemoryInfo{TotalBytes: 100, UsedBytes: 40, AvailableBytes: 60},
		UptimeSeconds: 99,
	}
	rec := get(t, New(fakeSource{resp: want}, static), "/api/system")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got api.SystemResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestSystemError(t *testing.T) {
	rec := get(t, New(fakeSource{err: errors.New("boom")}, static), "/api/system")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d", rec.Code)
	}
}

func TestUnknownAPIAndStatic(t *testing.T) {
	h := New(fakeSource{}, static)
	if rec := get(t, h, "/api/nope"); rec.Code != http.StatusNotFound {
		t.Errorf("/api/nope status %d", rec.Code)
	}
	if rec := get(t, h, "/index.html"); rec.Body.String() != "static" {
		t.Errorf("static body %q", rec.Body.String())
	}
}
