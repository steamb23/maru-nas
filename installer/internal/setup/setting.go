package setup

import (
	"fmt"
	"strings"

	"onmaru/installer/internal/prompt"
)

// FlagKind는 설정의 명령줄 옵션 꼴이다.
type FlagKind int

const (
	// FlagNone은 옵션이 없는 설정이다(묻지 않고 기존 값을 보존하는 줄).
	FlagNone FlagKind = iota
	// FlagValue는 --name VALUE(빈 값 불가)다.
	FlagValue
	// FlagValueEmptyOK는 --name VALUE이고 빈 값도 정한 값이다(빈 값이 "끔"인 설정).
	FlagValueEmptyOK
	// FlagToggle은 --name / --no-name이다. 값은 "yes" / "no"다.
	FlagToggle
	// FlagBool은 값 없는 --name이다. 주면 "true", 없으면 정하지 않은 것(거짓)이다.
	FlagBool
)

// EnvFileID는 설정 줄을 쓰는 환경 파일이다.
type EnvFileID int

const (
	// OnmaruEnv는 /etc/default/onmaru다.
	OnmaruEnv EnvFileID = iota
	// HelperEnv는 /etc/default/onmaru-helper다. 도우미를 설치할 때만 쓴다.
	HelperEnv
)

// envHeaders는 환경 파일 맨 앞의 주석이다.
var envHeaders = map[EnvFileID]string{
	OnmaruEnv: "# onmaru settings. Written by install.sh; re-run it to change them.\n# See README.md for every option.\n",
	HelperEnv: "# onmaru-helper settings. Written by install.sh; re-run it to change them.\n",
}

// keptComment는 레지스트리가 관리하지 않는 기존 ONMARU_* 줄을 보존할 때 그 앞에 쓰는 주석이다.
const keptComment = "# Kept from the previous file (not managed by install.sh).\n"

// EnvLine은 설정이 환경 파일에 쓰는 줄 하나다.
type EnvLine struct {
	File EnvFileID
	// Comment는 줄 앞에 쓸 영어 주석이다(여러 줄이면 \n으로 나눈다). 각 줄 앞에 "# "를 붙인다.
	Comment string
	// Key는 환경 변수 키(backend/config의 상수)다.
	Key string
	// Value는 쓸 값이다. nil이면 그 설정의 값이다(파생 값은 함수로).
	Value func(c *Context) string
	// Preserve면 묻지 않고 기존 파일의 값을 그대로 옮긴다. 기존 파일에 없으면 "#KEY=Default"를 쓴다.
	Preserve bool
	Default  string
}

// PromptKind는 일반 질문의 종류다.
type PromptKind int

const (
	// PromptInput은 한 줄 입력이다.
	PromptInput PromptKind = iota
	// PromptConfirm은 예·아니요다. 값은 "yes" / "no"다(FlagToggle 설정).
	PromptConfirm
)

// PromptSpec은 설정의 일반 질문이다. 특수한 질문은 Setting.Ask로 직접 구현한다.
type PromptSpec struct {
	Kind  PromptKind
	Title Text
	// Help는 질문 앞의 설명이다. 비어도 된다.
	Help Text
	// Dash면 빈 값을 "-"로 보여 주고 "-" 입력을 빈 값으로 받는다(빈 값이 "끔"인 설정).
	Dash bool
}

// SummarySpec은 요약 화면의 한 줄이다.
type SummarySpec struct {
	Label Text
	// Value는 보여 줄 값이다. nil이면 설정 값 그대로다.
	Value func(c *Context, v string) string
}

// Setting은 설치 설정 하나다. 플래그, 사용법, 업그레이드 로드, 질문, 검증, 요약, 환경 파일 줄, 2단계 인자가
// 모두 이 정의에서 나온다. 새 설정은 setting_<이름>.go에 정의하고 settings.go의 목록에 한 줄 더한다.
type Setting struct {
	// Name은 플래그 이름(--name)이자 Values의 키다.
	Name string
	Flag FlagKind
	// Arg는 사용법에 보일 값 자리(NAME, URL 등)다.
	Arg   string
	Usage Text
	// Env는 환경 파일에 쓰는 줄이다(순서대로).
	Env []EnvLine
	// When은 이 설정이 의미 있는 조건이다(예: 상승 그룹은 도우미를 설치할 때만). nil이면 항상이다.
	When func(v Values) bool
	// Default는 질문의 기본값이다(기존 설치 → 감지값 → 고정값).
	Default func(c *Context) string
	// Load는 --upgrade에서 기존 설치의 값을 읽는다. false면 이 설정만 묻는다.
	Load func(c *Context) (string, bool)
	// Prompt는 일반 질문이다. Ask가 있으면 Ask를 쓴다. 둘 다 없으면 묻지 않는다(다른 설정이 함께 정하거나 보존하는 설정).
	Prompt *PromptSpec
	Ask    func(c *Context) error
	// Notice는 질문 단계마다(이 설정을 이미 정했어도) 먼저 실행하는 확인·경고다.
	Notice func(c *Context) error
	// Validate는 값을 검사한다. 오류 문구는 현재 언어로 만든다. 질문 중 입력 검사에도 쓴다.
	Validate func(c *Context, v string) error
	Summary  *SummarySpec
}

