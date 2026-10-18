package setup

import (
	"errors"

	"onmaru/backend/config"
)

// settingHelper는 권한 도우미(onmaru-helper, root로 도는 소켓 활성화 서비스)를 설치할지다. 기본은 설치한다.
// 설치하지 않기로 하면 이미 설치된 도우미를 지운다.
var settingHelper = &Setting{
	Name: nameHelper,
	Flag: FlagToggle,
	Usage: L("install, or do not install (remove), the privileged helper (onmaru-helper, runs as root)",
		"권한 도우미(root로 도는 onmaru-helper)를 설치하거나 설치하지 않습니다(제거)"),
	Env: []EnvLine{{
		File:    OnmaruEnv,
		Comment: "Privileged helper socket; empty means the helper is not installed.",
		Key:     config.EnvHelperSocket,
		Value: func(c *Context) string {
			if helperWanted(c) {
				return helperSocket
			}
			return ""
		},
	}},
	// 기존 설치가 도우미를 설치하지 않기로 했으면(ONMARU_HELPER_SOCKET=이 빈 값) 그 선택을 기본값으로 둔다.
	Default: func(c *Context) string {
		if v, ok := c.Installed.Env.Get(config.EnvHelperSocket); ok && v == "" {
			return "no"
		}
		return "yes"
	},
	// 도우미를 설치하지 않기로 한 설치도 ONMARU_HELPER_SOCKET=(빈 값)을 남기므로 키가 있으면 정한 것이다.
	Load: func(c *Context) (string, bool) {
		v, ok := c.Installed.Env.Get(config.EnvHelperSocket)
		if !ok {
			return "", false
		}
		if v != "" && c.Installed.HelperServiceExists {
			return "yes", true
		}
		return "no", true
	},
	Prompt: &PromptSpec{
		Kind:  PromptConfirm,
		Title: L("Install the privileged helper?", "권한 도우미를 설치할까요?"),
		Help: L(`The privileged helper (onmaru-helper) is a separate service that runs as root.
It lets onmaru act with each signed-in user's own Linux account (for example for file access) and,
after an administrator enters the password again, run a fixed set of root operations for 5 minutes.
Without it, features that need it are unavailable. See README.md, "Privileged helper".`,
			`권한 도우미(onmaru-helper)는 root로 도는 별도 서비스입니다.
로그인한 사용자 자신의 Linux 계정 권한으로(예: 파일 접근), 그리고 관리자가 비밀번호를 다시 입력하면
5분 동안 정해진 root 연산을 onmaru가 실행할 수 있게 합니다. 설치하지 않으면 그런 기능을 쓸 수 없습니다.
README.md의 "권한 도우미"를 참고하세요.`),
	},
	Validate: func(c *Context, v string) error {
		if v != "yes" && v != "no" {
			return errors.New(c.L("choose --helper or --no-helper", "--helper 또는 --no-helper를 고르세요"))
		}
		return nil
	},
	Summary: &SummarySpec{
		Label: L("helper", "권한 도우미"),
		Value: func(c *Context, v string) string {
			if v == "yes" {
				return c.L("install", "설치") + " (" + helperBinDst + ", " + c.L("elevate group", "상승 그룹") + " " + c.Values.Get(nameElevateGroup) + ")"
			}
			return c.L("do not install", "설치 안 함")
		},
	},
}

// settingElevateGroup은 root 권한 상승에 openmediavault-admin과 함께 필요한 그룹이다. 도우미를 설치할 때만 쓴다.
var settingElevateGroup = &Setting{
	Name: nameElevateGroup,
	Flag: FlagValue,
	Arg:  "GROUP",
	Usage: L("group required together with openmediavault-admin to elevate to root (default sudo)",
		"root 권한 상승에 openmediavault-admin과 함께 필요한 그룹(기본값 sudo)"),
	Env: []EnvLine{{
		File:    HelperEnv,
		Comment: "Members of openmediavault-admin who are also in this group may elevate to root.",
		Key:     config.EnvHelperElevateGroup,
	}},
	When: func(v Values) bool { return v.Get(nameHelper) == "yes" },
	Default: func(c *Context) string {
		if v := c.Installed.HelperEnv.Value(config.EnvHelperElevateGroup); v != "" {
			return v
		}
		return config.DefaultElevateGroup
	},
	Load: func(c *Context) (string, bool) {
		v := c.Installed.HelperEnv.Value(config.EnvHelperElevateGroup)
		return v, v != ""
	},
	Prompt: &PromptSpec{
		Title: L("Group that may elevate to root (members must also be in openmediavault-admin)",
			"root 권한으로 상승할 수 있는 그룹(openmediavault-admin에도 속해야 합니다)"),
	},
	Validate: func(c *Context, v string) error {
		if !config.ValidGroupName(v) {
			return errors.New(c.L("invalid group name: "+v, "그룹 이름이 올바르지 않습니다: "+v))
		}
		return nil
	},
}
