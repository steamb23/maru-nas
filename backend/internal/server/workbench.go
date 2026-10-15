package server

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// CheckWorkbenchURL은 브라우저가 OMV 웹 UI를 열 주소(-workbench-url)를 확인한다. 빈 값은 기능을 끈 것이다.
// 웹에서 덮어쓰는 값(설정의 서버 페이지)도 같은 규칙으로 검사한다(install.sh의 valid_workbench_url과도 같다).
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
