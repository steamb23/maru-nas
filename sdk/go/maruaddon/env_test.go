// SPDX-License-Identifier: MIT

package maruaddon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeEnviron은 맵 환경과 정한 pid, 기록하는 fd 수신기다.
type fakeEnviron struct {
	env      map[string]string
	pid      int
	fds      []uintptr
	listener net.Listener
}

func (f *fakeEnviron) environ() environ {
	return environ{
		getenv:   func(k string) string { return f.env[k] },
		unsetenv: func(k string) error { delete(f.env, k); return nil },
		getpid:   func() int { return f.pid },
		fileListener: func(fd uintptr) (net.Listener, error) {
			f.fds = append(f.fds, fd)
			return f.listener, nil
		},
	}
}

func TestListenSocketActivation(t *testing.T) {
	l, err := net.Listen("unix", filepath.Join(shortTempDir(t), "a.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fe := &fakeEnviron{pid: 4242, listener: l, env: map[string]string{
		"LISTEN_PID": "4242", "LISTEN_FDS": "1", "LISTEN_FDNAMES": "maru-addon@x.socket",
		// 소켓 활성화가 MARU_ADDON_SOCKET보다 먼저다
		"MARU_ADDON_SOCKET": filepath.Join(shortTempDir(t), "dev.sock"),
		"OTHER":             "kept",
	}}
	got, err := listen(fe.environ())
	if err != nil {
		t.Fatal(err)
	}
	if got != l || len(fe.fds) != 1 || fe.fds[0] != 3 {
		t.Errorf("listener %v, fds %v; want fd 3", got, fe.fds)
	}
	for _, k := range []string{"LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES"} {
		if _, ok := fe.env[k]; ok {
			t.Errorf("%s not removed from the environment", k)
		}
	}
	if fe.env["OTHER"] != "kept" || fe.env["MARU_ADDON_SOCKET"] == "" {
		t.Errorf("unrelated variables changed: %v", fe.env)
	}
}

func TestListenActivationOtherProcess(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "dev.sock")
	for _, pid := range []string{"1", "", "not-a-pid"} {
		fe := &fakeEnviron{pid: 4242, env: map[string]string{
			"LISTEN_PID": pid, "LISTEN_FDS": "1", "MARU_ADDON_SOCKET": path,
		}}
		l, err := listen(fe.environ())
		if err != nil {
			t.Fatalf("LISTEN_PID %q: %v", pid, err)
		}
		if len(fe.fds) != 0 {
			t.Errorf("LISTEN_PID %q: used systemd fds", pid)
		}
		if l.Addr().String() != path {
			t.Errorf("listening on %s, want %s", l.Addr(), path)
		}
		l.Close()
	}
}

func TestListenActivationFDCount(t *testing.T) {
	for _, n := range []string{"0", "2", "", "x"} {
		fe := &fakeEnviron{pid: 4242, env: map[string]string{
			"LISTEN_PID": "4242", "LISTEN_FDS": n, "MARU_ADDON_SOCKET": "/nonexistent/dev.sock",
		}}
		if _, err := listen(fe.environ()); err == nil || !strings.Contains(err.Error(), "LISTEN_FDS") {
			t.Errorf("LISTEN_FDS %q: err = %v", n, err)
		}
		if len(fe.fds) != 0 {
			t.Errorf("LISTEN_FDS %q: used fds %v", n, fe.fds)
		}
	}
}

func TestListenNoSocket(t *testing.T) {
	fe := &fakeEnviron{pid: 4242, env: map[string]string{}}
	if _, err := listen(fe.environ()); err == nil || !strings.Contains(err.Error(), "MARU_ADDON_SOCKET") {
		t.Errorf("err = %v", err)
	}
}

func TestListenUnix(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "dev.sock")
	l, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want a 0600 socket", fi.Mode())
	}
	l.Close()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket file not removed on close: %v", err)
	}
}

