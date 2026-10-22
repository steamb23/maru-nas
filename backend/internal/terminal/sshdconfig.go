// Package terminal은 웹 터미널의 SSH 쪽이다. maru는 같은 호스트의 sshd(루프백)에 붙는 SSH
// 클라이언트가 되고, 브라우저와의 WebSocket 중계는 internal/server가 한다.
//
// 접속 계정은 대시보드 세션의 사용자로 고정한다. 누가 들어갈 수 있는지(AllowGroups, 셸, 인증 방식)는
// OMV가 관리하는 sshd가 정하고, 이 패키지는 sshd 설정을 읽기만 한다.
package terminal

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// DefaultSSHDConfig는 sshd 설정 파일 경로다. OMV가 Salt로 만든다(읽기만 한다).
const DefaultSSHDConfig = "/etc/ssh/sshd_config"

// SSHDConfig는 sshd 설정에서 터미널이 쓰는 값이다.
type SSHDConfig struct {
	// Port는 첫 Port 값이다. 없으면 0이다(sshd 기본값 22).
	Port int
	// AllowGroups는 SSH 로그인을 허용하는 그룹이다. 비어 있으면 제한이 없다. 실패 안내에만 쓴다.
	AllowGroups []string
}

// ParseSSHDConfig는 sshd_config 형식을 읽는다. 키워드는 대소문자를 가리지 않고, "키워드 값"과
// "키워드=값"을 모두 받는다. 첫 Match 줄부터는 조건부 설정이라 읽지 않는다(OMV 추가 옵션의
// Match 블록 안 Port를 전역 값으로 잘못 집지 않게). Include는 따라가지 않는다. OMV는 Include를
// 파일 끝에 두고 sshd는 먼저 나온 값을 쓰므로 Port는 본 파일의 값이 이긴다.
func ParseSSHDConfig(r io.Reader) (SSHDConfig, error) {
	var c SSHDConfig
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value := splitDirective(line)
		switch strings.ToLower(key) {
		case "match":
			return c, nil
		case "port":
			if c.Port == 0 {
				if p, err := strconv.Atoi(value); err == nil && p > 0 && p < 65536 {
					c.Port = p
				}
			}
		case "allowgroups":
			// 여러 줄이면 sshd처럼 합친다.
			c.AllowGroups = append(c.AllowGroups, strings.Fields(value)...)
		}
	}
	return c, sc.Err()
}

// splitDirective는 "키워드 값" 또는 "키워드=값"을 나눈다.
func splitDirective(line string) (key, value string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line, ""
	}
	key, rest := line[:i], strings.TrimLeft(line[i:], " \t")
	rest = strings.TrimPrefix(rest, "=")
	return key, strings.TrimSpace(rest)
}
