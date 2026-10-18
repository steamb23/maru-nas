package setup

import (
	"maps"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"onmaru/installer/internal/prompt"
)

// validationCase는 설정 하나의 검사 표 한 줄이다. values는 함께 정한 다른 설정이다.
type validationCase struct {
	setting *Setting
	value   string
	values  Values
	ok      bool
}

func TestSettingValidation(t *testing.T) {
	allow := Values{nameAllowNonLoopback: "true"}
	cases := []validationCase{
		{settingUser, "onmaru", nil, true},
		{settingUser, "svc_onmaru-2", nil, true},
		{settingUser, "_svc", nil, true},
		{settingUser, "root", nil, false},
		{settingUser, "Onmaru", nil, false},
		{settingUser, "-x", nil, false},
		{settingUser, "", nil, false},

		{settingListen, "127.0.0.1:8090", nil, true},
		{settingListen, "127.0.0.2:9000", nil, true},
		{settingListen, "[::1]:8090", nil, true},
		{settingListen, "unix:/run/onmaru/onmaru.sock", nil, true},
		{settingListen, "unix:/tmp/onmaru.sock", nil, false},
		{settingListen, "unix:onmaru.sock", nil, false},
		{settingListen, "127.0.0.1:0", nil, false},
		{settingListen, "127.0.0.1:65536", nil, false},
		{settingListen, "127.0.0.1:http", nil, false},
		{settingListen, "127.0.0.1:08090", nil, false},
		{settingListen, "localhost:8090", nil, false},
		{settingListen, "0.0.0.0:8090", nil, false},
		{settingListen, "0.0.0.0:8090", allow, true},
		{settingListen, ":8090", allow, true},
		{settingListen, "[::]:8090", allow, true},
		{settingListen, "192.0.2.10:8090", allow, true},
		{settingListen, "nas.example.lan:8090", allow, false},
		{settingListen, "999.1.1.1:8090", allow, false},
		{settingListen, "0.0.0.0", allow, false},

		{settingOMVURL, "http://127.0.0.1", nil, true},
		{settingOMVURL, "http://127.0.0.1:8001/", nil, true},
		{settingOMVURL, "https://localhost", nil, true},
		{settingOMVURL, "http://[::1]:8001", nil, true},
		{settingOMVURL, "http://127.0.0.1/omv", nil, false},
		{settingOMVURL, "http://127.0.0.1:0", nil, false},
		{settingOMVURL, "http://127.0.0.1/?x", nil, false},
		{settingOMVURL, "http://user@127.0.0.1", nil, false},
		{settingOMVURL, "http://192.0.2.1", nil, false},
		{settingOMVURL, "127.0.0.1", nil, false},

		{settingSmartRefreshAt, "00:00", nil, true},
		{settingSmartRefreshAt, "23:59", nil, true},
		{settingSmartRefreshAt, "6:30", nil, false},
		{settingSmartRefreshAt, "24:00", nil, false},
		{settingSmartRefreshAt, "12:60", nil, false},
		{settingSmartRefreshAt, "", nil, false},

		{settingWorkbenchURL, "", nil, true},
		{settingWorkbenchURL, "https://nas.example.lan/", nil, true},
		{settingWorkbenchURL, "/", nil, true},
		{settingWorkbenchURL, "../", nil, true},
		{settingWorkbenchURL, "nas.example.lan/", nil, false},
		{settingWorkbenchURL, "-", nil, false},
		{settingWorkbenchURL, "https://nas.example.lan/#/x", nil, false},

		{settingPublicOrigin, "", nil, true},
		{settingPublicOrigin, "https://desk.example.lan", nil, true},
		{settingPublicOrigin, "https://Desk.Example.LAN:443/", nil, true},
		{settingPublicOrigin, "http://desk.example.lan", nil, false},
		{settingPublicOrigin, "https://192.0.2.10", nil, false},
		{settingPublicOrigin, "https://desk.example.lan/desk/", nil, false},

		{settingHelper, "yes", nil, true},
		{settingHelper, "no", nil, true},
		{settingHelper, "", nil, false},

		{settingElevateGroup, "sudo", nil, true},
		{settingElevateGroup, "openmediavault-admin", nil, true},
		{settingElevateGroup, "-sudo", nil, false},
		{settingElevateGroup, "", nil, false},
	}
	for _, tc := range cases {
		c := &Context{Lang: "en", Values: Values{}}
		maps.Copy(c.Values, tc.values)
		err := tc.setting.Validate(c, tc.value)
		if (err == nil) != tc.ok {
			t.Errorf("%s %q (with %v): err %v, want ok=%v", tc.setting.Name, tc.value, tc.values, err, tc.ok)
		}
	}
}

// 오류 문구는 현재 언어로 만든다.
func TestSettingValidationKorean(t *testing.T) {
	c := &Context{Lang: "ko", Values: Values{}}
	if err := settingSmartRefreshAt.Validate(c, "25:00"); err == nil || err.Error() != "시각이 올바르지 않습니다: 25:00" {
		t.Errorf("err %v", err)
	}
}

