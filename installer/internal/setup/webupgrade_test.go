package setup

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/config"
	"onmaru/installer/internal/prompt"
)

// webUpgradeFake는 helper-web-upgrade 골든 결과(대시보드에서 업그레이드를 허용한 설치)가 설치된 가짜 시스템이다.
// 기존 빌드 PATH는 1단계가 다시 찾는 값(/usr/bin:/usr/local/bin:/bin)과 다르게 둔다.
func webUpgradeFake(t *testing.T) *fakeSystem {
	t.Helper()
	f := newFake(t)
	f.putRepo(t)
	f.copyTree(t, filepath.Join(goldenDir("helper-web-upgrade"), "want"))
	env, _ := f.read(t, config.UpgradeEnvFile)
	f.put(t, config.UpgradeEnvFile, strings.Replace(env, "ONMARU_UPGRADE_PATH=/usr/bin:/usr/local/bin:/bin", "ONMARU_UPGRADE_PATH=/opt/go/bin:/usr/bin", 1), 0o644)
	f.put(t, binDst, "old\n", 0o755)
	f.put(t, helperBinDst, "old\n", 0o755)
	f.put(t, upgradeInstallerDst, "old\n", 0o755)
	f.users["onmaru"] = Account{Name: "onmaru", UID: 999, Home: "/nonexistent"}
	f.groups["onmaru"] = "onmaru"
	return f
}

// 허용하려면 저장소 소유자가 일반 사용자여야 한다(확인과 빌드를 root로 하지 않게). 새 설치의 기본값도 그것을 따른다.
func TestWebUpgradeRepoOwner(t *testing.T) {
	f := newFake(t)
	f.putRepo(t)
	c, _, _ := newTestContext(t, f, prompt.Fail{})
	c.Installed = &Installed{}
	if err := settingWebUpgrade.Validate(c, "yes"); err != nil {
		t.Errorf("alice: %v", err)
	}
	if got := settingWebUpgrade.defaultValue(c); got != "yes" {
		t.Errorf("default for alice's repository %q", got)
	}
	f.owners["/repo"] = Account{Name: "root", UID: 0, Home: "/root"}
	if err := settingWebUpgrade.Validate(c, "yes"); err == nil || !strings.Contains(err.Error(), "owned by root") {
		t.Errorf("root: %v", err)
	}
	if got := settingWebUpgrade.defaultValue(c); got != "no" {
		t.Errorf("default for root's repository %q", got)
	}
	delete(f.owners, "/repo")
	c.RepoDir = "/missing"
	if err := settingWebUpgrade.Validate(c, "yes"); err == nil || !strings.Contains(err.Error(), "cannot find the owner") {
		t.Errorf("missing: %v", err)
	}

	// 루트 소유 저장소에서 허용을 고르면 빌드하기 전에 멈춘다.
	f2 := newFake(t)
	f2.putRepo(t)
	f2.owners["/repo"] = Account{Name: "root", UID: 0, Home: "/root"}
	c2, out, _ := newTestContext(t, f2, prompt.Fail{}, "--service-install", "--user", "onmaru", "--listen", "127.0.0.1:8090",
		"--omv-url", "http://127.0.0.1", "--smart-refresh-at", "00:00", "--workbench-url", "", "--public-origin", "", "--helper",
		"--elevate-group", "sudo", "--web-upgrade", "--build-path", "/usr/bin")
	c2.IsRoot = true
	if code, msg := ExitCode(Run(c2)); code != 1 || !strings.Contains(msg, "--no-web-upgrade") {
		t.Errorf("code %d %q\n%s", code, msg, out)
	}
	if len(f2.log) != 0 {
		t.Errorf("ran %v", f2.log)
	}
}

