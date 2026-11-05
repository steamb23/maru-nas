// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"
)

// 시험 하나의 결과(row.Result).
const (
	// resultReachable는 닿았다는 뜻이다: 파일·디렉터리를 열었다, 소켓에 연결했다, TCP 연결이 맺어지거나 상대가 거절했다
	// (ECONNREFUSED는 패킷이 상대에게 닿았다는 뜻이라 막힌 것이 아니다).
	resultReachable = "reachable"
	// resultBlocked는 막혔다는 뜻이다: 경로가 없거나(ENOENT, 숨긴 /run 등) 권한이 없다(EACCES, InaccessiblePaths),
	// 주소 계열이 막혔다(EAFNOSUPPORT, RestrictAddressFamilies), IP가 막혔다(EPERM, IPAddressDeny), 호스트 소켓이
	// 토큰 없는 호출을 거절했다(401 call_invalid).
	resultBlocked = "blocked"
	// resultTimeout은 제한 시간 안에 답이 없었다는 뜻이다(방화벽이 버림, 없는 주소). 샌드박스가 막은 것과 구별한다.
	resultTimeout = "timeout"
	// resultError는 샌드박스와 상관없는 실패다(ENETUNREACH 같은 네트워크 오류, 호스트 소켓이 없음).
	resultError = "error"
	// resultSkipped는 시도하지 않았다는 뜻이다(LAN 주소를 주지 않음).
	resultSkipped = "skipped"
)

// 판정(row.Verdict).
const (
	verdictPass = "pass"
	verdictFail = "fail"
	// verdictInfo는 판정하지 않는 행이다(시도하지 않음, 알려진 한계).
	verdictInfo = "info"
)

// 시험 종류(check.Kind).
const (
	kindDir      = "dir"      // 디렉터리를 열고 항목 하나를 읽는다
	kindFile     = "file"     // 파일을 열고 1바이트를 읽는다
	kindUnix     = "unix"     // 유닉스 소켓 경로에 연결한다
	kindAbstract = "abstract" // 보이는 추상 유닉스 소켓 하나에 연결한다
	kindTCP      = "tcp"      // TCP 연결
	kindHost     = "host"     // 호스트 소켓을 토큰 없이 부른다
)

// 네트워크 권한(manifest의 backend.network).
const (
	networkLAN      = "lan"
	networkLoopback = "loopback"
)

// check는 시도 하나다. 결과에는 닿았는지와 오류 종류만 담고 내용(파일, 목록, 소켓 이름, 응답 본문)은 담지 않는다.
type check struct {
	ID     string
	Kind   string
	Target string
	// Allowed는 이 행이 닿아도 되는 네트워크 권한이다. 비면 늘 막혀야 한다.
	Allowed string
	// Known이면 그 네트워크 권한에서 닿는 것이 알려진 한계라 판정하지 않는다(추상 유닉스 소켓).
	Known bool
}

