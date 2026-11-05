// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

// 시험은 실제 경로(/var/lib/maru, /run, /srv …)와 네트워크에 닿지 않는다. env의 함수를 바꿔 끼우고, 호스트 소켓만
// 임시 디렉터리의 유닉스 소켓으로 흉내 낸다.

// shortDir는 유닉스 소켓 경로 한도(107바이트) 안에 드는 임시 디렉터리다.
func shortDir(t *testing.T) string {
	t.Helper()
	base := os.TempDir()
	if len(base) > 40 {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "probe")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// timeoutErr는 net 패키지의 연결 시간 초과처럼 Timeout()이 참인 오류다.
type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

// dialErr는 net.Dialer가 돌려주는 꼴의 오류다.
func dialErr(errno syscall.Errno) error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: os.NewSyscallError("connect", errno)}
}

func TestClassify(t *testing.T) {
	for _, c := range []struct {
		err           error
		result, class string
	}{
		{nil, resultReachable, "ok"},
		{&fs.PathError{Op: "open", Path: "/x", Err: syscall.ENOENT}, resultBlocked, "ENOENT"},
		{&fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}, resultBlocked, "EACCES"},
		// IPAddressDeny(cgroup BPF)는 connect가 EPERM, RestrictAddressFamilies는 socket이 EAFNOSUPPORT다.
		{dialErr(syscall.EPERM), resultBlocked, "EPERM"},
		{&net.OpError{Op: "dial", Err: os.NewSyscallError("socket", syscall.EAFNOSUPPORT)}, resultBlocked, "EAFNOSUPPORT"},
		// 상대가 거절했다면 패킷은 닿았다.
		{dialErr(syscall.ECONNREFUSED), resultReachable, "ECONNREFUSED"},
		{dialErr(syscall.ENETUNREACH), resultError, "ENETUNREACH"},
		{dialErr(syscall.ETIMEDOUT), resultTimeout, "ETIMEDOUT"},
		{&net.OpError{Op: "dial", Err: timeoutErr{}}, resultTimeout, "timeout"},
		{context.DeadlineExceeded, resultTimeout, "timeout"},
		{syscall.Errno(200), resultError, "errno_200"},
		{errors.New("something else"), resultError, "other"},
	} {
		if r, cl := classify(c.err); r != c.result || cl != c.class {
			t.Errorf("classify(%v) = %s %s, want %s %s", c.err, r, cl, c.result, c.class)
		}
	}
}

func TestReadOne(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	os.WriteFile(file, []byte("secret"), 0o644)
	os.WriteFile(filepath.Join(dir, "empty"), nil, 0o644)
	if err := readOneEntry(dir); err != nil {
		t.Errorf("dir: %v", err)
	}
	if err := readOneEntry(t.TempDir()); err != nil {
		t.Errorf("empty dir: %v", err)
	}
	if err := readOneByte(file); err != nil {
		t.Errorf("file: %v", err)
	}
	if err := readOneByte(filepath.Join(dir, "empty")); err != nil {
		t.Errorf("empty file: %v", err)
	}
	if r, cl := classify(readOneEntry(filepath.Join(dir, "missing"))); r != resultBlocked || cl != "ENOENT" {
		t.Errorf("missing dir: %s %s", r, cl)
	}
	if r, cl := classify(readOneEntry(file)); r != resultBlocked || cl != "ENOTDIR" {
		t.Errorf("file as dir: %s %s", r, cl)
	}
	if os.Geteuid() != 0 {
		locked := filepath.Join(dir, "locked")
		os.Mkdir(locked, 0o000)
		t.Cleanup(func() { os.Chmod(locked, 0o755) })
		if r, cl := classify(readOneEntry(locked)); r != resultBlocked || cl != "EACCES" {
			t.Errorf("mode 000 dir: %s %s", r, cl)
		}
	}
}

func TestAbstractListener(t *testing.T) {
	data := []byte(`Num       RefCount Protocol Flags    Type St Inode Path
0000000000000000: 00000002 00000000 00010000 0001 01 101 /run/example.sock
0000000000000000: 00000002 00000000 00000000 0001 03 102 @connected
0000000000000000: 00000002 00000000 00010000 0002 01 103 @datagram
0000000000000000: 00000002 00000000 00010000 0001 01 104 @/example/listener
`)
	if name, ok := abstractListener(data); !ok || name != "@/example/listener" {
		t.Errorf("abstractListener = %q %v", name, ok)
	}
	if _, ok := abstractListener([]byte("Num       RefCount Protocol Flags    Type St Inode Path\n")); ok {
		t.Error("found a listener in an empty table")
	}
}

