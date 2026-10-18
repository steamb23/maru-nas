package setup

import (
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"onmaru/backend/config"
	"onmaru/installer/internal/prompt"
)

// planFake는 설정이 모두 있는 설치(helper-loopback 골든), 이 버전의 설치 기록, 요구 범위 안의 node가 있는 가짜 시스템이다.
func planFake(t *testing.T) *fakeSystem {
	t.Helper()
	f := installedFake(t)
	putRecord(t, f, config.InstallRecord{
		Format: config.InstallRecordFormat, Version: config.Version, Revision: revA, InstalledAt: "2026-10-01T00:00:00Z",
	})
	f.outputs["node --version"] = "v22.12.0\n"
	return f
}

func putRecord(t *testing.T, f *fakeSystem, rec config.InstallRecord) {
	t.Helper()
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	f.put(t, config.InstallRecordFile, string(data), 0o644)
}

// setEnvLine은 기존 /etc/default/onmaru의 key 줄을 바꾼다. value가 nil이면 그 줄을 지운다.
func setEnvLine(t *testing.T, f *fakeSystem, key string, value *string) {
	t.Helper()
	data, _ := f.read(t, envFile)
	var lines []string
	for _, l := range strings.Split(strings.TrimSuffix(data, "\n"), "\n") {
		if strings.HasPrefix(l, key+"=") {
			if value == nil {
				continue
			}
			l = key + "=" + *value
		}
		lines = append(lines, l)
	}
	f.put(t, envFile, strings.Join(lines, "\n")+"\n", 0o644)
}

func ptr(s string) *string { return &s }

// runPlan은 일반 사용자로 --upgrade-plan을 실행하고 표준 출력의 계획과 오류 출력을 돌려준다.
func runPlan(t *testing.T, f *fakeSystem, args ...string) (config.UpgradePlan, string, string) {
	t.Helper()
	c, _, _ := newTestContext(t, f, prompt.Fail{}, append([]string{"--upgrade-plan"}, args...)...)
	c.SelfBuild = vcsBuild(revB, false)
	var stdout, stderr strings.Builder
	c.Out, c.Err = &stdout, &stderr
	if err := Run(c); err != nil {
		t.Fatalf("Run: %v\n%s", err, stderr.String())
	}
	var p config.UpgradePlan
	if err := json.Unmarshal([]byte(stdout.String()), &p); err != nil {
		t.Fatalf("stdout is not a plan: %v\n%s", err, stdout.String())
	}
	return p, stdout.String(), stderr.String()
}

