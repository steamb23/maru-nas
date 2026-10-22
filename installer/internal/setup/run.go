// Package setup은 마루 NAS 설치기다. 설정 레지스트리(settings.go)와 설치·제거 단계(steps.go)로 install.sh가 하던 일을 한다.
//
//	./install.sh                    1단계: 설정을 묻고 실행한 계정으로 빌드한다(일반 사용자 또는 root만 있는 환경의 root).
//	                                이미 설치되어 있으면 설치된 설정으로 업그레이드한다(mode.go)
//	                                2단계: root가 아니면 sudo로 bin/maru-install --service-install을 실행한다
//	./install.sh --reinstall        설치된 값을 기본값으로 모든 설정을 다시 묻는다
//	./install.sh --upgrade          기존 설치의 설정 그대로 묻지 않고 빌드·설치한다(스크립트용). 기존 설치에 없는 설정만 묻는다
//	./install.sh --service-install  2단계만 실행한다(root 전용). 설정을 모두 주면 묻지 않는다
//	./install.sh --uninstall        제거한다(root가 아니면 sudo로 다시 실행한다)
//	./install.sh --dry-run          바꾸는 작업 없이 쓸 파일의 차이와 실행할 명령만 보여 준다
//	./install.sh --upgrade-plan     기존 설치를 읽기만 하고 이 버전으로의 업그레이드 계획을 JSON으로 출력한다(plan.go)
//	maru-install --upgrade-service  대시보드에서 업그레이드의 요청 하나를 처리한다(maru-upgrade.service, upgrade_service.go)
//
// 리버스 프록시 설정은 하지 않는다. 생성하는 설정 파일(유닛, /etc/default/*)의 주석은 언어와 상관없이 영어로 쓴다.
package setup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"

	"maru/installer/internal/prompt"
)

// Main은 설치기를 실행하고 종료 코드를 돌려준다.
func Main(args []string) int {
	lang := DetectLang(os.Getenv)
	opts, values, perr := parseArgs(settings, args)
	if opts.Lang != "" {
		lang = opts.Lang
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot find the installer executable:", err)
		return 1
	}
	// 설치기는 저장소의 bin/maru-install이다.
	repo := filepath.Dir(filepath.Dir(exe))
	name := entryName(repo)
	if perr != nil {
		fmt.Fprintf(os.Stderr, "%s\n\n%s", perr, usage(settings, lang, name))
		return 2
	}
	if opts.Help {
		fmt.Print(usage(settings, lang, name))
		return 0
	}

	stdinTTY, stdoutTTY := isTerminal(os.Stdin.Fd()), isTerminal(os.Stdout.Fd())
	tui := stdinTTY && stdoutTTY && os.Getenv("TERM") != "dumb" && !opts.Plain
	sys := NewRealSystem(os.Stdout, os.Stderr)
	if opts.DryRun {
		sys = NewDryRunSystem(sys, os.Stdout, lang)
	}
	self, _ := debug.ReadBuildInfo()
	c := &Context{
		Lang:        lang,
		Sys:         sys,
		P:           newPrompter(tui, lang),
		Out:         os.Stdout,
		Err:         os.Stderr,
		Interactive: stdinTTY,
		Color:       stdoutTTY,
		IsRoot:      os.Geteuid() == 0,
		RepoDir:     repo,
		Self:        exe,
		Exec:        syscall.Exec,
		Environ:     os.Environ(),
		LookPath:    exec.LookPath,
		SelfBuild:   self,
		Reg:         settings,
		Opts:        opts,
		Values:      values,
	}
	err = Run(c)
	code, msg := ExitCode(err)
	switch {
	case errors.Is(err, prompt.ErrAborted):
		msg = c.L("aborted", "중단했습니다")
	case errors.Is(err, prompt.ErrNoInput):
		msg = c.L("no answer: standard input was closed", "답을 받지 못했습니다: 표준 입력이 닫혔습니다")
	}
	if msg != "" {
		fmt.Fprintf(os.Stderr, "%s %s\n", c.color("1;31", c.L("error:", "오류:")), msg)
	}
	return code
}

// newPrompter는 질문 방식을 고른다. 표준 입력·출력이 터미널이고 TERM이 dumb가 아니며 --plain이 아니면 TUI다.
func newPrompter(tui bool, lang string) prompt.Prompter {
	if tui {
		return prompt.NewHuh(lang)
	}
	return prompt.NewPlain(os.Stdin, os.Stderr, lang)
}

// entryName은 사용법에 보일 설치 명령이다. 저장소 루트에서 실행했으면 ./install.sh다.
func entryName(repo string) string {
	if wd, err := os.Getwd(); err == nil && wd == repo {
		return "./install.sh"
	}
	return filepath.Join(repo, "install.sh")
}

