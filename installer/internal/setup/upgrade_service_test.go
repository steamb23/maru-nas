package setup

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

// 업그레이드 서비스 시험은 실제 git 저장소(원격 bare 저장소와 그 클론)와, 업그레이드 계획을 내고 설치 인자를 기록하는
// 가짜 설치기를 쓴다. git·go·make·npm·tar가 있어야 하고 지금 계정이 저장소 소유자다(권한을 바꾸지 않는다).

// fakeInstallerSource는 시험 저장소의 installer/main.go다. --upgrade-plan은 public-origin 하나를 묻는 계획을 내고
// (값 bad는 검사를 통과하지 못한다), --service-install은 인자를 저장소의 installed-args에, 설치 기록을 MARU_TEST_ROOT
// 아래에 쓴다. VERSION은 커밋마다 바꾼다.
const fakeInstallerSource = `package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

const version = "VERSION"

func main() {
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "--upgrade-plan":
		plan(os.Args[2:])
	case "--service-install":
		install(os.Args[1:])
	default:
		os.Exit(2)
	}
}

func plan(args []string) {
	origin, given := "", false
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "--public-origin="); ok {
			origin, given = v, true
		}
	}
	questions := []map[string]any{}
	if !given || origin == "bad" {
		q := map[string]any{"name": "public-origin", "kind": "input", "title": map[string]string{"en": "Public address", "ko": "공개 주소"},
			"help": map[string]string{"en": "", "ko": ""}, "default": "", "dash": true, "emptyOK": true, "reason": "new"}
		if given {
			q["reason"], q["error"] = "invalid", map[string]string{"en": "bad address", "ko": "잘못된 주소"}
		}
		questions = append(questions, q)
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"format": 1, "version": version, "revision": "", "installed": nil, "web": true,
		"manual": []any{}, "questions": questions, "notes": []any{}, "requires": map[string]string{"go": "1.21", "node": "", "nodeFound": ""}})
}

func install(args []string) {
	exe, _ := os.Executable()
	repo := filepath.Dir(filepath.Dir(exe))
	os.WriteFile(filepath.Join(repo, "installed-args"), []byte(strings.Join(args, "\n")+"\n"), 0o644)
	rev := ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" {
				rev = s.Value
			}
		}
	}
	rec, _ := json.Marshal(map[string]any{"format": 1, "version": version, "revision": rev, "installedAt": "2026-10-19T00:00:00Z"})
	path := filepath.Join(os.Getenv("MARU_TEST_ROOT"), "/var/lib/maru-upgrade/installed.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, rec, 0o644)
}
`

// fakeRepoFiles는 시험 저장소의 파일이다. make build가 bin/에 세 실행 파일을 빌드한다(git 저장소 안이라 커밋이 들어간다).
var fakeRepoFiles = map[string]string{
	".gitignore":                "bin/\nfrontend/node_modules/\ninstalled-args\n",
	"go.mod":                    "module fakemaru\n\ngo 1.21\n",
	"cmd/maru/main.go":          "package main\n\nfunc main() {}\n",
	"cmd/maru-helper/main.go":   "package main\n\nfunc main() {}\n",
	"installer/go.mod":          "module fakeinstaller\n\ngo 1.21\n",
	"backend/README":            "backend\n",
	"vendor/icons/colloid/icon": "icon\n",
	"Makefile": "build:\n\tgo build -o bin/maru ./cmd/maru\n\tgo build -o bin/maru-helper ./cmd/maru-helper\n" +
		"\tcd installer && go build -o ../bin/maru-install .\n",
}