func TestValidateInput(t *testing.T) {
	for _, ok := range []probeInput{
		{},
		{Network: []string{"lan", "loopback"}, LANAddress: "192.168.0.1:80"},
		{Network: []string{"lan"}, LANAddress: "10.1.2.3:22"},
		{LANAddress: "[fd00::1]:443"},
		{LANAddress: "169.254.1.1:80"},
	} {
		if err := ok.validate(); err != nil {
			t.Errorf("%+v: %v", ok, err)
		}
	}
	for _, bad := range []probeInput{
		{Network: []string{"internet"}},
		{Network: []string{"lan", "lan"}},
		{LANAddress: "1.1.1.1:443"},
		{LANAddress: "127.0.0.1:80"},
		{LANAddress: "nas.local:80"},
		{LANAddress: "192.168.0.1"},
		{LANAddress: "192.168.0.1:0"},
	} {
		if err := bad.validate(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

// fakeHostSocket는 maru의 호스트 소켓처럼 /v1/rpc에 status와 code로 답하는 임시 유닉스 소켓이다.
func fakeHostSocket(t *testing.T, status int, code string) string {
	t.Helper()
	path := filepath.Join(shortDir(t), "host.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Maru-Call") != "" {
			t.Error("probe sent a call token")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"code": code, "error": "secret host detail"})
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return path
}

// fakeEnv는 실제 경로·네트워크 대신 정한 결과를 준다. 호스트 소켓(host)만 실제 유닉스 소켓으로 연결한다.
func fakeEnv(host string, fsErr error, dial func(network, address string) error, procNetUnix string) env {
	var d net.Dialer
	return env{
		readDir:  func(string) error { return fsErr },
		readFile: func(string) error { return fsErr },
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network == "unix" && address == host {
				return d.DialContext(ctx, network, address)
			}
			if err := dial(network, address); err != nil {
				return nil, err
			}
			c1, c2 := net.Pipe()
			c2.Close()
			return c1, nil
		},
		procNetUnix: func() ([]byte, error) { return []byte(procNetUnix), nil },
		timeout:     time.Second,
	}
}

const procHeader = "Num       RefCount Protocol Flags    Type St Inode Path\n"

func rows(rep report) map[string]row {
	m := map[string]row{}
	for _, r := range rep.Rows {
		m[r.Check] = r
	}
	return m
}

// TestRunSandboxed는 네트워크 없는 유닛(maru-addon@)에서 기대하는 결과다: 모든 행이 막힌다.
func TestRunSandboxed(t *testing.T) {
	host := fakeHostSocket(t, http.StatusUnauthorized, "call_invalid")
	enoent := &fs.PathError{Op: "open", Err: syscall.ENOENT}
	e := fakeEnv(host, enoent, func(network, _ string) error {
		if network == "tcp" {
			return &net.OpError{Op: "dial", Err: os.NewSyscallError("socket", syscall.EAFNOSUPPORT)}
		}
		return &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ENOENT)}
	}, procHeader)
	rep := e.run(context.Background(), probeInput{}, host)
	if rep.Failed != 0 || rep.Passed != len(rep.Rows)-1 {
		t.Errorf("passed %d failed %d of %d: %+v", rep.Passed, rep.Failed, len(rep.Rows), rep.Rows)
	}
	got := rows(rep)
	if r := got["lan"]; r.Result != resultSkipped || r.Verdict != verdictInfo {
		t.Errorf("lan without address: %+v", r)
	}
	if r := got["abstract-unix"]; r.Result != resultBlocked || r.Class != "none_visible" || r.Verdict != verdictPass {
		t.Errorf("abstract: %+v", r)
	}
	if r := got["host-without-token"]; r.Result != resultBlocked || r.Class != "call_invalid" {
		t.Errorf("host: %+v", r)
	}
	if r := got["public"]; r.Result != resultBlocked || r.Class != "EAFNOSUPPORT" {
		t.Errorf("public: %+v", r)
	}
}

