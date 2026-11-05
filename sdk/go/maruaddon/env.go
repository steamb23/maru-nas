// SPDX-License-Identifier: MIT

package maruaddon

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// 백엔드 프로세스의 환경 변수(.claude/docs/addon-packages.md의 "호스트 소켓").
const (
	envListenPID     = "LISTEN_PID"
	envListenFDs     = "LISTEN_FDS"
	envListenFDNames = "LISTEN_FDNAMES"
	envSocket        = "MARU_ADDON_SOCKET"
	envHost          = "MARU_ADDON_HOST"
	envState         = "MARU_ADDON_STATE"
	envStateDir      = "STATE_DIRECTORY"
)

// listenFDsStart는 systemd가 넘기는 첫 파일 기술자다(sd_listen_fds의 SD_LISTEN_FDS_START).
const listenFDsStart = 3

// maxSocketPath는 리눅스 sockaddr_un의 sun_path(108바이트)에서 끝의 NUL을 뺀 길이다. Go는 이보다 긴 경로를
// EINVAL로만 거부하므로 먼저 알아보기 쉬운 오류로 막는다.
const maxSocketPath = 107

// StateDir returns the directory for the backend's own persistent data, or "" if there is none.
//
// Installed backends get a private directory from systemd (StateDirectory=, in STATE_DIRECTORY; the first entry
// if there are several). In development it is MARU_ADDON_STATE, which is optional. The directory is shared by all
// dashboard users, and an administrator may delete it when removing the package.
func StateDir() string {
	return stateDir(os.Getenv)
}

func stateDir(getenv func(string) string) string {
	if v := getenv(envStateDir); v != "" {
		if first, _, _ := strings.Cut(v, ":"); first != "" {
			return first
		}
	}
	return getenv(envState)
}

// DefaultHostSocket is the host socket path inside an installed backend's systemd unit. In development the path
// comes from MARU_ADDON_HOST.
const DefaultHostSocket = "/run/maru-host/host.sock"

func hostSocketPath(getenv func(string) string) string {
	if v := getenv(envHost); v != "" {
		return v
	}
	return DefaultHostSocket
}

// environ은 수신 소켓을 고르는 데 쓰는 프로세스 환경이다. 시험에서 바꿔 끼운다.
type environ struct {
	getenv       func(string) string
	unsetenv     func(string) error
	getpid       func() int
	fileListener func(fd uintptr) (net.Listener, error)
}

func osEnviron() environ {
	return environ{getenv: os.Getenv, unsetenv: os.Unsetenv, getpid: os.Getpid, fileListener: fdListener}
}

// fdListener는 넘겨받은 파일 기술자로 수신한다. net.FileListener가 close-on-exec로 복제하므로 원래 기술자는 닫는다.
// 소켓 파일은 systemd가 지우므로(RemoveOnStop) 닫을 때 지우지 않는다(FileListener의 기본값).
func fdListener(fd uintptr) (net.Listener, error) {
	f := os.NewFile(fd, "systemd-socket")
	if f == nil {
		return nil, fmt.Errorf("maruaddon: file descriptor %d is not valid", fd)
	}
	defer f.Close()
	l, err := net.FileListener(f)
	if err != nil {
		return nil, fmt.Errorf("maruaddon: file descriptor %d from systemd: %w", fd, err)
	}
	return l, nil
}

// listen은 소켓 활성화 → MARU_ADDON_SOCKET 순서로 수신 소켓을 고른다.
func listen(e environ) (net.Listener, error) {
	// LISTEN_PID가 이 프로세스일 때만 소켓 활성화로 본다(물려받은 환경 변수는 무시한다)
	if pid, err := strconv.Atoi(e.getenv(envListenPID)); err == nil && pid == e.getpid() {
		if n := e.getenv(envListenFDs); n != "1" {
			return nil, fmt.Errorf("maruaddon: systemd passed LISTEN_FDS=%q, want exactly one socket", n)
		}
		l, err := e.fileListener(listenFDsStart)
		if err != nil {
			return nil, err
		}
		// 백엔드가 띄우는 자식 프로세스가 자기 것으로 알지 않게 지운다(sd_listen_fds의 unset_environment)
		for _, k := range []string{envListenPID, envListenFDs, envListenFDNames} {
			e.unsetenv(k)
		}
		return l, nil
	}
	if path := e.getenv(envSocket); path != "" {
		return listenUnix(path)
	}
	return nil, errors.New("maruaddon: no socket to listen on: not started by systemd socket activation " +
		"and " + envSocket + " is not set")
}

// listenUnix는 개발용 소켓 경로에서 수신한다. 남은 소켓 파일은 아무도 받고 있지 않을 때만 지우고, 소켓이 아닌
// 파일은 건드리지 않는다. 디렉터리는 만들지 않는다.
func listenUnix(path string) (net.Listener, error) {
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("maruaddon: %s: socket path is %d bytes, Unix socket paths are limited to %d bytes",
			envSocket, len(path), maxSocketPath)
	}
	fi, err := os.Lstat(path)
	switch {
	case err == nil:
		if fi.Mode()&fs.ModeSocket == 0 {
			return nil, fmt.Errorf("maruaddon: %s: %s exists and is not a socket", envSocket, path)
		}
		c, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			c.Close()
			return nil, fmt.Errorf("maruaddon: %s: another process is listening on %s", envSocket, path)
		}
		// 받는 프로세스가 없다는 것(ECONNREFUSED)이 확실할 때만 지운다
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("maruaddon: %s: %s: %w", envSocket, path, err)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("maruaddon: %s: remove stale socket: %w", envSocket, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("maruaddon: %s: %w", envSocket, err)
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("maruaddon: %s: %w", envSocket, err)
	}
	ul := l.(*net.UnixListener)
	// 닫을 때 소켓 파일을 지운다(net.Listen의 기본값이지만 뜻을 드러낸다)
	ul.SetUnlinkOnClose(true)
	if err := os.Chmod(path, 0o600); err != nil {
		ul.Close()
		return nil, fmt.Errorf("maruaddon: %s: %w", envSocket, err)
	}
	return ul, nil
}
