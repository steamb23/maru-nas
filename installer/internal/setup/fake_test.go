package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"onmaru/installer/internal/prompt"
)

// fakeSystem은 테스트용 System이다. 파일은 임시 디렉터리(가짜 루트) 아래에서 실제로 읽고 쓰고, 명령은 실행하지 않고
// 기록한다. 기록 꼴은 골든의 commands와 같다(systemctl ..., install -o root -g root -m 0755 SRC DST).
type fakeSystem struct {
	fsOps
	log []string
	// users는 계정, groups는 계정의 기본 그룹이다.
	users  map[string]Account
	groups map[string]string
	// missingGroups는 없는 그룹이다(나머지는 있다).
	missingGroups map[string]bool
	// active는 systemctl is-active가 성공하는 유닛이다.
	active map[string]bool
	// unitFiles는 systemctl list-unit-files가 성공하는 유닛이다.
	unitFiles map[string]bool
	// noCommands는 PATH에 없는 명령이다.
	noCommands map[string]bool
	portsInUse map[string]bool
	health     error
	sleeps     int
}

func newFake(t *testing.T) *fakeSystem {
	t.Helper()
	return &fakeSystem{
		fsOps:         fsOps{root: t.TempDir()},
		users:         map[string]Account{},
		groups:        map[string]string{},
		missingGroups: map[string]bool{},
		active:        map[string]bool{},
		unitFiles:     map[string]bool{},
		noCommands:    map[string]bool{},
		portsInUse:    map[string]bool{},
	}
}

func (f *fakeSystem) record(name string, args ...string) {
	f.log = append(f.log, commandLine(name, args))
}

func (f *fakeSystem) Run(name string, args ...string) error {
	f.record(name, args...)
	switch name {
	case "useradd":
		u := args[len(args)-1]
		f.users[u] = Account{Name: u, UID: 999, Home: "/nonexistent"}
		f.groups[u] = u
	case "userdel":
		delete(f.users, args[len(args)-1])
	}
	return nil
}

func (f *fakeSystem) RunQuiet(name string, args ...string) error  { return f.Run(name, args...) }
func (f *fakeSystem) RunSilent(name string, args ...string) error { return f.Run(name, args...) }

func (f *fakeSystem) Check(name string, args ...string) error {
	f.record(name, args...)
	if name == "systemctl" && len(args) > 0 {
		unit := args[len(args)-1]
		switch args[0] {
		case "is-active":
			if f.active[unit] {
				return nil
			}
		case "list-unit-files":
			if f.unitFiles[unit] {
				return nil
			}
		}
	}
	return errors.New("exit status 1")
}

func (f *fakeSystem) RunUser(dir, name string, args ...string) error {
	f.record(name, args...)
	return nil
}

func (f *fakeSystem) InstallFile(src, dst string, perm fs.FileMode) error {
	f.record("install", "-o", "root", "-g", "root", "-m", fmt.Sprintf("%04o", perm), src, dst)
	return f.fsOps.InstallFile(src, dst, perm)
}

func (f *fakeSystem) LookPath(name string) bool { return !f.noCommands[name] }
func (f *fakeSystem) CurrentUser() string       { return "tester" }

func (f *fakeSystem) LookupUser(name string) (Account, bool, error) {
	a, ok := f.users[name]
	return a, ok, nil
}

func (f *fakeSystem) PrimaryGroup(name string) (string, error) {
	if g, ok := f.groups[name]; ok {
		return g, nil
	}
	return "", errors.New("no such user")
}

func (f *fakeSystem) GroupExists(name string) (bool, error) { return !f.missingGroups[name], nil }
func (f *fakeSystem) PortInUse(addr string) bool            { return f.portsInUse[addr] }
func (f *fakeSystem) HealthCheck(string) error              { return f.health }
func (f *fakeSystem) Sleep(time.Duration)                   { f.sleeps++ }
func (f *fakeSystem) Now() time.Time                        { return time.Date(2026, 10, 18, 9, 30, 0, 0, time.UTC) }
func (f *fakeSystem) DryRun() bool                          { return false }

