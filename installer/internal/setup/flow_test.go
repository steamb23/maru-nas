package setup

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/config"
	"onmaru/installer/internal/prompt"
)

func goldenDir(name string) string { return filepath.Join("testdata/golden", name) }

// installedFake는 helper-loopback 골든 결과가 설치된 가짜 시스템이다.
func installedFake(t *testing.T) *fakeSystem {
	t.Helper()
	f := newFake(t)
	f.putRepo(t)
	f.copyTree(t, filepath.Join(goldenDir("helper-loopback"), "want"))
	f.put(t, binDst, "old\n", 0o755)
	f.put(t, helperBinDst, "old\n", 0o755)
	f.users["onmaru"] = Account{Name: "onmaru", UID: 999, Home: "/nonexistent"}
	f.groups["onmaru"] = "onmaru"
	return f
}

// 새 설치: 일반 사용자가 질문에 기본값으로 답하면 1단계가 sudo로 2단계를 실행하고, 2단계는 받은 옵션만으로
// 묻지 않고 helper-loopback 골든과 같은 파일을 만든다.
func TestFlowFreshInstall(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	script := &prompt.Script{Answers: map[string]string{
		"user": "", "listen-mode": "", "listen": "", "omv-url": "", "smart-refresh-at": "",
		"workbench-url": "", "public-origin": "", "helper": "", "elevate-group": "", "web-upgrade": "", "proceed": "",
	}}
	c, out, calls := newTestContext(t, f, script, "--no-build")
	if err := Run(c); !errors.Is(err, errExecIntercepted) {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	want := []string{"user", "listen-mode", "listen", "omv-url", "smart-refresh-at", "workbench-url", "public-origin", "helper", "elevate-group", "web-upgrade", "proceed"}
	if !slices.Equal(script.Asked, want) {
		t.Errorf("asked %v, want %v", script.Asked, want)
	}
	if len(*calls) != 1 {
		t.Fatalf("exec calls %v", *calls)
	}
	call := (*calls)[0]
	if call.path != "/usr/bin/sudo" || !slices.Equal(call.argv[:5], []string{"sudo", "/repo/bin/onmaru-install", "--service-install", "--lang", "en"}) {
		t.Fatalf("exec %v", call)
	}
	if len(f.log) != 0 {
		t.Errorf("stage 1 ran commands: %v", f.log)
	}

	// 2단계: sudo가 실행할 인자 그대로, root로, 질문 없이
	f.users["onmaru"] = Account{Name: "onmaru", UID: 999, Home: "/nonexistent"}
	f.groups["onmaru"] = "onmaru"
	c2, out2, _ := newTestContext(t, f, prompt.Fail{}, call.argv[2:]...)
	c2.IsRoot = true
	if err := Run(c2); err != nil {
		t.Fatalf("stage 2: %v\n%s", err, out2)
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-web-upgrade"), "want"))
}

// --upgrade는 기존 설치에 없는 설정(여기서는 공개 주소)만 묻고, 요약 확인 없이 진행한다.
func TestFlowUpgradeAsksOnlyMissing(t *testing.T) {
	f := installedFake(t)
	env, _ := f.read(t, envFile)
	f.put(t, envFile, strings.Replace(env, "ONMARU_PUBLIC_ORIGIN=\n", "", 1), 0o644)
	script := &prompt.Script{Answers: map[string]string{"public-origin": "-"}}
	c, out, calls := newTestContext(t, f, script, "--upgrade", "--no-build")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	if !slices.Equal(script.Asked, []string{"public-origin"}) {
		t.Errorf("asked %v", script.Asked)
	}
	if len(*calls) != 0 {
		t.Errorf("root should continue in-process, exec %v", *calls)
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
	s := out.String()
	if !strings.Contains(s, "Some settings are new in this version") {
		t.Errorf("no new-settings notice:\n%s", s)
	}
	// 수신 주소와 서비스 사용자가 그대로면 리버스 프록시 안내를 줄인다.
	if strings.Contains(s, "Next: point your reverse proxy") {
		t.Errorf("proxy note on unchanged upgrade:\n%s", s)
	}
}

// 설정이 모두 있는 기존 설치의 --upgrade는 질문이 하나도 없고, 파일이 그대로면 백업도 없다. 도우미도 다시 시작한다.
func TestFlowUpgradeNoQuestions(t *testing.T) {
	f := installedFake(t)
	f.active["onmaru.service"] = true
	f.active["onmaru-helper.service"] = true
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--upgrade", "--no-build")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
	if b := backups(t, f); len(b) != 0 {
		t.Errorf("backups %v", b)
	}
	for _, want := range []string{"systemctl restart onmaru-helper.service", "systemctl restart onmaru.service"} {
		if !slices.Contains(f.log, want) {
			t.Errorf("missing %q in %v", want, f.log)
		}
	}
	if got, _ := f.read(t, binDst); got != "#!/bin/sh\n" {
		t.Errorf("binary not installed: %q", got)
	}
}

// 일반 사용자의 --upgrade는 질문 없이 sudo로 2단계를 실행하며 --upgrade를 넘긴다.
func TestFlowUpgradeReexec(t *testing.T) {
	f := installedFake(t)
	c, out, calls := newTestContext(t, f, prompt.Fail{}, "--upgrade", "--no-build", "--plain", "--lang", "ko")
	if err := Run(c); !errors.Is(err, errExecIntercepted) {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	argv := (*calls)[0].argv
	if !slices.Equal(argv[:5], []string{"sudo", "/repo/bin/onmaru-install", "--service-install", "--lang", "ko"}) ||
		!slices.Contains(argv, "--upgrade") || argv[len(argv)-1] != "--plain" {
		t.Errorf("argv %q", argv)
	}
}

func TestFlowUpgradeNeedsInstallation(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	c, _, _ := newTestContext(t, f, prompt.Fail{}, "--upgrade", "--no-build")
	err := Run(c)
	if code, msg := ExitCode(err); code != 1 || !strings.Contains(msg, "--upgrade needs an existing installation") {
		t.Errorf("got %d %q", code, msg)
	}
}

// 도우미를 설치했다가 --no-helper로 다시 설치하면 도우미 파일과 유닛을 지운다.
func TestFlowHelperRemoved(t *testing.T) {
	f := installedFake(t)
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--service-install", "--user", "onmaru", "--listen", "127.0.0.1:8090",
		"--omv-url", "http://127.0.0.1", "--smart-refresh-at", "00:00", "--workbench-url", "", "--public-origin", "", "--no-helper")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	for _, p := range []string{helperSocketUnit, helperServiceUnit, helperBinDst, helperEnvFile} {
		if _, ok := f.read(t, p); ok {
			t.Errorf("%s not removed", p)
		}
	}
	env, _ := f.read(t, envFile)
	if !strings.Contains(env, "\nONMARU_HELPER_SOCKET=\n") {
		t.Errorf("env %s", env)
	}
	unit, _ := f.read(t, unitFile)
	if strings.Contains(unit, "onmaru-helper") {
		t.Errorf("unit still wants the helper:\n%s", unit)
	}
	i := slices.Index(f.log, "systemctl disable --now onmaru-helper.socket onmaru-helper.service")
	if i < 0 || f.log[i+1] != "systemctl daemon-reload" {
		t.Errorf("log %v", f.log)
	}
	// 다시 --upgrade하면 도우미를 설치하지 않은 설치로 읽는다.
	c2, out2, _ := newTestContext(t, f, prompt.Fail{}, "--upgrade", "--no-build")
	c2.IsRoot = true
	if err := Run(c2); err != nil {
		t.Fatalf("upgrade: %v\n%s", err, out2)
	}
	if c2.Values.Get(nameHelper) != "no" {
		t.Errorf("helper %q", c2.Values.Get(nameHelper))
	}
}

// 루트 단계의 새 사용자 만들기, 포트 사용 중 확인.
func TestFlowNewUserAndPortInUse(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	f.portsInUse["127.0.0.1:8090"] = true
	args := []string{"--service-install", "--user", "dash", "--listen", "127.0.0.1:8090", "--omv-url", "http://127.0.0.1",
		"--smart-refresh-at", "00:00", "--workbench-url", "", "--public-origin", "", "--no-helper"}

	// 포트가 쓰이고 있으면 묻고, 아니요면 1로 멈춘다.
	script := &prompt.Script{Answers: map[string]string{"port-continue": "n"}}
	c, out, _ := newTestContext(t, f, script, args...)
	c.IsRoot = true
	if code, _ := ExitCode(Run(c)); code != 1 || !strings.Contains(out.String(), "port 8090 is already in use") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !slices.Contains(f.log, "useradd --system --user-group --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin dash") {
		t.Errorf("log %v", f.log)
	}

	// 서비스가 이미 돌고 있으면 그 포트는 자기 것이다.
	f.active["onmaru.service"] = true
	c2, out2, _ := newTestContext(t, f, prompt.Fail{}, args...)
	c2.IsRoot = true
	if err := Run(c2); err != nil {
		t.Fatalf("Run: %v\n%s", err, out2)
	}
	if !strings.Contains(out2.String(), "Health check OK") {
		t.Errorf("output:\n%s", out2)
	}
}

func TestFlowServiceInstallNeedsRoot(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	c, _, _ := newTestContext(t, f, prompt.Fail{}, "--service-install")
	if code, msg := ExitCode(Run(c)); code != 1 || !strings.Contains(msg, "must run as root") {
		t.Errorf("got %d %q", code, msg)
	}
}

// 비TTY에서 질문이 필요하면 멈추지 않고 빠진 옵션 목록을 알린다.
func TestFlowNonInteractiveMissing(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	c, _, calls := newTestContext(t, f, prompt.Fail{}, "--no-build", "--user", "onmaru", "--omv-url", "http://127.0.0.1",
		"--smart-refresh-at", "00:00", "--workbench-url", "")
	c.Interactive = false
	code, msg := ExitCode(Run(c))
	if code != 2 || len(*calls) != 0 {
		t.Fatalf("got %d %q", code, msg)
	}
	for _, want := range []string{"--listen ADDR", "--public-origin URL", "--helper | --no-helper", "--elevate-group GROUP"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q missing from %q", want, msg)
		}
	}
	if strings.Contains(msg, "--user") || strings.Contains(msg, "--allow-non-loopback") {
		t.Errorf("unexpected flag in %q", msg)
	}
}

// 비TTY에서 설정을 모두 줘도 진행 확인은 물을 수 없으므로 멈춘다(--upgrade·--service-install·--dry-run은 묻지 않는다).
func TestFlowNonInteractiveProceed(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	c, _, calls := newTestContext(t, f, prompt.Fail{}, allSettingArgs("--no-build")...)
	c.Interactive = false
	code, msg := ExitCode(Run(c))
	if code != 1 || !strings.Contains(msg, "Proceed?") || len(*calls) != 0 {
		t.Errorf("got %d %q", code, msg)
	}
}

func allSettingArgs(extra ...string) []string {
	return append(extra, "--user", "onmaru", "--listen", "unix:/run/onmaru/onmaru.sock", "--omv-url", "http://127.0.0.1",
		"--smart-refresh-at", "00:00", "--workbench-url", "", "--public-origin", "", "--helper", "--elevate-group", "sudo", "--no-web-upgrade")
}

// --dry-run은 일반 사용자로 sudo 없이 끝까지 가며, 파일을 쓰지 않고 쓸 파일의 차이와 명령만 보여 준다.
func TestFlowDryRun(t *testing.T) {
	f := installedFake(t)
	var out testOut
	sys := NewDryRunSystem(f, &out, "en")
	c, _, calls := newTestContext(t, sys, prompt.Fail{}, allSettingArgs("--no-build", "--dry-run")...)
	c.Interactive = false
	c.Out, c.Err = &out, &out
	before, _ := f.read(t, envFile)
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	if len(*calls) != 0 {
		t.Errorf("dry run exec'd sudo: %v", *calls)
	}
	if after, _ := f.read(t, envFile); after != before {
		t.Error("dry run changed the env file")
	}
	if _, ok := f.read(t, "/etc/systemd/system/onmaru.service"); !ok {
		t.Error("unit missing")
	}
	s := out.String()
	for _, want := range []string{
		"[dry-run] write /etc/default/onmaru (0644, changes):",
		"    - ONMARU_LISTEN=127.0.0.1:8090",
		"    + ONMARU_LISTEN=unix:/run/onmaru/onmaru.sock",
		"[dry-run] run: cp -p -- /etc/default/onmaru /etc/default/onmaru.bak-",
		"[dry-run] run: install -o root -g root -m 0755 /repo/bin/onmaru /usr/local/bin/onmaru",
		"[dry-run] run: systemctl daemon-reload",
		"[dry-run] check http://localhost/api/health",
		"Caddy:  reverse_proxy unix//run/onmaru/onmaru.sock",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
	// 읽기 전용 확인(systemctl is-active)만 실제로(여기서는 가짜에) 실행한다.
	for _, l := range f.log {
		if !strings.HasPrefix(l, "systemctl is-active") {
			t.Errorf("dry run ran %q", l)
		}
	}
}

// 제거: 확인을 받고 서비스·도우미·설정을 지운다. 상태 디렉터리와 설치기가 만든 형태의 사용자는 따로 묻는다.
func TestFlowUninstall(t *testing.T) {
	f := installedFake(t)
	f.put(t, stateDir+"/site.json", "{}", 0o600)
	f.put(t, runtimeDir+"/onmaru.sock", "", 0o660)
	f.put(t, "/etc/default/onmaru-other", "keep\n", 0o644)
	f.put(t, config.InstallRecordFile, "{}\n", 0o644)
	f.put(t, config.UpgradeEnvFile, "ONMARU_UPGRADE_REPO=/repo\n", 0o644)
	f.put(t, upgradeServiceUnit, "x", 0o644)
	f.put(t, upgradePathUnit, "x", 0o644)
	f.put(t, upgradeInstallerDst, "x", 0o755)
	f.put(t, upgradeCheckRequest, "{}", 0o644)
	f.put(t, upgradeApplyRequest, "{}", 0o600)
	f.unitFiles["onmaru.service"] = true
	script := &prompt.Script{Answers: map[string]string{"uninstall": "y", "uninstall-state-dir": "y", "uninstall-user": "y"}}
	c, out, _ := newTestContext(t, f, script, "--uninstall")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	for _, p := range append(configFiles, binDst, helperBinDst, stateDir+"/site.json", runtimeDir+"/onmaru.sock",
		upgradeInstallerDst, upgradeCheckRequest, upgradeApplyRequest) {
		if _, ok := f.read(t, p); ok {
			t.Errorf("%s not removed", p)
		}
	}
	for _, dir := range []string{config.UpgradeStateDir, config.UpgradeRequestDir, upgradeInstallerDir} {
		if _, err := os.Stat(f.path(dir)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s not removed (%v)", dir, err)
		}
	}
	// 설치기가 만들지 않은 파일은 건드리지 않는다.
	if _, ok := f.read(t, "/etc/default/onmaru-other"); !ok {
		t.Error("unrelated file removed")
	}
	want := []string{
		"systemctl list-unit-files onmaru.service",
		"systemctl disable --now onmaru.service",
		"systemctl disable --now onmaru-upgrade.path",
		"systemctl daemon-reload",
		"systemctl disable --now onmaru-helper.socket onmaru-helper.service",
		"systemctl daemon-reload",
		"systemctl daemon-reload",
		"userdel onmaru",
	}
	if !slices.Equal(f.log, want) {
		t.Errorf("log %q", f.log)
	}
	if !slices.Equal(script.Asked, []string{"uninstall", "uninstall-state-dir", "uninstall-user"}) {
		t.Errorf("asked %v", script.Asked)
	}
}

// 제거를 확인하지 않으면 아무것도 하지 않고 0으로 끝낸다. 공용 계정(홈이 있는 계정)은 지우자고 묻지 않는다.
func TestFlowUninstallDeclinedAndSharedUser(t *testing.T) {
	f := installedFake(t)
	c, _, _ := newTestContext(t, f, &prompt.Script{Answers: map[string]string{"uninstall": ""}}, "--uninstall")
	c.IsRoot = true
	if code, msg := ExitCode(Run(c)); code != 0 || msg != "" {
		t.Fatalf("got %d %q", code, msg)
	}
	if _, ok := f.read(t, envFile); !ok || len(f.log) != 0 {
		t.Fatalf("declined uninstall changed something: %v", f.log)
	}

	f.users["onmaru"] = Account{Name: "onmaru", UID: 1000, Home: "/home/onmaru"}
	script := &prompt.Script{Answers: map[string]string{"uninstall": "y"}}
	c2, out, _ := newTestContext(t, f, script, "--uninstall")
	c2.IsRoot = true
	if err := Run(c2); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	if slices.Contains(f.log, "userdel onmaru") {
		t.Error("shared user deleted")
	}
}

// 일반 사용자의 제거는 sudo로 다시 실행한다.
func TestFlowUninstallReexec(t *testing.T) {
	f := installedFake(t)
	c, _, calls := newTestContext(t, f, prompt.Fail{}, "--uninstall")
	if err := Run(c); !errors.Is(err, errExecIntercepted) {
		t.Fatal(err)
	}
	if argv := (*calls)[0].argv; !slices.Equal(argv, []string{"sudo", "/repo/bin/onmaru-install", "--uninstall", "--lang", "en"}) {
		t.Errorf("argv %q", argv)
	}
}

// root가 남의 저장소에서 빌드하려 하면 빌드하지 않는다. --upgrade는 묻지 않고 멈추고, 아니면 기존 실행 파일로
// 설치할지 묻는다.
func TestFlowRootOnForeignRepo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("repository owner check needs a non-root test user")
	}
	f := installedFake(t)
	c, _, _ := newTestContext(t, f, prompt.Fail{}, "--upgrade")
	c.IsRoot = true
	if code, msg := ExitCode(Run(c)); code != 1 || !strings.Contains(msg, "running as root on a repository owned by") {
		t.Errorf("upgrade: %d %q", code, msg)
	}
	script := &prompt.Script{Answers: map[string]string{"install-existing": "n"}}
	c2, out, _ := newTestContext(t, f, script, allSettingArgs()...)
	c2.IsRoot = true
	if code, _ := ExitCode(Run(c2)); code != 1 || !strings.Contains(out.String(), "the build is skipped") {
		t.Errorf("code %d\n%s", code, out)
	}
	if len(f.log) != 0 {
		t.Errorf("ran %v", f.log)
	}
}

// 빌드: 처음 클론한 상태면 서브모듈과 npm 의존성부터 준비한다.
func TestFlowBuildStep(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	f.put(t, "/repo/frontend/package-lock.json", "{}", 0o644)
	c, out, _ := newTestContext(t, f, prompt.Fail{}, allSettingArgs("--dry-run")...)
	c.Sys = NewDryRunSystem(f, out, "en")
	c.Interactive = false
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	want := []string{
		"git -C /repo submodule update --init --depth 1",
		"npm --prefix /repo/frontend ci",
		"make -C /repo build",
	}
	var got []string
	for _, l := range f.log {
		if !strings.HasPrefix(l, "systemctl") {
			got = append(got, l)
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("build commands %q", got)
	}

	f.noCommands["npm"] = true
	c2, _, _ := newTestContext(t, f, prompt.Fail{}, allSettingArgs("--dry-run")...)
	c2.Sys = NewDryRunSystem(f, &testOut{}, "en")
	c2.Interactive = false
	if code, msg := ExitCode(Run(c2)); code != 1 || !strings.Contains(msg, "'npm' not found in PATH") {
		t.Errorf("got %d %q", code, msg)
	}
}

// 대화형 설정 질문: 모든 주소의 TCP는 위험 동의(yes)를 받고 allow-non-loopback을 함께 정한다.
func TestAskListenAnyAddress(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	script := &prompt.Script{Answers: map[string]string{"listen-mode": listenAny, "listen": "", "listen-risk": "yes"}}
	c, _, _ := newTestContext(t, f, script)
	c.Installed = &Installed{}
	if err := askListen(c); err != nil {
		t.Fatal(err)
	}
	if c.Values.Get(nameListen) != "0.0.0.0:8090" || !c.Values.Bool(nameAllowNonLoopback) {
		t.Errorf("values %v", c.Values)
	}
	script.Answers["listen-risk"] = "no"
	if code, msg := ExitCode(askListen(c)); code != 1 || msg != "aborted" {
		t.Errorf("got %d %q", code, msg)
	}
	// 기존 설치가 Unix 소켓이면 기본 선택도 Unix 소켓이다.
	c.Installed = &Installed{Env: ParseEnv([]byte("ONMARU_LISTEN=unix:/run/onmaru/onmaru.sock\nONMARU_ALLOW_NON_LOOPBACK=true\n"))}
	script.Answers = map[string]string{"listen-mode": ""}
	if err := askListen(c); err != nil {
		t.Fatal(err)
	}
	if c.Values.Get(nameListen) != "unix:/run/onmaru/onmaru.sock" || c.Values.Bool(nameAllowNonLoopback) {
		t.Errorf("values %v", c.Values)
	}
}

// OMV가 HTTPS를 강제하면 경고하고 계속할지 묻는다(--upgrade는 알리기만 한다).
func TestNoticeForceSSL(t *testing.T) {
	f := newFake(t)
	f.put(t, nginxWebGUI, "server {\n    listen 80;\n    return 301 https://$host$request_uri;\n}\n", 0o644)
	script := &prompt.Script{Answers: map[string]string{"force-ssl-continue": "n"}}
	c, out, _ := newTestContext(t, f, script)
	if code, _ := ExitCode(noticeForceSSL(c)); code != 1 || !strings.Contains(out.String(), "Force SSL/TLS") {
		t.Errorf("code %d\n%s", code, out)
	}
	c2, _, _ := newTestContext(t, f, prompt.Fail{}, "--upgrade")
	c2.mode = modeUpgrade
	if err := noticeForceSSL(c2); err != nil {
		t.Error(err)
	}
}
