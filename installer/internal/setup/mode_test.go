package setup

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"onmaru/installer/internal/prompt"
)

// allSettingIDs는 기본값으로 답할 때 묻는 설정 질문 ID다(수신 방식은 루프백).
var allSettingIDs = []string{"user", "listen-mode", "listen", "omv-url", "smart-refresh-at", "workbench-url", "public-origin", "helper", "elevate-group", "web-upgrade"}

// defaultAnswers는 모든 설정 질문에 기본값으로 답하는 Script 답이다. extra가 덮어쓴다.
func defaultAnswers(extra map[string]string) map[string]string {
	a := map[string]string{}
	for _, id := range allSettingIDs {
		a[id] = ""
	}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

func TestResolveMode(t *testing.T) {
	cases := []struct {
		name      string
		unit, env bool
		args      []string
		want      installMode
		wantErr   bool
		warn      bool
	}{
		{name: "fresh", want: modeFresh},
		{name: "installed", unit: true, env: true, want: modeUpgrade},
		{name: "unit only", unit: true, want: modeReinstall, warn: true},
		{name: "env only", env: true, want: modeReinstall, warn: true},
		{name: "reinstall on fresh", args: []string{"--reinstall"}, want: modeReinstall},
		{name: "reinstall on installed", unit: true, env: true, args: []string{"--reinstall"}, want: modeReinstall},
		{name: "upgrade on installed", unit: true, env: true, args: []string{"--upgrade"}, want: modeUpgrade},
		{name: "upgrade on fresh", args: []string{"--upgrade"}, wantErr: true},
		{name: "upgrade on partial", unit: true, args: []string{"--upgrade"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			if tc.unit {
				f.put(t, unitFile, "User=onmaru\n", 0o644)
			}
			if tc.env {
				f.put(t, envFile, "ONMARU_LISTEN=127.0.0.1:8090\n", 0o644)
			}
			c, out, _ := newTestContext(t, f, prompt.Fail{}, tc.args...)
			inst, err := readInstalled(f)
			if err != nil {
				t.Fatal(err)
			}
			c.Installed = inst
			err = resolveMode(c)
			if tc.wantErr {
				if code, msg := ExitCode(err); code != 1 || !strings.Contains(msg, "--upgrade needs an existing installation") || !strings.Contains(msg, "--reinstall") {
					t.Errorf("got %d %q", code, msg)
				}
				return
			}
			if err != nil || c.mode != tc.want {
				t.Errorf("mode %v %v, want %v", c.mode, err, tc.want)
			}
			if got := strings.Contains(out.String(), "partial installation found"); got != tc.warn {
				t.Errorf("partial warning %v:\n%s", got, out)
			}
		})
	}
}

// 설치되어 있으면 옵션 없이 실행해도 업그레이드다. 설정이 모두 있으면 설정 질문 없이 요약 뒤의 선택만 묻고,
// "업그레이드"는 --upgrade와 같은 결과를 만든다.
func TestDefaultUpgrade(t *testing.T) {
	f := installedFake(t)
	script := &prompt.Script{Answers: map[string]string{"upgrade-action": ""}}
	c, out, _ := newTestContext(t, f, script, "--no-build")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	if !slices.Equal(script.Asked, []string{"upgrade-action"}) {
		t.Errorf("asked %v", script.Asked)
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
	s := out.String()
	if !strings.Contains(s, "Existing installation found: upgrading with the installed settings. To change settings, run with --reinstall (or choose \"Change settings\").") {
		t.Errorf("no upgrade notice:\n%s", s)
	}
	if strings.Contains(s, "Next: point your reverse proxy") {
		t.Errorf("proxy note on unchanged upgrade:\n%s", s)
	}

	// 같은 설치에서 --upgrade와 명령·파일이 같다.
	g := installedFake(t)
	c2, out2, _ := newTestContext(t, g, prompt.Fail{}, "--upgrade", "--no-build")
	c2.IsRoot = true
	if err := Run(c2); err != nil {
		t.Fatalf("--upgrade: %v\n%s", err, out2)
	}
	if !slices.Equal(f.log, g.log) {
		t.Errorf("commands differ\n default  %q\n upgrade  %q", f.log, g.log)
	}
	compareTree(t, g, filepath.Join(goldenDir("helper-loopback"), "want"))
}

// "취소"는 아무것도 하지 않고 0으로 끝낸다.
func TestDefaultUpgradeCancel(t *testing.T) {
	f := installedFake(t)
	script := &prompt.Script{Answers: map[string]string{"upgrade-action": actionCancel}}
	c, _, _ := newTestContext(t, f, script, "--no-build")
	c.IsRoot = true
	if code, msg := ExitCode(Run(c)); code != 0 || msg != "" {
		t.Fatalf("got %d %q", code, msg)
	}
	if len(f.log) != 0 || len(backups(t, f)) != 0 {
		t.Errorf("cancel changed something: %v", f.log)
	}
	if got, _ := f.read(t, binDst); got != "old\n" {
		t.Errorf("binary replaced: %q", got)
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
}

// "설정 바꾸기"는 지금 값(기존 설치와 옵션)을 기본값으로 모든 설정을 다시 묻고 진행을 확인한다.
func TestDefaultUpgradeChangeSettings(t *testing.T) {
	f := installedFake(t)
	script := &prompt.Script{Answers: defaultAnswers(map[string]string{
		"upgrade-action": actionChange, "listen-mode": listenUnix, "public-origin": "https://desk.example.lan", "proceed": "",
	})}
	c, out, _ := newTestContext(t, f, script, "--no-build", "--smart-refresh-at", "03:30")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	want := []string{"upgrade-action", "user", "listen-mode", "omv-url", "smart-refresh-at", "workbench-url", "public-origin", "helper", "elevate-group", "web-upgrade", "proceed"}
	if !slices.Equal(script.Asked, want) {
		t.Errorf("asked %v\nwant  %v", script.Asked, want)
	}
	env, _ := f.read(t, envFile)
	for _, line := range []string{
		"ONMARU_LISTEN=unix:/run/onmaru/onmaru.sock\n", "ONMARU_ALLOW_NON_LOOPBACK=false\n",
		// 옵션으로 준 값이 다시 묻는 질문의 기본값이다.
		"ONMARU_SMART_REFRESH_AT=03:30\n",
		"ONMARU_WORKBENCH_URL=\n", "ONMARU_PUBLIC_ORIGIN=https://desk.example.lan\n",
		"ONMARU_HELPER_SOCKET=/run/onmaru-helper/helper.sock\n",
	} {
		if !strings.Contains(env, line) {
			t.Errorf("env misses %q:\n%s", line, env)
		}
	}
	// 다시 설치이므로 리버스 프록시 안내를 줄이지 않는다.
	if !strings.Contains(out.String(), "Next: point your reverse proxy") {
		t.Errorf("no proxy note:\n%s", out)
	}
}

// 비TTY에서 옵션 없이 실행하면 설치된 설정으로 묻지 않고 업그레이드한다.
func TestDefaultUpgradeNonInteractive(t *testing.T) {
	f := installedFake(t)
	c, out, _ := newTestContext(t, f, prompt.Fail{}, "--no-build")
	c.IsRoot = true
	c.Interactive = false
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
	if b := backups(t, f); len(b) != 0 {
		t.Errorf("backups %v", b)
	}
	s := out.String()
	if !strings.Contains(s, "To change settings, run with --reinstall.") || strings.Contains(s, "Change settings") {
		t.Errorf("notice:\n%s", s)
	}
}

// --reinstall은 기존 설치의 값을 기본값으로 모든 설정을 묻고 진행을 확인한다.
func TestReinstallAsksEverySetting(t *testing.T) {
	f := installedFake(t)
	env, _ := f.read(t, envFile)
	env = strings.Replace(env, "ONMARU_SMART_REFRESH_AT=00:00", "ONMARU_SMART_REFRESH_AT=03:30", 1)
	env = strings.Replace(env, "ONMARU_WORKBENCH_URL=\n", "ONMARU_WORKBENCH_URL=https://nas.example.lan/\n", 1)
	f.put(t, envFile, env, 0o644)
	script := &prompt.Script{Answers: defaultAnswers(map[string]string{"proceed": ""})}
	c, out, _ := newTestContext(t, f, script, "--reinstall", "--no-build")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	if !slices.Equal(script.Asked, append(slices.Clone(allSettingIDs), "proceed")) {
		t.Errorf("asked %v", script.Asked)
	}
	if got, _ := f.read(t, envFile); got != env {
		t.Errorf("defaults did not come from the installation:\n%s", got)
	}
	if !strings.Contains(out.String(), "Existing installation found. Current settings are used as defaults.") {
		t.Errorf("notice:\n%s", out)
	}
}

// 일부만 설치된 상태(유닛만 있음)는 경고하고 다시 설치처럼 모든 설정을 묻는다.
func TestPartialInstallation(t *testing.T) {
	f := installedFake(t)
	if err := f.Remove(envFile); err != nil {
		t.Fatal(err)
	}
	script := &prompt.Script{Answers: defaultAnswers(map[string]string{"proceed": ""})}
	c, out, _ := newTestContext(t, f, script, "--no-build")
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	if !slices.Equal(script.Asked, append(slices.Clone(allSettingIDs), "proceed")) {
		t.Errorf("asked %v", script.Asked)
	}
	if !strings.Contains(out.String(), "partial installation found (only "+unitFile+" exists)") {
		t.Errorf("no partial warning:\n%s", out)
	}
	compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
}

// 2단계에는 1단계가 정한 설치 방식을 넘긴다. --upgrade는 업그레이드로 끝났을 때만 넘긴다.
func TestStage2ModeArgs(t *testing.T) {
	cases := []struct {
		name      string
		installed bool
		args      []string
		answers   map[string]string
		want      string // "--upgrade", "--reinstall", 또는 ""
	}{
		{name: "fresh", answers: defaultAnswers(map[string]string{"proceed": ""})},
		{name: "default upgrade", installed: true, answers: map[string]string{"upgrade-action": ""}, want: "--upgrade"},
		{name: "change settings", installed: true, answers: defaultAnswers(map[string]string{"upgrade-action": actionChange, "proceed": ""}), want: "--reinstall"},
		{name: "reinstall", installed: true, args: []string{"--reinstall"}, answers: defaultAnswers(map[string]string{"proceed": ""}), want: "--reinstall"},
		{name: "explicit upgrade", installed: true, args: []string{"--upgrade"}, want: "--upgrade"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t)
			f.putRepo(t)
			if tc.installed {
				f = installedFake(t)
			}
			c, out, calls := newTestContext(t, f, &prompt.Script{Answers: tc.answers}, append(tc.args, "--no-build")...)
			if err := Run(c); !errors.Is(err, errExecIntercepted) {
				t.Fatalf("Run: %v\n%s", err, out)
			}
			argv := (*calls)[0].argv
			hasUpgrade, hasReinstall := slices.Contains(argv, "--upgrade"), slices.Contains(argv, "--reinstall")
			if hasUpgrade != (tc.want == "--upgrade") || hasReinstall != (tc.want == "--reinstall") {
				t.Errorf("argv %q, want mode %q", argv, tc.want)
			}
			// 2단계는 받은 인자만으로 묻지 않고 설치한다.
			c2, out2, _ := newTestContext(t, f, prompt.Fail{}, argv[2:]...)
			c2.IsRoot = true
			if err := Run(c2); err != nil {
				t.Fatalf("stage 2: %v\n%s", err, out2)
			}
		})
	}
}

// 기존 설치의 값이 이 버전의 검사를 통과하지 못하면 그 설정을 빠진 것으로 보고, 터미널이면 기존 값이 아닌
// 기본값으로 다시 묻고, 아니면 그 옵션을 알리고 2로 끝낸다.
func TestInvalidInstalledValue(t *testing.T) {
	broken := func(t *testing.T, from, to string) *fakeSystem {
		f := installedFake(t)
		env, _ := f.read(t, envFile)
		f.put(t, envFile, strings.Replace(env, from, to, 1), 0o644)
		return f
	}

	t.Run("tty default", func(t *testing.T) {
		f := broken(t, "ONMARU_SMART_REFRESH_AT=00:00", "ONMARU_SMART_REFRESH_AT=25:00")
		script := &prompt.Script{Answers: map[string]string{"smart-refresh-at": "", "upgrade-action": ""}}
		c, out, _ := newTestContext(t, f, script, "--no-build")
		c.IsRoot = true
		if err := Run(c); err != nil {
			t.Fatalf("Run: %v\n%s", err, out)
		}
		if !slices.Equal(script.Asked, []string{"smart-refresh-at", "upgrade-action"}) {
			t.Errorf("asked %v", script.Asked)
		}
		if !strings.Contains(out.String(), "--smart-refresh-at from the existing installation is not valid for this version: invalid time: 25:00") {
			t.Errorf("no warning:\n%s", out)
		}
		// 기본값은 잘못된 기존 값이 아니라 00:00이다.
		compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
	})

	t.Run("tty explicit upgrade", func(t *testing.T) {
		f := broken(t, "ONMARU_SMART_REFRESH_AT=00:00", "ONMARU_SMART_REFRESH_AT=25:00")
		script := &prompt.Script{Answers: map[string]string{"smart-refresh-at": "01:00"}}
		c, out, _ := newTestContext(t, f, script, "--upgrade", "--no-build")
		c.IsRoot = true
		if err := Run(c); err != nil {
			t.Fatalf("Run: %v\n%s", err, out)
		}
		if env, _ := f.read(t, envFile); !strings.Contains(env, "ONMARU_SMART_REFRESH_AT=01:00\n") {
			t.Errorf("env:\n%s", env)
		}
	})

	t.Run("tty listen", func(t *testing.T) {
		// 루프백이 아닌 주소인데 허용하지 않은 설치: 수신 방식을 기본(루프백)부터 다시 고른다.
		f := broken(t, "ONMARU_LISTEN=127.0.0.1:8090", "ONMARU_LISTEN=0.0.0.0:8090")
		script := &prompt.Script{Answers: map[string]string{"listen-mode": "", "listen": "", "upgrade-action": ""}}
		c, out, _ := newTestContext(t, f, script, "--no-build")
		c.IsRoot = true
		if err := Run(c); err != nil {
			t.Fatalf("Run: %v\n%s", err, out)
		}
		compareTree(t, f, filepath.Join(goldenDir("helper-loopback"), "want"))
	})

	t.Run("non-tty", func(t *testing.T) {
		for _, args := range [][]string{{"--no-build"}, {"--upgrade", "--no-build"}} {
			f := broken(t, "ONMARU_SMART_REFRESH_AT=00:00", "ONMARU_SMART_REFRESH_AT=25:00")
			c, _, _ := newTestContext(t, f, prompt.Fail{}, args...)
			c.IsRoot = true
			c.Interactive = false
			code, msg := ExitCode(Run(c))
			if code != 2 {
				t.Fatalf("%v: got %d %q", args, code, msg)
			}
			for _, want := range []string{"--smart-refresh-at HH:MM", "invalid time: 25:00", "--reinstall"} {
				if !strings.Contains(msg, want) {
					t.Errorf("%v: %q missing from %q", args, want, msg)
				}
			}
			if len(f.log) != 0 {
				t.Errorf("%v: ran %v", args, f.log)
			}
		}
	})
}

// 옵션 없는 직접 --service-install도 같은 규칙을 따른다. 설정을 모두 주면 설치되어 있어도 묻지 않는다.
func TestServiceInstallDetectsUpgrade(t *testing.T) {
	f := installedFake(t)
	gc := goldenDir("helper-loopback")
	args, err := os.ReadFile(filepath.Join(gc, "args"))
	if err != nil {
		t.Fatal(err)
	}
	c, out, _ := newTestContext(t, f, prompt.Fail{}, append([]string{"--service-install"}, strings.Split(strings.TrimSuffix(string(args), "\n"), "\n")...)...)
	c.IsRoot = true
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, out)
	}
	if c.mode != modeUpgrade {
		t.Errorf("mode %v", c.mode)
	}
	compareTree(t, f, filepath.Join(gc, "want"))
}
