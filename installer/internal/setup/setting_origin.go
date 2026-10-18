package setup

import (
	"errors"

	"onmaru/backend/config"
)

// settingPublicOrigin은 브라우저가 대시보드에 접속하는 공개 HTTPS 주소다. 보안 강화 모드의 패스키(RP)에 쓴다.
// 빈 값은 패스키를 끈다는 정한 값이다. 질문에서는 -가 빈 값이다.
var settingPublicOrigin = &Setting{
	Name: namePublicOrigin,
	Flag: FlagValueEmptyOK,
	Arg:  "URL",
	Usage: L("public HTTPS address browsers use for the dashboard (for passkeys, empty turns them off)",
		"브라우저가 대시보드에 접속하는 공개 HTTPS 주소(패스키용, 빈 값이면 패스키를 끕니다)"),
	Env: []EnvLine{{
		File:    OnmaruEnv,
		Comment: "Public HTTPS address browsers use for the dashboard, for passkeys (security hardening mode); empty turns passkeys off.",
		Key:     config.EnvPublicOrigin,
	}},
	Default: func(c *Context) string { return c.Installed.Env.Value(config.EnvPublicOrigin) },
	// 빈 값도 정한 값(패스키 끔)이므로 키가 있는지로 판단한다.
	Load: func(c *Context) (string, bool) { return c.Installed.Env.Get(config.EnvPublicOrigin) },
	Prompt: &PromptSpec{
		Title: L("Public dashboard address for passkeys (- to turn off)", "패스키에 쓸 대시보드 공개 주소(끄려면 -)"),
		Help: L(`Security hardening mode can ask for a passkey before sensitive features (web terminal, root elevation).
Passkeys need the public HTTPS address browsers use for the dashboard, e.g. https://desk.example.lan
The browser must trust its certificate and it must be a domain name, not an IP address.
Enter - to turn passkeys off (authenticator apps and recovery codes still work).`,
			`보안 강화 모드는 보안에 민감한 기능(웹 터미널, root 권한 상승) 앞에서 패스키를 물을 수 있습니다.
패스키에는 브라우저가 대시보드에 접속하는 공개 HTTPS 주소가 필요합니다. 예: https://desk.example.lan
브라우저가 믿는 인증서여야 하고 IP 주소가 아닌 도메인 이름이어야 합니다.
-를 입력하면 패스키를 끕니다(인증 앱과 복구 코드는 그대로 쓸 수 있습니다).`),
		Dash: true,
	},
	Validate: func(c *Context, v string) error {
		if v == "" {
			return nil
		}
		if _, _, err := config.ParsePublicOrigin(v); err != nil {
			return errors.New(c.L("public address must be https://host[:port] with a domain name (no IP address, path, query or fragment): "+v,
				"공개 주소는 도메인 이름의 https://호스트[:포트] 형식이어야 합니다(IP 주소·경로·query·fragment 없이): "+v))
		}
		return nil
	},
	Summary: &SummarySpec{Label: L("passkeys", "패스키 주소"), Value: offIfEmpty},
}
