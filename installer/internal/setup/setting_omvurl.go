package setup

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"

	"onmaru/backend/config"
	"onmaru/installer/internal/prompt"
)

// settingOMVURL은 백엔드가 OMV rpc.php에 접속할 주소다(루프백만). 기본값은 OMV nginx 설정의 수신 포트로 만든다.
var settingOMVURL = &Setting{
	Name:  nameOMVURL,
	Flag:  FlagValue,
	Arg:   "URL",
	Usage: L("OMV web UI address (loopback)", "OMV 웹 UI 주소(루프백)"),
	Env:   []EnvLine{{File: OnmaruEnv, Key: config.EnvOMVURL}},
	Default: func(c *Context) string {
		if v := c.Installed.Env.Value(config.EnvOMVURL); v != "" {
			return v
		}
		return detectOMVURL(c.Sys)
	},
	Load: func(c *Context) (string, bool) {
		v := c.Installed.Env.Value(config.EnvOMVURL)
		return v, v != ""
	},
	Prompt: &PromptSpec{
		Title: L("OMV web UI address (loopback, detected from nginx)", "OMV 웹 UI 주소(루프백, nginx 설정에서 찾음)"),
	},
	Notice:   noticeForceSSL,
	Validate: validateOMVURL,
	Summary:  &SummarySpec{Label: L("OMV address", "OMV 주소")},
}

// validateOMVURL은 백엔드 규칙(config.CheckOMVURL, 루프백 http(s))에 더해 경로·query·fragment·사용자 정보가 없는
// 주소만 받는다(설치기만의 규칙).
func validateOMVURL(c *Context, v string) error {
	bad := errors.New(c.L("OMV address must be a loopback http(s) URL: "+v, "OMV 주소는 루프백 http(s) URL이어야 합니다: "+v))
	if config.CheckOMVURL(v) != nil {
		return bad
	}
	u, err := url.Parse(v)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") || (u.Port() != "" && !validPort(u.Port())) {
		return bad
	}
	return nil
}

// nginxListenRE는 OMV nginx 설정의 평문 수신 줄(listen [주소:]포트 ...)이다.
var nginxListenRE = regexp.MustCompile(`^[ \t\r\f\v]*listen[ \t\r\f\v]+([^; \t\r\f\v]*:)?([0-9]+)([ \t\r\f\v;].*)?$`)

// detectOMVURL은 OMV 웹 UI(nginx)의 수신 포트로 루프백 주소를 만든다. 80/443을 앞단 프록시에 내주고 OMV를
// 다른 포트로 옮긴 경우가 있어 설정 파일에서 읽는다. ssl이 들어간 줄은 건너뛴다.
func detectOMVURL(sys System) string {
	port := ""
	if data, err := sys.ReadFile(nginxWebGUI); err == nil {
		port = nginxPlainPort(data)
	}
	if port == "" || port == "80" {
		return "http://127.0.0.1"
	}
	return "http://127.0.0.1:" + port
}

// nginxPlainPort는 ssl이 없는 첫 listen 줄의 포트다.
func nginxPlainPort(data []byte) string {
	for _, line := range splitLines(data) {
		if bytes.Contains([]byte(line), []byte("ssl")) {
			continue
		}
		if m := nginxListenRE.FindStringSubmatch(line); m != nil {
			return m[2]
		}
	}
	return ""
}

// omvForcesHTTPS는 OMV의 "Force SSL/TLS"가 켜져 평문 포트가 https로 리다이렉트하는지다.
// 백엔드의 OMV 클라이언트는 자체 서명 인증서를 받지 않으므로 이 구성에서는 로그인할 수 없다.
func omvForcesHTTPS(sys System) bool {
	data, err := sys.ReadFile(nginxWebGUI)
	return err == nil && bytes.Contains(data, []byte("return 301 https://"))
}

// noticeForceSSL은 OMV가 HTTPS를 강제하면 경고한다. 업그레이드는 묻지 않고 알리기만 한다(이미 설치된 구성이다).
func noticeForceSSL(c *Context) error {
	if !omvForcesHTTPS(c.Sys) {
		return nil
	}
	c.Warn(L(`OMV redirects HTTP to HTTPS ("Force SSL/TLS" is on). onmaru talks to OMV over plain HTTP on loopback and cannot log in this way. Turn off Force SSL/TLS in the OMV web UI (System > Workbench) first.`,
		`OMV가 HTTP를 HTTPS로 리다이렉트합니다("Force SSL/TLS" 켜짐). onmaru는 루프백의 평문 HTTP로 OMV에 접속하므로 이 상태로는 로그인할 수 없습니다. 먼저 OMV 웹 UI(System > Workbench)에서 Force SSL/TLS를 끄세요.`))
	if c.upgrading() {
		return nil
	}
	ok, err := c.confirm(prompt.Confirm{ID: "force-ssl-continue", Title: c.L("Continue anyway?", "그래도 계속할까요?")})
	if err != nil {
		return err
	}
	if !ok {
		return quit(1)
	}
	return nil
}
