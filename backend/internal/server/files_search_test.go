package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/omv/omvfake"
)

// searchEvents는 GET /api/files/search 응답의 사건들이다.
func searchEvents(t *testing.T, rec *httptest.ResponseRecorder) []api.FilesSearchEvent {
	t.Helper()
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("search response %d %v: %s", rec.Code, rec.Header(), rec.Body.String())
	}
	var out []api.FilesSearchEvent
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev api.FilesSearchEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("event %q: %v", data, err)
		}
		out = append(out, ev)
	}
	return out
}

func TestFilesSearch(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.shares(omvfake.Version85)
	e.file("media/영화/Trip 2024.mp4", "x")
	e.file("media/trip.txt", "x")
	e.file("docs/trip-plan.md", "x")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	b := e.browser()
	b.login("user", "user")
	media := e.disk + "/media"
	search := func(p, query string) *httptest.ResponseRecorder {
		return b.do(http.MethodGet, "/api/files/search?path="+q(p)+"&q="+q(query), nil)
	}

	evs := searchEvents(t, search(media, "TRIP"))
	var hits []string
	for _, ev := range evs {
		for _, h := range ev.Hits {
			hits = append(hits, h.Path+":"+h.Type)
		}
	}
	slices.Sort(hits)
	if !slices.Equal(hits, []string{media + "/trip.txt:file", media + "/영화/Trip 2024.mp4:file"}) {
		t.Fatalf("hits = %v", hits)
	}
	if last := evs[len(evs)-1]; last.Result == nil || last.Result.Dirs != 2 || last.Result.Truncated {
		t.Fatalf("last event = %+v", last)
	}

	// 시작하기 전의 실패는 보통의 오류 응답이다.
	wantStatus(t, search(media, ""), http.StatusBadRequest)
	wantStatus(t, search(media, strings.Repeat("a", 256)), http.StatusBadRequest)
	wantStatus(t, search("/etc", "a"), http.StatusForbidden)
	wantStatus(t, search(media+"/trip.txt", "a"), http.StatusBadRequest)
}