// 레지스트리 왕복: 정한 값 → 2단계 인자 → 파싱 = 같은 값. 레지스트리의 모든 옵션 설정을 거친다.
func TestRegistryRoundtrip(t *testing.T) {
	for _, v := range []Values{
		{nameUser: "onmaru", nameListen: "0.0.0.0:8090", nameAllowNonLoopback: "true", nameOMVURL: "http://127.0.0.1:8001",
			nameSmartRefreshAt: "03:30", nameWorkbenchURL: "", namePublicOrigin: "", nameHelper: "yes", nameElevateGroup: "wheel"},
		{nameUser: "dash", nameListen: "unix:/run/onmaru/onmaru.sock", nameOMVURL: "http://127.0.0.1",
			nameSmartRefreshAt: "00:00", nameWorkbenchURL: "https://nas.example.lan/a b", namePublicOrigin: "https://desk.example.lan",
			nameHelper: "no"},
	} {
		args := settings.stage2Args(v)
		_, got, err := parseArgs(settings, args)
		if err != nil {
			t.Fatalf("parse %q: %v", args, err)
		}
		if !maps.Equal(got, v) {
			t.Errorf("roundtrip\n got  %v\n want %v\n args %q", got, v, args)
		}
	}
	// 모든 옵션 설정이 2단계 인자에 나올 수 있어야 한다(새 설정을 더하면 위 표에도 넣는다).
	seen := map[string]bool{}
	for _, v := range []Values{
		{nameUser: "u", nameListen: "l", nameAllowNonLoopback: "true", nameOMVURL: "o", nameSmartRefreshAt: "s",
			nameWorkbenchURL: "", namePublicOrigin: "", nameHelper: "yes", nameElevateGroup: "g"},
	} {
		for _, a := range settings.stage2Args(v) {
			seen[strings.TrimPrefix(a, "--")] = true
		}
	}
	for _, s := range settings {
		if s.Flag != FlagNone && !seen[s.Name] {
			t.Errorf("setting %s is not passed to stage 2", s.Name)
		}
	}
}

