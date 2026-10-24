package main

import (
	"testing"

	"maru/backend/internal/api"
)

func TestRuntimeSettings(t *testing.T) {
	env := map[string]string{
		"MARU_LISTEN":        "unix:/run/maru/maru.sock",
		"MARU_OMV_URL":       "",
		"MARU_WORKBENCH_URL": "",
		"STATE_DIRECTORY":    "/var/lib/maru",
	}
	lookup := func(k string) (string, bool) {
		v, ok := env[k]
		return v, ok
	}
	got := runtimeSettings(map[string]bool{"cookie-path": true}, lookup, []runtimeEntry{
		{flag: "listen", env: "MARU_LISTEN", value: "unix:/run/maru/maru.sock"},
		{flag: "omv-url", env: "MARU_OMV_URL", value: "http://127.0.0.1"},
		{flag: "cookie-path", env: "MARU_COOKIE_PATH", value: "/desk/"},
		{flag: "state-dir", env: "MARU_STATE_DIR", value: "/var/lib/maru", systemdEnv: "STATE_DIRECTORY"},
		{flag: "workbench-url", env: "MARU_WORKBENCH_URL", value: "", emptyIsSet: true},
		{flag: "smart-refresh-at", env: "MARU_SMART_REFRESH_AT", value: "00:00"},
	})
	want := map[string]string{
		"listen":           api.RuntimeSourceEnv,
		"omv-url":          api.RuntimeSourceDefault, // 빈 환경 변수는 기본값을 쓴다
		"cookie-path":      api.RuntimeSourceFlag,
		"state-dir":        api.RuntimeSourceSystemd,
		"workbench-url":    api.RuntimeSourceEnv, // 빈 값은 "끔"으로 설정한 것이다
		"smart-refresh-at": api.RuntimeSourceDefault,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries", len(got))
	}
	for _, s := range got {
		if s.Source != want[s.Flag] {
			t.Errorf("%s: source %q, want %q", s.Flag, s.Source, want[s.Flag])
		}
	}
	if got[0].Env != "MARU_LISTEN" || got[0].Value != "unix:/run/maru/maru.sock" {
		t.Errorf("first entry %+v", got[0])
	}
}
