package server

import (
	"net/http"
	"strings"
	"testing"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/omv/omvfake"
)

func TestPreviewHeaders(t *testing.T) {
	for _, c := range []struct{ path, ct, csp string }{
		{"/srv/a/사진.JPG", "image/jpeg", sandboxCSP},
		{"/srv/a/그림.svg", "image/svg+xml", sandboxCSP},
		{"/srv/a/영상.mp4", "video/mp4", sandboxCSP},
		{"/srv/a/노래.flac", "audio/flac", sandboxCSP},
		{"/srv/a/문서.pdf", "application/pdf", pdfCSP},
		// 목록에 없는 형식(HTML 포함)과 확장자 없는 파일은 텍스트다.
		{"/srv/a/page.html", "text/plain; charset=utf-8", sandboxCSP},
		{"/srv/a/Makefile", "text/plain; charset=utf-8", sandboxCSP},
		{"/srv/a/.bashrc", "text/plain; charset=utf-8", sandboxCSP},
	} {
		h := previewHeaders(c.path)
		if h.contentType != c.ct || h.csp != c.csp || !strings.HasPrefix(h.disposition, "inline; ") {
			t.Errorf("%s: %+v", c.path, h)
		}
	}
	if !strings.Contains(sandboxCSP, "sandbox") || strings.Contains(pdfCSP, "sandbox") || !strings.Contains(pdfCSP, "frame-ancestors 'self'") {
		t.Fatal("csp")
	}
}

func TestFilesPreview(t *testing.T) {
	e := newFilesEnv(t, envOptions{})
	e.shares(omvfake.Version85)
	img := e.file("media/그림.png", "\x89PNG fake")
	txt := e.file("media/읽어 주세요.md", "# 제목\n본문")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	b := e.browser()
	wantStatus(t, b.do(http.MethodGet, "/api/files/preview?path="+q(img), nil), http.StatusUnauthorized)
	b.login("user", "user")

	rec := b.do(http.MethodGet, "/api/files/preview?path="+q(img), nil)
	wantStatus(t, rec, http.StatusOK)
	h := rec.Result().Header
	if rec.Body.String() != "\x89PNG fake" || h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" ||
		h.Get("Content-Security-Policy") != sandboxCSP || !strings.HasPrefix(h.Get("Content-Disposition"), "inline; ") ||
		h.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("headers = %v", h)
	}

	// 텍스트는 앞부분만 범위로 받는다.
	req, _ := http.NewRequest(http.MethodGet, "/api/files/preview?path="+q(txt), nil)
	req.Header.Set("Range", "bytes=0-1")
	rec = b.send(req)
	wantStatus(t, rec, http.StatusPartialContent)
	if rec.Body.String() != "# " || rec.Result().Header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("text range = %q %v", rec.Body.String(), rec.Result().Header)
	}

	wantStatus(t, b.do(http.MethodGet, "/api/files/preview?path="+q(e.disk+"/media"), nil), http.StatusBadRequest)
	wantStatus(t, b.do(http.MethodGet, "/api/files/preview?path="+q("/etc/passwd"), nil), http.StatusForbidden)
}
