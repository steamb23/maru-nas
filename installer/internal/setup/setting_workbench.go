package setup

import (
	"errors"

	"onmaru/backend/config"
)

// settingWorkbenchURL은 대시보드의 "OMV 워크벤치" 앱이 OMV 웹 UI를 새 탭으로 열 주소다(브라우저 기준).
// 빈 값은 앱을 숨긴다는 정한 값이다. 질문에서는 -가 빈 값이다.
var settingWorkbenchURL = &Setting{
	Name: nameWorkbenchURL,
	Flag: FlagValueEmptyOK,
	Arg:  "URL",
	Usage: L("OMV web UI address for browsers (empty hides the OMV Workbench app)",
		"브라우저에서 OMV 웹 UI를 열 주소(빈 값이면 OMV 워크벤치 앱을 숨깁니다)"),
	Env:     []EnvLine{{File: OnmaruEnv, Key: config.EnvWorkbenchURL}},
	Default: func(c *Context) string { return c.Installed.Env.Value(config.EnvWorkbenchURL) },
	// 빈 값도 정한 값(끔)이므로 키가 있는지로 판단한다.
	Load: func(c *Context) (string, bool) { return c.Installed.Env.Get(config.EnvWorkbenchURL) },
	Prompt: &PromptSpec{
		Title: L("OMV Workbench address for browsers (- to turn off)", "브라우저에서 OMV 워크벤치를 열 주소(끄려면 -)"),
		Help: L(`The dashboard's "OMV Workbench" app opens the OMV web UI in a new browser tab at this address.
Enter the address you open OMV with in the browser, e.g. https://nas.example.lan/
(if the dashboard is under a path on the same host as OMV, / also works). Enter - to hide the app.`,
			`대시보드의 "OMV 워크벤치" 앱이 이 주소로 OMV 웹 UI를 새 탭에 엽니다.
브라우저에서 OMV에 접속하는 주소를 입력하세요. 예: https://nas.example.lan/
(대시보드를 OMV와 같은 호스트의 경로 아래에 두었다면 /도 됩니다). -를 입력하면 앱을 숨깁니다.`),
		Dash: true,
	},
	Validate: func(c *Context, v string) error {
		if config.CheckWorkbenchURL(v) != nil {
			return errors.New(c.L("OMV Workbench address must start with http:// or https://, or be a path starting with / or ./ or ../ (no query or fragment): "+v,
				"OMV 워크벤치 주소는 http:// 또는 https://로 시작하거나 /·./·../로 시작하는 경로여야 합니다(query·fragment 없이): "+v))
		}
		return nil
	},
	Summary: &SummarySpec{Label: L("OMV Workbench", "OMV 워크벤치"), Value: offIfEmpty},
}

// offIfEmpty는 빈 값을 "(끔)"으로 보여 준다.
func offIfEmpty(c *Context, v string) string {
	if v == "" {
		return "(" + c.L("off", "끔") + ")"
	}
	return v
}
