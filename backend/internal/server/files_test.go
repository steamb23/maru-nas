package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"onmaru/backend/internal/addons/storage"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/omv/omvfake"
	"onmaru/backend/internal/prefs"
)

// fakeMounts는 /srv 아래 마운트 목록이다(파일 탐색기의 디스크).
type fakeMounts []storage.MountLabel

func (m fakeMounts) MountsUnder(string) ([]storage.MountLabel, error) { return m, nil }

// filesEnv는 가짜 도우미의 가짜 디스크에 공유 폴더 docs·media와 루트 파일시스템의 공유 폴더 kvm을 둔 환경이다.
type filesEnv struct {
	*env
	disk string
}

func newFilesEnv(t *testing.T, o envOptions) *filesEnv {
	t.Helper()
	o.helper, o.files = true, true
	e := newEnv(t, o)
	return &filesEnv{env: e, disk: e.helper.Disk()}
}

func (e *filesEnv) file(rel, content string) string {
	e.t.Helper()
	p := filepath.Join(e.disk, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *filesEnv) shares(v omvfake.Version) {
	e.fake.SetVersion(v)
	d := omvfake.DefaultPluginData()
	d.SharedFolders = []omvfake.SharedFolder{
		{UUID: "u-media", Name: "미디어", MountDir: e.disk, RelDirPath: "media/"},
		{UUID: "u-docs", Name: "docs", MountDir: e.disk, RelDirPath: "docs/"},
		{UUID: "u-kvm", Name: "kvm", MountDir: "/", RelDirPath: "kvm/"},
		{UUID: "u-gone", Name: "gone", MountDir: e.disk, RelDirPath: "gone/", Missing: true},
	}
	e.fake.SetPluginData(d)
}

func q(p string) string { return url.QueryEscape(p) }

func TestFilesNeedHelper(t *testing.T) {
	e := newEnv(t, envOptions{})
	b := e.browser()
	b.login("user", "user")
	rec := b.do(http.MethodGet, "/api/files/roots", nil)
	wantStatus(t, rec, http.StatusServiceUnavailable)
	if decode[api.ErrorResponse](t, rec).Code != api.ErrHelperUnavailable {
		t.Fatal(rec.Body.String())
	}
	wantStatus(t, e.browser().do(http.MethodGet, "/api/files/roots", nil), http.StatusUnauthorized)
}

func TestFilesRootsAndSnapshot(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.shares(omvfake.Version85)
	e.file("media/a.txt", "a")
	e.file("docs/b.txt", "b")

	// 스냅숏이 없으면 홈과 디스크를 보인다(대체).
	user := e.browser()
	user.login("user", "user")
	roots := decode[api.FilesRoots](t, user.do(http.MethodGet, "/api/files/roots", nil))
	if roots.Home == nil || roots.Home.Path != e.helper.Home("user") || !roots.Fallback || len(roots.Shares) != 0 ||
		len(roots.Disks) != 1 || roots.Disks[0].Name != "data1" || roots.Disks[0].Path != e.disk {
		t.Fatalf("fallback roots = %+v", roots)
	}

	// 관리자가 로그인하면 스냅숏을 뒤에서 받는다.
	admin := e.browser()
	admin.login("admin", "admin")
	var settings api.FilesSettings
	for range 200 {
		settings = decode[api.FilesSettings](t, admin.do(http.MethodGet, "/api/site/files", nil))
		if settings.SharesUpdatedAt != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	want := []api.SharedFolder{{UUID: "u-docs", Name: "docs", Path: e.disk + "/docs"}, {UUID: "u-kvm", Name: "kvm", Path: "/kvm"},
		{UUID: "u-media", Name: "미디어", Path: e.disk + "/media"}}
	if !slices.Equal(settings.Shares, want) || settings.ShowDisks {
		t.Fatalf("settings = %+v", settings)
	}
	roots = decode[api.FilesRoots](t, user.do(http.MethodGet, "/api/files/roots", nil))
	if roots.Fallback || len(roots.Disks) != 0 || len(roots.Shares) != 3 || roots.Shares[1].Reachable || !roots.Shares[2].Reachable {
		t.Fatalf("roots = %+v", roots)
	}

	// 디스크 최상위 보기는 관리자에게만 디스크를 보인다.
	wantStatus(t, user.do(http.MethodPut, "/api/site/files", api.FilesSettingsRequest{ShowDisks: true}), http.StatusForbidden)
	wantStatus(t, admin.do(http.MethodPut, "/api/site/files", api.FilesSettingsRequest{ShowDisks: true}), http.StatusOK)
	if r := decode[api.FilesRoots](t, admin.do(http.MethodGet, "/api/files/roots", nil)); len(r.Disks) != 1 {
		t.Fatalf("admin disks = %+v", r)
	}
	if r := decode[api.FilesRoots](t, user.do(http.MethodGet, "/api/files/roots", nil)); len(r.Disks) != 0 {
		t.Fatalf("user disks = %+v", r)
	}
	// 디스크 맨 위는 관리자만 연다.
	wantStatus(t, admin.do(http.MethodGet, "/api/files/list?path="+q(e.disk), nil), http.StatusOK)
	rec := user.do(http.MethodGet, "/api/files/list?path="+q(e.disk), nil)
	wantStatus(t, rec, http.StatusForbidden)
	if decode[api.ErrorResponse](t, rec).Code != api.ErrOutsideRoots {
		t.Fatal(rec.Body.String())
	}

	// 지금 갱신(관리자 전용, JSON 본문)
	wantStatus(t, user.do(http.MethodPost, "/api/site/files/refresh", struct{}{}), http.StatusForbidden)
	e.fake.SetPluginData(omvfake.PluginData{SharedFolders: []omvfake.SharedFolder{{UUID: "u-x", Name: "x", MountDir: e.disk, RelDirPath: "x/"}}})
	settings = decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	if len(settings.Shares) != 1 || settings.Shares[0].Name != "x" || !settings.ShowDisks {
		t.Fatalf("refreshed = %+v", settings)
	}
}

func TestFilesSharesOMV80(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.shares(omvfake.Version80)
	admin := e.browser()
	admin.login("admin", "admin")
	settings := decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	// 8.0은 status가 없어 폴더가 없는 공유 폴더도 마운트 지점과 상대 경로로 만든다.
	if len(settings.Shares) != 4 || settings.Shares[3].Path != e.disk+"/media" {
		t.Fatalf("8.0 shares = %+v", settings.Shares)
	}
}

func TestFilesListing(t *testing.T) {
	e := newFilesEnv(t, envOptions{site: prefs.NewMemoryStore()})
	e.shares(omvfake.Version85)
	for i := range 30 {
		e.file(fmt.Sprintf("media/f%02d.txt", i), strings.Repeat("x", i))
	}
	if err := os.Mkdir(filepath.Join(e.disk, "media", "Sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 같은 초 안의 변경도 가려내도록 수정 시각을 다르게 둔다.
	for i := range 30 {
		mt := time.Unix(1_700_000_000+int64((i*7)%30), 0)
		os.Chtimes(filepath.Join(e.disk, "media", fmt.Sprintf("f%02d.txt", i)), mt, mt)
	}
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	b := e.browser()
	b.login("user", "user")
	media := e.disk + "/media"

	l := decode[api.FilesListing](t, b.do(http.MethodGet, "/api/files/list?path="+q(media), nil))
	if l.Total != 31 || l.Path != media || !l.Writable || l.Truncated || l.ID == "" {
		t.Fatalf("listing = %+v", l)
	}
	page := decode[api.FilesPage](t, b.do(http.MethodGet, "/api/files/list/"+l.ID+"?offset=0&limit=3", nil))
	if page.Total != 31 || len(page.Items) != 3 || page.Items[0].Name != "Sub" || page.Items[0].Type != "dir" ||
		page.Items[1].Name != "f00.txt" || page.Items[2].Size != 1 || page.Items[2].State != "ok" || page.Items[2].ModTime == 0 {
		t.Fatalf("page = %+v", page)
	}
	// 이름만
	page = decode[api.FilesPage](t, b.do(http.MethodGet, "/api/files/list/"+l.ID+"?offset=29&limit=5&names=1", nil))
	if len(page.Items) != 2 || page.Items[1].Name != "f29.txt" || page.Items[1].State != "pending" {
		t.Fatalf("names = %+v", page)
	}
	// 찾기
	page = decode[api.FilesPage](t, b.do(http.MethodGet, "/api/files/list/"+l.ID+"?q=F1&limit=20", nil))
	if page.Total != 10 || page.Items[0].Name != "f10.txt" {
		t.Fatalf("query = %+v", page)
	}

	// 날짜순은 정보를 다 읽은 뒤에만 된다.
	rec := b.do(http.MethodGet, "/api/files/list/"+l.ID+"?sort=mtime&limit=5", nil)
	wantStatus(t, rec, http.StatusConflict)
	if decode[api.ErrorResponse](t, rec).Code != api.ErrSortPending {
		t.Fatal(rec.Body.String())
	}
	prep := decode[api.FilesPrepareResponse](t, b.do(http.MethodPost, "/api/files/list/"+l.ID+"/prepare", struct{}{}))
	if prep.Filled != 31 || prep.Total != 31 {
		t.Fatalf("prepare = %+v", prep)
	}
	page = decode[api.FilesPage](t, b.do(http.MethodGet, "/api/files/list/"+l.ID+"?sort=mtime&order=desc&limit=3", nil))
	if page.Items[0].Name != "Sub" || page.Items[1].ModTime < page.Items[2].ModTime {
		t.Fatalf("mtime desc = %+v", page)
	}
	page = decode[api.FilesPage](t, b.do(http.MethodGet, "/api/files/list/"+l.ID+"?sort=size&order=desc&offset=1&limit=1", nil))
	if page.Items[0].Name != "f29.txt" {
		t.Fatalf("size desc = %+v", page)
	}

	// 폴더가 그대로면 같은 목록을 다시 쓰고, 항목이 바뀌거나 새로 고치면 다시 읽는다.
	if again := decode[api.FilesListing](t, b.do(http.MethodGet, "/api/files/list?path="+q(media), nil)); again.ID != l.ID {
		t.Fatalf("listing not reused: %+v", again)
	}
	e.file("media/new.txt", "n")
	again := decode[api.FilesListing](t, b.do(http.MethodGet, "/api/files/list?path="+q(media), nil))
	if again.ID == l.ID || again.Total != 32 {
		t.Fatalf("listing after change = %+v", again)
	}
	fresh := decode[api.FilesListing](t, b.do(http.MethodGet, "/api/files/list?path="+q(media)+"&refresh=1", nil))
	if fresh.ID == again.ID {
		t.Fatal("refresh reused the listing")
	}
	// 버린 목록·다른 세션의 목록은 410이다.
	wantStatus(t, b.do(http.MethodGet, "/api/files/list/"+again.ID+"?limit=1", nil), http.StatusGone)
	wantStatus(t, admin.do(http.MethodGet, "/api/files/list/"+fresh.ID+"?limit=1", nil), http.StatusGone)

	// 잘못된 범위·경로
	for _, bad := range []string{"?limit=0", "?limit=501", "?limit=10&names=1&offset=-1", "?sort=kind", "?order=up"} {
		wantStatus(t, b.do(http.MethodGet, "/api/files/list/"+fresh.ID+bad, nil), http.StatusBadRequest)
	}
	for _, p := range []string{"", "media", media + "/", media + "/../docs"} {
		wantStatus(t, b.do(http.MethodGet, "/api/files/list?path="+q(p), nil), http.StatusBadRequest)
	}
	for _, p := range []string{"/etc", e.helper.Home("admin"), e.disk, "/kvm"} {
		rec := b.do(http.MethodGet, "/api/files/list?path="+q(p), nil)
		wantStatus(t, rec, http.StatusForbidden)
		if decode[api.ErrorResponse](t, rec).Code != api.ErrOutsideRoots {
			t.Fatalf("%s: %s", p, rec.Body.String())
		}
	}
	rec = b.do(http.MethodGet, "/api/files/list?path="+q(media+"/f01.txt"), nil)
	wantStatus(t, rec, http.StatusBadRequest)
	if decode[api.ErrorResponse](t, rec).Code != api.ErrNotDir {
		t.Fatal(rec.Body.String())
	}
	wantStatus(t, b.do(http.MethodGet, "/api/files/list?path="+q(media+"/missing"), nil), http.StatusNotFound)
	// 홈 폴더
	if err := os.WriteFile(filepath.Join(e.helper.Home("user"), "h.txt"), []byte("h"), 0o644); err != nil {
		t.Fatal(err)
	}
	if home := decode[api.FilesListing](t, b.do(http.MethodGet, "/api/files/list?path="+q(e.helper.Home("user")), nil)); home.Total != 1 {
		t.Fatalf("home = %+v", home)
	}
}

func TestFilesInfo(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.shares(omvfake.Version85)
	p := e.file("media/a.txt", "hello")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	info := decode[api.FileInfo](t, admin.do(http.MethodGet, "/api/files/info?path="+q(p), nil))
	if info.Name != "a.txt" || info.Size != 5 || info.Type != "file" || info.UID != os.Getuid() || info.Access == nil ||
		!info.Access.Read || info.ShareUUID != "u-media" || info.ModTime == 0 || info.Path != p {
		t.Fatalf("admin info = %+v", info)
	}
	user := e.browser()
	user.login("user", "user")
	if info := decode[api.FileInfo](t, user.do(http.MethodGet, "/api/files/info?path="+q(p), nil)); info.ShareUUID != "" {
		t.Fatalf("user got share uuid: %+v", info)
	}
	wantStatus(t, user.do(http.MethodGet, "/api/files/info?path="+q(e.disk+"/media/missing"), nil), http.StatusNotFound)
}

func TestFilesDownload(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.shares(omvfake.Version85)
	p := e.file("media/영화 목록 \"1\".txt", "0123456789")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	b := e.browser()
	b.login("user", "user")
	path := "/api/files/download?path=" + q(p)

	rec := b.do(http.MethodGet, path, nil)
	wantStatus(t, rec, http.StatusOK)
	h := rec.Result().Header
	if rec.Body.String() != "0123456789" || h.Get("Content-Type") != "application/octet-stream" ||
		h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") ||
		h.Get("Accept-Ranges") != "bytes" || h.Get("Content-Length") != "10" || h.Get("ETag") == "" {
		t.Fatalf("headers = %v body %q", h, rec.Body.String())
	}
	if cd := h.Get("Content-Disposition"); cd != `attachment; filename="__ __ _1_.txt"; filename*=UTF-8''%EC%98%81%ED%99%94%20%EB%AA%A9%EB%A1%9D%20%221%22.txt` {
		t.Fatalf("content disposition = %q", cd)
	}
	etag := h.Get("ETag")

	get := func(rng, ifRange string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Range", rng)
		if ifRange != "" {
			req.Header.Set("If-Range", ifRange)
		}
		return b.send(req)
	}
	rec = get("bytes=2-5", etag)
	wantStatus(t, rec, http.StatusPartialContent)
	if rec.Body.String() != "2345" || rec.Result().Header.Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("range = %q %v", rec.Body.String(), rec.Result().Header)
	}
	if rec = get("bytes=-3", ""); rec.Code != http.StatusPartialContent || rec.Body.String() != "789" {
		t.Fatalf("suffix = %d %q", rec.Code, rec.Body.String())
	}
	if rec = get("bytes=7-", ""); rec.Code != http.StatusPartialContent || rec.Body.String() != "789" {
		t.Fatalf("open = %d %q", rec.Code, rec.Body.String())
	}
	// ETag가 다르거나 여러 범위면 전체를 보낸다.
	if rec = get("bytes=2-5", `"other"`); rec.Code != http.StatusOK || rec.Body.String() != "0123456789" {
		t.Fatalf("if-range mismatch = %d", rec.Code)
	}
	if rec = get("bytes=0-1,4-5", ""); rec.Code != http.StatusOK {
		t.Fatalf("multi range = %d", rec.Code)
	}
	rec = get("bytes=20-", "")
	wantStatus(t, rec, http.StatusRequestedRangeNotSatisfiable)
	if rec.Result().Header.Get("Content-Range") != "bytes */10" {
		t.Fatal(rec.Result().Header)
	}

	rec = b.do(http.MethodGet, "/api/files/download?path="+q(e.disk+"/media"), nil)
	wantStatus(t, rec, http.StatusBadRequest)
	if decode[api.ErrorResponse](t, rec).Code != api.ErrIsDir {
		t.Fatal(rec.Body.String())
	}
	wantStatus(t, b.do(http.MethodGet, "/api/files/download?path="+q(e.disk+"/media/missing"), nil), http.StatusNotFound)
	wantStatus(t, b.do(http.MethodGet, "/api/files/download?path="+q("/etc/passwd"), nil), http.StatusForbidden)
}

func TestFilesUpload(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.shares(omvfake.Version85)
	e.file("media/.keep", "")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	b := e.browser()
	b.login("user", "user")
	target := e.disk + "/media/올린 파일.txt"
	up := func(p, contentType, query string, body string) *httptest.ResponseRecorder {
		return b.raw(http.MethodPut, "/api/files/upload?path="+q(p)+query, contentType, []byte(body))
	}

	rec := up(target, "application/octet-stream", "", "hello")
	wantStatus(t, rec, http.StatusOK)
	if it := decode[api.FileItem](t, rec); it.Name != "올린 파일.txt" || it.Size != 5 || it.State != "ok" {
		t.Fatalf("uploaded = %+v", it)
	}
	if got, _ := os.ReadFile(target); string(got) != "hello" {
		t.Fatalf("content = %q", got)
	}
	rec = up(target, "application/octet-stream", "", "again")
	wantStatus(t, rec, http.StatusConflict)
	if decode[api.ErrorResponse](t, rec).Code != api.ErrFileExists {
		t.Fatal(rec.Body.String())
	}
	wantStatus(t, up(target, "application/octet-stream", "&overwrite=1", "again!"), http.StatusOK)
	if got, _ := os.ReadFile(target); string(got) != "again!" {
		t.Fatalf("overwritten = %q", got)
	}

	// 원시 본문만 받는다(CSRF 보조 장치).
	for _, ct := range []string{"", "application/json", "multipart/form-data; boundary=x", "text/plain"} {
		wantStatus(t, up(e.disk+"/media/x.txt", ct, "", "x"), http.StatusUnsupportedMediaType)
	}
	for _, p := range []string{e.disk + "/media/.onmaru-upload-x.part", e.disk + "/media/", e.disk + "/media/a\xffb", "media/x"} {
		wantStatus(t, up(p, "application/octet-stream", "", "x"), http.StatusBadRequest)
	}
	wantStatus(t, up("/etc/x", "application/octet-stream", "", "x"), http.StatusForbidden)
	wantStatus(t, up(e.disk+"/x.txt", "application/octet-stream", "", "x"), http.StatusForbidden)

	// 길이를 모르는 본문은 받지 않는다.
	req, _ := http.NewRequest(http.MethodPut, "/api/files/upload?path="+q(e.disk+"/media/chunked.txt"), strings.NewReader("abc"))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = -1
	wantStatus(t, b.send(req), http.StatusLengthRequired)
}

func TestFilesSecurityPolicy(t *testing.T) {
	site := prefs.NewMemoryStore()
	site.UpdateSite(func(v *api.SiteSettings) { v.Security = &api.SecurityPolicy{Files: api.SecurityPassword} })
	e := newFilesEnv(t, envOptions{site: site})
	b := e.browser()
	b.login("user", "user")
	for _, p := range []string{"/api/files/roots", "/api/files/list?path=" + q(e.helper.Home("user")), "/api/files/download?path=" + q("/x")} {
		rec := b.do(http.MethodGet, p, nil)
		wantStatus(t, rec, http.StatusForbidden)
		if er := decode[api.ErrorResponse](t, rec); er.Code != api.ErrSecurityRequired || er.Required != api.SecurityPassword {
			t.Fatalf("%s: %+v", p, er)
		}
	}
	// 비밀번호를 다시 확인하면 쓸 수 있다.
	wantStatus(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "user"}), http.StatusOK)
	wantStatus(t, b.do(http.MethodGet, "/api/files/roots", nil), http.StatusOK)
}