// checks는 차례대로 시도할 목록이다. 경로·주소는 설치한 maru(상태 디렉터리 /var/lib/maru, 도우미 소켓
// /run/maru-helper/helper.sock)와 OMV·데비안의 기본값이다.
func checks(lanAddress, hostSocket string) []check {
	lan := check{ID: "lan", Kind: kindTCP, Target: lanAddress, Allowed: networkLAN}
	return []check{
		// maru 상태 디렉터리: 사용자 설정과 보안 수단 기록(TOTP 비밀). 유닛은 /var/lib/maru를 빈 tmpfs로 덮고 자기
		// 패키지 디렉터리만 다시 보이므로 /var/lib/maru 자체가 아니라 그 안의 항목을 본다.
		{ID: "state-users", Kind: kindDir, Target: "/var/lib/maru/users"},
		{ID: "state-security", Kind: kindDir, Target: "/var/lib/maru/security"},
		{ID: "state-site", Kind: kindFile, Target: "/var/lib/maru/site.json"},
		// root 동기화의 기록 디렉터리(InaccessiblePaths)
		{ID: "addon-sync", Kind: kindDir, Target: "/var/lib/maru-addons"},
		// /run은 유닛 안에서 빈 tmpfs다(호스트 소켓 디렉터리만 /run/maru-host로 보인다). 다른 애드온의 백엔드 소켓
		// (/run/maru-addon)과 호스트 소켓 디렉터리(/run/maru-addon-host)도 보이지 않아야 한다.
		{ID: "maru-runtime", Kind: kindDir, Target: "/run/maru"},
		{ID: "addon-sockets", Kind: kindDir, Target: "/run/maru-addon"},
		{ID: "addon-hosts", Kind: kindDir, Target: "/run/maru-addon-host"},
		{ID: "helper-socket", Kind: kindUnix, Target: "/run/maru-helper/helper.sock"},
		{ID: "dbus-system", Kind: kindUnix, Target: "/run/dbus/system_bus_socket"},
		{ID: "resolved-varlink", Kind: kindUnix, Target: "/run/systemd/resolve/io.systemd.Resolve"},
		// OMV 설정 DB(서비스 비밀번호가 들어 있다)와 공유 폴더·홈
		{ID: "omv-config", Kind: kindFile, Target: "/etc/openmediavault/config.xml"},
		{ID: "srv", Kind: kindDir, Target: "/srv"},
		{ID: "home", Kind: kindDir, Target: "/home"},
		// 네트워크가 있는 단계는 호스트 네트워크 이름공간이라 추상 유닉스 소켓에 닿는다(알려진 한계)
		{ID: "abstract-unix", Kind: kindAbstract, Target: "/proc/net/unix", Allowed: "any", Known: true},
		// 루프백(OMV nginx), LAN, 인터넷
		{ID: "loopback-http", Kind: kindTCP, Target: "127.0.0.1:80", Allowed: networkLoopback},
		lan,
		{ID: "public", Kind: kindTCP, Target: "1.1.1.1:443"},
		// 호스트 소켓은 닿아야 하지만(백엔드가 OMV를 부르는 길) 토큰 없는 호출은 거절해야 한다
		{ID: "host-without-token", Kind: kindHost, Target: hostSocket},
	}
}

