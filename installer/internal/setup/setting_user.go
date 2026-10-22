package setup

import (
	"errors"
	"regexp"

	"maru/backend/config"
)

// userNameRE는 서비스 사용자 이름이다(useradd에 넘기는 시스템 사용자 이름. 설치기만의 규칙).
var userNameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// settingUser는 maru를 실행할 서비스 사용자다. 없으면 시스템 사용자로 만든다. 도우미는 이 사용자의 연결만 받는다.
var settingUser = &Setting{
	Name:  "user",
	Flag:  FlagValue,
	Arg:   "NAME",
	Usage: L("service user (default maru)", "서비스 사용자(기본값 maru)"),
	Env: []EnvLine{{
		File:    HelperEnv,
		Comment: "Only this user may connect to the helper socket.",
		Key:     config.EnvHelperClientUser,
	}},
	Default: func(c *Context) string {
		if u := c.Installed.UnitUser; u != "" {
			return u
		}
		return serviceName
	},
	Load: func(c *Context) (string, bool) {
		u := c.Installed.UnitUser
		return u, u != ""
	},
	Prompt: &PromptSpec{
		Title: L("Service user (created as a system user if missing)", "서비스 사용자(없으면 시스템 사용자로 만듭니다)"),
	},
	Validate: func(c *Context, v string) error {
		if !userNameRE.MatchString(v) {
			return errors.New(c.L("invalid user name: "+v, "사용자 이름이 올바르지 않습니다: "+v))
		}
		if v == "root" {
			return errors.New(c.L("do not run maru as root", "maru를 root로 실행하지 마세요"))
		}
		return nil
	},
	Summary: &SummarySpec{Label: L("service user", "서비스 사용자")},
}
