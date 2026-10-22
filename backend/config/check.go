package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// UnixPrefix는 -listen 값에서 Unix 소켓 경로를 나타내는 접두사다.
const UnixPrefix = "unix:"

// CheckListen은 -listen 값을 확인한다. unix:/절대/경로 또는 host:port를 받는다.
// 백엔드는 로컬 리버스 프록시를 통해서만 접근하는 것이 기본이므로, allowNonLoopback이 아니면
// 루프백 IP가 아닌 TCP 주소를 거부한다. Unix 소켓은 파일 권한으로 접근을 제한한다.
func CheckListen(addr string, allowNonLoopback bool) error {
	if path, ok := strings.CutPrefix(addr, UnixPrefix); ok {
		if !filepath.IsAbs(path) {
			return errors.New("unix socket path must be absolute (e.g. unix:/run/maru/maru.sock), got " + addr)
		}
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if !allowNonLoopback {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return errors.New("listen address must be a loopback IP (e.g. 127.0.0.1:8090) or unix:/path, got " +
				addr + " (use -allow-non-loopback to listen on other addresses)")
		}
	}
	return nil
}

// CheckOMVURL은 OMV 주소가 루프백 호스트의 http(s) URL인지 확인한다.
// OMV 세션 ID가 네트워크로 나가지 않게 한다.
func CheckOMVURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("omv url must be http(s), got " + raw)
	}
	if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("omv url must point to a loopback address (e.g. http://127.0.0.1), got " + raw)
	}
	return nil
}

// CheckHelperSocket은 도우미 소켓 경로가 비었거나 절대 경로인지 확인한다.
func CheckHelperSocket(path string) error {
	if path != "" && (!filepath.IsAbs(path) || filepath.Clean(path) != path) {
		return errors.New("helper socket must be an absolute path (e.g. /run/maru-helper/helper.sock), got " + path)
	}
	return nil
}

// CheckWorkbenchURL은 브라우저가 OMV 웹 UI를 열 주소(-workbench-url)를 확인한다. 빈 값은 기능을 끈 것이다.
// 웹에서 덮어쓰는 값(설정의 서버 페이지)과 설치기의 입력도 같은 규칙으로 검사한다.
// http(s) 절대 URL이거나, 대시보드 문서 기준으로 해석할 경로 참조(/, ./, ../로 시작)만 받는다.
// scheme 없는 호스트 이름(nas.example.lan/)은 브라우저가 상대 경로로 해석해 대시보드 안을 가리키므로 거부한다.
func CheckWorkbenchURL(raw string) error {
	if raw == "" {
		return nil
	}
	if strings.ContainsAny(raw, "?# \t") {
		return errors.New("workbench url must not contain a query, fragment or spaces, got " + raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("workbench url: %w", err)
	}
	switch {
	case u.Scheme == "http" || u.Scheme == "https":
		if u.Host == "" || u.User != nil {
			return errors.New("workbench url must have a host and no user info, got " + raw)
		}
	case u.Scheme == "" && u.Host == "" && isPathRef(raw):
	default:
		return errors.New("workbench url must start with http:// or https://, or be a path starting with / or ./ or ../ " +
			"(add the scheme to a host name, e.g. https://nas.example.lan/), got " + raw)
	}
	return nil
}

// isPathRef는 /, ./, ../로 시작하는 경로 참조인지 확인한다. //host 형태는 아니다.
func isPathRef(s string) bool {
	if strings.HasPrefix(s, "//") {
		return false
	}
	return strings.HasPrefix(s, "/") || s == "." || s == ".." || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../")
}

// ParseRefreshAt은 "HH:MM" 형식의 SMART 기준 시각(-smart-refresh-at)을 읽는다.
func ParseRefreshAt(s string) (hour, minute int, err error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("smart refresh time must be HH:MM, got %q", s)
	}
	return t.Hour(), t.Minute(), nil
}

// ParsePublicOrigin은 공개 주소(-public-origin)를 검사해 패스키의 RP ID(호스트 이름)와 출처(https://호스트[:포트])를
// 돌려준다. https://호스트[:포트]만 받는다. 경로·query·fragment·사용자 정보가 있으면 거부하고,
// IP 주소는 RP ID가 될 수 없으므로 거부한다. 기본 포트 443은 출처에서 뺀다.
func ParsePublicOrigin(raw string) (id, origin string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	switch {
	case u.Scheme != "https":
		return "", "", errors.New("public origin must start with https://, got " + raw)
	case u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/"):
		return "", "", errors.New("public origin must be https://host[:port] without path, query or fragment, got " + raw)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || strings.HasSuffix(host, ".") {
		return "", "", errors.New("public origin needs a host name, got " + raw)
	}
	if net.ParseIP(host) != nil {
		return "", "", errors.New("public origin must use a domain name, not an IP address (passkeys cannot use IP addresses), got " + raw)
	}
	origin = "https://" + host
	if p := u.Port(); p != "" && p != "443" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return "", "", errors.New("public origin has an invalid port, got " + raw)
		}
		origin += ":" + p
	}
	return host, origin, nil
}

// groupNameRE는 받는 그룹 이름 꼴이다. 실행 인자로 넘길 수 있으므로 -로 시작하지 않게 한다.
var groupNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)

// ValidGroupName은 그룹 이름(maru-helper -elevate-group 등) 꼴이 맞는지다.
func ValidGroupName(name string) bool { return groupNameRE.MatchString(name) }
