package daemon

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strconv"
	"syscall"
)

// peerUID는 Unix 소켓 상대 프로세스의 uid(SO_PEERCRED)다. 커널이 connect 때 기록한 값이라 위조할 수 없다.
func peerUID(c *net.UnixConn) (int, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return -1, err
	}
	var cred *syscall.Ucred
	var serr error
	if err := raw.Control(func(fd uintptr) {
		cred, serr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return -1, err
	}
	if serr != nil {
		return -1, serr
	}
	return int(cred.Uid), nil
}

// ActivationListener는 systemd 소켓 활성화로 받은 수신 소켓이다(sd_listen_fds). 소켓 활성화가
// 아니면 nil, nil이다. 자식(worker)에 넘어가지 않게 LISTEN_* 환경 변수를 지운다.
func ActivationListener() (*net.UnixListener, error) {
	pid, fds := os.Getenv("LISTEN_PID"), os.Getenv("LISTEN_FDS")
	os.Unsetenv("LISTEN_PID")
	os.Unsetenv("LISTEN_FDS")
	os.Unsetenv("LISTEN_FDNAMES")
	if pid == "" || pid != strconv.Itoa(os.Getpid()) {
		return nil, nil
	}
	if fds != "1" {
		return nil, fmt.Errorf("expected exactly one socket from systemd, got LISTEN_FDS=%q", fds)
	}
	const fd = 3
	syscall.CloseOnExec(fd)
	f := os.NewFile(fd, "maru-helper.sock")
	l, err := net.FileListener(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	ul, ok := l.(*net.UnixListener)
	if !ok {
		l.Close()
		return nil, errors.New("systemd socket is not a Unix stream socket")
	}
	return ul, nil
}

// ListenUnix는 path에 Unix 소켓을 만든다(소켓 활성화 없이 띄울 때). 권한은 0600이며, 남은 소켓
// 파일은 지우되 소켓이 아닌 파일이나 사용 중인 소켓은 건드리지 않는다.
func ListenUnix(path string) (*net.UnixListener, error) {
	if st, err := os.Lstat(path); err == nil {
		if st.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		if c, err := net.Dial("unix", path); err == nil {
			c.Close()
			return nil, fmt.Errorf("%s is in use", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// 만든 직후의 권한(umask 022면 0755)으로도 다른 사용자는 connect할 수 없다(쓰기 권한 필요).
	if err := os.Chmod(path, 0o600); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}
