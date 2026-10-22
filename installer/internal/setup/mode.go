package setup

import (
	"maru/installer/internal/prompt"
)

// installMode는 이 실행의 설치 방식이다. 1단계와 직접 실행한 2단계가 같은 규칙(resolveMode)으로 정한다.
type installMode int

const (
	// modeFresh는 기존 설치가 없는 새 설치다. 옵션으로 주지 않은 설정을 모두 묻는다.
	modeFresh installMode = iota
	// modeReinstall은 다시 설치다. 옵션으로 주지 않은 설정을 모두 다시 묻고 기존 설치의 값을 기본값으로 쓴다
	// (--reinstall, 일부만 설치된 상태, 업그레이드 중 "설정 바꾸기").
	modeReinstall
	// modeUpgrade는 기존 설치의 설정으로 하는 업그레이드다(기존 설치가 있을 때의 기본, 또는 --upgrade). 기존 설치에
	// 없는 설정과 이 버전의 검사를 통과하지 못한 설정만 묻는다.
	modeUpgrade
)

// upgrading은 업그레이드(자동 감지 또는 --upgrade)인지다. 경고만 하고 묻지 않는 동작, 리버스 프록시 안내 생략이
// 이것을 따른다.
func (c *Context) upgrading() bool { return c.mode == modeUpgrade }

// autoUpgrade는 --upgrade 없이 기존 설치를 찾아 업그레이드하는지다. 요약 뒤에 업그레이드·설정 바꾸기·취소를 고른다.
func (c *Context) autoUpgrade() bool { return c.mode == modeUpgrade && !c.Opts.Upgrade }

// offerChange는 요약 뒤에 "설정 바꾸기"를 고를 수 있는지다(터미널에서 실제로 설치할 때만).
func (c *Context) offerChange() bool { return c.autoUpgrade() && c.Interactive && !c.Sys.DryRun() }

// resolveMode는 설치 방식을 정한다. 설치되어 있다는 것은 유닛 파일과 /etc/default/maru가 모두 있다는 뜻이다.
//   - --upgrade: 기존 설치가 있어야 한다(없으면 오류)
//   - --reinstall: 모든 설정을 다시 묻는다(새 시스템에서는 새 설치와 같다)
//   - 옵션 없음: 설치되어 있으면 업그레이드, 둘 중 하나만 있으면 경고하고 다시 설치, 둘 다 없으면 새 설치
func resolveMode(c *Context) error {
	inst := c.Installed
	installed := inst.UnitExists && inst.Env != nil
	switch {
	case c.Opts.Upgrade:
		if !installed {
			return fail(c.L("--upgrade needs an existing installation ("+unitFile+", "+envFile+"). To install, run "+c.entry()+" without --upgrade (or with --reinstall).",
				"--upgrade는 기존 설치("+unitFile+", "+envFile+")가 있어야 합니다. 설치하려면 --upgrade 없이(또는 --reinstall로) "+c.entry()+" 를 실행하세요."))
		}
		c.mode = modeUpgrade
	case c.Opts.Reinstall:
		c.mode = modeReinstall
	case installed:
		c.mode = modeUpgrade
	case inst.UnitExists || inst.Env != nil:
		have := unitFile
		if !inst.UnitExists {
			have = envFile
		}
		c.Warn(L("partial installation found (only "+have+" exists): every setting is asked, with what exists as defaults.",
			"일부만 설치된 상태입니다("+have+"만 있음). 있는 값을 기본값으로 모든 설정을 묻습니다."))
		c.mode = modeReinstall
	default:
		c.mode = modeFresh
	}
	return nil
}

// stage2ModeArgs는 2단계가 1단계와 같은 방식으로 설치하도록 넘길 옵션이다. 2단계가 기존 설치를 보고 방식을 다시
// 정하지 않게 한다("설정 바꾸기" 뒤에는 업그레이드가 아니다).
func stage2ModeArgs(c *Context) []string {
	switch c.mode {
	case modeUpgrade:
		return []string{"--upgrade"}
	case modeReinstall:
		return []string{"--reinstall"}
	}
	return nil
}

