package worker

import (
	"bufio"
	"io"
	"strings"
)

// ParseMountInfo는 /proc/self/mountinfo 형식에서 마운트 지점 목록을 읽는다(proc(5)).
// 줄마다 다섯째 칸이 마운트 지점이고, 공백·탭·줄바꿈·역슬래시는 8진 이스케이프(\040 등)로 들어 있다.
func ParseMountInfo(r io.Reader) ([]string, error) {
	var out []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 {
			continue
		}
		out = append(out, unescapeOctal(fields[4]))
	}
	return out, sc.Err()
}

// unescapeOctal은 \ooo 꼴 8진 이스케이프를 푼다. 맞지 않는 꼴은 그대로 둔다.
func unescapeOctal(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && isOctal(s[i+1]) && isOctal(s[i+2]) && isOctal(s[i+3]) {
			b.WriteByte((s[i+1]-'0')<<6 | (s[i+2]-'0')<<3 | (s[i+3] - '0'))
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctal(c byte) bool { return c >= '0' && c <= '7' }
