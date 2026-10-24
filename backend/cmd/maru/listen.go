package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strings"

	"maru/backend/config"
)

// parseListen은 -listen 값을 확인하고 net.Listen 인자로 바꾼다. 검사 규칙은 config.CheckListen이다.
func parseListen(addr string, allowNonLoopback bool) (network, address string, err error) {
	if err := config.CheckListen(addr, allowNonLoopback); err != nil {
		return "", "", err
	}
	if path, ok := strings.CutPrefix(addr, config.UnixPrefix); ok {
		return "unix", path, nil
	}
	return "tcp", addr, nil
}

// listenOn은 수신을 시작한다. Unix 소켓이면 이전 실행이 남긴 소켓 파일을 지우고
// 권한을 0660(소유자·그룹만 접속)으로 둔다. 종료할 때 소켓 파일은 리스너가 지운다.
func listenOn(network, address string) (net.Listener, error) {
	if network != "unix" {
		return net.Listen(network, address)
	}
	if fi, err := os.Lstat(address); err == nil {
		// 소켓이 아닌 파일은 경로 실수일 수 있으므로 지우지 않는다.
		if fi.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket", address)
		}
		// 다른 인스턴스가 쓰고 있는 소켓은 빼앗지 않는다.
		if c, err := net.Dial("unix", address); err == nil {
			c.Close()
			return nil, fmt.Errorf("%s is in use by another process", address)
		}
		if err := os.Remove(address); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", address)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(address, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}