func (s *Setting) active(v Values) bool { return s.When == nil || s.When(v) }

// asked는 질문으로 정해야 하는 설정인지다.
func (s *Setting) asked() bool { return s.Prompt != nil || s.Ask != nil }

func (s *Setting) defaultValue(c *Context) string {
	if s.Default == nil {
		return ""
	}
	return s.Default(c)
}

// flagSpec은 사용법·빠진 옵션 목록에 보일 옵션 꼴이다.
func (s *Setting) flagSpec() string {
	switch s.Flag {
	case FlagToggle:
		return "--" + s.Name + " | --no-" + s.Name
	case FlagBool:
		return "--" + s.Name
	default:
		return "--" + s.Name + " " + s.Arg
	}
}

// Registry는 순서 있는 설정 목록이다. 질문, 요약, 환경 파일의 줄이 이 순서를 따른다.
type Registry []*Setting

// flag는 옵션 이름에 맞는 설정과, 토글이면 그 값("yes"/"no")을 찾는다.
func (r Registry) flag(name string) (*Setting, string) {
	for _, s := range r {
		switch {
		case s.Flag == FlagNone:
		case s.Name == name:
			if s.Flag == FlagToggle {
				return s, "yes"
			}
			return s, ""
		case s.Flag == FlagToggle && "no-"+s.Name == name:
			return s, "no"
		}
	}
	return nil, ""
}

// missing은 아직 정하지 않았고 물어야 하는 설정이다. 정하지 않은 토글은 기본값으로 보고 조건을 판단한다
// (도우미는 기본이 설치이므로 상승 그룹도 필요한 것으로 본다).
func (r Registry) missing(c *Context) []*Setting {
	assumed := Values{}
	for k, v := range c.Values {
		assumed[k] = v
	}
	for _, s := range r {
		if s.Flag == FlagToggle && !assumed.Has(s.Name) {
			assumed.Set(s.Name, s.defaultValue(c))
		}
	}
	var out []*Setting
	for _, s := range r {
		if s.asked() && s.active(assumed) && !c.Values.Has(s.Name) {
			out = append(out, s)
		}
	}
	return out
}

// complete는 물을 설정이 남지 않았는지다.
func (r Registry) complete(c *Context) bool { return len(r.missing(c)) == 0 }

// load는 --upgrade에서 아직 정하지 않은 설정을 기존 설치에서 채운다. 기존 설치에 없는 설정은 비워 둔다.
func (r Registry) load(c *Context) {
	for _, s := range r {
		if s.Load == nil || c.Values.Has(s.Name) || !s.active(c.Values) {
			continue
		}
		if v, ok := s.Load(c); ok {
			c.Values.Set(s.Name, v)
		}
	}
}