// serviceFixture는 업그레이드 서비스 시험 환경이다. origin(bare)에 커밋 A·B가 있고, work는 A를 체크아웃한 클론이다
// (origin/main은 아직 A). 설치 기록은 A다.
type serviceFixture struct {
	t                  *testing.T
	root, tmp          string
	origin, seed, work string
	revA, revB         string
	path               string
	gitEnv             []string
	out                strings.Builder
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("the upgrade service test runs git as the current (non-root) user")
	}
	var dirs []string
	for _, tool := range []string{"git", "go", "make", "npm", "tar"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skip(tool + " not found")
		}
		if p, err = filepath.Abs(p); err != nil {
			t.Fatal(err)
		}
		if d := filepath.Dir(p); !slices.Contains(dirs, filepath.Clean(d)) {
			dirs = append(dirs, filepath.Clean(d))
		}
	}
	for _, d := range []string{"/usr/bin", "/bin"} {
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	base := t.TempDir()
	f := &serviceFixture{
		t: t, root: filepath.Join(base, "root"), tmp: filepath.Join(base, "tmp"),
		origin: filepath.Join(base, "origin.git"), seed: filepath.Join(base, "seed"), work: filepath.Join(base, "work"),
		path: strings.Join(dirs, ":"),
	}
	gitconfig := filepath.Join(base, "gitconfig")
	if err := os.WriteFile(gitconfig, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.gitEnv = []string{"GIT_CONFIG_GLOBAL=" + gitconfig, "GIT_CONFIG_NOSYSTEM=1"}
	for _, d := range []string{f.root, f.tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.git(base, "init", "-q", "--bare", "-b", "main", f.origin)
	f.git(base, "clone", "-q", f.origin, f.seed)
	for name, content := range fakeRepoFiles {
		f.write(f.seed, name, content)
	}
	f.revA = f.commit(f.seed, "0.9.0", "first")
	f.git(f.seed, "push", "-q", "origin", "main")
	f.git(base, "clone", "-q", f.origin, f.work)
	if err := os.MkdirAll(filepath.Join(f.work, "frontend", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.revB = f.commit(f.seed, "0.9.1", "second")
	f.git(f.seed, "push", "-q", "origin", "main")

	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	f.put(config.UpgradeEnvFile, config.EnvUpgradeRepo+"="+f.work+"\n"+config.EnvUpgradeUser+"="+u.Username+"\n"+config.EnvUpgradePath+"="+f.path+"\n", 0o644)
	f.record(f.revA, "0.9.0")
	return f
}

// git은 시험 준비용 git이다(사용자 설정 없이, 서명 없이).
func (f *serviceFixture) git(dir string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append(f.gitEnv, "GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.invalid",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.invalid")...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *serviceFixture) write(dir, name, content string) {
	f.t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// commit은 가짜 설치기의 버전을 바꿔 커밋한다.
func (f *serviceFixture) commit(dir, version, msg string) string {
	f.t.Helper()
	f.write(dir, "installer/main.go", strings.Replace(fakeInstallerSource, "VERSION", version, 1))
	f.git(dir, "add", "-A")
	f.git(dir, "commit", "-q", "-m", msg)
	return f.git(dir, "rev-parse", "HEAD")
}

// put은 가짜 루트에 파일을 쓴다.
func (f *serviceFixture) put(path, content string, perm fs.FileMode) {
	f.t.Helper()
	f.write(f.root, path, content)
	if err := os.Chmod(filepath.Join(f.root, path), perm); err != nil {
		f.t.Fatal(err)
	}
}

func (f *serviceFixture) record(rev, version string) {
	f.t.Helper()
	data, _ := json.Marshal(config.InstallRecord{Format: config.InstallRecordFormat, Version: version, Revision: rev, InstalledAt: "2026-10-01T00:00:00Z"})
	f.put(config.InstallRecordFile, string(data), 0o644)
}

func (f *serviceFixture) requestCheck(mode string) {
	f.t.Helper()
	f.requestChannelCheck(mode, config.UpgradeChannelCurrent)
}

func (f *serviceFixture) requestChannelCheck(mode, channel string) {
	f.t.Helper()
	data, _ := json.Marshal(config.UpgradeCheckRequest{Format: config.UpgradeRequestFormat, Mode: mode, Channel: channel})
	f.put(upgradeCheckRequest, string(data), 0o644)
}

func (f *serviceFixture) requestApply(target string, settings map[string]string) {
	f.t.Helper()
	data, _ := json.Marshal(config.UpgradeApplyRequest{Format: config.UpgradeRequestFormat, Target: target, Settings: settings,
		User: "bob", RequestedAt: "2026-10-19T01:00:00Z"})
	f.put(upgradeApplyRequest, string(data), 0o600)
}

// serve는 업그레이드 서비스를 한 번 실행하고 상태 파일을 돌려준다.
func (f *serviceFixture) serve() config.UpgradeStatus {
	f.t.Helper()
	s := &upgradeService{
		root: f.root, tmp: f.tmp, out: &f.out,
		now:      func() time.Time { return time.Date(2026, 10, 19, 1, 2, 3, 0, time.UTC) },
		extraEnv: append(slices.Clone(f.gitEnv), "MARU_TEST_ROOT="+f.root),
	}
	if err := s.serve(); err != nil {
		f.t.Fatalf("serve: %v\n%s", err, f.out.String())
	}
	return f.status()
}

func (f *serviceFixture) status() config.UpgradeStatus {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, config.UpgradeStatusFile))
	if err != nil {
		f.t.Fatalf("status: %v\n%s", err, f.out.String())
	}
	var st config.UpgradeStatus
	if err := json.Unmarshal(data, &st); err != nil {
		f.t.Fatal(err)
	}
	return st
}

func (f *serviceFixture) exists(path string) bool {
	_, err := os.Lstat(filepath.Join(f.root, path))
	return err == nil
}

func reasonIDs(rs []config.UpgradeReason) []string {
	var ids []string
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	return ids
}

func errorText(st config.UpgradeStatus) string {
	if st.Error == nil {
		return ""
	}
	return st.Error.Text.En
}

func TestParseID(t *testing.T) {
	for s, want := range map[string]int{"0": 0, "1000": 1000, "2147483647": 2147483647} {
		if got, err := parseID(s); err != nil || got != want {
			t.Errorf("parseID(%q) = %d, %v", s, got, err)
		}
	}
	// uid_t보다 큰 값이 uint32로 잘려 0(root)이 되지 않게 거부한다
	for _, s := range []string{"4294967296", "2147483648", "-1", "", "1e3"} {
		if _, err := parseID(s); err == nil {
			t.Errorf("parseID(%q) accepted", s)
		}
	}
}

// 원격 확인: 소유자로 git fetch를 하고 추적 브랜치(origin/main = B)를 대상으로 정한다. 대상 커밋의 설치기만 임시로 빌드해
// 계획을 받고 작업 트리는 그대로 둔다. 적용: 답한 설정 값을 새 버전으로 검사한 뒤 fast-forward, 빌드, 새 설치기를
// --service-install --upgrade로 실행한다.
func TestUpgradeServiceRemote(t *testing.T) {
	f := newServiceFixture(t)
	f.requestCheck(config.UpgradeModeRemote)
	st := f.serve()
	chk := st.Check
	if st.State != config.UpgradeStateChecked || chk == nil {
		t.Fatalf("status %+v (error %q)\n%s", st, errorText(st), f.out.String())
	}
	if chk.Target != f.revB || chk.Head != f.revA || chk.Upstream != "origin/main" || !chk.Web || chk.UpToDate || len(chk.Manual) != 0 {
		t.Errorf("check %+v", chk)
	}
	if len(chk.Commits) != 1 || chk.Commits[0].Hash != f.revB || chk.Commits[0].Subject != "second" || chk.Commits[0].Author != "Tester" {
		t.Errorf("commits %+v", chk.Commits)
	}
	if chk.Plan == nil || chk.Plan.Version != "0.9.1" || len(chk.Plan.Questions) != 1 || chk.Installed == nil || chk.Installed.Revision != f.revA {
		t.Errorf("plan %+v installed %+v", chk.Plan, chk.Installed)
	}
	if f.exists(upgradeCheckRequest) {
		t.Error("check request not removed")
	}
	if head := f.git(f.work, "rev-parse", "HEAD"); head != f.revA {
		t.Errorf("check moved the work tree to %s", head)
	}
	if entries, _ := os.ReadDir(f.tmp); len(entries) != 0 {
		t.Errorf("temporary build left behind: %v", entries)
	}

	// 설정 파일과 적용 요청은 4KiB를 넘을 수 있다(긴 빌드 PATH, 설정 32개 × 값 512바이트).
	u, _ := user.Current()
	long := f.path + ":" + strings.TrimSuffix(strings.Repeat("/nonexistent/"+strings.Repeat("d", 90)+":", 30), ":")
	f.put(config.UpgradeEnvFile, "# "+strings.Repeat("comment ", 250)+"\n"+config.EnvUpgradeRepo+"="+f.work+"\n"+config.EnvUpgradeUser+"="+u.Username+"\n"+config.EnvUpgradePath+"="+long+"\n", 0o644)
	settings := map[string]string{"public-origin": "https://desk.example.lan"}
	data, _ := json.Marshal(config.UpgradeApplyRequest{Format: config.UpgradeRequestFormat, Target: f.revB, Settings: settings,
		User: "bob", RequestedAt: "2026-10-19T01:00:00Z" + strings.Repeat(" ", 20<<10)})
	f.put(upgradeApplyRequest, string(data), 0o600)
	st = f.serve()
	if st.State != config.UpgradeStateApplied || st.Apply == nil || st.Apply.Version != "0.9.1" || st.Apply.User != "bob" || st.Apply.FinishedAt == "" {
		t.Fatalf("status %+v apply %+v (error %q)\n%s", st, st.Apply, errorText(st), f.out.String())
	}
	if head := f.git(f.work, "rev-parse", "HEAD"); head != f.revB {
		t.Errorf("work tree at %s, want %s", head, f.revB)
	}
	args, _ := os.ReadFile(filepath.Join(f.work, "installed-args"))
	if want := "--service-install\n--upgrade\n--plain\n--lang\nen\n--public-origin=https://desk.example.lan\n"; string(args) != want {
		t.Errorf("installer args %q", args)
	}
	if f.exists(upgradeApplyRequest) {
		t.Error("apply request not removed")
	}

	t.Log(f.out.String())

	// 확인 없이 다시 적용하면 거절한다.
	f.requestApply(f.revB, map[string]string{"public-origin": "https://desk.example.lan"})
	if st = f.serve(); st.State != config.UpgradeStateApplyFailed || !strings.Contains(errorText(st), "Check again") {
		t.Errorf("second apply %+v %q", st, errorText(st))
	}
}

// 업데이트 채널: 채널을 고른 원격 확인은 origin의 그 브랜치를 이름으로 가져와(클론의 fetch 설정과 상관없이) 대상으로
// 삼는다. 지금 브랜치와 다르면 업그레이드할 때 저장소를 채널 브랜치로 옮기고, 이전 버전이어도 덮어써서 설치한다.
func TestUpgradeServiceChannel(t *testing.T) {
	f := newServiceFixture(t)
	f.git(f.seed, "push", "-q", "origin", f.revA+":refs/heads/stable")
	f.git(f.work, "config", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")
	good := map[string]string{"public-origin": "https://desk.example.lan"}
	check := func(channel string) config.UpgradeStatus {
		t.Helper()
		f.requestChannelCheck(config.UpgradeModeRemote, channel)
		st := f.serve()
		if st.State != config.UpgradeStateChecked || st.Check == nil {
			t.Fatalf("check %q: %+v %q\n%s", channel, st, errorText(st), f.out.String())
		}
		return st
	}
	apply := func(target string) config.UpgradeStatus {
		t.Helper()
		f.requestApply(target, good)
		st := f.serve()
		if st.State != config.UpgradeStateApplied {
			t.Fatalf("apply %s: %+v %q\n%s", target, st, errorText(st), f.out.String())
		}
		return st
	}
	branchOf := func() string { return f.git(f.work, "rev-parse", "--abbrev-ref", "HEAD") }

	// main(A)에서 stable 채널: 설치된 A와 같아 최신이고, 브랜치는 그대로 둔다.
	st := check(config.UpgradeChannelStable)
	if chk := st.Check; chk.Channel != config.UpgradeChannelStable || chk.Upstream != "origin/stable" || chk.Branch != "main" ||
		!chk.Switch || !chk.UpToDate || chk.Web || st.Branch != "main" || st.Detached {
		t.Errorf("stable at the installed commit: %+v (status branch %q)", chk, st.Branch)
	}
	// main 채널은 지금 브랜치라 옮기지 않고 fast-forward한다.
	st = check(config.UpgradeChannelMain)
	if chk := st.Check; chk.Switch || chk.Target != f.revB || !chk.Web || chk.Dropped != 0 || len(chk.Commits) != 1 {
		t.Errorf("main: %+v", chk)
	}
	if st = apply(f.revB); st.Branch != "main" || branchOf() != "main" {
		t.Errorf("after main: status branch %q, work tree on %s", st.Branch, branchOf())
	}

	// main(B)에서 stable(A): 이전 버전이지만 채널을 바꾸므로 덮어써서 설치한다.
	st = check(config.UpgradeChannelStable)
	if chk := st.Check; !chk.Switch || chk.Target != f.revA || chk.Dropped != 1 || len(chk.Commits) != 0 || !chk.Web || len(chk.Manual) != 0 {
		t.Errorf("stable from main: %+v", chk)
	}
	st = apply(f.revA)
	if st.Apply.Version != "0.9.0" || st.Branch != "stable" || st.Detached || branchOf() != "stable" {
		t.Errorf("after stable: %+v branch %q, work tree on %s", st.Apply, st.Branch, branchOf())
	}
	if up := f.git(f.work, "rev-parse", "--abbrev-ref", "stable@{upstream}"); up != "origin/stable" {
		t.Errorf("stable tracks %q", up)
	}
	if head := f.git(f.work, "rev-parse", "HEAD"); head != f.revA {
		t.Errorf("work tree at %s, want %s", head, f.revA)
	}

	// stable(A)에서 다시 main(B): 로컬 main은 B까지라 옮겨도 잃는 커밋이 없다.
	st = check(config.UpgradeChannelMain)
	if chk := st.Check; !chk.Switch || chk.Branch != "stable" || chk.Target != f.revB || chk.Dropped != 0 || len(chk.Commits) != 1 || !chk.Web {
		t.Errorf("main from stable: %+v", chk)
	}
	// 확인한 뒤 브랜치가 바뀌면 옮기지 않는다.
	f.git(f.work, "checkout", "-q", "-b", "other")
	f.requestApply(f.revB, good)
	if st = f.serve(); st.State != config.UpgradeStateApplyFailed || !strings.Contains(errorText(st), "repository changed") || branchOf() != "other" {
		t.Errorf("branch changed: %s %q on %s", st.State, errorText(st), branchOf())
	}
	f.git(f.work, "checkout", "-q", "stable")
	check(config.UpgradeChannelMain)
	if st = apply(f.revB); st.Branch != "main" || branchOf() != "main" {
		t.Errorf("after main again: status branch %q, work tree on %s", st.Branch, branchOf())
	}

	// 로컬 stable 브랜치에 원격에 없는 커밋이 있으면 덮어쓰지 않는다. SSH로 정리한 뒤 다시 확인하면 되므로 명령은 없다.
	f.git(f.work, "checkout", "-q", "stable")
	f.write(f.work, "backend/README", "local\n")
	f.git(f.work, "commit", "-q", "-am", "local")
	f.git(f.work, "checkout", "-q", "main")
	if chk := check(config.UpgradeChannelStable).Check; chk.Web || !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonDiverged}) ||
		chk.Manual[0].Command != "" || chk.Command != "" {
		t.Errorf("local stable commits: %+v, command %q", chk.Manual, chk.Command)
	}

	// 브랜치가 아닌 체크아웃(태그·커밋)도 채널 브랜치로 옮길 수 있다. 지금 브랜치를 따르면 추적 브랜치가 없다.
	f.git(f.work, "branch", "-q", "-D", "stable")
	f.git(f.work, "checkout", "-q", "--detach", f.revB)
	st = check(config.UpgradeChannelStable)
	if chk := st.Check; chk.Branch != "" || !st.Detached || st.Branch != "" || !chk.Switch || chk.Target != f.revA || chk.Dropped != 1 || !chk.Web {
		t.Errorf("detached: %+v (status branch %q detached %t)", chk, st.Branch, st.Detached)
	}
	if chk := check(config.UpgradeChannelCurrent).Check; !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonNoUpstream}) || chk.Channel != "" {
		t.Errorf("detached without a channel: %+v", chk)
	}

	// origin에 없는 채널 브랜치는 가져오지 못한다.
	f.git(f.seed, "push", "-q", "origin", ":refs/heads/stable")
	f.requestChannelCheck(config.UpgradeModeRemote, config.UpgradeChannelStable)
	if st = f.serve(); st.State != config.UpgradeStateCheckFailed || !strings.Contains(errorText(st), "has a stable branch") {
		t.Errorf("missing channel branch: %s %q", st.State, errorText(st))
	}
}

// 적용은 마지막 확인을 믿지 않는다. 대상·설정·설치 기록·작업 트리가 확인과 맞지 않으면 빌드하기 전에 멈추고 작업 트리를
// 옮기지 않는다. 새 버전의 검사를 통과하지 못한 값은 그 오류를 알린다.
func TestUpgradeServiceApplyRefused(t *testing.T) {
	f := newServiceFixture(t)
	good := map[string]string{"public-origin": "https://desk.example.lan"}
	cases := []struct {
		name     string
		prepare  func()
		target   string
		settings map[string]string
		want     string
		stage    string
	}{
		{name: "other target", target: strings.Repeat("a", 40), settings: good, want: "no finished check", stage: config.UpgradeStageInspect},
		{name: "unknown setting", settings: map[string]string{"public-origin": "", "helper": "yes"}, want: "does not ask for --helper", stage: config.UpgradeStageInspect},
		{name: "missing answer", settings: map[string]string{}, want: "--public-origin needs an answer", stage: config.UpgradeStageInspect},
		{name: "rejected value", settings: map[string]string{"public-origin": "bad"}, want: "--public-origin: bad address", stage: config.UpgradeStagePlan},
		{name: "record changed", prepare: func() { f.record(strings.Repeat("b", 40), "0.9.0") }, settings: good, want: "installed version changed", stage: config.UpgradeStageInspect},
		{name: "dirty", prepare: func() { f.write(f.work, "Makefile", "changed\n") }, settings: good, want: "uncommitted changes", stage: config.UpgradeStageInspect},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.git(f.work, "checkout", "-q", "--", ".")
			f.record(f.revA, "0.9.0")
			f.requestCheck(config.UpgradeModeRemote)
			if st := f.serve(); st.State != config.UpgradeStateChecked || !st.Check.Web {
				t.Fatalf("check %+v %q", st, errorText(st))
			}
			if tc.prepare != nil {
				tc.prepare()
			}
			target := tc.target
			if target == "" {
				target = f.revB
			}
			f.requestApply(target, tc.settings)
			st := f.serve()
			if st.State != config.UpgradeStateApplyFailed || !strings.Contains(errorText(st), tc.want) || st.Stage != tc.stage {
				t.Errorf("status %s at %s: %q, want %q at %s", st.State, st.Stage, errorText(st), tc.want, tc.stage)
			}
			if head := f.git(f.work, "rev-parse", "HEAD"); head != f.revA {
				t.Errorf("work tree moved to %s", head)
			}
			if _, err := os.Stat(filepath.Join(f.work, "installed-args")); err == nil {
				t.Error("installer ran")
			}
		})
	}
}

