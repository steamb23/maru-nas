package setup

import (
	"errors"
	"maps"
	"strings"
	"testing"

	"maru/installer/internal/prompt"
)

// testRegistry는 레지스트리 동작을 시험하는 합성 설정 목록이다(옵션 꼴마다 하나).
func testRegistry() Registry {
	return Registry{
		{Name: "name", Flag: FlagValue, Arg: "NAME", Usage: L("a name", "이름"),
			Env:      []EnvLine{{File: MaruEnv, Key: "MARU_NAME"}, {File: HelperEnv, Comment: "Copy.", Key: "MARU_HELPER_NAME"}},
			Default:  func(*Context) string { return "def" },
			Load:     func(c *Context) (string, bool) { return c.Installed.Env.Get("MARU_NAME") },
			Prompt:   &PromptSpec{Title: L("Name", "이름")},
			Validate: func(_ *Context, v string) error { return map[bool]error{true: errors.New("bad name")}[v == "bad"] },
			Summary:  &SummarySpec{Label: L("name", "이름")}},
		{Name: "off", Flag: FlagValueEmptyOK, Arg: "URL", Usage: L("off", "끔"),
			Env:    []EnvLine{{File: MaruEnv, Comment: "Empty is off.", Key: "MARU_OFF"}},
			Prompt: &PromptSpec{Title: L("Off", "끔"), Dash: true}},
		{Name: "flag", Flag: FlagBool, Usage: L("a flag", "플래그"),
			Env: []EnvLine{{File: MaruEnv, Key: "MARU_FLAG", Value: func(c *Context) string {
				return map[bool]string{true: "true", false: "false"}[c.Values.Bool("flag")]
			}}}},
		{Name: "feature", Flag: FlagToggle, Usage: L("feature", "기능"),
			Default: func(*Context) string { return "yes" },
			Prompt:  &PromptSpec{Kind: PromptConfirm, Title: L("Feature?", "기능?")}},
		{Name: "group", Flag: FlagValue, Arg: "GROUP", Usage: L("group", "그룹"),
			When:   func(v Values) bool { return v.Get("feature") == "yes" },
			Env:    []EnvLine{{File: HelperEnv, Key: "MARU_GROUP"}},
			Prompt: &PromptSpec{Title: L("Group", "그룹")}},
		{Name: "kept", Flag: FlagNone,
			Env: []EnvLine{{File: MaruEnv, Comment: "Kept as is.\nSecond line.", Key: "MARU_KEPT", Preserve: true, Default: "dflt"}}},
	}
}

func TestRegistryArgsRoundtrip(t *testing.T) {
	reg := testRegistry()
	for _, v := range []Values{
		{"name": "x", "off": "", "flag": "true", "feature": "yes", "group": "g"},
		{"name": "x y", "off": "https://a/", "feature": "no"},
	} {
		args := reg.stage2Args(v)
		_, got, err := parseArgs(reg, args)
		if err != nil || !maps.Equal(got, v) {
			t.Errorf("roundtrip %v -> %q -> %v %v", v, args, got, err)
		}
	}
	// 조건이 맞지 않는 설정은 넘기지 않는다.
	if args := reg.stage2Args(Values{"feature": "no", "group": "g"}); strings.Join(args, " ") != "--no-feature" {
		t.Errorf("args %q", args)
	}
	if _, _, err := parseArgs(reg, []string{"--kept", "x"}); err == nil {
		t.Error("FlagNone setting accepted as an option")
	}
}

func TestRegistryMissingAndAsk(t *testing.T) {
	reg := testRegistry()
	c := &Context{Lang: "en", Installed: &Installed{}, Values: Values{"name": "x"}, Reg: reg}
	var names []string
	for _, s := range reg.missing(c) {
		names = append(names, s.flagSpec())
	}
	// 정하지 않은 토글은 기본값(yes)으로 보고 그 조건의 설정도 빠진 것으로 본다.
	if strings.Join(names, ", ") != "--off URL, --feature | --no-feature, --group GROUP" {
		t.Errorf("missing %v", names)
	}
	script := &prompt.Script{Answers: map[string]string{"off": "", "feature": "n"}}
	c.P = script
	if err := reg.ask(c); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(c.Values, Values{"name": "x", "off": "", "feature": "no"}) || !reg.complete(c) {
		t.Errorf("values %v", c.Values)
	}
	if strings.Join(script.Asked, ",") != "off,feature" {
		t.Errorf("asked %v", script.Asked)
	}
	// 검사: 질문 중에도 같은 함수로 거절한다.
	c2 := &Context{Lang: "en", Installed: &Installed{}, Values: Values{}, Reg: reg,
		P: &prompt.Script{Answers: map[string]string{"name": "bad"}}}
	if err := askPrompt(c2, reg[0]); err == nil || !strings.Contains(err.Error(), "bad name") {
		t.Errorf("err %v", err)
	}
	c2.Values.Set("name", "bad")
	if code, msg := ExitCode(reg.validate(c2)); code != 1 || msg != "bad name" {
		t.Errorf("validate %d %q", code, msg)
	}
}

