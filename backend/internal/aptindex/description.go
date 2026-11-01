package aptindex

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

// DpkgStatusPath는 루트(fs.FS) 기준 dpkg 상태 파일이다(0644, 설치된 패키지의 control 필드).
const DpkgStatusPath = "var/lib/dpkg/status"

// 설명을 읽는 상한.
const (
	// maxDescriptionBytes는 긴 설명 하나의 상한이다. 넘으면 줄 경계에서 자른다.
	maxDescriptionBytes = 16 << 10
	// maxStatusBytes는 dpkg 상태 파일을 읽는 상한이다(패키지가 수천 개여도 수 MB다).
	maxStatusBytes = 64 << 20
	// maxStatusLine은 dpkg 상태 파일 한 줄의 상한이다.
	maxStatusLine = 1 << 20
)

// 긴 설명은 OMV가 만드는 업데이트 목록(후보 버전)과 dpkg 상태(설치된 버전) 두 곳에서 읽는다.
//
// OMV는 apt 번역 파일을 받지 않는다(Acquire::Languages "none"). Debian 공식 저장소의 Packages에는 한 줄
// 요약만 있어서 Debian 패키지는 업데이트 목록의 extendeddescription이 비고, 긴 설명을 Packages에 담는
// 저장소(OMV, 제3자)만 값이 있다. 설치된 패키지의 긴 설명은 .deb의 control 그대로 dpkg 상태에 있다.
// 번역을 받지 않으므로 두 곳 모두 영어 원문이다.
//
// 두 출처는 같은 꼴로 맞춘다. 문단은 빈 줄 하나로 나누고, 문단 안의 줄바꿈은 원문 그대로 둔다. dpkg 상태는
// 들여쓰기(Debian 정책에서 두 칸 이상은 "그대로 표시")를 남기지만 업데이트 목록은 omv-mkaptidx가 줄마다 앞뒤
// 공백을 지워 남지 않는다. 문단을 다시 흐르게 하고 글머리표를 목록으로 바꾸는 일은 화면이 한다.

// IndexDescription은 업데이트 목록의 extendeddescription을 다듬는다. omv-mkaptidx는 줄마다 공백을 지우고
// 문단 나눔(" .")을 "\n"으로 바꿔 줄과 함께 이으므로 문단 사이에 빈 줄이 둘 생긴다.
func IndexDescription(s string) string {
	return tidyDescription(strings.Split(s, "\n"))
}

// ParseInstalledDescription은 dpkg 상태 파일에서 name 패키지(설치된 것)의 긴 설명을 읽는다. 첫 줄(요약)은
// 빼고 연속 줄만 다듬어 돌려준다. name에 ":아키텍처"가 붙어 있으면(다른 아키텍처 패키지) Architecture도
// 맞춰 본다. 없거나 설치되지 않았으면 found가 false다.
func ParseInstalledDescription(r io.Reader, name string) (desc string, found bool, err error) {
	pkgName, arch, _ := strings.Cut(name, ":")
	var (
		match    bool   // 지금 문단이 name 패키지인지(dpkg는 Package를 문단 맨 앞에 쓴다)
		field    string // 지금 줄이 속한 필드(연속 줄은 앞 필드에 붙는다)
		pkgArch  string
		state    string
		lines    []string
		hasField bool
	)
	flush := func() bool {
		ok := match && hasField && installedState(state) && (arch == "" || pkgArch == arch)
		match, field, pkgArch, state, lines, hasField = false, "", "", "", nil, false
		return ok
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxStatusLine)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			l := lines
			if flush() {
				return tidyDescription(l), true, nil
			}
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			if match && field == "Description" {
				lines = append(lines, continuation(line))
			}
			continue
		}
		key, value, _ := strings.Cut(line, ":")
		field = key
		value = strings.TrimSpace(value)
		switch key {
		case "Package":
			match = value == pkgName
		case "Architecture":
			pkgArch = value
		case "Status":
			state = value
		case "Description":
			// 첫 줄은 요약이다. 요약은 업데이트 목록의 것(후보 버전)을 쓴다.
			hasField = true
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, err
	}
	l := lines
	if flush() {
		return tidyDescription(l), true, nil
	}
	return "", false, nil
}

// installedState는 dpkg Status("원하는 상태 오류 플래그 현재 상태")의 현재 상태가 설치됨인지다.
// 고정한 패키지(hold ok installed)도 설치된 것이다.
func installedState(status string) bool {
	f := strings.Fields(status)
	return len(f) == 3 && f[2] == "installed"
}

// continuation은 control 연속 줄의 앞 공백 하나를 지운다. " ."는 빈 줄(문단 나눔)이다.
func continuation(line string) string {
	line = line[1:]
	if strings.TrimSpace(line) == "." {
		return ""
	}
	return line
}

// tidyDescription은 줄 끝 공백을 지우고, 이어진 빈 줄을 하나로 줄이고, 앞뒤 빈 줄을 지운 뒤 상한에서 줄
// 경계로 자른다.
func tidyDescription(lines []string) string {
	var b strings.Builder
	blank := false
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		if l == "" {
			blank = b.Len() > 0
			continue
		}
		add := len(l) + 1
		if blank {
			add++
		}
		if b.Len()+add > maxDescriptionBytes {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		if blank {
			b.WriteByte('\n')
			blank = false
		}
		b.WriteString(l)
	}
	return b.String()
}

// InstalledDescription은 dpkg 상태 파일에서 name 패키지(설치된 것)의 긴 설명을 읽는다. 패키지가 없거나
// 설치되지 않았으면 빈 문자열이다. 파일을 읽지 못하면 오류다.
func (r *Reader) InstalledDescription(name string) (string, error) {
	f, err := r.root.Open(DpkgStatusPath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("dpkg status: not a regular file")
	}
	desc, _, err := ParseInstalledDescription(io.LimitReader(f, maxStatusBytes), name)
	if err != nil {
		return "", fmt.Errorf("dpkg status: %w", err)
	}
	return desc, nil
}