// snapshot은 가짜 루트의 모든 파일 내용이다.
func snapshot(t *testing.T, f *fakeSystem) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(f.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		out[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func manualIDs(p config.UpgradePlan) []string {
	var ids []string
	for _, r := range p.Manual {
		ids = append(ids, r.ID)
	}
	return ids
}

// 설정이 모두 있고 기록이 있는 설치는 질문도 수동 사유도 없다. 계획은 파일을 쓰지 않고 node --version만 실행한다.
func TestUpgradePlanReady(t *testing.T) {
	f := planFake(t)
	before := snapshot(t, f)
	p, stdout, stderr := runPlan(t, f)
	if !p.Web || p.Command != "" || len(p.Manual) != 0 || len(p.Questions) != 0 || len(p.Notes) != 0 {
		t.Errorf("plan %+v", p)
	}
	if p.Format != config.UpgradePlanFormat || p.Version != config.Version || p.Revision != revB {
		t.Errorf("header %+v", p)
	}
	if p.Installed == nil || p.Installed.Revision != revA || p.Installed.Version != config.Version {
		t.Errorf("installed %+v", p.Installed)
	}
	if p.Requires != (config.UpgradeRequires{Go: requireGo, Node: requireNode, NodeFound: "22.12.0"}) {
		t.Errorf("requires %+v", p.Requires)
	}
	// 빈 목록은 null이 아니라 []다.
	for _, field := range []string{`"manual": []`, `"questions": []`, `"notes": []`} {
		if !strings.Contains(stdout, field) {
			t.Errorf("stdout misses %s:\n%s", field, stdout)
		}
	}
	if stderr != "" {
		t.Errorf("stderr %q", stderr)
	}
	if !slices.Equal(f.log, []string{"node --version"}) {
		t.Errorf("commands %q", f.log)
	}
	if after := snapshot(t, f); !maps.Equal(before, after) {
		t.Error("plan changed files")
	}
}

// 기존 설치에 없는 일반 질문 설정은 웹에서 물을 질문이 된다(문구는 설정 정의 그대로 두 언어).
func TestUpgradePlanNewSetting(t *testing.T) {
	f := planFake(t)
	setEnvLine(t, f, config.EnvPublicOrigin, nil)
	p, _, _ := runPlan(t, f)
	if !p.Web || len(p.Questions) != 1 {
		t.Fatalf("plan %+v", p)
	}
	want := config.UpgradeQuestion{
		Name: namePublicOrigin, Kind: config.UpgradeQuestionInput,
		Title: settingPublicOrigin.Prompt.Title.localized(), Help: settingPublicOrigin.Prompt.Help.localized(),
		Default: "", Dash: true, EmptyOK: true, Reason: config.UpgradeQuestionNew,
	}
	if got := p.Questions[0]; got.Error != nil || got.Name != want.Name || got.Kind != want.Kind || got.Title != want.Title ||
		got.Help != want.Help || got.Default != want.Default || !got.Dash || !got.EmptyOK || got.Reason != want.Reason {
		t.Errorf("question %+v", got)
	}
	if p.Questions[0].Title.En == "" || p.Questions[0].Title.Ko == "" {
		t.Error("empty title")
	}
}

// 이 버전의 검사를 통과하지 못한 기존 값은 invalid 질문이 되고, 오류는 두 언어로, 기본값은 기존 값이 아닌 기본값이다.
// 예·아니요 설정은 confirm이다. 정하지 않은 예·아니요에 딸린 설정(도우미의 상승 그룹)은 기본값(예)으로 보고 함께 묻고,
// 아니요를 옵션으로 주어 다시 계획하면 사라진다.
func TestUpgradePlanInvalidAndConfirm(t *testing.T) {
	f := planFake(t)
	setEnvLine(t, f, config.EnvSmartRefreshAt, ptr("25:00"))
	setEnvLine(t, f, config.EnvHelperSocket, nil)
	p, _, _ := runPlan(t, f)
	if !p.Web || len(p.Questions) != 4 {
		t.Fatalf("plan %+v", p)
	}
	smart, helper, group, web := p.Questions[0], p.Questions[1], p.Questions[2], p.Questions[3]
	if smart.Name != nameSmartRefreshAt || smart.Reason != config.UpgradeQuestionInvalid || smart.Default != config.DefaultSmartRefreshAt ||
		smart.Error == nil || !strings.Contains(smart.Error.En, "invalid time: 25:00") || !strings.Contains(smart.Error.Ko, "시각이 올바르지 않습니다") {
		t.Errorf("smart %+v (error %+v)", smart, smart.Error)
	}
	if helper.Name != nameHelper || helper.Kind != config.UpgradeQuestionConfirm || helper.Default != "yes" || helper.Reason != config.UpgradeQuestionNew {
		t.Errorf("helper %+v", helper)
	}
	if group.Name != nameElevateGroup || group.Kind != config.UpgradeQuestionInput || group.Default != "sudo" {
		t.Errorf("group %+v", group)
	}
	// 대시보드에서 업그레이드도 도우미에 딸린 예·아니요다. 기존 설치에 설정 파일이 없으므로 기본값은 아니요다.
	if web.Name != nameWebUpgrade || web.Kind != config.UpgradeQuestionConfirm || web.Default != "no" {
		t.Errorf("web upgrade %+v", web)
	}
	p, _, _ = runPlan(t, f, "--no-helper", "--smart-refresh-at", "03:00")
	if !p.Web || len(p.Questions) != 0 {
		t.Errorf("answered: %+v", p.Questions)
	}
}

// 계획에 설정 옵션을 주면 그 값을 쓰고 검사한다(입력한 값 확인). 통과하면 질문이 사라지고, 아니면 invalid 질문이다.
func TestUpgradePlanOptions(t *testing.T) {
	f := planFake(t)
	setEnvLine(t, f, config.EnvPublicOrigin, nil)
	p, _, _ := runPlan(t, f, "--public-origin", "")
	if !p.Web || len(p.Questions) != 0 {
		t.Errorf("valid option: %+v", p)
	}
	p, _, _ = runPlan(t, f, "--public-origin", "http://desk.example.lan")
	if len(p.Questions) != 1 || p.Questions[0].Reason != config.UpgradeQuestionInvalid || p.Questions[0].Error == nil {
		t.Errorf("invalid option: %+v", p)
	}
	// 기존 값이 잘못됐어도 옵션으로 고친 값을 주면 묻지 않는다.
	setEnvLine(t, f, config.EnvSmartRefreshAt, ptr("25:00"))
	p, _, _ = runPlan(t, f, "--public-origin", "", "--smart-refresh-at", "03:00")
	if !p.Web || len(p.Questions) != 0 {
		t.Errorf("fixed by option: %+v", p)
	}
}

// 웹으로 넘을 수 없는 경우는 모두 수동 사유로 모으고, 명령은 그중 가장 강한 것이다.
func TestUpgradePlanManual(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(t *testing.T, f *fakeSystem)
		ids     []string
		command string
		stderr  string
	}{
		{"no record", func(t *testing.T, f *fakeSystem) { _ = f.Remove(config.InstallRecordFile) },
			[]string{config.UpgradeReasonNoRecord}, config.UpgradeCommandInstall, ""},
		{"record without version", func(t *testing.T, f *fakeSystem) {
			putRecord(t, f, config.InstallRecord{Format: config.InstallRecordFormat, Revision: revA})
		}, []string{config.UpgradeReasonNoRecord}, config.UpgradeCommandInstall, ""},
		{"record without revision", func(t *testing.T, f *fakeSystem) {
			putRecord(t, f, config.InstallRecord{Format: config.InstallRecordFormat, Version: config.Version})
		}, []string{config.UpgradeReasonNoRecord}, config.UpgradeCommandInstall, ""},
		{"broken record", func(t *testing.T, f *fakeSystem) { f.put(t, config.InstallRecordFile, "{", 0o644) },
			[]string{config.UpgradeReasonNoRecord}, config.UpgradeCommandInstall, "cannot read the install record"},
		{"unknown record format", func(t *testing.T, f *fakeSystem) {
			putRecord(t, f, config.InstallRecord{Format: 99, Version: config.Version, Revision: revA})
		}, []string{config.UpgradeReasonNoRecord}, config.UpgradeCommandInstall, "unknown format"},
		{"downgrade", func(t *testing.T, f *fakeSystem) {
			putRecord(t, f, config.InstallRecord{Format: config.InstallRecordFormat, Version: "99.0.0", Revision: revA})
		}, []string{config.UpgradeReasonDowngrade}, config.UpgradeCommandInstall, ""},
		{"invalid listen (asked specially)", func(t *testing.T, f *fakeSystem) {
			setEnvLine(t, f, config.EnvListen, ptr("example.lan:8090"))
		}, []string{config.UpgradeReasonSetting}, config.UpgradeCommandInstall, ""},
		{"old node", func(t *testing.T, f *fakeSystem) { f.outputs["node --version"] = "v20.18.3\n" },
			[]string{config.UpgradeReasonNode}, config.UpgradeCommandInstall, ""},
		{"node 21 is outside ^20", func(t *testing.T, f *fakeSystem) { f.outputs["node --version"] = "v21.7.0\n" },
			[]string{config.UpgradeReasonNode}, config.UpgradeCommandInstall, ""},
		{"no node", func(t *testing.T, f *fakeSystem) { delete(f.outputs, "node --version") },
			[]string{config.UpgradeReasonNode}, config.UpgradeCommandInstall, ""},
		{"several reasons", func(t *testing.T, f *fakeSystem) {
			_ = f.Remove(config.InstallRecordFile)
			setEnvLine(t, f, config.EnvListen, ptr("example.lan:8090"))
			delete(f.outputs, "node --version")
		}, []string{config.UpgradeReasonNoRecord, config.UpgradeReasonSetting, config.UpgradeReasonNode}, config.UpgradeCommandInstall, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := planFake(t)
			tc.setup(t, f)
			p, _, stderr := runPlan(t, f)
			if p.Web || p.Command != tc.command || !slices.Equal(manualIDs(p), tc.ids) {
				t.Errorf("web %v command %q manual %+v", p.Web, p.Command, p.Manual)
			}
			for _, r := range p.Manual {
				if r.Text.En == "" || r.Text.Ko == "" || r.Command == "" {
					t.Errorf("reason %+v", r)
				}
			}
			if (tc.stderr == "") != (stderr == "") || !strings.Contains(stderr, tc.stderr) {
				t.Errorf("stderr %q, want %q", stderr, tc.stderr)
			}
		})
	}
}