// 확인은 대시보드에서 업그레이드할 수 없는 상태를 수동 재설치 사유로 알린다.
func TestUpgradeServiceCheckReasons(t *testing.T) {
	f := newServiceFixture(t)
	check := func(mode string) *config.UpgradeCheck {
		t.Helper()
		f.requestCheck(mode)
		st := f.serve()
		if st.State != config.UpgradeStateChecked || st.Check == nil {
			t.Fatalf("status %+v %q\n%s", st, errorText(st), f.out.String())
		}
		return st.Check
	}
	// 로컬 확인: 설치된 커밋을 체크아웃하고 있으면 최신이다. 계획을 만들지 않는다.
	if chk := check(config.UpgradeModeLocal); !chk.UpToDate || chk.Web || chk.Target != f.revA || chk.Plan != nil || len(chk.Manual) != 0 {
		t.Errorf("up to date %+v", chk)
	}
	// 커밋하지 않은 변경
	f.write(f.work, "Makefile", "changed\n")
	if chk := check(config.UpgradeModeRemote); chk.Web || !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonDirty}) || chk.Command != config.UpgradeCommandInstall {
		t.Errorf("dirty %+v", chk.Manual)
	}
	f.git(f.work, "checkout", "-q", "--", ".")
	// 로컬 커밋: 원격으로는 fast-forward할 수 없지만 로컬 확인은 그 커밋을 설치할 수 있다.
	f.write(f.work, "backend/README", "local\n")
	f.git(f.work, "commit", "-q", "-am", "local")
	local := f.git(f.work, "rev-parse", "HEAD")
	if chk := check(config.UpgradeModeRemote); chk.Web || !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonDiverged}) {
		t.Errorf("diverged %+v", chk.Manual)
	}
	if chk := check(config.UpgradeModeLocal); !chk.Web || chk.Target != local || len(chk.Commits) != 1 || chk.Plan.Version != "0.9.0" {
		t.Errorf("local commit %+v", chk)
	}
	// 설치된 커밋에서 이어지지 않는 대상(되돌아가는 설치)
	f.record(local, "0.9.0")
	f.git(f.work, "checkout", "-q", "--detach", f.revA)
	if chk := check(config.UpgradeModeLocal); chk.Web || !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonNotDescendant}) {
		t.Errorf("not descendant %+v", chk.Manual)
	}
	// 추적 브랜치가 없는 체크아웃(커밋·태그)
	if chk := check(config.UpgradeModeRemote); chk.Target != "" || !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonNoUpstream}) {
		t.Errorf("no upstream %+v", chk)
	}
	// 저장소에 없는 설치 커밋, 설치 기록 없음
	f.record(strings.Repeat("c", 40), "0.9.0")
	if chk := check(config.UpgradeModeLocal); !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonCommitMissing}) || chk.Plan == nil {
		t.Errorf("commit missing %+v", chk.Manual)
	}
	if err := os.Remove(filepath.Join(f.root, config.InstallRecordFile)); err != nil {
		t.Fatal(err)
	}
	if chk := check(config.UpgradeModeLocal); !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonNoRecord}) {
		t.Errorf("no record %+v", chk.Manual)
	}
	f.record(f.revA, "0.9.0")
	// 저장소 경로가 심볼릭 링크면 쓰지 않는다.
	link := filepath.Join(filepath.Dir(f.work), "link")
	if err := os.Symlink(f.work, link); err != nil {
		t.Fatal(err)
	}
	u, _ := user.Current()
	f.put(config.UpgradeEnvFile, config.EnvUpgradeRepo+"="+link+"\n"+config.EnvUpgradeUser+"="+u.Username+"\n"+config.EnvUpgradePath+"="+f.path+"\n", 0o644)
	if chk := check(config.UpgradeModeLocal); !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonRepository}) {
		t.Errorf("symlinked repository %+v", chk.Manual)
	}
	// 빌드 PATH에 도구가 없다.
	f.put(config.UpgradeEnvFile, config.EnvUpgradeRepo+"="+f.work+"\n"+config.EnvUpgradeUser+"="+u.Username+"\n"+config.EnvUpgradePath+"=/nonexistent\n", 0o644)
	if chk := check(config.UpgradeModeLocal); !slices.Equal(reasonIDs(chk.Manual), []string{config.UpgradeReasonTools}) ||
		!strings.Contains(chk.Manual[0].Text.En, "git, go, npm, make") {
		t.Errorf("tools %+v", chk.Manual)
	}
}