// TestRunUnsandboxed는 샌드박스가 없는 개발 서버처럼 모두 닿는 경우다. 네트워크 권한이 허용한 행만 통과한다.
func TestRunUnsandboxed(t *testing.T) {
	host := fakeHostSocket(t, http.StatusOK, "")
	e := fakeEnv(host, nil, func(string, string) error { return nil },
		procHeader+"0000000000000000: 00000002 00000000 00010000 0001 01 104 @/example/listener\n")
	rep := e.run(context.Background(), probeInput{Network: []string{"loopback"}, LANAddress: "192.168.0.10:80"}, host)
	got := rows(rep)
	if r := got["loopback-http"]; r.Expect != resultReachable || r.Verdict != verdictPass {
		t.Errorf("loopback with the loopback permission: %+v", r)
	}
	if r := got["lan"]; r.Expect != resultBlocked || r.Verdict != verdictFail {
		t.Errorf("lan without the lan permission: %+v", r)
	}
	// 네트워크가 있는 단계에서 추상 소켓에 닿는 것은 알려진 한계라 판정하지 않는다.
	if r := got["abstract-unix"]; r.Result != resultReachable || r.Verdict != verdictInfo {
		t.Errorf("abstract: %+v", r)
	}
	if r := got["host-without-token"]; r.Result != resultReachable || r.Class != "http_200" || r.Verdict != verdictFail {
		t.Errorf("host: %+v", r)
	}
	if rep.Passed != 1 || rep.Failed != len(rep.Rows)-2 {
		t.Errorf("passed %d failed %d of %d", rep.Passed, rep.Failed, len(rep.Rows))
	}
}

func TestHostWithoutToken(t *testing.T) {
	e := osEnv()
	e.timeout = time.Second
	if r, cl := e.hostWithoutToken(context.Background(), fakeHostSocket(t, http.StatusUnauthorized, "call_invalid")); r != resultBlocked || cl != "call_invalid" {
		t.Errorf("401 call_invalid: %s %s", r, cl)
	}
	if r, cl := e.hostWithoutToken(context.Background(), fakeHostSocket(t, http.StatusBadRequest, "bad_request")); r != resultError || cl != "http_400" {
		t.Errorf("400: %s %s", r, cl)
	}
	// 호스트 소켓이 없는 것은 샌드박스가 막은 것이 아니다.
	if r, cl := e.hostWithoutToken(context.Background(), filepath.Join(shortDir(t), "missing.sock")); r != resultError || cl != "ENOENT" {
		t.Errorf("missing socket: %s %s", r, cl)
	}
}

// TestHandler는 조작 run을 SDK로 띄워 maru처럼 부른다. 결과에 파일 내용과 호스트 응답 본문이 없어야 한다.
func TestHandler(t *testing.T) {
	dir := shortDir(t)
	secret := filepath.Join(dir, "secret.txt")
	os.WriteFile(secret, []byte("TOP-SECRET-CONTENT"), 0o644)
	host := fakeHostSocket(t, http.StatusUnauthorized, "call_invalid")
	e := fakeEnv(host, nil, func(string, string) error { return dialErr(syscall.EPERM) }, procHeader)
	// 파일 행은 실제로 읽되 임시 파일만 읽는다.
	e.readFile = func(string) error { return readOneByte(secret) }

	t.Setenv("MARU_ADDON_HOST", host)
	l, err := net.Listen("unix", filepath.Join(dir, "backend.sock"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- maruaddon.ServeContext(ctx, l, newHandler(e, host)) }()
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dir, "backend.sock"))
	}}, Timeout: 10 * time.Second}
	t.Cleanup(func() {
		client.CloseIdleConnections()
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	post := func(admin, body string) (int, string) {
		req, _ := http.NewRequest(http.MethodPost, "http://addon/v1/actions/run", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Maru-User", "admin")
		req.Header.Set("X-Maru-Admin", admin)
		req.Header.Set("X-Maru-Call", "fake-token")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(data)
	}

	if status, body := post("0", `{"target":"","input":null}`); status != http.StatusUnprocessableEntity || !strings.Contains(body, "admin_required") {
		t.Errorf("non-admin: %d %s", status, body)
	}
	if status, body := post("1", `{"target":"","input":{"lanAddress":"8.8.8.8:53"}}`); status != http.StatusBadRequest || !strings.Contains(body, "bad_input") {
		t.Errorf("public LAN address: %d %s", status, body)
	}
	status, body := post("1", `{"target":"","input":{"network":["lan"]}}`)
	if status != http.StatusOK {
		t.Fatalf("run: %d %s", status, body)
	}
	if strings.Contains(body, "TOP-SECRET") || strings.Contains(body, "secret host detail") {
		t.Errorf("result leaks contents: %s", body)
	}
	var rep report
	if err := json.Unmarshal([]byte(body), &rep); err != nil {
		t.Fatal(err)
	}
	if got := rows(rep); got["state-site"].Result != resultReachable || got["state-site"].Verdict != verdictFail ||
		got["public"].Class != "EPERM" || len(rep.Network) != 1 {
		t.Errorf("report = %+v", rep)
	}
}