// Run은 옵션에 따라 설치·2단계 설치·제거를 한다.
func Run(c *Context) error {
	if c.Values == nil {
		c.Values = Values{}
	}
	inst, err := readInstalled(c.Sys)
	if err != nil {
		return err
	}
	c.Installed = inst
	switch c.Opts.Action {
	case ActionUpgradePlan:
		// 계획은 읽기만 한다. 설치 방식을 정하지 않고(방식 안내·일부 설치 경고가 필요 없다) 아무것도 묻지 않는다.
		c.P = prompt.Fail{}
		return runUpgradePlan(c)
	case ActionUpgradeService:
		// 업그레이드 서비스는 저장소 밖의 root 소유 복사본으로 돌므로 c.RepoDir를 쓰지 않는다(저장소는 설정 파일에서 정한다).
		c.P = prompt.Fail{}
		if err := requireSystemd(c); err != nil {
			return err
		}
		return runUpgradeService(c)
	case ActionUninstall:
		if !c.IsRoot && !c.Sys.DryRun() {
			return reexecSudo(c, "--uninstall", nil)
		}
		if err := requireSystemd(c); err != nil {
			return err
		}
		return runSteps(c, uninstallSteps, PhaseRoot)
	case ActionServiceInstall:
		if !c.IsRoot && !c.Sys.DryRun() {
			return fail(c.L("--service-install must run as root. Run "+c.entry()+" without options instead.",
				"--service-install은 root로만 실행할 수 있습니다. 대신 옵션 없이 "+c.entry()+" 를 실행하세요."))
		}
		if err := requireSystemd(c); err != nil {
			return err
		}
		if err := resolveMode(c); err != nil {
			return err
		}
		return installService(c)
	}
	if err := resolveMode(c); err != nil {
		return err
	}
	return runInstall(c)
}

// runInstall은 1단계다. 질문은 빌드 전에 모두 받고, 2단계는 이 값을 옵션으로 받아 묻지 않고 설치한다.
func runInstall(c *Context) error {
	c.build = !c.Opts.NoBuild
	// root로 다른 사용자의 저장소를 빌드하면 root 소유 파일이 생긴다(sudo, sudo -i, su 모두).
	// 저장소 소유자가 root가 아니면 root로 빌드하지 않는다.
	if c.build && c.IsRoot {
		if uid, owner := repoOwner(c); uid != 0 {
			// --upgrade(또는 물을 수 없는 자동 업그레이드)는 묻지 않으므로, 빌드하지 않은 예전 실행 파일을 몰래
			// 설치하지 않고 멈춘다.
			if c.Opts.Upgrade || (c.autoUpgrade() && !c.Interactive && !c.Sys.DryRun()) {
				return fail(c.L("running as root on a repository owned by "+owner+": the build would create root-owned files. Run "+c.entry()+" as that user without sudo, or add --no-build to install the existing "+c.binSrc()+".",
					"root로 "+owner+" 소유 저장소에서 실행했습니다. root로 빌드하면 root 소유 파일이 생깁니다. 그 사용자로 sudo 없이 "+c.entry()+" 를 실행하거나, 기존 "+c.binSrc()+" 를 설치하려면 --no-build를 더하세요."))
			}
			c.Warn(L("running as root on a repository owned by "+owner+": the build is skipped to avoid root-owned files. To build, run "+c.entry()+" as that user without sudo.",
				"root로 "+owner+" 소유 저장소에서 실행되어 빌드를 건너뜁니다(root 소유 파일이 생기지 않게). 빌드하려면 그 사용자로 sudo 없이 "+c.entry()+" 를 실행하세요."))
			ok, err := c.confirm(prompt.Confirm{ID: "install-existing", Title: c.L("Install the existing "+c.binSrc()+" instead?", "대신 기존 "+c.binSrc()+" 로 설치할까요?")})
			if err != nil {
				return err
			}
			if !ok {
				return quit(1)
			}
			c.build = false
		}
	}
	if !c.build {
		if err := requireBinary(c); err != nil {
			return err
		}
	}
	if err := settingsPhase(c, true); err != nil {
		return err
	}
	if err := runSteps(c, installSteps, PhaseUser); err != nil {
		return err
	}
	return asRoot(c)
}

// repoOwner는 저장소 디렉터리 소유자의 uid와 이름이다. 알 수 없으면 uid -1이다.
func repoOwner(c *Context) (int, string) {
	a, err := c.Sys.FileOwner(c.RepoDir)
	if err != nil {
		return -1, "?"
	}
	return a.UID, a.Name
}

// installService는 2단계다(root). 설정이 모두 정해져 있으면 묻지 않고 설치한다. root가 옵션 없이
// --service-install을 직접 실행했다면 여기서 묻는다(업그레이드면 기존 설치에 없는 설정만).
func installService(c *Context) error {
	if err := requireBinary(c); err != nil {
		return err
	}
	if err := settingsPhase(c, false); err != nil {
		return err
	}
	return runSteps(c, installSteps, PhaseRoot)
}