// 요청은 내용과 상관없이 읽은 뒤 지운다. 적용 요청이 먼저다. 대시보드에서 업그레이드가 꺼져 있으면(설정 파일 없음)
// 요청을 지우기만 하고 상태 파일을 만들지 않는다. maru가 쓴 확인 요청은 심볼릭 링크를 따라가지 않는다.
func TestUpgradeServiceRequests(t *testing.T) {
	f := newServiceFixture(t)
	s := func() error {
		return (&upgradeService{root: f.root, tmp: f.tmp, out: &f.out, now: time.Now, extraEnv: f.gitEnv}).serve()
	}
	if err := s(); err != nil || f.exists(config.UpgradeStatusFile) {
		t.Fatalf("no request: %v", err)
	}

	f.requestCheck("sideways")
	if st := f.serve(); st.State != config.UpgradeStateCheckFailed || !strings.Contains(errorText(st), "check request from the dashboard is not valid") {
		t.Errorf("bad mode %+v %q", st, errorText(st))
	}
	f.requestChannelCheck(config.UpgradeModeRemote, "refs/heads/x")
	if st := f.serve(); st.State != config.UpgradeStateCheckFailed || !strings.Contains(errorText(st), "check request from the dashboard is not valid") {
		t.Errorf("bad channel %+v %q", st, errorText(st))
	}
	secret := filepath.Join(f.root, "secret")
	f.put("/secret", `{"format":1,"mode":"local"}`, 0o600)
	if err := os.Symlink(secret, filepath.Join(f.root, upgradeCheckRequest)); err != nil {
		t.Fatal(err)
	}
	if st := f.serve(); st.State != config.UpgradeStateCheckFailed || f.exists(upgradeCheckRequest) || !f.exists("/secret") {
		t.Errorf("symlink %+v exists=%v", st, f.exists(upgradeCheckRequest))
	}

	f.requestCheck(config.UpgradeModeLocal)
	f.put(upgradeApplyRequest, "not json", 0o600)
	if st := f.serve(); st.State != config.UpgradeStateApplyFailed || !f.exists(upgradeCheckRequest) || f.exists(upgradeApplyRequest) {
		t.Errorf("apply first %+v", st)
	}

	if err := os.Remove(filepath.Join(f.root, config.UpgradeStatusFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.root, config.UpgradeEnvFile)); err != nil {
		t.Fatal(err)
	}
	if err := s(); err != nil {
		t.Fatal(err)
	}
	if f.exists(upgradeCheckRequest) || f.exists(config.UpgradeStatusFile) {
		t.Error("disabled: request kept or status written")
	}
	if _, err := os.Stat(filepath.Join(f.root, config.UpgradeStatusFile)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("status %v", err)
	}
}