func TestListenUnixStale(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "dev.sock")
	old, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	// 지난 실행이 남긴 소켓 파일(받는 프로세스 없음)
	old.(*net.UnixListener).SetUnlinkOnClose(false)
	old.Close()
	if _, err := os.Lstat(path); err != nil {
		t.Fatal(err)
	}
	l, err := listenUnix(path)
	if err != nil {
		t.Fatalf("stale socket not replaced: %v", err)
	}
	defer l.Close()
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestListenUnixInUse(t *testing.T) {
	path := filepath.Join(shortTempDir(t), "dev.sock")
	live, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	go func() {
		for {
			c, err := live.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	if _, err := listenUnix(path); err == nil || !strings.Contains(err.Error(), "another process") {
		t.Fatalf("err = %v", err)
	}
	// 살아 있는 소켓은 그대로다
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("live socket broken: %v", err)
	}
	c.Close()
}

func TestListenUnixNotSocket(t *testing.T) {
	dir := shortTempDir(t)
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{file, sub, link} {
		if _, err := listenUnix(p); err == nil || !strings.Contains(err.Error(), "not a socket") {
			t.Errorf("%s: err = %v", p, err)
		}
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
		t.Errorf("file changed: %q, %v", data, err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink changed: %v", err)
	}
	if fi, err := os.Stat(sub); err != nil || !fi.IsDir() {
		t.Errorf("directory changed: %v", err)
	}
}

func TestListenUnixBadPath(t *testing.T) {
	dir := shortTempDir(t)
	// 디렉터리는 만들지 않는다
	if _, err := listenUnix(filepath.Join(dir, "missing", "dev.sock")); err == nil {
		t.Error("missing directory: no error")
	}
	long := "/tmp/" + strings.Repeat("a", maxSocketPath)
	if _, err := listenUnix(long); err == nil || !strings.Contains(err.Error(), "107 bytes") {
		t.Errorf("long path: err = %v", err)
	}
}

func TestStateDir(t *testing.T) {
	cases := []struct {
		stateDirectory, maruState, want string
	}{
		{"/var/lib/maru-addon/x", "/dev/state", "/var/lib/maru-addon/x"},
		{"/var/lib/a:/var/lib/b", "", "/var/lib/a"},
		{"", "/dev/state", "/dev/state"},
		{":/var/lib/b", "/dev/state", "/dev/state"},
		{"", "", ""},
	}
	for _, c := range cases {
		env := map[string]string{"STATE_DIRECTORY": c.stateDirectory, "MARU_ADDON_STATE": c.maruState}
		if got := stateDir(func(k string) string { return env[k] }); got != c.want {
			t.Errorf("%+v: got %q", c, got)
		}
	}
	t.Setenv("STATE_DIRECTORY", "")
	t.Setenv("MARU_ADDON_STATE", "/from/env")
	if got := StateDir(); got != "/from/env" {
		t.Errorf("StateDir() = %q", got)
	}
}

// 자식 프로세스 시험의 환경 변수. childEnv가 "1"이면 이 시험 실행 파일이 자식으로 돈다. childModeEnv는 자식의
// 데이터 처리기가 할 일이다(비면 환경을 돌려주고, "block"이면 표시를 출력한 뒤 문맥이 끝날 때까지 멈춘다).
const (
	childEnv     = "MARUADDON_TEST_SOCKET_ACTIVATION_CHILD"
	childModeEnv = "MARUADDON_TEST_CHILD_MODE"
)

// childStarted는 "block" 자식의 처리기가 요청을 받았을 때 출력하는 표시다.
const childStarted = "maruaddon-test: handler started"

// activatedChild는 소켓 활성화로 띄운 자식 프로세스(이 시험 실행 파일)다.
type activatedChild struct {
	path    string // systemd(여기서는 시험)가 만든 소켓 파일
	cmd     *exec.Cmd
	out     *syncBuffer
	exited  chan struct{} // 자식이 끝나면 닫히고, 그때 waitErr에 결과가 있다
	waitErr error
}

// startActivatedChild는 받는 소켓을 fd 3으로 넘겨 자식을 띄운다. 시험이 끝날 때 자식이 남아 있으면 죽인다.
func startActivatedChild(t *testing.T, mode string) *activatedChild {
	t.Helper()
	path := filepath.Join(shortTempDir(t), "act.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ul := l.(*net.UnixListener)
	f, err := ul.File()
	if err != nil {
		t.Fatal(err)
	}
	// 부모는 받지 않는다. 소켓 파일은 systemd(여기서는 시험)가 지운다
	ul.SetUnlinkOnClose(false)
	ul.Close()
	defer f.Close()

	cmd := exec.Command(os.Args[0], "-test.run=^TestSocketActivationChild$")
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "LISTEN_") && !strings.HasPrefix(kv, "MARU_ADDON_") &&
			!strings.HasPrefix(kv, childModeEnv+"=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, childEnv+"=1", childModeEnv+"="+mode, "LISTEN_FDS=1", "LISTEN_FDNAMES=act.socket")
	cmd.ExtraFiles = []*os.File{f} // 자식의 fd 3
	c := &activatedChild{path: path, cmd: cmd, out: &syncBuffer{}, exited: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = c.out, c.out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		c.waitErr = cmd.Wait()
		close(c.exited)
	}()
	t.Cleanup(func() {
		select {
		case <-c.exited:
		default:
			cmd.Process.Kill()
			<-c.exited
		}
	})
	return c
}

// get은 자식에 데이터 요청 하나를 보낸다. 자식이 아직 받지 못하면 deadline까지 다시 시도한다.
func (c *activatedChild) get(t *testing.T, client *http.Client) (int, []byte) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		req, _ := http.NewRequest("GET", "http://addon/v1/data", nil)
		req.Header = defaultHeaders()
		resp, err := client.Do(req)
		if err == nil {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatalf("read response: %v", err)
			}
			return resp.StatusCode, body
		}
		if time.Now().After(deadline) {
			t.Fatalf("child never answered: %v\n%s", err, c.out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestSocketActivationProcess는 systemd 소켓 활성화를 흉내 낸다. 받는 소켓을 fd 3으로 넘긴 자식 프로세스(이 시험
// 실행 파일)가 Serve로 답하고, SIGTERM에 0으로 끝나며, systemd의 소켓 파일을 지우지 않는지 본다.
func TestSocketActivationProcess(t *testing.T) {
	if os.Getenv(childEnv) == "1" {
		t.Skip("running as the child")
	}
	c := startActivatedChild(t, "")
	client := unixClient(c.path)
	defer client.CloseIdleConnections()
	status, body := c.get(t, client)
	if status != 200 {
		t.Fatalf("status = %d, body %s", status, body)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	// 자식은 소켓 활성화 변수를 지웠다
	if got["LISTEN_PID"] != "" || got["LISTEN_FDS"] != "" || got["LISTEN_FDNAMES"] != "" || got["pid"] == "" {
		t.Errorf("child environment = %v", got)
	}
	client.CloseIdleConnections()

	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.exited:
		if c.waitErr != nil {
			t.Fatalf("child exited with %v\n%s", c.waitErr, c.out.String())
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("child did not stop on SIGTERM\n%s", c.out.String())
	}
	if !strings.Contains(c.out.String(), "PASS") {
		t.Errorf("child did not pass:\n%s", c.out.String())
	}
	if _, err := os.Lstat(c.path); err != nil {
		t.Errorf("socket-activated socket file was removed: %v", err)
	}
}

// TestSecondSignalExits는 끝내기를 기다리는 동안(처리기가 돌고 있음) 첫 SIGTERM에는 자식이 남아 있고, 두 번째
// SIGTERM에는 신호의 기본 동작대로 바로 끝나는지 본다(개발 중 두 번째 Ctrl+C).
func TestSecondSignalExits(t *testing.T) {
	if os.Getenv(childEnv) == "1" {
		t.Skip("running as the child")
	}
	c := startActivatedChild(t, "block")
	client := unixClient(c.path)
	defer client.CloseIdleConnections()
	go func() {
		req, _ := http.NewRequest("GET", "http://addon/v1/data", nil)
		req.Header = defaultHeaders()
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	// 처리기가 요청을 받을 때까지 기다린다. 소켓은 부모가 만들어 이미 받고 있으므로 연결은 자식이 Accept할 때까지
	// 대기열에 있다
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(c.out.String(), childStarted) {
		select {
		case <-c.exited:
			if strings.Contains(c.out.String(), "SIGTERM ignored") {
				t.Skip("SIGTERM is ignored in this environment, so its default action cannot be tested")
			}
			t.Fatalf("child exited early: %v\n%s", c.waitErr, c.out.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("handler never started\n%s", c.out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	// 첫 신호: 처리기가 돌고 있으므로 끝내기를 기다린다
	select {
	case <-c.exited:
		t.Fatalf("child exited on the first SIGTERM with a request in flight: %v\n%s", c.waitErr, c.out.String())
	case <-time.After(300 * time.Millisecond):
	}
	// 두 번째 신호: 신호 등록을 푸는 것은 비동기이므로 끝날 때까지 몇 번 보낸다
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(10 * time.Second)
	for done := false; !done; {
		if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Fatal(err)
		}
		select {
		case <-c.exited:
			done = true
		case <-tick.C:
		case <-timeout:
			t.Fatalf("child did not exit on a second SIGTERM\n%s", c.out.String())
		}
	}
	var ee *exec.ExitError
	if !errors.As(c.waitErr, &ee) {
		t.Fatalf("child exited with %v, want killed by SIGTERM\n%s", c.waitErr, c.out.String())
	}
	if ws, ok := ee.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
		t.Errorf("child exit = %v, want killed by SIGTERM\n%s", ee, c.out.String())
	}
}

// TestSocketActivationChild는 TestSocketActivationProcess·TestSecondSignalExits가 띄우는 자식 프로세스에서만 돈다.
func TestSocketActivationChild(t *testing.T) {
	if os.Getenv(childEnv) != "1" {
		t.Skip("run by TestSocketActivationProcess")
	}
	if os.Getenv(childModeEnv) == "block" && signal.Ignored(syscall.SIGTERM) {
		// 처음부터 무시된 신호는 등록을 풀면 다시 무시되므로 기본 동작을 시험할 수 없다
		fmt.Println("SIGTERM ignored")
		return
	}
	// systemd는 fork 뒤 exec 전에 LISTEN_PID를 자식 pid로 둔다. os/exec로는 그럴 수 없어 자식이 스스로 둔다
	os.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
	data := func(ctx context.Context, r *Request) (any, error) {
		return map[string]string{
			"pid":            strconv.Itoa(os.Getpid()),
			"LISTEN_PID":     os.Getenv("LISTEN_PID"),
			"LISTEN_FDS":     os.Getenv("LISTEN_FDS"),
			"LISTEN_FDNAMES": os.Getenv("LISTEN_FDNAMES"),
		}, nil
	}
	if os.Getenv(childModeEnv) == "block" {
		data = func(ctx context.Context, r *Request) (any, error) {
			fmt.Println(childStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	if err := Serve(Handler{Data: data}); err != nil {
		t.Fatal(err)
	}
}
