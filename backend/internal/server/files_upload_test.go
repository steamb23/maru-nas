package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/omv/omvfake"
	"maru/backend/internal/prefs"
)

func TestFilesUploadCheck(t *testing.T) {
	site := prefs.NewMemoryStore()
	site.UpdateSite(func(v *api.SiteSettings) {
		v.Security = &api.SecurityPolicy{Files: api.SecurityPassword, FilesScope: api.FilesScopeWrites}
	})
	e := newFilesEnv(t, envOptions{site: site})
	e.shares(omvfake.Version85)
	e.file("media/.keep", "")
	admin := e.browser()
	admin.login("admin", "admin")
	decode[api.FilesSettings](t, admin.do(http.MethodPost, "/api/site/files/refresh", struct{}{}))
	b := e.browser()
	b.login("user", "user")
	check := func(p string) *httptest.ResponseRecorder {
		return b.do(http.MethodPost, "/api/files/upload/check", api.FilesUploadCheckRequest{Path: p})
	}
	target := e.disk + "/media/올릴 파일.txt"

	// 올리기와 같은 가드라 쓰기 전용 정책에서도 보안 확인을 먼저 요구한다.
	if er := wantCode(t, check(target), http.StatusForbidden, api.ErrSecurityRequired); er.Required != api.SecurityPassword {
		t.Fatalf("required = %q", er.Required)
	}
	wantStatus(t, b.do(http.MethodPost, "/api/security/verify", api.SecurityVerifyRequest{Method: api.FactorPassword, Password: "user"}), http.StatusOK)
	wantStatus(t, check(target), http.StatusNoContent)
	// 확인은 아무것도 만들지 않는다.
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("stat = %v", err)
	}
	// 이름과 범위는 올리기와 같게 거른다(UTF-8이 아닌 이름은 JSON으로 보낼 수 없다).
	for _, p := range []string{e.disk + "/media/.maru-upload-x.part", e.disk + "/media/", "media/x"} {
		wantStatus(t, check(p), http.StatusBadRequest)
	}
	wantCode(t, check("/etc/x"), http.StatusForbidden, api.ErrOutsideRoots)
	wantCode(t, check(e.disk+"/x.txt"), http.StatusForbidden, api.ErrOutsideRoots)
	// JSON 본문만 받는다(CSRF 보조 장치).
	wantStatus(t, b.raw(http.MethodPost, "/api/files/upload/check", "text/plain", []byte(`{"path":"`+target+`"}`)), http.StatusUnsupportedMediaType)
	wantCode(t, e.browser().do(http.MethodPost, "/api/files/upload/check", api.FilesUploadCheckRequest{Path: target}), http.StatusUnauthorized, api.ErrUnauthorized)
}

// readCounter는 읽힌 양을 세는 본문이다.
type readCounter struct {
	left, read int64
}

func (r *readCounter) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), r.left))
	clear(p[:n])
	r.left -= int64(n)
	r.read += int64(n)
	return n, nil
}

func TestDrainRejected(t *testing.T) {
	const size = 8 << 20
	reject := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			io.CopyN(io.Discard, r.Body, 1000)
			writeError(w, status, "test", "rejected")
		}
	}
	for _, tc := range []struct {
		name   string
		h      http.HandlerFunc
		length int64
		read   int64
		close  bool
	}{
		// 세션이 있는 거절은 남은 본문을 끝까지 받고 연결을 그대로 둔다.
		{"403", reject(http.StatusForbidden), size, size, false},
		// 세션이 없으면 상한까지만 받는다.
		{"401", reject(http.StatusUnauthorized), size, 1000 + rejectDrainAnon, true},
		// 길이를 모르는 본문도 상한까지만 받는다.
		{"chunked", reject(http.StatusForbidden), -1, 1000 + rejectDrainAnon, true},
		// 본문을 다 읽은 뒤의 오류는 그대로 답한다.
		{"after body", func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			writeError(w, http.StatusConflict, "test", "exists")
		}, size, size, false},
		// 성공 응답은 건드리지 않는다.
		{"ok", func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			writeJSON(w, http.StatusOK, api.HealthResponse{Status: "ok"})
		}, size, size, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &readCounter{left: size}
			req := httptest.NewRequest(http.MethodPut, "/api/files/upload", body)
			req.ContentLength = tc.length
			rec := httptest.NewRecorder()
			drainRejected(tc.h)(rec, req)
			if body.read != tc.read {
				t.Fatalf("read %d, want %d", body.read, tc.read)
			}
			if got := rec.Header().Get("Connection") == "close"; got != tc.close {
				t.Fatalf("Connection = %q", rec.Header().Get("Connection"))
			}
			if rec.Code >= 400 && rec.Header().Get("Content-Length") != fmt.Sprint(rec.Body.Len()) {
				t.Fatalf("Content-Length = %q, body %d bytes", rec.Header().Get("Content-Length"), rec.Body.Len())
			}
			var er api.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &er); err != nil || rec.Code >= 400 && er.Code != "test" {
				t.Fatalf("body = %q (%v)", rec.Body.String(), err)
			}
		})
	}
}

// putLikeBrowser는 브라우저(HTTP/1.1)처럼 본문을 끝까지 보낸 뒤에야 응답을 읽는다. 보내다 실패하면 그때 응답을 읽는다.
func putLikeBrowser(t *testing.T, addr, target, cookie string, size int) (*http.Response, error) {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetDeadline(time.Now().Add(20 * time.Second))
	fmt.Fprintf(c, "PUT %s HTTP/1.1\r\nHost: maru\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\nCookie: %s\r\n\r\n",
		target, size, cookie)
	buf := make([]byte, 64<<10)
	var werr error
	for sent := 0; sent < size && werr == nil; sent += len(buf) {
		_, werr = c.Write(buf[:min(len(buf), size-sent)])
	}
	res, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatalf("read response: %v (write: %v)", err, werr)
	}
	res.Body.Close()
	return res, werr
}

// 본문을 다 받기 전에 거절해도 남은 본문을 먼저 받으므로, 본문을 끝까지 보내는 클라이언트가 쓰기 오류(RST) 없이
// 응답을 받는다. 프록시는 이 클라이언트와 같은 자리에 있다(브라우저 → 프록시 → maru).
func TestFilesUploadEarlyRejectNoReset(t *testing.T) {
	site := prefs.NewMemoryStore()
	site.UpdateSite(func(v *api.SiteSettings) {
		v.Security = &api.SecurityPolicy{Files: api.SecurityPassword, FilesScope: api.FilesScopeWrites}
	})
	e := newFilesEnv(t, envOptions{site: site})
	b := e.browser()
	b.login("user", "user")
	cookie := ""
	for _, ck := range b.cookies {
		cookie += ck.Name + "=" + ck.Value + "; "
	}
	srv := httptest.NewServer(e.h)
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	target := "/api/files/upload?path=" + url.QueryEscape(e.helper.Home("user")+"/big.bin")

	// 루프백 소켓 버퍼보다 큰 본문이어야 Go의 기본 동작(500ms 뒤 닫기)에서 쓰기가 실패한다.
	const size = 32 << 20
	res, werr := putLikeBrowser(t, addr, target, cookie, size)
	if werr != nil {
		t.Fatalf("write: %v", werr)
	}
	if res.StatusCode != http.StatusForbidden || res.Close || res.ContentLength <= 0 {
		t.Fatalf("status %d, close %v, length %d", res.StatusCode, res.Close, res.ContentLength)
	}
}