// settingsPhase는 설정을 정한다. 업그레이드면 기존 설치에서 읽고(검사를 통과하지 못한 값은 버리고), 빠진 설정을
// 묻고, 검사하고, 요약을 보여 준 뒤 확인한다. always가 아니면(2단계) 설정이 모두 정해졌을 때 검사만 한다.
//   - 새 설치·다시 설치: "진행할까요?"
//   - 자동 감지한 업그레이드: 터미널이면 업그레이드·설정 바꾸기·취소, 아니면 묻지 않는다
//   - --upgrade, dry-run: 묻지 않는다
func settingsPhase(c *Context, always bool) error {
	if c.upgrading() {
		c.invalid = c.Reg.dropInvalid(c, c.Reg.load(c))
	}
	if !always && c.Reg.complete(c) {
		c.Reg.fill(c)
		return c.Reg.validate(c)
	}
	if err := askSettings(c); err != nil {
		return err
	}
	c.Reg.fill(c)
	if err := c.Reg.validate(c); err != nil {
		return err
	}
	if err := c.P.Note(c.L("Summary", "요약"), c.Reg.summary(c)); err != nil {
		return err
	}
	switch {
	case c.Opts.Upgrade || c.Sys.DryRun():
		return nil
	case c.autoUpgrade():
		if !c.Interactive {
			return nil
		}
		return chooseUpgradeAction(c)
	}
	return confirmProceed(c)
}

// askSettings는 아직 정하지 않은 설정만 묻는다. 기존 설치가 있으면 그 값을 기본값으로 쓴다.
// 물을 수 없으면(비TTY) 빠진 옵션 목록을 알리고 끝낸다.
func askSettings(c *Context) error {
	announceMode(c)
	if missing := c.Reg.missing(c); len(missing) > 0 && !c.Interactive {
		specs := make([]string, len(missing))
		for i, s := range missing {
			specs[i] = s.flagSpec()
		}
		msg := c.L("standard input is not a terminal, so these settings cannot be asked; pass them as options: "+strings.Join(specs, ", "),
			"표준 입력이 터미널이 아니라 다음 설정을 물을 수 없습니다. 옵션으로 주세요: "+strings.Join(specs, ", "))
		var bad []string
		for _, s := range c.Reg {
			if err, ok := c.invalid[s.Name]; ok {
				bad = append(bad, "--"+s.Name+" ("+err.Error()+")")
			}
		}
		if len(bad) > 0 {
			msg += c.L(". Not valid for this version in the existing installation: "+strings.Join(bad, "; ")+". Pass a valid value, or run with --reinstall",
				". 기존 설치의 값이 이 버전의 검사를 통과하지 못했습니다: "+strings.Join(bad, "; ")+". 올바른 값을 옵션으로 주거나 --reinstall로 실행하세요")
		}
		return &exitError{code: 2, msg: msg}
	}
	return c.Reg.ask(c)
}

// asRoot는 2단계로 넘어간다. root(또는 dry-run)면 같은 프로세스에서 이어 가고, 아니면 1단계에서 정한 설정을
// 옵션으로 넘겨 sudo로 설치기를 다시 실행한다(2단계가 다시 묻지 않게).
func asRoot(c *Context) error {
	if c.IsRoot || c.Sys.DryRun() {
		if err := requireSystemd(c); err != nil {
			return err
		}
		return installService(c)
	}
	// 2단계는 설정이 모두 정해졌으므로 묻지 않고, 1단계가 정한 설치 방식을 그대로 따른다.
	args := append(c.Reg.stage2Args(c.Values), stage2ModeArgs(c)...)
	return reexecSudo(c, "--service-install", args)
}

// reexecSudo는 sudo로 설치기를 다시 실행한다(sudo <설치기> <action> --lang <언어> <args>). 같은 언어와 질문 방식을
// 유지한다. 성공하면 돌아오지 않는다.
func reexecSudo(c *Context, action string, args []string) error {
	sudo, err := c.LookPath("sudo")
	if err != nil {
		return fail(c.L("root privileges are required and sudo is not installed. Run this as root.",
			"root 권한이 필요한데 sudo가 없습니다. root로 실행하세요."))
	}
	argv := []string{"sudo", c.Self, action, "--lang", c.Lang}
	argv = append(argv, args...)
	if c.Opts.Plain {
		argv = append(argv, "--plain")
	}
	c.Info(L("Root privileges are needed for the next step; running sudo", "다음 단계에는 root 권한이 필요해 sudo를 실행합니다"))
	if err := c.Exec(sudo, argv, c.Environ); err != nil {
		return fmt.Errorf("exec %s: %w", sudo, err)
	}
	return nil
}

// requireSystemd는 systemctl이 있는지 확인한다.
func requireSystemd(c *Context) error {
	if !c.Sys.LookPath("systemctl") {
		return fail(c.L("systemd is required", "systemd가 필요합니다"))
	}
	return nil
}
