package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"maru/backend/internal/api"
	"maru/backend/internal/omv/omvfake"
	"maru/backend/internal/prefs"
)

// copyEvents는 POST /api/files/copy 응답의 사건들이다.
func copyEvents(t *testing.T, rec *httptest.ResponseRecorder) []api.FilesCopyEvent {
	t.Helper()
	wantStatus(t, rec, http.StatusOK)
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	var out []api.FilesCopyEvent
	for _, block := range strings.Split(strings.TrimSpace(rec.Body.String()), "\n\n") {
		data, ok := strings.CutPrefix(block, "data: ")
		if !ok {
			t.Fatalf("event block %q", block)
		}
		var ev api.FilesCopyEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

func TestFilesCopy(t *testing.T) {
	e := newFilesEnv(t, envOptions{site: prefs.NewMemoryStore()})
	e.shares(omvfake.Version85)
	e.file("media/a.txt", "a")
	e.file("media/sub/b.txt", "bb")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	b := e.browser()
	b.login("user", "user")
	media, home := e.disk+"/media", e.helper.Home("user")
	cp := func(req api.FilesCopyRequest) *httptest.ResponseRecorder {
		return b.do(http.MethodPost, "/api/files/copy", req)
	}

	// 다른 디스크(홈)로 복사한다. 진행이 먼저 오고 마지막이 결과다.
	evs := copyEvents(t, cp(api.FilesCopyRequest{Dir: media, Names: []string{"a.txt", "sub"}, To: home}))
	last := evs[len(evs)-1]
	if last.Result == nil || !slices.Equal(last.Result.Copied, []string{"a.txt", "sub"}) || len(last.Result.Failed) != 0 {
		t.Fatalf("events = %+v", evs)
	}
	if p := evs[len(evs)-2].Progress; p == nil || p.Scanning || p.Files != 2 || p.Bytes != 3 || p.TotalBytes != 3 {
		t.Fatalf("last progress = %+v", p)
	}
	if b, err := os.ReadFile(filepath.Join(home, "sub", "b.txt")); err != nil || string(b) != "bb" {
		t.Fatalf("copied file = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(media, "a.txt")); err != nil {
		t.Fatalf("source removed: %v", err)
	}

	// 같은 이름은 복사하지 않고 결과에 API 오류 코드로 담는다.
	evs = copyEvents(t, cp(api.FilesCopyRequest{Dir: media, Names: []string{"a.txt"}, To: home}))
	if r := evs[len(evs)-1].Result; r == nil || len(r.Copied) != 0 ||
		!slices.Equal(r.Failed, []api.FilesCopyFailure{{Name: "a.txt", Code: api.ErrFileExists}}) {
		t.Fatalf("events = %+v", evs)
	}

	// 시작하기 전의 실패는 보통의 오류 응답이다.
	wantCode := func(rec *httptest.ResponseRecorder, status int, code string) {
		t.Helper()
		wantStatus(t, rec, status)
		if got := decode[api.ErrorResponse](t, rec).Code; got != code {
			t.Fatalf("code = %q, want %q (%s)", got, code, rec.Body.String())
		}
	}
	wantCode(cp(api.FilesCopyRequest{Dir: media, To: home}), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(cp(api.FilesCopyRequest{Dir: media, Names: []string{"../x"}, To: home}), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(cp(api.FilesCopyRequest{Dir: media, Names: []string{".maru-upload-x"}, To: home}), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(cp(api.FilesCopyRequest{Dir: media, Names: []string{"sub"}, To: media + "/sub/x"}), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(cp(api.FilesCopyRequest{Dir: media, Names: []string{"a.txt"}, To: "/etc"}), http.StatusForbidden, api.ErrOutsideRoots)
	wantCode(cp(api.FilesCopyRequest{Dir: media, Names: []string{"a.txt"}, To: home + "/a.txt"}), http.StatusBadRequest, api.ErrNotDir)
	long := make([]string, 300)
	for i := range long {
		long[i] = strings.Repeat("가", 80) + string(rune('a'+i%26)) + strings.Repeat("b", i/26)
	}
	wantCode(cp(api.FilesCopyRequest{Dir: media, Names: long, To: home}), http.StatusBadRequest, api.ErrBadRequest)
}