// 수동 사유의 설정 이름과, 질문으로 만들 수 없는 기존 값의 오류를 담는다.
func TestUpgradePlanAskSetting(t *testing.T) {
	f := planFake(t)
	setEnvLine(t, f, config.EnvListen, ptr("example.lan:8090"))
	p, _, _ := runPlan(t, f)
	if len(p.Manual) != 1 || p.Manual[0].Setting != nameListen || !strings.Contains(p.Manual[0].Text.En, "invalid address: example.lan:8090") ||
		!strings.Contains(p.Manual[0].Text.Ko, "주소가 올바르지 않습니다") {
		t.Errorf("manual %+v", p.Manual)
	}
	// 수신 주소는 질문이 아니다(웹에서 묻지 않는다).
	for _, q := range p.Questions {
		if q.Name == nameListen || q.Name == nameAllowNonLoopback {
			t.Errorf("question %+v", q)
		}
	}
}

// 설치되어 있지 않으면(일부만 있어도) 그 사유만 내고 node도 실행하지 않는다. 일부 설치 경고를 찍지 않는다.
func TestUpgradePlanNotInstalled(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T, f *fakeSystem){
		"nothing":  func(t *testing.T, f *fakeSystem) {},
		"env only": func(t *testing.T, f *fakeSystem) { f.put(t, envFile, "ONMARU_LISTEN=127.0.0.1:8090\n", 0o644) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			f.putRepo(t)
			setup(t, f)
			p, _, stderr := runPlan(t, f)
			if p.Web || !slices.Equal(manualIDs(p), []string{config.UpgradeReasonNotInstalled}) || p.Command != config.UpgradeCommandInstall ||
				p.Installed != nil || len(p.Questions) != 0 {
				t.Errorf("plan %+v", p)
			}
			if len(f.log) != 0 || stderr != "" {
				t.Errorf("commands %q stderr %q", f.log, stderr)
			}
		})
	}
}