func TestParseArgs(t *testing.T) {
	opts, v, err := parseArgs(settings, []string{"--upgrade", "--no-build", "--dry-run", "--plain", "--lang=ko",
		"--user=dash", "--workbench-url=", "--public-origin", "", "--allow-non-loopback", "--no-helper", "--listen", "[::1]:1"})
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Upgrade || !opts.NoBuild || !opts.DryRun || !opts.Plain || opts.Lang != "ko" || opts.Action != ActionInstall {
		t.Errorf("opts %+v", opts)
	}
	want := Values{nameUser: "dash", nameWorkbenchURL: "", namePublicOrigin: "", nameAllowNonLoopback: "true", nameHelper: "no", nameListen: "[::1]:1"}
	if !maps.Equal(v, want) {
		t.Errorf("values %v", v)
	}
	if opts, _, _ := parseArgs(settings, []string{"-h"}); !opts.Help {
		t.Error("-h")
	}
	if opts, _, _ := parseArgs(settings, []string{"--service-install"}); opts.Action != ActionServiceInstall {
		t.Error("--service-install")
	}
	if opts, _, err := parseArgs(settings, []string{"--service-install", "--reinstall"}); err != nil || !opts.Reinstall {
		t.Errorf("--reinstall: %+v %v", opts, err)
	}
	for _, bad := range [][]string{
		{"--user"}, {"--user", ""}, {"--user="}, {"--lang", "fr"}, {"--upgrade=1"}, {"--helper=yes"}, {"--bogus"},
		{"extra"}, {"-x"}, {"--upgrade", "--uninstall"}, {"--terminal"}, {"--"},
		{"--upgrade", "--reinstall"}, {"--reinstall", "--uninstall"}, {"--reinstall=1"},
	} {
		if _, _, err := parseArgs(settings, bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestUsageListsEverySetting(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		u := usage(settings, lang, "./install.sh")
		for _, s := range settings {
			if s.Flag != FlagNone && !strings.Contains(u, s.flagSpec()) {
				t.Errorf("%s usage misses %s", lang, s.flagSpec())
			}
		}
		for _, o := range []string{"--upgrade", "--reinstall", "--no-build", "--uninstall", "--service-install", "--dry-run", "--plain", "--lang en|ko", "--help"} {
			if !strings.Contains(u, o) {
				t.Errorf("%s usage misses %s", lang, o)
			}
		}
	}
}

func TestSummary(t *testing.T) {
	c := &Context{Lang: "ko", RepoDir: "/repo", Values: Values{nameUser: "onmaru", nameListen: "0.0.0.0:8090", nameAllowNonLoopback: "true",
		nameOMVURL: "http://127.0.0.1", nameSmartRefreshAt: "00:00", nameWorkbenchURL: "", namePublicOrigin: "https://desk.example.lan",
		nameHelper: "yes", nameElevateGroup: "sudo"}}
	got := settings.summary(c)
	for _, want := range []string{
		"  실행 파일     /repo/bin/onmaru -> /usr/local/bin/onmaru",
		"  서비스 사용자 onmaru",
		"  수신          0.0.0.0:8090  (루프백 외 주소 허용)",
		"  OMV 워크벤치  (끔)",
		"  권한 도우미   설치 (/usr/local/bin/onmaru-helper, 상승 그룹 sudo)",
		"  설정 파일     /etc/systemd/system/onmaru.service, /etc/default/onmaru",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("summary misses %q:\n%s", want, got)
		}
	}
}

// --upgrade 로드: 합성한 기존 설치에서 읽고, 없는 설정은 비워 둔다.
func TestLoadInstalled(t *testing.T) {
	f := newFake(t)
	f.put(t, unitFile, "[Service]\nUser=dash\nUser=other\n", 0o644)
	f.put(t, envFile, "ONMARU_LISTEN=0.0.0.0:8090\nONMARU_ALLOW_NON_LOOPBACK=true\nONMARU_OMV_URL=\"http://127.0.0.1:8001\"\n"+
		"ONMARU_WORKBENCH_URL=\nONMARU_HELPER_SOCKET=/run/onmaru-helper/helper.sock\n", 0o644)
	f.put(t, helperEnvFile, "ONMARU_HELPER_ELEVATE_GROUP=wheel\n", 0o644)
	f.put(t, helperServiceUnit, "x", 0o644)
	inst, err := readInstalled(f)
	if err != nil {
		t.Fatal(err)
	}
	c := &Context{Lang: "en", Sys: f, Installed: inst, Values: Values{nameUser: "given"}}
	settings.load(c)
	want := Values{nameUser: "given", nameListen: "0.0.0.0:8090", nameAllowNonLoopback: "true", nameOMVURL: "http://127.0.0.1:8001",
		nameWorkbenchURL: "", nameHelper: "yes", nameElevateGroup: "wheel"}
	if !maps.Equal(c.Values, want) {
		t.Errorf("loaded %v", c.Values)
	}
	missing := settings.missing(c)
	var names []string
	for _, s := range missing {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{nameSmartRefreshAt, namePublicOrigin}) {
		t.Errorf("missing %v", names)
	}

	// 도우미 서비스 유닛이 없으면 소켓 경로가 있어도 설치하지 않은 것으로 본다.
	f.Remove(helperServiceUnit)
	inst, _ = readInstalled(f)
	c = &Context{Lang: "en", Sys: f, Installed: inst, Values: Values{}}
	settings.load(c)
	if c.Values.Get(nameHelper) != "no" || c.Values.Has(nameElevateGroup) {
		t.Errorf("values %v", c.Values)
	}
}

// 질문의 기본값은 기존 설치 → 감지값 → 고정값 순이다. 빈 값이 "끔"인 설정은 -로 보여 준다.
func TestPromptDefaults(t *testing.T) {
	f := newFake(t)
	f.put(t, nginxWebGUI, "server {\n    listen 443 ssl;\n    listen *:8001 default_server;\n}\n", 0o644)
	f.put(t, envFile, "ONMARU_HELPER_SOCKET=\nONMARU_PUBLIC_ORIGIN=https://desk.example.lan\n", 0o644)
	inst, _ := readInstalled(f)
	c := &Context{Lang: "en", Sys: f, Installed: inst, Values: Values{}}
	for s, want := range map[*Setting]string{
		settingUser: "onmaru", settingOMVURL: "http://127.0.0.1:8001", settingSmartRefreshAt: "00:00",
		settingWorkbenchURL: "", settingPublicOrigin: "https://desk.example.lan", settingHelper: "no", settingElevateGroup: "sudo",
	} {
		if got := s.defaultValue(c); got != want {
			t.Errorf("%s default %q, want %q", s.Name, got, want)
		}
	}
	// - 입력은 빈 값이고, 검사는 빈 값으로 한다.
	script := &prompt.Script{Answers: map[string]string{nameWorkbenchURL: "-"}}
	c.P = script
	if err := askPrompt(c, settingWorkbenchURL); err != nil {
		t.Fatal(err)
	}
	if v, ok := c.Values[nameWorkbenchURL]; !ok || v != "" {
		t.Errorf("workbench %q %v", v, ok)
	}
}

// 설치기는 backend 모듈에서 공개 패키지 onmaru/backend/config만 쓴다(huh 등 설치기 의존성이 백엔드에 섞이지 않게,
// 백엔드 내부 코드가 설치기에 딸려 오지 않게).
func TestBackendDependencies(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	out, err := exec.Command(gobin, "list", "-deps", "onmaru/installer").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	found := false
	for _, dep := range strings.Fields(string(out)) {
		if dep == "onmaru/backend/config" {
			found = true
		} else if dep == "onmaru/backend" || strings.HasPrefix(dep, "onmaru/backend/") {
			t.Errorf("installer depends on %s", dep)
		}
	}
	if !found {
		t.Error("installer does not use onmaru/backend/config")
	}
}