// 기본 빌드 PATH는 빌드 도구를 찾은 디렉터리(찾은 순서, 중복 없이) 뒤에 시스템 디렉터리를 붙인다.
func TestDefaultBuildPath(t *testing.T) {
	f := newFake(t)
	c := &Context{Lang: "en", Sys: f}
	if got := defaultBuildPath(c); got != "/usr/bin:/usr/local/bin:/bin" {
		t.Errorf("all in /usr/bin: %q", got)
	}
	f.noCommands["node"] = true
	f.noCommands["git"] = true
	sys := &whichSystem{fakeSystem: f, paths: map[string]string{
		"go": "/usr/local/go/bin/go", "npm": "/home/alice/.nvm/versions/node/v22.12.0/bin/npm", "make": "/usr/bin/make",
	}}
	c.Sys = sys
	if got, want := defaultBuildPath(c), "/usr/local/go/bin:/home/alice/.nvm/versions/node/v22.12.0/bin:/usr/bin:/usr/local/bin:/bin"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// whichSystem은 명령마다 찾은 경로를 정하는 fakeSystem이다.
type whichSystem struct {
	*fakeSystem
	paths map[string]string
}

func (w *whichSystem) Which(name string) (string, bool) {
	if p, ok := w.paths[name]; ok {
		return p, true
	}
	return w.fakeSystem.Which(name)
}

// 1단계(저장소 소유자)는 업그레이드할 때도 빌드 PATH를 지금 PATH에서 다시 찾아 2단계에 넘기고, 2단계를 직접 실행하면
// (대시보드에서 업그레이드가 그렇게 한다) 기존 값을 쓴다. 경로 유닛이 그대로면 다시 시작하지 않는다.
func TestWebUpgradeBuildPathStages(t *testing.T) {
	f := webUpgradeFake(t)
	c, out, calls := newTestContext(t, f, prompt.Fail{}, "--upgrade", "--no-build")
	if err := Run(c); !errors.Is(err, errExecIntercepted) {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	argv := (*calls)[0].argv
	i := slices.Index(argv, "--build-path")
	if !slices.Contains(argv, "--web-upgrade") || i < 0 || argv[i+1] != "/usr/bin:/usr/local/bin:/bin" {
		t.Errorf("argv %q", argv)
	}

	c2, out2, _ := newTestContext(t, f, prompt.Fail{}, "--service-install", "--upgrade")
	c2.IsRoot = true
	if err := Run(c2); err != nil {
		t.Fatalf("stage 2: %v\n%s", err, out2)
	}
	env, _ := f.read(t, config.UpgradeEnvFile)
	if !strings.Contains(env, "\nONMARU_UPGRADE_PATH=/opt/go/bin:/usr/bin\n") || !strings.Contains(env, "\nONMARU_UPGRADE_USER=alice\n") {
		t.Errorf("upgrade env\n%s", env)
	}
	if !strings.Contains(out2.String(), config.UpgradeEnvFile+" unchanged") {
		t.Errorf("upgrade env rewritten:\n%s", out2)
	}
	if got, _ := f.read(t, upgradeInstallerDst); got != "#!/bin/sh\n" {
		t.Errorf("installer copy not replaced: %q", got)
	}
	if !slices.Contains(f.log, "systemctl enable --now onmaru-upgrade.path") || slices.ContainsFunc(f.log, func(l string) bool {
		return strings.Contains(l, "restart onmaru-upgrade") || strings.Contains(l, "onmaru-upgrade.service")
	}) {
		t.Errorf("log %q", f.log)
	}
	unit, _ := f.read(t, helperServiceUnit)
	if !strings.Contains(unit, "\nRuntimeDirectory=onmaru-upgrade\n") {
		t.Errorf("helper unit\n%s", unit)
	}
}

// 허용하지 않기로 하거나 도우미를 설치하지 않기로 하면 업그레이드 서비스, 설치기 복사본, 설정, 남은 요청·상태 파일을
// 지우고 도우미 유닛의 요청 디렉터리 줄을 뺀다. 돌고 있는 업그레이드 서비스는 멈추지 않는다.
func TestWebUpgradeRemoved(t *testing.T) {
	for _, toggle := range []string{"--no-web-upgrade", "--no-helper"} {
		t.Run(toggle, func(t *testing.T) {
			f := webUpgradeFake(t)
			f.put(t, upgradeCheckRequest, "{}", 0o644)
			f.put(t, upgradeApplyRequest, "{}", 0o600)
			f.put(t, config.UpgradeStatusFile, "{}", 0o644)
			c, out, _ := newTestContext(t, f, prompt.Fail{}, "--service-install", "--upgrade", toggle)
			c.IsRoot = true
			if err := Run(c); err != nil {
				t.Fatalf("Run: %v\n%s", err, out)
			}
			for _, p := range []string{config.UpgradeEnvFile, upgradeServiceUnit, upgradePathUnit, upgradeInstallerDst,
				upgradeCheckRequest, upgradeApplyRequest, config.UpgradeStatusFile} {
				if _, ok := f.read(t, p); ok {
					t.Errorf("%s not removed", p)
				}
			}
			if _, ok := f.read(t, config.InstallRecordFile); !ok {
				t.Error("install record removed")
			}
			if !slices.Contains(f.log, "systemctl disable --now onmaru-upgrade.path") ||
				slices.ContainsFunc(f.log, func(l string) bool { return strings.Contains(l, "onmaru-upgrade.service") }) {
				t.Errorf("log %q", f.log)
			}
			if unit, ok := f.read(t, helperServiceUnit); ok && strings.Contains(unit, "RuntimeDirectory") {
				t.Errorf("helper unit\n%s", unit)
			}
		})
	}
}
