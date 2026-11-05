package server

import (
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"maru/backend/config"
	"maru/backend/internal/addonpkg"
	"maru/backend/internal/api"
)

// 운영 호스트 소켓의 연결한 uid 확인(BackendPaths.HostPeer)과 패키지별 디렉터리 시험. 시험은 자기 uid로 붙으므로
// 운영 술어(InstalledHostPeer) 대신 자기 uid를 받거나 거절하는 술어를 끼운다.

// peerEnv는 개발 패키지 probe의 호스트 소켓을 술어 allow로 연 환경이다.
func peerEnv(t *testing.T, allow func(uid int) bool) (*env, string) {
	t.Helper()
	paths := backendPaths(shortSocketDir(t))
	paths.HostPeer = allow
	e := newEnv(t, envOptions{addonHost: &paths, devAddons: []string{devPackage(t, probeManifest("probe"))}})
	return e, paths.Host("probe")
}

// closedByServer는 연결이 응답 없이 닫히는지다(요청을 보내고 EOF를 기다린다).
func closedByServer(t *testing.T, c net.Conn) bool {
	t.Helper()
	io.WriteString(c, "POST /v1/rpc HTTP/1.1\r\nHost: maru\r\nContent-Length: 0\r\n\r\n")
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := c.Read(make([]byte, 64))
	return n == 0 && (errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || isReset(err))
}

func isReset(err error) bool {
	return err != nil && strings.Contains(err.Error(), "connection reset")
}

func TestHostSocketPeerReject(t *testing.T) {
	e, path := peerEnv(t, func(uid int) bool { return uid != os.Getuid() })
	c, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if !closedByServer(t, c) {
		t.Fatal("connection from a rejected uid was served")
	}
	if !strings.Contains(e.logs.String(), "closed a connection from uid") {
		t.Errorf("rejection not logged: %s", e.logs.String())
	}
}

func TestHostSocketPeerAccept(t *testing.T) {
	_, path := peerEnv(t, func(uid int) bool { return uid == os.Getuid() })
	res, _, _ := rawHostRequest(t, path, "POST /v1/rpc HTTP/1.1\r\nHost: maru\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}", 2*time.Second)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("accepted peer: status %d", res.StatusCode)
	}
}

// 거절한 연결은 동시 연결 칸을 쓰지 않는다: 칸보다 많은 연결을 거절한 뒤(클라이언트는 닫지 않고 둔다) 받는 연결이
// 바로 처리된다.
func TestHostSocketPeerRejectsWithoutSlots(t *testing.T) {
	var allow atomic.Bool
	_, path := peerEnv(t, func(uid int) bool { return allow.Load() && uid == os.Getuid() })
	for i := 0; i < hostMaxConns+4; i++ {
		c, err := net.Dial("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		if !closedByServer(t, c) {
			t.Fatalf("rejected connection %d was served", i)
		}
	}
	allow.Store(true)
	res, _, _ := rawHostRequest(t, path, "POST /v1/rpc HTTP/1.1\r\nHost: maru\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}", 2*time.Second)
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("after rejections: status %d", res.StatusCode)
	}
}

// 운영 경로처럼 <dir>/<id>/host.sock이면 maru는 패키지별 디렉터리를 만들지 않고(백엔드의 소켓 유닛이 root 소유 상위
// 디렉터리 안에 만든다) 그 안에 소켓만 연다. 디렉터리가 생기기 전에는 열지 않고 그 백엔드로 요청을 보낼 때 다시 본다.
// 소켓을 닫아도(끄기, 제거, 서버 닫기) 디렉터리는 남고, 다시 열 때 같은 디렉터리에 연다.
func TestHostSocketDirectoryFromUnit(t *testing.T) {
	dir := shortSocketDir(t)
	paths := addonpkg.BackendPaths{
		Backend:         func(id string) string { return filepath.Join(dir, id+".b") },
		Host:            func(id string) string { return filepath.Join(dir, id, config.AddonHostSocketName) },
		HostDirFromUnit: true,
		HostMode:        0o666,
	}
	e := newEnv(t, envOptions{packages: true, addonHost: &paths})
	admin := e.adminWithManage()
	admin.installAddon(backendArchive(t))
	idDir := filepath.Join(dir, "hello")
	if _, err := os.Lstat(idDir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("maru created the package directory: %v", err)
	}
	if _, ok := e.srv.h.hosts.socketPath("hello"); ok {
		t.Fatal("host socket opened without its directory")
	}
	// 호스트 소켓을 열지 못하면 백엔드로 보내지 않는다. 같은 이유는 한 번만 로그에 남긴다.
	for range 2 {
		got := wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable)
		if got.Error != errBackendNotReady {
			t.Errorf("error = %q", got.Error)
		}
	}
	if n := strings.Count(e.logs.String(), "does not exist yet"); n != 1 {
		t.Errorf("missing directory logged %d times", n)
	}

	// 유닛이 디렉터리를 만들면 다음 요청이 소켓을 연다(백엔드가 없어 연결은 실패한다).
	if err := os.Mkdir(idDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := wantCode(t, admin.do(http.MethodGet, "/api/addons/hello", nil), http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable); got.Error == errBackendNotReady {
		t.Error("request not sent after the directory appeared")
	}
	check := func(stage string, socket bool) {
		t.Helper()
		fi, err := os.Stat(idDir)
		if err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o755 {
			t.Errorf("%s: package directory = %v %v", stage, fi, err)
		}
		_, err = os.Stat(paths.Host("hello"))
		if (err == nil) != socket {
			t.Errorf("%s: socket exists = %v", stage, err == nil)
		}
	}
	check("directory created", true)
	if fi, err := os.Stat(paths.Host("hello")); err != nil || fi.Mode().Perm() != 0o666 {
		t.Errorf("socket mode = %v %v", fi, err)
	}
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: false}), http.StatusOK)
	check("disabled", false)
	wantStatus(t, admin.do(http.MethodPut, "/api/site/addons/hello", api.AddonEnabledRequest{Enabled: true}), http.StatusOK)
	check("enabled again", true)
	wantStatus(t, admin.do(http.MethodDelete, "/api/site/addons/hello", nil), http.StatusNoContent)
	check("uninstalled", false)
	admin.installAddon(backendArchive(t))
	check("reinstalled", true)
	e.srv.Close()
	check("server closed", false)
}

// 패키지별 디렉터리 자리에 디렉터리가 아닌 것(심볼릭 링크)이 있으면 열지 않는다.
func TestHostSocketDirectoryNotDirectory(t *testing.T) {
	dir := shortSocketDir(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "hello")); err != nil {
		t.Fatal(err)
	}
	paths := addonpkg.BackendPaths{
		Backend:         func(id string) string { return filepath.Join(dir, id+".b") },
		Host:            func(id string) string { return filepath.Join(dir, id, config.AddonHostSocketName) },
		HostDirFromUnit: true,
		HostMode:        0o666,
	}
	e := newEnv(t, envOptions{packages: true, addonHost: &paths})
	e.adminWithManage().installAddon(backendArchive(t))
	if _, ok := e.srv.h.hosts.socketPath("hello"); ok {
		t.Error("host socket opened through a symbolic link")
	}
	if !strings.Contains(e.logs.String(), "is not a directory") {
		t.Errorf("not logged: %s", e.logs.String())
	}
}