// 새 버전이 정한 수동 재설치 경계와 업그레이드 뒤 할 일은 설치된 버전과 비교한다. --reinstall 항목이 하나라도 있으면
// 명령은 --reinstall이다.
func TestUpgradePlanLists(t *testing.T) {
	saveM, saveN := manualUpgrades, upgradeNotes
	t.Cleanup(func() { manualUpgrades, upgradeNotes = saveM, saveN })
	manualUpgrades = []manualUpgrade{
		{Below: "0.0.1", Reason: L("too old", "너무 오래됨"), Command: config.UpgradeCommandInstall},
		{Below: "99.0.0", Reason: L("socket moved", "소켓 이동"), Command: config.UpgradeCommandReinstall},
		{Below: "99.1.0", Reason: L("new unit", "새 유닛"), Command: config.UpgradeCommandInstall},
	}
	upgradeNotes = []upgradeNote{
		{Since: "0.0.1", Text: L("old note", "옛 안내")},
		{Since: "99.0.0", Text: L("recreate the proxy container", "프록시 컨테이너를 다시 만드세요")},
	}
	p, _, _ := runPlan(t, planFake(t))
	if p.Web || p.Command != config.UpgradeCommandReinstall || len(p.Manual) != 2 ||
		p.Manual[0].Text != (config.LocalizedText{En: "socket moved", Ko: "소켓 이동"}) || p.Manual[0].ID != config.UpgradeReasonManualList ||
		p.Manual[1].Command != config.UpgradeCommandInstall {
		t.Errorf("manual %v %+v", p.Command, p.Manual)
	}
	if len(p.Notes) != 1 || p.Notes[0].En != "recreate the proxy container" {
		t.Errorf("notes %+v", p.Notes)
	}
}

// --upgrade-plan은 다른 동작·방식 옵션과 함께 쓸 수 없다. 동작 옵션은 하나만 받는다.
func TestUpgradePlanOptionConflicts(t *testing.T) {
	for _, args := range [][]string{
		{"--upgrade-plan", "--upgrade"},
		{"--reinstall", "--upgrade-plan"},
		{"--upgrade-plan", "--uninstall"},
		{"--service-install", "--upgrade-plan"},
		{"--uninstall", "--service-install"},
		{"--upgrade-plan=x"},
	} {
		if _, _, err := parseArgs(settings, args); err == nil {
			t.Errorf("%q accepted", args)
		}
	}
	opts, values, err := parseArgs(settings, []string{"--upgrade-plan", "--lang", "ko", "--public-origin", ""})
	if err != nil || opts.Action != ActionUpgradePlan || !values.Has(namePublicOrigin) {
		t.Errorf("opts %+v values %v err %v", opts, values, err)
	}
}
