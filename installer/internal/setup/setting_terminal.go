package setup

import "maru/backend/config"

// settingTerminal은 웹 터미널 설정 줄이다. 설치기는 묻지 않으므로(설정 앱에서도 켠다) 기존 파일의 값을 그대로 옮기고,
// 없으면 기본값을 주석으로 적는다. 손으로 고친 값이 업그레이드로 사라지지 않게 한다.
var settingTerminal = &Setting{
	Name: nameTerminal,
	Flag: FlagNone,
	Env: []EnvLine{
		{
			File:     MaruEnv,
			Comment:  "Web terminal (SSH to this host's sshd). Off by default; admins can also turn it on in the settings app.\ninstall.sh keeps these lines as they are.",
			Key:      config.EnvTerminal,
			Preserve: true,
			Default:  "false",
		},
		{File: MaruEnv, Key: config.EnvSSHAddr, Preserve: true, Default: config.DefaultSSHAddr},
		{File: MaruEnv, Key: config.EnvSSHHostKeys, Preserve: true, Default: config.DefaultSSHHostKeys},
	},
}