// ask는 아직 정하지 않은 설정만 순서대로 묻는다. Notice는 설정을 이미 정했어도 실행한다.
func (r Registry) ask(c *Context) error {
	for _, s := range r {
		if s.Notice != nil {
			if err := s.Notice(c); err != nil {
				return err
			}
		}
		if c.Values.Has(s.Name) || !s.active(c.Values) || !s.asked() {
			continue
		}
		var err error
		if s.Ask != nil {
			err = s.Ask(c)
		} else {
			err = askPrompt(c, s)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// askPrompt는 설정의 일반 질문을 한다. 입력 중 검사는 Validate와 같다.
func askPrompt(c *Context, s *Setting) error {
	p := s.Prompt
	switch p.Kind {
	case PromptConfirm:
		b, err := c.P.Confirm(prompt.Confirm{
			ID: s.Name, Title: c.T(p.Title), Help: c.T(p.Help), Default: s.defaultValue(c) == "yes",
		})
		if err != nil {
			return err
		}
		c.Values.Set(s.Name, map[bool]string{true: "yes", false: "no"}[b])
	default:
		def := s.defaultValue(c)
		if p.Dash && def == "" {
			def = "-"
		}
		fromInput := func(in string) string {
			if p.Dash && in == "-" {
				return ""
			}
			return in
		}
		v, err := c.P.Input(prompt.Input{
			ID: s.Name, Title: c.T(p.Title), Help: c.T(p.Help), Default: def,
			Validate: func(in string) error {
				if s.Validate == nil {
					return nil
				}
				return s.Validate(c, fromInput(in))
			},
		})
		if err != nil {
			return err
		}
		c.Values.Set(s.Name, fromInput(v))
	}
	return nil
}

// validate는 정한 값을 모두 검사한다. 옵션으로 받은 값과 2단계가 받은 값도 같은 규칙으로 검사한다.
func (r Registry) validate(c *Context) error {
	for _, s := range r {
		if s.Validate == nil || !s.active(c.Values) {
			continue
		}
		if err := s.Validate(c, c.Values.Get(s.Name)); err != nil {
			return fail(err.Error())
		}
	}
	return nil
}

// stage2Args는 2단계(--service-install)로 넘길 옵션이다. 2단계가 다시 묻지 않게 정한 설정을 모두 넘긴다.
func (r Registry) stage2Args(v Values) []string {
	var args []string
	for _, s := range r {
		if !s.active(v) {
			continue
		}
		switch s.Flag {
		case FlagValue, FlagValueEmptyOK:
			if v.Has(s.Name) {
				args = append(args, "--"+s.Name, v.Get(s.Name))
			}
		case FlagToggle:
			switch v.Get(s.Name) {
			case "yes":
				args = append(args, "--"+s.Name)
			case "no":
				args = append(args, "--no-"+s.Name)
			}
		case FlagBool:
			if v.Bool(s.Name) {
				args = append(args, "--"+s.Name)
			}
		}
	}
	return args
}

// summary는 요약 화면의 내용이다.
func (r Registry) summary(c *Context) string {
	type row struct{ label, value string }
	rows := []row{{c.L("binary", "실행 파일"), c.binSrc() + " -> " + binDst}}
	for _, s := range r {
		if s.Summary == nil || !s.active(c.Values) {
			continue
		}
		v := c.Values.Get(s.Name)
		if s.Summary.Value != nil {
			v = s.Summary.Value(c, v)
		}
		rows = append(rows, row{c.T(s.Summary.Label), v})
	}
	rows = append(rows,
		row{c.L("state dir", "상태 디렉터리"), stateDir},
		row{c.L("files", "설정 파일"), unitFile + ", " + envFile})
	width := 0
	for _, r := range rows {
		width = max(width, displayWidth(r.label))
	}
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "  %s %s\n", padRight(r.label, width), r.value)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// managedKeys는 레지스트리가 그 환경 파일에 쓰는 키다(조건과 상관없이).
func (r Registry) managedKeys(file EnvFileID) map[string]bool {
	keys := map[string]bool{}
	for _, s := range r {
		for _, l := range s.Env {
			if l.File == file {
				keys[l.Key] = true
			}
		}
	}
	return keys
}

// renderEnv는 환경 파일 내용을 만든다. existing은 기존 파일이다(보존할 값, 관리하지 않는 ONMARU_* 줄).
func (r Registry) renderEnv(c *Context, file EnvFileID, existing *EnvFile) []byte {
	var b strings.Builder
	b.WriteString(envHeaders[file])
	for _, s := range r {
		if !s.active(c.Values) {
			continue
		}
		for _, l := range s.Env {
			if l.File != file {
				continue
			}
			if l.Comment != "" {
				for _, line := range strings.Split(l.Comment, "\n") {
					b.WriteString("# " + line + "\n")
				}
			}
			switch {
			case l.Preserve:
				if v, ok := existing.Get(l.Key); ok {
					fmt.Fprintf(&b, "%s=%s\n", l.Key, v)
				} else {
					fmt.Fprintf(&b, "#%s=%s\n", l.Key, l.Default)
				}
			case l.Value != nil:
				fmt.Fprintf(&b, "%s=%s\n", l.Key, l.Value(c))
			default:
				fmt.Fprintf(&b, "%s=%s\n", l.Key, c.Values.Get(s.Name))
			}
		}
	}
	if kept := existing.Unmanaged(r.managedKeys(file)); len(kept) > 0 {
		b.WriteString(keptComment)
		for _, l := range kept {
			b.WriteString(l + "\n")
		}
	}
	return []byte(b.String())
}

// usageSettings는 사용법의 설정 옵션 줄이다.
func (r Registry) usageSettings(lang string) string {
	var b strings.Builder
	for _, s := range r {
		if s.Flag == FlagNone {
			continue
		}
		fmt.Fprintf(&b, "  %-26s %s\n", s.flagSpec(), s.Usage.In(lang))
	}
	return b.String()
}