// put은 가짜 루트에 파일을 만든다.
func (f *fakeSystem) put(t *testing.T, path, content string, perm fs.FileMode) {
	t.Helper()
	p := f.path(path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
}

// read는 가짜 루트의 파일 내용이다. 없으면 ok가 false다.
func (f *fakeSystem) read(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(f.path(path))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// copyTree는 dir 아래 파일을 가짜 루트에 복사한다.
func (f *fakeSystem) copyTree(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		f.put(t, "/"+rel, string(b), 0o644)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// putRepo는 가짜 저장소(/repo)에 빌드 결과를 만든다.
func (f *fakeSystem) putRepo(t *testing.T) {
	t.Helper()
	f.put(t, "/repo/bin/onmaru", "#!/bin/sh\n", 0o755)
	f.put(t, "/repo/bin/onmaru-helper", "#!/bin/sh\n", 0o755)
	f.put(t, "/repo/"+distIndexRel, "<!doctype html>\n", 0o644)
	f.put(t, "/repo/install.sh", "#!/bin/sh\n", 0o755)
}

// testOut은 출력을 모으는 곳이다.
type testOut struct{ strings.Builder }

// execCall은 가로챈 sudo 재실행이다.
type execCall struct {
	path string
	argv []string
}

var errExecIntercepted = errors.New("exec intercepted")

// newTestContext는 가짜 시스템으로 설치기를 실행할 Context를 만든다.
func newTestContext(t *testing.T, sys System, p prompt.Prompter, args ...string) (*Context, *testOut, *[]execCall) {
	t.Helper()
	opts, values, err := parseArgs(settings, args)
	if err != nil {
		t.Fatalf("parseArgs(%q): %v", args, err)
	}
	lang := opts.Lang
	if lang == "" {
		lang = "en"
	}
	out := &testOut{}
	var calls []execCall
	c := &Context{
		Lang:        lang,
		Sys:         sys,
		P:           p,
		Out:         out,
		Err:         out,
		Interactive: true,
		RepoDir:     "/repo",
		Self:        "/repo/bin/onmaru-install",
		Exec: func(path string, argv []string, _ []string) error {
			calls = append(calls, execCall{path, argv})
			return errExecIntercepted
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		Reg:      settings,
		Opts:     opts,
		Values:   values,
	}
	return c, out, &calls
}

// configFiles는 설치기가 쓰는 설정 파일이다(골든 비교 대상).
var configFiles = []string{envFile, helperEnvFile, unitFile, helperSocketUnit, helperServiceUnit}

// compareTree는 가짜 루트의 설정 파일이 want 디렉터리(가짜 루트 기준 트리)와 같은지 본다.
func compareTree(t *testing.T, f *fakeSystem, want string) {
	t.Helper()
	for _, p := range configFiles {
		wb, werr := os.ReadFile(filepath.Join(want, p))
		got, ok := f.read(t, p)
		switch {
		case errors.Is(werr, fs.ErrNotExist) && ok:
			t.Errorf("%s should not exist, got:\n%s", p, got)
		case werr == nil && !ok:
			t.Errorf("%s missing", p)
		case werr == nil && got != string(wb):
			t.Errorf("%s differs\n--- got\n%s--- want\n%s", p, got, wb)
		case werr != nil && !errors.Is(werr, fs.ErrNotExist):
			t.Fatal(werr)
		}
	}
}

// backups는 가짜 루트에 생긴 백업 파일이다.
func backups(t *testing.T, f *fakeSystem) []string {
	t.Helper()
	var out []string
	for _, dir := range []string{"/etc/default", "/etc/systemd/system"} {
		entries, _ := os.ReadDir(f.path(dir))
		for _, e := range entries {
			if strings.Contains(e.Name(), ".bak-") {
				out = append(out, filepath.Join(dir, e.Name()))
			}
		}
	}
	return out
}
