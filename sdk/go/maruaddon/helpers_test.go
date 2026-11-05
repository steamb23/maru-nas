// SPDX-License-Identifier: MIT

package maruaddon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testToken은 시험용 가짜 호출 토큰이다. 로그에 남지 않는지 볼 때 찾는 값이다.
const testToken = "fake-call-token-for-tests"

// shortTempDir는 유닉스 소켓 경로 한도(107바이트) 안에 드는 임시 디렉터리다. TMPDIR가 길면 /tmp를 쓴다.
func shortTempDir(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if len(base) > 40 {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "maruaddon")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// syncBuffer는 서버 고루틴이 쓰는 동안 시험이 읽을 수 있는 로그 버퍼다.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// unixClient는 path의 유닉스 소켓에만 붙는 HTTP 클라이언트다.
func unixClient(path string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}, Timeout: 10 * time.Second}
}

type testServer struct {
	path   string
	client *http.Client
	logs   *syncBuffer
}

// startServer는 짧은 임시 경로의 소켓에서 h를 돌린다. 시험이 끝나면 내리고 정상 종료했는지 본다.
func startServer(t *testing.T, h Handler, hostPath string) *testServer {
	t.Helper()
	logs := &syncBuffer{}
	if h.ErrorLog == nil {
		h.ErrorLog = log.New(logs, "", 0)
	}
	path := filepath.Join(shortTempDir(t), "backend.sock")
	l, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, l, h, hostPath, 5*time.Second) }()
	ts := &testServer{path: path, client: unixClient(path), logs: logs}
	t.Cleanup(func() {
		ts.client.CloseIdleConnections()
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	return ts
}

// defaultHeaders는 maru가 보내는 헤더다.
func defaultHeaders() http.Header {
	h := http.Header{}
	h.Set("X-Maru-User", "alice")
	h.Set("X-Maru-Admin", "0")
	h.Set("X-Maru-Lang", "ko")
	h.Set("X-Maru-Call", testToken)
	return h
}

// do는 요청 하나를 보내고 상태와 본문을 돌려준다. hdr이 nil이면 defaultHeaders를 쓴다.
func (ts *testServer) do(t *testing.T, method, target string, hdr http.Header, body string) (*http.Response, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, "http://addon"+target, r)
	if err != nil {
		t.Fatal(err)
	}
	if hdr == nil {
		hdr = defaultHeaders()
	}
	req.Header = hdr
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := ts.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(data)
}

// wantError는 응답이 status와 {"code": code, …} JSON인지 본다.
func wantError(t *testing.T, resp *http.Response, body string, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status = %d, want %d (body %s)", resp.StatusCode, status, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	if e.Code != code {
		t.Errorf("code = %q, want %q (message %q)", e.Code, code, e.Message)
	}
}

// hostRequest는 가짜 호스트가 받은 요청 하나다.
type hostRequest struct {
	Method      string
	Path        string
	Host        string
	ContentType string
	Token       string
	HasToken    bool
	Body        string
}

// fakeHost는 유닉스 소켓에서 maru의 호스트 소켓을 흉내 낸다. reply가 경로와 본문으로 답을 정한다.
type fakeHost struct {
	path  string
	mu    sync.Mutex
	reqs  []hostRequest
	reply func(path, body string) (int, string)
}

func startFakeHost(t *testing.T, reply func(path, body string) (int, string)) *fakeHost {
	t.Helper()
	fh := &fakeHost{path: filepath.Join(shortTempDir(t), "host.sock"), reply: reply}
	l, err := net.Listen("unix", fh.path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		tok, has := r.Header["X-Maru-Call"]
		hr := hostRequest{Method: r.Method, Path: r.URL.Path, Host: r.Host, ContentType: r.Header.Get("Content-Type"),
			HasToken: has, Body: string(data)}
		if has {
			hr.Token = tok[0]
		}
		fh.mu.Lock()
		fh.reqs = append(fh.reqs, hr)
		fh.mu.Unlock()
		status, resp := fh.reply(r.URL.Path, string(data))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, resp)
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return fh
}

func (fh *fakeHost) requests() []hostRequest {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	return append([]hostRequest(nil), fh.reqs...)
}

// host는 가짜 호스트에 붙는 Host다.
func (fh *fakeHost) host(token string) *Host {
	return &Host{c: newHostClient(fh.path), token: token}
}

// jsonEqual은 두 JSON 문서가 같은 값인지 본다.
func jsonEqual(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got %q: %v", got, err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want %q: %v", want, err)
	}
	gb, _ := json.Marshal(g)
	wb, _ := json.Marshal(w)
	if !bytes.Equal(gb, wb) {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

var errBoom = errors.New("boom: disk exploded")