// 계획의 질문에 대한 답을 새 설치기 옵션으로 바꾼다.
func TestSettingArgs(t *testing.T) {
	qs := []config.UpgradeQuestion{
		{Name: "public-origin", Kind: config.UpgradeQuestionInput, Dash: true, EmptyOK: true},
		{Name: "helper", Kind: config.UpgradeQuestionConfirm},
		{Name: "elevate-group", Kind: config.UpgradeQuestionInput},
	}
	args, err := settingArgs(qs, map[string]string{"public-origin": "-", "helper": "no", "elevate-group": "--wheel"})
	if want := []string{"--public-origin=", "--no-helper", "--elevate-group=--wheel"}; err != nil || !slices.Equal(args, want) {
		t.Errorf("args %q %v", args, err)
	}
	for _, bad := range []map[string]string{
		{"public-origin": "", "helper": "yes"},
		{"public-origin": "", "helper": "maybe", "elevate-group": "g"},
		{"public-origin": "", "helper": "yes", "elevate-group": ""},
		{"public-origin": "", "helper": "yes", "elevate-group": "g", "listen": "x"},
		{"public-origin": "a b", "helper": "yes", "elevate-group": "g"},
	} {
		if args, err := settingArgs(qs, bad); err == nil {
			t.Errorf("%v accepted: %q", bad, args)
		}
	}
}

// --upgrade-service는 다른 옵션을 받지 않고 root로만 돈다.
func TestUpgradeServiceOptions(t *testing.T) {
	for _, args := range [][]string{
		{"--upgrade-service", "--upgrade-plan"},
		{"--upgrade-service", "--upgrade"},
		{"--upgrade-service", "--dry-run"},
		{"--upgrade-service", "--public-origin", ""},
		{"--upgrade-service=x"},
	} {
		if _, _, err := parseArgs(settings, args); err == nil {
			t.Errorf("%q accepted", args)
		}
	}
	f := newFake(t)
	c, _, _ := newTestContext(t, f, prompt.Fail{}, "--upgrade-service")
	if code, msg := ExitCode(Run(c)); code != 1 || !strings.Contains(msg, "must run as root") {
		t.Errorf("non-root: %d %q", code, msg)
	}
}
