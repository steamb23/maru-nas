package setup

import (
	"errors"
	"regexp"

	"onmaru/backend/config"
)

// smartAtRE는 설치기가 받는 HH:MM 꼴이다(두 자리 시·분). 백엔드(config.ParseRefreshAt)는 한 자리 시도 받는다.
var smartAtRE = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// settingSmartRefreshAt은 SMART 정보를 하루 한 번 다시 가져오는 기준 시각이다.
var settingSmartRefreshAt = &Setting{
	Name:  nameSmartRefreshAt,
	Flag:  FlagValue,
	Arg:   "HH:MM",
	Usage: L("daily SMART refresh time", "SMART 정보를 매일 다시 가져올 시각"),
	Env:   []EnvLine{{File: OnmaruEnv, Key: config.EnvSmartRefreshAt}},
	Default: func(c *Context) string {
		if v := c.Installed.Env.Value(config.EnvSmartRefreshAt); v != "" {
			return v
		}
		return config.DefaultSmartRefreshAt
	},
	Load: func(c *Context) (string, bool) {
		v := c.Installed.Env.Value(config.EnvSmartRefreshAt)
		return v, v != ""
	},
	Prompt: &PromptSpec{
		Title: L("Daily SMART refresh time (HH:MM, local time)", "SMART 정보를 매일 다시 가져올 시각(HH:MM, 서버 시간)"),
	},
	Validate: func(c *Context, v string) error {
		if _, _, err := config.ParseRefreshAt(v); err != nil || !smartAtRE.MatchString(v) {
			return errors.New(c.L("invalid time: "+v, "시각이 올바르지 않습니다: "+v))
		}
		return nil
	},
	Summary: &SummarySpec{Label: L("SMART refresh", "SMART 갱신")},
}