func TestRegistryLoad(t *testing.T) {
	reg := testRegistry()
	c := &Context{Installed: &Installed{Env: ParseEnv([]byte("MARU_NAME=old\n"))}, Values: Values{}}
	reg.load(c)
	if c.Values.Get("name") != "old" || len(c.Values) != 1 {
		t.Errorf("values %v", c.Values)
	}
	c.Values = Values{"name": "given"}
	reg.load(c)
	if c.Values.Get("name") != "given" {
		t.Errorf("load overwrote an option: %v", c.Values)
	}
}

func TestRegistryRenderEnv(t *testing.T) {
	reg := testRegistry()
	c := &Context{Values: Values{"name": "n", "off": "", "feature": "yes", "group": "g"}}
	existing := ParseEnv([]byte("MARU_KEPT=\"v\"\nMARU_NAME=old\nMARU_OTHER=1\n#MARU_C=2\nMARU_HELPER_NAME=h\n"))
	got := string(reg.renderEnv(c, MaruEnv, existing))
	want := envHeaders[MaruEnv] + "MARU_NAME=n\n# Empty is off.\nMARU_OFF=\nMARU_FLAG=false\n" +
		"# Kept as is.\n# Second line.\nMARU_KEPT=v\n" + keptComment + "MARU_OTHER=1\nMARU_HELPER_NAME=h\n"
	if got != want {
		t.Errorf("maru env\n--- got\n%s--- want\n%s", got, want)
	}
	// 보존할 값이 없으면 기본값을 주석으로, 관리하지 않는 줄이 없으면 보존 주석도 쓰지 않는다.
	got = string(reg.renderEnv(c, MaruEnv, nil))
	if !strings.Contains(got, "#MARU_KEPT=dflt\n") || strings.Contains(got, keptComment) {
		t.Errorf("env without existing file\n%s", got)
	}
	got = string(reg.renderEnv(c, HelperEnv, existing))
	want = envHeaders[HelperEnv] + "# Copy.\nMARU_HELPER_NAME=n\nMARU_GROUP=g\n" + keptComment + "MARU_KEPT=\"v\"\nMARU_NAME=old\nMARU_OTHER=1\n"
	if got != want {
		t.Errorf("helper env\n--- got\n%s--- want\n%s", got, want)
	}
}

func TestRegistryUsage(t *testing.T) {
	u := testRegistry().usageSettings("ko")
	for _, want := range []string{"  --name NAME                이름\n", "  --feature | --no-feature   기능\n", "  --flag "} {
		if !strings.Contains(u, want) {
			t.Errorf("usage misses %q:\n%s", want, u)
		}
	}
	if strings.Contains(u, "kept") {
		t.Errorf("usage lists FlagNone:\n%s", u)
	}
}

// writeConfig: 같은 내용이면 그대로 두고, 달라지면 백업(요청한 경우만) 뒤에 쓴다.
func TestWriteConfig(t *testing.T) {
	f := newFake(t)
	out := &testOut{}
	c := &Context{Lang: "en", Sys: f, Out: out, Err: out}
	if err := c.writeConfig("/etc/default/x", []byte("a\n"), true); err != nil {
		t.Fatal(err)
	}
	if err := c.writeConfig("/etc/default/x", []byte("a\n"), true); err != nil {
		t.Fatal(err)
	}
	if err := c.writeConfig("/etc/default/x", []byte("b\n"), true); err != nil {
		t.Fatal(err)
	}
	if err := c.writeConfig("/etc/default/x", []byte("c\n"), false); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.read(t, "/etc/default/x"); got != "c\n" {
		t.Errorf("content %q", got)
	}
	if got, ok := f.read(t, "/etc/default/x.bak-20261018-093000"); !ok || got != "a\n" {
		t.Errorf("backup %q %v", got, ok)
	}
	want := "==> Wrote /etc/default/x\n==> /etc/default/x unchanged\n==> Wrote /etc/default/x\n==> Wrote /etc/default/x\n"
	if out.String() != want {
		t.Errorf("output %q", out.String())
	}
}
