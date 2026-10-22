package setup

import (
	"bytes"
	"errors"
	"io/fs"
	"regexp"
	"strings"
)

// EnvFile은 systemd EnvironmentFile 형식의 설정 파일(/etc/default/maru 등)이다. 셸로 source하지 않고 줄 단위로 읽는다.
// nil은 파일이 없다는 뜻이고, 모든 메서드가 nil에서도 동작한다.
type EnvFile struct {
	lines []string
}

// ParseEnv는 설정 파일 내용을 읽는다.
func ParseEnv(data []byte) *EnvFile {
	return &EnvFile{lines: splitLines(data)}
}

// Get은 KEY= 로 시작하는 마지막 줄의 값이다. 값 전체를 감싼 큰따옴표 한 쌍은 벗긴다(설치 스크립트의 env_get과 같다).
func (e *EnvFile) Get(key string) (string, bool) {
	if e == nil {
		return "", false
	}
	v, found := "", false
	for _, l := range e.lines {
		if rest, ok := strings.CutPrefix(l, key+"="); ok {
			v, found = rest, true
		}
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return v, found
}

// Value는 Get의 값만이다(없으면 빈 값).
func (e *EnvFile) Value(key string) string {
	v, _ := e.Get(key)
	return v
}

// Has는 KEY= 줄이 있는지다. 빈 값도 정한 값이다.
func (e *EnvFile) Has(key string) bool {
	_, ok := e.Get(key)
	return ok
}

// envLineRE는 보존 대상이 될 수 있는 maru 설정 줄이다.
var envLineRE = regexp.MustCompile(`^(MARU_[A-Z0-9_]+)=`)

// Unmanaged는 managed에 없는 MARU_* 줄을 원래 모습 그대로 돌려준다. 손으로 넣은 설정(예: MARU_COOKIE_PATH)을
// 다시 설치할 때 지우지 않고 보존하는 데 쓴다.
func (e *EnvFile) Unmanaged(managed map[string]bool) []string {
	if e == nil {
		return nil
	}
	var out []string
	for _, l := range e.lines {
		if m := envLineRE.FindStringSubmatch(l); m != nil && !managed[m[1]] {
			out = append(out, l)
		}
	}
	return out
}

// readEnvFile은 설정 파일을 읽는다. 없으면 nil이다.
func readEnvFile(sys System, path string) (*EnvFile, error) {
	data, err := sys.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseEnv(data), nil
}

// unitUser는 유닛 파일의 첫 User= 값이다.
func unitUser(data []byte) string {
	for _, l := range splitLines(data) {
		if v, ok := strings.CutPrefix(l, "User="); ok {
			return v
		}
	}
	return ""
}

// writeConfig는 설정 파일을 쓴다. 내용이 같으면 그대로 두고 알린다. 내용이 달라졌을 때 backup이면 기존 파일을
// PATH.bak-<시각>으로 남긴다(환경 파일만. 유닛은 설정으로 언제든 다시 만들 수 있으므로 백업하지 않는다).
func (c *Context) writeConfig(path string, data []byte, backup bool) error {
	old, err := c.Sys.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if exists && bytes.Equal(old, data) {
		c.Info(L(path+" unchanged", path+" 변경 없음"))
		return nil
	}
	if exists && backup {
		if err := c.Sys.CopyFile(path, path+".bak-"+c.Sys.Now().Format("20060102-150405")); err != nil {
			return err
		}
	}
	if err := c.Sys.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if !c.Sys.DryRun() {
		c.Info(L("Wrote "+path, path+" 작성"))
	}
	return nil
}