// announceMode는 질문 전에 설치 방식을 알린다.
func announceMode(c *Context) {
	switch {
	case c.mode == modeUpgrade && c.Opts.Upgrade:
		c.Info(L("Upgrading with the installed settings.", "기존 설치의 설정으로 업그레이드합니다."))
	case c.mode == modeUpgrade && c.offerChange():
		c.Info(L(`Existing installation found: upgrading with the installed settings. To change settings, run with --reinstall (or choose "Change settings").`,
			`기존 설치를 찾았습니다: 설치된 설정으로 업그레이드합니다. 설정을 바꾸려면 --reinstall로 실행하세요(또는 "설정 바꾸기"를 고르세요).`))
	case c.mode == modeUpgrade:
		c.Info(L("Existing installation found: upgrading with the installed settings. To change settings, run with --reinstall.",
			"기존 설치를 찾았습니다: 설치된 설정으로 업그레이드합니다. 설정을 바꾸려면 --reinstall로 실행하세요."))
	case c.Installed.UnitExists || c.Installed.Env != nil:
		c.Info(L("Existing installation found. Current settings are used as defaults.",
			"기존 설치를 찾았습니다. 현재 설정을 기본값으로 씁니다."))
	}
	if !c.upgrading() {
		return
	}
	// 기존 설치에 없는 설정(이 버전에서 새로 생긴 설정)과 기존 값이 잘못된 설정을 나눠 알린다.
	newSettings := false
	for _, s := range c.Reg.missing(c) {
		if _, bad := c.invalid[s.Name]; !bad {
			newSettings = true
		}
	}
	if newSettings {
		c.Info(L("Some settings are new in this version. Please answer them.",
			"이 버전에서 새로 생긴 설정이 있습니다. 그 설정만 입력해 주세요."))
	}
	if !c.Interactive {
		return
	}
	for _, s := range c.Reg {
		if err, bad := c.invalid[s.Name]; bad {
			c.Warn(L("--"+s.Name+" from the existing installation is not valid for this version: "+err.Error(),
				"기존 설치의 --"+s.Name+" 값이 이 버전의 검사를 통과하지 못했습니다: "+err.Error()))
		}
	}
}

// 업그레이드 요약 뒤의 선택지
const (
	actionUpgrade = "upgrade"
	actionChange  = "change"
	actionCancel  = "cancel"
)

// chooseUpgradeAction은 자동 감지한 업그레이드의 요약 뒤에 업그레이드·설정 바꾸기·취소를 고르게 한다.
func chooseUpgradeAction(c *Context) error {
	choice, err := c.P.Select(prompt.Select{
		ID:    "upgrade-action",
		Title: c.L("Upgrade with these settings?", "이 설정으로 업그레이드할까요?"),
		Options: []prompt.Option{
			{Value: actionUpgrade, Label: c.L("Upgrade", "업그레이드")},
			{Value: actionChange, Label: c.L("Change settings", "설정 바꾸기"),
				Help: c.L("ask every setting again, with the current values as defaults", "지금 값을 기본값으로 모든 설정을 다시 묻습니다")},
			{Value: actionCancel, Label: c.L("Cancel", "취소")},
		},
		Default: actionUpgrade,
	})
	if err != nil {
		return err
	}
	switch choice {
	case actionCancel:
		return quit(0)
	case actionChange:
		return changeSettings(c)
	}
	return nil
}

// changeSettings는 업그레이드를 그 자리에서 다시 설치로 바꾼다. 지금 값(기존 설치와 옵션)을 기본값으로 모든 설정을
// 다시 묻고, 요약을 보여 주고, 진행을 확인한다.
func changeSettings(c *Context) error {
	c.mode = modeReinstall
	c.invalid = nil
	c.prefill = Values{}
	for k, v := range c.Values {
		c.prefill[k] = v
	}
	for _, s := range c.Reg {
		if s.asked() {
			c.Values.Unset(s.Name)
		}
	}
	// 루프백 외 주소 허용은 수신 방식 질문이 다시 정한다.
	c.Values.Unset(nameAllowNonLoopback)
	c.Info(L("Changing settings: every setting is asked again, with the current values as defaults.",
		"설정을 바꿉니다: 지금 값을 기본값으로 모든 설정을 다시 묻습니다."))
	if err := c.Reg.ask(c); err != nil {
		return err
	}
	if err := c.Reg.validate(c); err != nil {
		return err
	}
	if err := c.P.Note(c.L("Summary", "요약"), c.Reg.summary(c)); err != nil {
		return err
	}
	return confirmProceed(c)
}

// confirmProceed는 요약 뒤에 진행을 확인한다. 아니요면 아무것도 하지 않고 0으로 끝낸다. 물을 수 없으면(비TTY) 멈춘다.
func confirmProceed(c *Context) error {
	if !c.Interactive {
		return fail(c.L("cannot ask \"Proceed?\": standard input is not a terminal. Without a terminal, upgrade an existing installation (no option or --upgrade), or run --service-install as root with every setting option.",
			"\"진행할까요?\" 질문을 할 수 없습니다: 표준 입력이 터미널이 아닙니다. 터미널 없이는 기존 설치를 업그레이드하거나(옵션 없음 또는 --upgrade), root로 설정 옵션을 모두 준 --service-install을 실행하세요."))
	}
	ok, err := c.P.Confirm(prompt.Confirm{ID: "proceed", Title: c.L("Proceed?", "진행할까요?"), Default: true})
	if err != nil {
		return err
	}
	if !ok {
		return quit(0)
	}
	return nil
}
