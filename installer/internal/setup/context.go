package setup

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime/debug"
	"strconv"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

// Context는 설치기 한 번의 실행 상태다. 설정 정의와 단계는 이것만 받는다.
type Context struct {
	Lang string
	Sys  System
	P    prompt.Prompter
	Out  io.Writer
	Err  io.Writer
	// Interactive는 사용자에게 물을 수 있는지다(표준 입력이 터미널). 아니면 질문이 필요할 때 멈추고 알린다.
	Interactive bool
	// Color는 출력에 ANSI 색을 쓰는지다.
	Color bool
	// IsRoot는 root로 실행 중인지다.
	IsRoot bool
	// RepoDir은 저장소 루트의 절대 경로다.
	RepoDir string
	// Self는 설치기 실행 파일의 절대 경로다(sudo로 2단계를 실행할 때 쓴다).
	Self string
	// Exec는 현재 프로세스를 다른 프로그램으로 바꾼다(syscall.Exec). 테스트는 가로챈다.
	Exec func(path string, argv []string, env []string) error
	// Environ은 Exec에 넘길 환경 변수다.
	Environ []string
	// LookPath는 sudo의 절대 경로를 찾는다(exec.LookPath).
	LookPath func(name string) (string, error)
	// SelfBuild는 이 설치기 실행 파일의 빌드 정보다(debug.ReadBuildInfo). 없으면 nil이다.
	SelfBuild *debug.BuildInfo

	Reg       Registry
	Opts      Options
	Values    Values
	Installed *Installed

	// mode는 이 실행의 설치 방식이다(resolveMode).
	mode installMode
	// prefill은 "설정 바꾸기"에서 질문의 기본값으로 쓸 지금 값이다.
	prefill Values
	// invalid는 기존 설치에서 읽었지만 이 버전의 검사를 통과하지 못해 다시 묻는 설정과 그 오류다.
	invalid map[string]error

	// 단계 사이에 넘기는 값
	build          bool // 1단계에서 빌드하는지
	distWarned     bool // 프론트엔드 빌드가 없다고 이미 경고했는지
	group          string
	proxyUnchanged bool
	uninstallUser  string
	// upgradePathChanged는 대시보드 업그레이드의 경로 유닛을 새로 쓰거나 바꿨는지다(다시 시작할지).
	upgradePathChanged bool
}

// L은 현재 언어의 문구다.
func (c *Context) L(en, ko string) string {
	if c.Lang == "ko" {
		return ko
	}
	return en
}

// T는 Text의 현재 언어 문구다.
func (c *Context) T(t Text) string { return t.In(c.Lang) }

