package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func distFS(index string) fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(index)},
		"assets/index-AbC1.js": {Data: []byte("console.log(1)")},
		"assets/app-XyZ2.css":  {Data: []byte("body{}")},
	}
}

func get(t *testing.T, h http.Handler, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAssetsAreImmutable(t *testing.T) {
	h := newHandler(distFS("<!doctype html>"))
	for _, target := range []string{"/assets/index-AbC1.js", "/assets/app-XyZ2.css"} {
		rec := get(t, h, target, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", target, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != immutableCache {
			t.Errorf("%s: Cache-Control %q", target, got)
		}
		etag := rec.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no ETag", target)
		}
		// 강력 새로 고침처럼 다시 확인하면 304로 답한다.
		if rec := get(t, h, target, map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
			t.Errorf("%s: revalidate status %d", target, rec.Code)
		}
	}
}

func TestIndexRevalidates(t *testing.T) {
	h := newHandler(distFS("<!doctype html>v1"))
	rec := get(t, h, "/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "<!doctype html>v1" {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != revalidateCache {
		t.Errorf("Cache-Control %q", got)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	if rec := get(t, h, "/", map[string]string{"If-None-Match": etag}); rec.Code != http.StatusNotModified {
		t.Errorf("same build: status %d, want 304", rec.Code)
	}

	// 새 빌드(업그레이드)의 index.html은 ETag가 달라 새 내용을 받는다.
	next := newHandler(distFS("<!doctype html>v2"))
	rec = get(t, next, "/", map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusOK || rec.Body.String() != "<!doctype html>v2" {
		t.Errorf("new build: status %d, body %q", rec.Code, rec.Body.String())
	}
}

func TestMissingAssetIsNotCached(t *testing.T) {
	h := newHandler(distFS("<!doctype html>"))
	// 업그레이드 전 탭이 요청한 옛 청크. 404를 오래 캐시하면 그 버전으로 되돌렸을 때도 받지 못한다.
	rec := get(t, h, "/assets/index-Old9.js", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control %q on 404", got)
	}
}

func TestDirectoryIsNotImmutable(t *testing.T) {
	h := newHandler(distFS("<!doctype html>"))
	rec := get(t, h, "/assets/", nil)
	if got := rec.Header().Get("Cache-Control"); got == immutableCache {
		t.Errorf("directory listing marked immutable")
	}
}

func TestNotBuilt(t *testing.T) {
	h := newHandler(fstest.MapFS{})
	rec := get(t, h, "/", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control %q", got)
	}
}
