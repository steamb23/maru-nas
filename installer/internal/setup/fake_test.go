package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"maru/backend/config"
	"maru/installer/internal/prompt"
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
	// noCommands는 PATH에 없는 명령이다. 나머지는 /usr/bin에 있다.
	noCommands map[string]bool
	// owners는 경로별 소유자다. 없는 경로는 실제 파일 소유자다.
	owners     map[string]Account
	portsInUse map[string]bool
	// outputs는 Output이 돌려줄 명령별 표준 출력이다(키는 명령 기록 꼴). 없는 명령은 실패한다(PATH에 없음).
	outputs map[string]string
	// buildInfo는 경로별 Go 빌드 정보다. 없는 경로는 Go 실행 파일이 아닌 것으로 본다(가짜 저장소의 실행 파일은 셸 스크립트다).
	buildInfo map[string]*debug.BuildInfo
	health    error
	sleeps    int
	// groupIDs는 GroupID가 돌려줄 그룹의 gid다. 없는 그룹은 찾지 못한 것이다.
	groupIDs map[string]int
	// fail은 실패시킬 명령(명령 기록 꼴)과 그 오류다. 기록한 뒤 실패하고, 아래 흉내도 내지 않는다.
	fail map[string]error
	// units면 systemctl enable --now·disable이 백엔드 소켓 인스턴스의 sockets.target.wants 링크를 만들고 지우고,
	// enable --now·start가 그 유닛을 active로, stop이 이름을 준 유닛을 멈춘 것으로 바꾸며, systemctl clean --what=state가
	// 백엔드 데이터를 지운다(root 동기화 시험).
	units bool
	// cleanKeeps면 units에서도 systemctl clean이 데이터를 지우지 않는다(성공했다고 하고 아무것도 지우지 않는 경우).
	cleanKeeps bool
	// onRun은 시스템을 바꾸는 명령을 기록한 뒤 부른다(시험이 실행 도중 파일을 바꾸거나 잠금을 확인한다).
	onRun func(cmd string)
	// keepRequests면 TakeFile이 요청을 지우지 못한 것처럼 errRequestKept로 감싼 오류를 돌려준다(파일은 그대로 둔다).
	keepRequests bool
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
		// 가짜 저장소는 일반 사용자 alice의 것이다(대시보드에서 업그레이드의 빌드 계정).
		owners:     map[string]Account{"/repo": {Name: "alice", UID: 1000, Home: "/home/alice"}},
		portsInUse: map[string]bool{},
		buildInfo:  map[string]*debug.BuildInfo{},
		outputs:    map[string]string{},
		groupIDs:   map[string]int{},
		fail:       map[string]error{},
	}
}

func (f *fakeSystem) Output(name string, args ...string) ([]byte, error) {
	f.record(name, args...)
	if out, ok := f.outputs[commandLine(name, args)]; ok {
		return []byte(out), nil
	}
	return nil, errors.New(name + ": executable file not found in $PATH")
}

func (f *fakeSystem) ReadBuildInfo(path string) (*debug.BuildInfo, error) {
	if bi, ok := f.buildInfo[path]; ok {
		return bi, nil
	}
	return nil, errors.New("not a Go executable")
}

// vcsBuild는 git 커밋 정보가 들어 있는 합성 빌드 정보다.
func vcsBuild(revision string, modified bool) *debug.BuildInfo {
	return &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs", Value: "git"},
		{Key: "vcs.revision", Value: revision},
		{Key: "vcs.time", Value: "2026-10-01T02:03:04Z"},
		{Key: "vcs.modified", Value: map[bool]string{true: "true", false: "false"}[modified]},
	}}
}

func (f *fakeSystem) record(name string, args ...string) {
	f.log = append(f.log, commandLine(name, args))
}

func (f *fakeSystem) Run(name string, args ...string) error {
	f.record(name, args...)
	line := commandLine(name, args)
	if f.onRun != nil {
		f.onRun(line)
	}
	if err, ok := f.fail[line]; ok {
		return err
	}
	switch name {
	case "useradd":
		u := args[len(args)-1]
		f.users[u] = Account{Name: u, UID: 999, Home: "/nonexistent"}
		f.groups[u] = u
	case "userdel":
		delete(f.users, args[len(args)-1])
	case "systemctl":
		if f.units {
			f.systemctl(args)
		}
	}
	return nil
}

// systemctl은 백엔드 인스턴스에 대한 systemctl의 파일 쪽 결과를 흉내 낸다(units). 소켓을 켜면 sockets.target.wants에
// 템플릿을 가리키는 링크가 생기고 끄면 지워진다. clean --what=state는 DynamicUser의 StateDirectory(private 아래의
// 디렉터리와 바깥의 링크)를 지운다.
func (f *fakeSystem) systemctl(args []string) {
	switch {
	case len(args) == 3 && args[0] == "enable" && args[1] == "--now":
		if unit, _, ok := strings.Cut(strings.TrimSuffix(args[2], ".socket"), "@"); ok && strings.HasSuffix(args[2], ".socket") {
			_ = os.MkdirAll(f.path(addonWantsDir), 0o755)
			_ = os.Symlink("/etc/systemd/system/"+unit+"@.socket", f.path(addonWantsDir+"/"+args[2]))
			f.active[args[2]] = true
		}
	case len(args) == 2 && args[0] == "start":
		f.active[args[1]] = true
	case len(args) > 1 && args[0] == "stop":
		for _, u := range args[1:] {
			delete(f.active, u)
		}
	case len(args) == 2 && args[0] == "disable" && strings.HasSuffix(args[1], ".socket"):
		_ = os.Remove(f.path(addonWantsDir + "/" + args[1]))
	case len(args) == 3 && args[0] == "clean" && args[1] == "--what=state" && !f.cleanKeeps:
		name, _ := strings.CutSuffix(args[2], ".service")
		if _, id, ok := strings.Cut(name, "@"); ok {
			_ = os.RemoveAll(f.path(addonPrivateDataDir + "/" + id))
			_ = os.Remove(f.path(addonDataDir + "/" + id))
		}
	}
}

func (f *fakeSystem) TakeFile(p string, max int64) ([]byte, error) {
	if f.keepRequests {
		return nil, fmt.Errorf("%w: remove %s: read-only file system", errRequestKept, p)
	}
	return f.fsOps.TakeFile(p, max)
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
func (f *fakeSystem) Which(name string) (string, bool) {
	if f.noCommands[name] {
		return "", false
	}
	return "/usr/bin/" + name, true
}

func (f *fakeSystem) FileOwner(path string) (Account, error) {
	if a, ok := f.owners[path]; ok {
		return a, nil
	}
	return f.fsOps.FileOwner(path)
}
func (f *fakeSystem) CurrentUser() string { return "tester" }

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

func (f *fakeSystem) GroupID(name string) (int, bool, error) {
	gid, ok := f.groupIDs[name]
	return gid, ok, nil
}

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
	f.put(t, "/repo/bin/maru", "#!/bin/sh\n", 0o755)
	f.put(t, "/repo/bin/maru-helper", "#!/bin/sh\n", 0o755)
	f.put(t, "/repo/bin/maru-install", "#!/bin/sh\n", 0o755)
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
		Self:        "/repo/bin/maru-install",
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
var configFiles = append([]string{envFile, helperEnvFile, unitFile, helperSocketUnit, helperServiceUnit,
	config.UpgradeEnvFile, upgradeServiceUnit, upgradePathUnit, config.AddonSyncStatusFile}, addonUnitFiles()...)

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