func (c *Context) color(code, s string) string {
	if !c.Color {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

// Info는 진행 상황을 표준 출력에 쓴다.
func (c *Context) Info(t Text) {
	fmt.Fprintf(c.Out, "%s %s\n", c.color("1;34", "==>"), c.T(t))
}

// Warn은 경고를 오류 출력에 쓴다.
func (c *Context) Warn(t Text) {
	fmt.Fprintf(c.Err, "%s %s\n", c.color("1;33", c.L("warning:", "경고:")), c.T(t))
}

// Print는 한 줄을 그대로 표준 출력에 쓴다.
func (c *Context) Print(s string) { fmt.Fprintln(c.Out, s) }

// confirm은 예·아니요를 묻는다. dry-run은 묻지 않고 "예"로 보고 진행하며 그렇게 했다고 알린다
// (무엇을 할지 모두 보여 주기 위해). 물을 수 없으면(비TTY) 멈춘다.
func (c *Context) confirm(q prompt.Confirm) (bool, error) {
	if c.Sys.DryRun() {
		c.Print(c.L("[dry-run] would ask: "+q.Title+" (continuing as if answered yes)",
			"[dry-run] 질문: "+q.Title+" (예로 답한 것으로 보고 계속합니다)"))
		return true, nil
	}
	if !c.Interactive {
		return false, fail(L(
			fmt.Sprintf("cannot ask %q: standard input is not a terminal", q.Title),
			fmt.Sprintf("%q 질문을 할 수 없습니다: 표준 입력이 터미널이 아닙니다", q.Title)).In(c.Lang))
	}
	return c.P.Confirm(q)
}

// withoutInstalled는 기존 설치를 보지 않는 Context 사본이다(기존 값이 잘못됐을 때의 기본값 계산용).
func (c *Context) withoutInstalled() *Context {
	cc := *c
	cc.Installed = &Installed{}
	return &cc
}

// binSrc 등은 저장소 안의 빌드 결과 경로다.
func (c *Context) binSrc() string       { return filepath.Join(c.RepoDir, binSrcRel) }
func (c *Context) helperBinSrc() string { return filepath.Join(c.RepoDir, helperBinSrcRel) }
func (c *Context) installerBinSrc() string {
	return filepath.Join(c.RepoDir, installerBinSrcRel)
}

// entry는 사용자에게 안내하는 설치 명령(저장소의 install.sh)이다.
func (c *Context) entry() string { return filepath.Join(c.RepoDir, "install.sh") }

// exitError는 종료 코드와 함께 끝내는 오류다. msg가 비어 있으면 아무것도 쓰지 않고 끝낸다.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string {
	if e.msg == "" {
		return "exit status " + strconv.Itoa(e.code)
	}
	return e.msg
}

// fail은 오류 문구를 쓰고 1로 끝내는 오류다.
func fail(msg string) error { return &exitError{code: 1, msg: msg} }

// quit은 아무것도 쓰지 않고 code로 끝내는 오류다(진행 확인에서 아니요 등).
func quit(code int) error { return &exitError{code: code} }

// ExitCode는 Run이 돌려준 오류의 종료 코드와 쓸 문구다.
func ExitCode(err error) (int, string) {
	if err == nil {
		return 0, ""
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code, ee.msg
	}
	if errors.Is(err, prompt.ErrAborted) {
		return 130, err.Error()
	}
	return 1, err.Error()
}

// Values는 설정 이름(플래그 이름)별 값이다. 키가 없으면 아직 정하지 않은 것이다.
type Values map[string]string

func (v Values) Has(name string) bool   { _, ok := v[name]; return ok }
func (v Values) Get(name string) string { return v[name] }
func (v Values) Set(name, value string) { v[name] = value }
func (v Values) Unset(name string)      { delete(v, name) }
func (v Values) Bool(name string) bool  { return v[name] == "true" }
func (v Values) SetBool(name string, b bool) {
	if b {
		v[name] = "true"
	} else {
		delete(v, name)
	}
}

// Installed는 기존 설치(유닛, 환경 파일)에서 읽은 내용이다. 설치 단계가 파일을 다시 쓰기 전의 모습이다.
// 파일은 모두 0644라 일반 사용자도 읽는다.
type Installed struct {
	// UnitExists는 maru 유닛 파일이 있는지다.
	UnitExists bool
	// UnitUser는 유닛의 User= 값이다.
	UnitUser string
	// Env는 /etc/default/maru다. nil이면 없다.
	Env *EnvFile
	// HelperEnv는 /etc/default/maru-helper다. nil이면 없다.
	HelperEnv *EnvFile
	// HelperServiceExists는 도우미 서비스 유닛이 있는지다.
	HelperServiceExists bool
	// UpgradeEnv는 /etc/default/maru-upgrade다(대시보드에서 업그레이드). nil이면 없다(꺼져 있다).
	UpgradeEnv *EnvFile
}

// readInstalled는 기존 설치를 읽는다.
func readInstalled(sys System) (*Installed, error) {
	inst := &Installed{}
	if data, err := sys.ReadFile(unitFile); err == nil {
		inst.UnitExists = true
		inst.UnitUser = unitUser(data)
	}
	var err error
	if inst.Env, err = readEnvFile(sys, envFile); err != nil {
		return nil, err
	}
	if inst.HelperEnv, err = readEnvFile(sys, helperEnvFile); err != nil {
		return nil, err
	}
	if inst.UpgradeEnv, err = readEnvFile(sys, config.UpgradeEnvFile); err != nil {
		return nil, err
	}
	if fi, err := sys.Stat(helperServiceUnit); err == nil && fi.Mode().IsRegular() {
		inst.HelperServiceExists = true
	}
	return inst, nil
}

// exists는 일반 파일이 있는지다(bash의 [[ -f ]]).
func exists(sys System, path string) bool {
	fi, err := sys.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}