// row는 결과 표의 행 하나다.
type row struct {
	Check  string `json:"check"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Result string `json:"result"`
	// Class는 오류 종류(ENOENT, EPERM 등 errno 이름, timeout, call_invalid, http_<상태>, none_visible) 또는 ok다.
	Class string `json:"class"`
	// Expect는 이 네트워크 권한에서 기대하는 결과(blocked, reachable)다. 판정하지 않는 행은 비어 있다.
	Expect  string `json:"expect,omitempty"`
	Verdict string `json:"verdict"`
}

// report는 조작 run의 결과다.
type report struct {
	Network []string `json:"network"`
	Rows    []row    `json:"rows"`
	Passed  int      `json:"passed"`
	Failed  int      `json:"failed"`
}

// probeInput은 조작 run의 입력이다. network는 이 패키지 manifest의 backend.network와 같게 준다(기대 결과를 정한다).
// lanAddress는 LAN에서 TCP로 연결해 볼 "IP:포트"다(비면 그 행은 건너뛴다).
type probeInput struct {
	Network    []string `json:"network"`
	LANAddress string   `json:"lanAddress"`
}

// validate는 입력을 검사한다. LAN 주소는 사설·링크 로컬 IP 리터럴과 포트만 받는다(이름 풀이를 하지 않고, 아무 주소나
// 두드리는 도구가 되지 않게).
func (in probeInput) validate() error {
	seen := map[string]bool{}
	for _, n := range in.Network {
		if n != networkLAN && n != networkLoopback {
			return fmt.Errorf("network: %q is not lan or loopback", n)
		}
		if seen[n] {
			return fmt.Errorf("network: %q listed twice", n)
		}
		seen[n] = true
	}
	if in.LANAddress == "" {
		return nil
	}
	ap, err := netip.ParseAddrPort(in.LANAddress)
	if err != nil || ap.Port() == 0 {
		return fmt.Errorf("lanAddress: want a private IP address and port such as 192.168.0.1:80")
	}
	if a := ap.Addr().Unmap(); !a.IsPrivate() && !a.IsLinkLocalUnicast() {
		return fmt.Errorf("lanAddress: %s is not a private or link-local address", a)
	}
	return nil
}

// env는 시험이 바깥에 닿는 함수다. 시험은 바꿔 끼워 실제 경로·네트워크에 닿지 않는다.
type env struct {
	readDir  func(path string) error
	readFile func(path string) error
	dial     func(ctx context.Context, network, address string) (net.Conn, error)
	// procNetUnix는 /proc/net/unix의 내용이다(이 이름공간에서 보이는 유닉스 소켓).
	procNetUnix func() ([]byte, error)
	// timeout은 연결 하나의 제한 시간이다.
	timeout time.Duration
}

func osEnv() env {
	var d net.Dialer
	return env{
		readDir:     readOneEntry,
		readFile:    readOneByte,
		dial:        d.DialContext,
		procNetUnix: func() ([]byte, error) { return os.ReadFile("/proc/net/unix") },
		timeout:     2 * time.Second,
	}
}

// readOneEntry는 디렉터리를 열고 항목 하나를 읽는다. 빈 디렉터리도 연 것이다. 이름은 버린다.
func readOneEntry(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.ReadDir(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// readOneByte는 파일을 열고 1바이트를 읽는다. 빈 파일도 읽은 것이다. 내용은 버린다.
func readOneByte(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Read(make([]byte, 1)); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// run은 시험을 하나씩 차례로 한다.
func (e env) run(ctx context.Context, in probeInput, hostSocket string) report {
	rep := report{Network: append([]string{}, in.Network...), Rows: []row{}}
	for _, c := range checks(in.LANAddress, hostSocket) {
		r := row{Check: c.ID, Kind: c.Kind, Target: c.Target}
		r.Result, r.Class = e.try(ctx, c)
		r.Expect = expect(c, in.Network)
		switch {
		case r.Result == resultSkipped || r.Expect == "":
			r.Verdict = verdictInfo
		case r.Result == r.Expect:
			r.Verdict = verdictPass
			rep.Passed++
		default:
			r.Verdict = verdictFail
			rep.Failed++
		}
		rep.Rows = append(rep.Rows, r)
	}
	return rep
}

// expect는 그 네트워크 권한에서 기대하는 결과다. 판정하지 않는 행은 ""다.
func expect(c check, network []string) string {
	switch {
	case c.Allowed == "":
		return resultBlocked
	case c.Known:
		if len(network) > 0 {
			return ""
		}
		return resultBlocked
	case slices.Contains(network, c.Allowed):
		return resultReachable
	}
	return resultBlocked
}

func (e env) try(ctx context.Context, c check) (result, class string) {
	switch c.Kind {
	case kindDir:
		return classify(e.readDir(c.Target))
	case kindFile:
		return classify(e.readFile(c.Target))
	case kindUnix:
		return e.connect(ctx, "unix", c.Target)
	case kindTCP:
		if c.Target == "" {
			return resultSkipped, "no_address"
		}
		return e.connect(ctx, "tcp", c.Target)
	case kindAbstract:
		data, err := e.procNetUnix()
		if err != nil {
			return classify(err)
		}
		name, ok := abstractListener(data)
		if !ok {
			// 이 이름공간에서 받고 있는 추상 소켓이 보이지 않는다(PrivateNetwork면 호스트의 것이 보이지 않는다)
			return resultBlocked, "none_visible"
		}
		return e.connect(ctx, "unix", name)
	case kindHost:
		return e.hostWithoutToken(ctx, c.Target)
	}
	return resultError, "unknown_check"
}

// connect는 연결해 보고 바로 닫는다. 아무것도 보내지 않는다.
func (e env) connect(ctx context.Context, network, address string) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	conn, err := e.dial(ctx, network, address)
	if err == nil {
		conn.Close()
	}
	return classify(err)
}

// hostWithoutToken은 호스트 소켓의 /v1/rpc를 호출 토큰 없이 부른다. 401 call_invalid면 막힌 것이다. 응답 본문은
// 코드만 읽고 버린다.
func (e env) hostWithoutToken(ctx context.Context, path string) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	client := &http.Client{Transport: &http.Transport{
		DialContext:       func(ctx context.Context, _, _ string) (net.Conn, error) { return e.dial(ctx, "unix", path) },
		DisableKeepAlives: true,
	}}
	defer client.CloseIdleConnections()
	body := strings.NewReader(`{"service":"System","method":"getInformation","params":null}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://maru/v1/rpc", body)
	if err != nil {
		return resultError, "other"
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		result, class := classify(err)
		// 호스트 소켓이 없는 것은 샌드박스가 막은 것이 아니다(백엔드는 이 소켓으로 OMV를 부른다)
		if result == resultBlocked {
			result = resultError
		}
		return result, class
	}
	defer resp.Body.Close()
	var e2 struct {
		Code string `json:"code"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&e2)
	switch {
	case resp.StatusCode == http.StatusUnauthorized && e2.Code == "call_invalid":
		return resultBlocked, "call_invalid"
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return resultReachable, fmt.Sprintf("http_%d", resp.StatusCode)
	}
	return resultError, fmt.Sprintf("http_%d", resp.StatusCode)
}

// abstractListener는 /proc/net/unix에서 받고 있는(__SO_ACCEPTCON) 스트림 추상 소켓 하나의 주소("@이름")를 찾는다.
//
//	Num       RefCount Protocol Flags    Type St Inode Path
//	0000000000000000: 00000002 00000000 00010000 0001 01 12345 @/example/socket
func abstractListener(data []byte) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 8 || f[3] != "00010000" || f[4] != "0001" || !strings.HasPrefix(f[7], "@") {
			continue
		}
		// 이름에 공백이 있으면 나머지 칸도 이름이다
		return strings.Join(f[7:], " "), true
	}
	return "", false
}

// classify는 오류를 결과와 오류 종류로 나눈다.
func classify(err error) (result, class string) {
	if err == nil {
		return resultReachable, "ok"
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		var ne net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) ||
			(errors.As(err, &ne) && ne.Timeout()) {
			return resultTimeout, "timeout"
		}
		return resultError, "other"
	}
	name := errnoName(errno)
	switch errno {
	case syscall.ENOENT, syscall.ENOTDIR, syscall.EACCES, syscall.EPERM, syscall.EAFNOSUPPORT:
		return resultBlocked, name
	case syscall.ECONNREFUSED:
		return resultReachable, name
	case syscall.ETIMEDOUT:
		return resultTimeout, name
	}
	return resultError, name
}

// errnoNames는 결과 표에 쓰는 errno 이름이다. 그 밖의 값은 번호로 쓴다.
var errnoNames = map[syscall.Errno]string{
	syscall.EPERM:           "EPERM",
	syscall.ENOENT:          "ENOENT",
	syscall.EACCES:          "EACCES",
	syscall.ENOTDIR:         "ENOTDIR",
	syscall.EINVAL:          "EINVAL",
	syscall.EAFNOSUPPORT:    "EAFNOSUPPORT",
	syscall.EPROTONOSUPPORT: "EPROTONOSUPPORT",
	syscall.EADDRNOTAVAIL:   "EADDRNOTAVAIL",
	syscall.ENETUNREACH:     "ENETUNREACH",
	syscall.EHOSTUNREACH:    "EHOSTUNREACH",
	syscall.ECONNREFUSED:    "ECONNREFUSED",
	syscall.ECONNRESET:      "ECONNRESET",
	syscall.ETIMEDOUT:       "ETIMEDOUT",
	syscall.ENOTSOCK:        "ENOTSOCK",
}

func errnoName(e syscall.Errno) string {
	if n, ok := errnoNames[e]; ok {
		return n
	}
	return fmt.Sprintf("errno_%d", int(e))
}
