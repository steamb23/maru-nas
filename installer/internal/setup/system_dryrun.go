package setup

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"time"
)

// dryRunSystem은 바꾸는 작업(파일 쓰기·삭제, root 명령)을 하지 않고 보여 준다. 읽기, 읽기 전용 확인,
// 실행한 계정의 빌드는 실제 시스템(base)에서 한다. 쓸 파일은 지금 내용과의 차이를 보여 준다.
type dryRunSystem struct {
	System
	out  io.Writer
	lang string
	// users는 이 실행에서 만들었다고 가정한 계정이다(useradd를 실행하지 않으므로).
	users map[string]bool
}

// NewDryRunSystem은 base에서 읽고 바꾸는 작업은 out에 보여 주기만 하는 System을 만든다.
func NewDryRunSystem(base System, out io.Writer, lang string) System {
	return &dryRunSystem{System: base, out: out, lang: lang, users: map[string]bool{}}
}

func (d *dryRunSystem) say(t Text) { fmt.Fprintf(d.out, "[dry-run] %s\n", t.In(d.lang)) }

func (d *dryRunSystem) cmd(name string, args ...string) error {
	line := commandLine(name, args)
	d.say(L("run: "+line, "실행: "+line))
	if name == "useradd" && len(args) > 0 {
		d.users[args[len(args)-1]] = true
	}
	return nil
}

func (d *dryRunSystem) WriteFile(path string, data []byte, perm fs.FileMode) error {
	old, err := d.System.ReadFile(path)
	mode := fmt.Sprintf("%04o", perm)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		d.say(L(fmt.Sprintf("write %s (%s, new file):", path, mode), fmt.Sprintf("%s 작성(%s, 새 파일):", path, mode)))
		old = nil
	case err != nil:
		return err
	default:
		d.say(L(fmt.Sprintf("write %s (%s, changes):", path, mode), fmt.Sprintf("%s 작성(%s, 바뀌는 내용):", path, mode)))
	}
	writeDiff(d.out, old, data)
	return nil
}

func (d *dryRunSystem) CopyFile(src, dst string) error { return d.cmd("cp", "-p", "--", src, dst) }

func (d *dryRunSystem) InstallFile(src, dst string, perm fs.FileMode) error {
	return d.cmd("install", "-o", "root", "-g", "root", "-m", fmt.Sprintf("%04o", perm), src, dst)
}

func (d *dryRunSystem) Remove(path string) error {
	if _, err := d.System.Stat(path); err != nil {
		return nil
	}
	return d.cmd("rm", "-f", "--", path)
}

func (d *dryRunSystem) RemoveAll(path string) error {
	if _, err := d.System.Stat(path); err != nil {
		return nil
	}
	return d.cmd("rm", "-rf", "--", path)
}

func (d *dryRunSystem) Run(name string, args ...string) error       { return d.cmd(name, args...) }
func (d *dryRunSystem) RunQuiet(name string, args ...string) error  { return d.cmd(name, args...) }
func (d *dryRunSystem) RunSilent(name string, args ...string) error { return d.cmd(name, args...) }

func (d *dryRunSystem) LookupUser(name string) (Account, bool, error) {
	if d.users[name] {
		return Account{Name: name, Home: "/nonexistent"}, true, nil
	}
	return d.System.LookupUser(name)
}

// PrimaryGroup은 이 실행에서 만든다고 가정한 계정이면 같은 이름의 그룹이다(useradd --user-group).
func (d *dryRunSystem) PrimaryGroup(name string) (string, error) {
	if d.users[name] {
		return name, nil
	}
	return d.System.PrimaryGroup(name)
}

func (d *dryRunSystem) HealthCheck(listen string) error {
	_, url := healthRequest(listen)
	d.say(L("check "+url, url+" 확인"))
	return nil
}

func (d *dryRunSystem) Sleep(time.Duration) {}
func (d *dryRunSystem) DryRun() bool        { return true }

// writeDiff는 old에서 new로 바뀌는 줄을 보여 준다. 설정 파일은 짧으므로 모든 줄을 보여 준다.
func writeDiff(w io.Writer, old, new []byte) {
	if bytes.Equal(old, new) {
		return
	}
	for _, l := range diffLines(splitLines(old), splitLines(new)) {
		fmt.Fprintf(w, "    %c %s\n", l.op, l.text)
	}
}

func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

type diffLine struct {
	op   byte // ' ' 같음, '-' 지움, '+' 더함
	text string
}

// diffLines는 최장 공통 부분열로 두 줄 목록의 차이를 만든다.
func diffLines(a, b []string) []diffLine {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []diffLine
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, diffLine{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffLine{'-', a[i]})
			i++
		default:
			out = append(out, diffLine{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, diffLine{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, diffLine{'+', b[j]})
	}
	return out
}
