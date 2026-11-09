package power

import (
	"os"
	"path/filepath"
	"strings"
)

// SystemZone은 이 시스템 시간대의 IANA 이름이다(예: Asia/Seoul). TZ 환경 변수, /etc/localtime 링크, /etc/timezone
// 순으로 본다. 알 수 없으면 빈 문자열이다. 브라우저가 NAS 시각을 보일 때 쓴다.
func SystemZone() string {
	if tz := strings.TrimPrefix(os.Getenv("TZ"), ":"); tz != "" {
		return zoneName(tz)
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if name := zoneName(target); name != "" {
			return name
		}
	}
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		return zoneName(strings.TrimSpace(string(b)))
	}
	return ""
}

// zoneName은 경로나 이름에서 IANA 이름을 꺼낸다(.../zoneinfo/Asia/Seoul → Asia/Seoul). 형식이 맞지 않으면 빈 문자열이다.
func zoneName(s string) string {
	if _, rest, ok := strings.Cut(s, "zoneinfo/"); ok {
		s = rest
	}
	s = filepath.ToSlash(s)
	if s == "" || len(s) > 64 || strings.HasPrefix(s, "/") || strings.Contains(s, "..") {
		return ""
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("/_+-", r)) {
			return ""
		}
	}
	return s
}
