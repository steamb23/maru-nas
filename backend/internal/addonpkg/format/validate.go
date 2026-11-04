package format

import (
	"fmt"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

// Languages는 패키지 번역·문구가 쓸 수 있는 언어 코드다. 프론트의 SUPPORTED_LANGUAGES
// (frontend/src/i18n/languages.ts)와 같아야 한다(테스트가 본다).
var Languages = []string{"cs", "de", "en", "es", "fr", "it", "nl", "pl", "pt-PT", "ru", "uk", "ja", "zh-Hans", "zh-Hant", "ko"}

// FallbackLanguage는 반드시 있어야 하는 언어다.
const FallbackLanguage = "en"

// 문구 길이 상한(글자 수).
const (
	MaxNameLen        = 40
	MaxDescriptionLen = 200
	MaxLabelLen       = 40
	MaxConfirmLen     = 300
	MaxAuthorLen      = 60
	MaxURLLen         = 200
	MaxVersionLen     = 32
)

// 조작 시간 제한(초).
const (
	DefaultActionTimeout = 60
	MaxActionTimeout     = 600
)

// MaxWindowSide는 창 보기 크기 한 변의 상한(픽셀)이다.
const MaxWindowSide = 4096

// IconFile은 아이콘 파일 이름이다.
const IconFile = "icon.svg"

var (
	// versionPattern은 SemVer 코어와 선택적 사전 배포 표시다. 빌드 메타데이터(+)는 받지 않는다(rev에 쓰기 때문이다).
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
	// entryPattern은 ui/ 바로 아래의 HTML 파일 이름이다.
	entryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,94}\.html$`)
	// pluginPattern은 OMV 플러그인 패키지 이름이다.
	pluginPattern  = regexp.MustCompile(`^openmediavault-[a-z0-9.+-]+$`)
	rpcFilePattern = regexp.MustCompile(`^[a-z0-9_-]+\.inc$`)
)

// Problem은 manifest 검사에 걸린 이유다. Field는 JSON 경로(예: views.window.entry)다.
type Problem struct {
	Field  string
	Detail string
	// Incompatible은 형식은 맞지만 이 대시보드가 지원하지 않는 SDK 수준이라는 뜻이다.
	Incompatible bool
}

func (p *Problem) Error() string { return p.Field + ": " + p.Detail }

func problem(field, format string, args ...any) *Problem {
	return &Problem{Field: field, Detail: fmt.Sprintf(format, args...)}
}

// Validate는 manifest의 의미를 검사한다. 레지스트리 검사(예약 id, 금지 표, 다른 애드온과의 충돌)는
// addons.Extend가 따로 한다. 걸리면 *Problem을 돌려준다.
func Validate(m *Manifest) error {
	if m.Format != Version {
		return problem("format", "must be %d", Version)
	}
	if !addons.ValidID(m.ID) || len(m.ID) > addons.MaxPackageIDLen {
		return problem("id", "must be lowercase letters, digits and '-' (at most %d characters)", addons.MaxPackageIDLen)
	}
	if len(m.Version) > MaxVersionLen || !versionPattern.MatchString(m.Version) {
		return problem("version", "must be a semantic version without build metadata")
	}
	if m.SDK < 1 {
		return problem("sdk", "must be 1 or greater")
	}
	if m.SDK > api.AddonSDKLevel {
		return &Problem{Field: "sdk", Detail: fmt.Sprintf("requires SDK level %d (this dashboard supports %d)", m.SDK, api.AddonSDKLevel), Incompatible: true}
	}
	if err := validateText("name", m.Name, MaxNameLen, true); err != nil {
		return err
	}
	if err := validateText("description", m.Description, MaxDescriptionLen, false); err != nil {
		return err
	}
	if err := validateAuthor(m.Author); err != nil {
		return err
	}
	if m.Icon != IconFile {
		return problem("icon", "must be %q", IconFile)
	}
	if err := validateViews(m.Views); err != nil {
		return err
	}
	if err := validateRequires(m.Requires); err != nil {
		return err
	}
	if err := validateRPC(m.RPC); err != nil {
		return err
	}
	if len(m.RPC.Write) > 0 && len(m.Actions) == 0 {
		return problem("rpc.write", "write methods are only called by actions; declare at least one action")
	}
	if err := validateActions(m.Actions); err != nil {
		return err
	}
	if len(m.Actions) > 0 && m.Backend == nil {
		return problem("actions", "actions are implemented by the package backend; declare backend")
	}
	if b := m.Backend; b != nil {
		seen := map[string]bool{}
		for i, n := range b.Network {
			if n != NetworkLAN && n != NetworkLoopback {
				return problem(fmt.Sprintf("backend.network[%d]", i), "must be %q or %q", NetworkLAN, NetworkLoopback)
			}
			if seen[n] {
				return problem(fmt.Sprintf("backend.network[%d]", i), "%q listed twice", n)
			}
			seen[n] = true
		}
	}
	return nil
}

// validateText는 언어별 문구를 검사한다. 키는 지원 언어, en은 required일 때 필수, 값은 1~max 글자이고
// 제어·서식 문자(양방향 재정의 포함)가 없어야 한다.
func validateText(field string, t Text, max int, required bool) error {
	if len(t) == 0 {
		if required {
			return problem(field, "%q text is required", FallbackLanguage)
		}
		return nil
	}
	if _, ok := t[FallbackLanguage]; !ok {
		return problem(field, "%q text is required", FallbackLanguage)
	}
	for _, lang := range slices.Sorted(maps.Keys(t)) {
		if !slices.Contains(Languages, lang) {
			return problem(field+"."+lang, "unsupported language")
		}
		if err := checkString(t[lang], max); err != "" {
			return problem(field+"."+lang, "%s", err)
		}
	}
	return nil
}

// checkString은 화면에 보일 평문을 검사한다. 문제가 없으면 빈 문자열이다.
func checkString(s string, max int) string {
	if !utf8.ValidString(s) {
		return "must be valid UTF-8"
	}
	n := utf8.RuneCountInString(s)
	if n == 0 || strings.TrimSpace(s) == "" {
		return "must not be empty"
	}
	if n > max {
		return fmt.Sprintf("must be at most %d characters", max)
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return fmt.Sprintf("must not contain control or format characters (U+%04X)", r)
		}
	}
	// 호스트는 패키지 이름·조작 문구를 번역 리소스(i18next)에 넣어 그린다. 치환·중첩 문법이 있으면 다른 문구
	// (내장 앱 이름 등)를 끌어올 수 있으므로 받지 않는다.
	for _, seq := range []string{"{{", "}}", "$t("} {
		if strings.Contains(s, seq) {
			return fmt.Sprintf("must not contain %q", seq)
		}
	}
	return ""
}

// CheckText는 패키지가 보낸 평문(번역 문구 등)이 화면에 보일 수 있는지다. 문제가 없으면 빈 문자열이다.
func CheckText(s string, max int) string { return checkString(s, max) }

func validateAuthor(a Author) error {
	if err := checkString(a.Name, MaxAuthorLen); err != "" {
		return problem("author.name", "%s", err)
	}
	if a.URL == "" {
		return nil
	}
	if len(a.URL) > MaxURLLen {
		return problem("author.url", "must be at most %d characters", MaxURLLen)
	}
	u, err := url.Parse(a.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return problem("author.url", "must be an http or https URL")
	}
	if e := checkString(a.URL, MaxURLLen); e != "" {
		return problem("author.url", "%s", e)
	}
	return nil
}

func validateViews(v Views) error {
	if v.Window == nil && v.Widget == nil {
		return problem("views", "at least one view (window or widget) is required")
	}
	if w := v.Window; w != nil {
		if !entryPattern.MatchString(w.Entry) {
			return problem("views.window.entry", "must be an .html file directly under ui/")
		}
		side := func(s api.Size) bool {
			return s.Width > 0 && s.Height > 0 && s.Width <= MaxWindowSide && s.Height <= MaxWindowSide
		}
		if !side(w.DefaultSize) {
			return problem("views.window.defaultSize", "must be 1..%d pixels", MaxWindowSide)
		}
		if mn := w.MinSize; mn != nil && (!side(*mn) || mn.Width > w.DefaultSize.Width || mn.Height > w.DefaultSize.Height) {
			return problem("views.window.minSize", "must be positive and not exceed the default size")
		}
	}
	if w := v.Widget; w != nil {
		if !entryPattern.MatchString(w.Entry) {
			return problem("views.widget.entry", "must be an .html file directly under ui/")
		}
		if err := addons.ValidateWidgetView(&api.WidgetView{DefaultSize: w.DefaultSize, MinSize: w.MinSize, MaxSize: w.MaxSize}); err != nil {
			return problem("views.widget", "%v", err)
		}
	}
	return nil
}

func validateRequires(r Requires) error {
	seen := map[string]bool{}
	for i, p := range r.Plugins {
		field := fmt.Sprintf("requires.plugins[%d]", i)
		if !pluginPattern.MatchString(p.Package) || len(p.Package) > 100 {
			return problem(field+".package", "must be an openmediavault-* package name")
		}
		if !rpcFilePattern.MatchString(p.RPCFile) || len(p.RPCFile) > 100 {
			return problem(field+".rpcFile", "must be a file name like nut.inc")
		}
		if seen[p.Package] {
			return problem(field, "plugin %q listed twice", p.Package)
		}
		seen[p.Package] = true
	}
	return nil
}

// MaxMethods는 읽기·쓰기 메서드 수의 상한(합)이다.
const MaxMethods = 64

func validateRPC(r RPC) error {
	if len(r.Read)+len(r.Write) > MaxMethods {
		return problem("rpc", "at most %d methods", MaxMethods)
	}
	for kind, list := range map[string][]string{"rpc.read": r.Read, "rpc.write": r.Write} {
		for i, m := range list {
			if !addons.ValidMethod(m) || len(m) > 100 {
				return problem(kind+"["+strconv.Itoa(i)+"]", "must be Service.method")
			}
		}
	}
	return nil
}

// MaxActions는 조작 수의 상한이다.
const MaxActions = 32

func validateActions(list []Action) error {
	if len(list) > MaxActions {
		return problem("actions", "at most %d actions", MaxActions)
	}
	seen := map[string]bool{}
	for i, a := range list {
		field := fmt.Sprintf("actions[%d]", i)
		if !addons.ValidID(a.Name) || len(a.Name) > 32 {
			return problem(field+".name", "must be lowercase letters, digits and '-'")
		}
		if seen[a.Name] {
			return problem(field+".name", "action %q declared twice", a.Name)
		}
		seen[a.Name] = true
		if a.TimeoutSeconds < 0 || a.TimeoutSeconds > MaxActionTimeout {
			return problem(field+".timeoutSeconds", "must be 1..%d seconds", MaxActionTimeout)
		}
		if err := validateText(field+".label", a.Label, MaxLabelLen, false); err != nil {
			return err
		}
		if err := validateText(field+".confirmText", a.ConfirmText, MaxConfirmLen, a.Confirm); err != nil {
			return err
		}
	}
	return nil
}

// Timeout은 조작의 시간 제한(초)이다.
func (a Action) Timeout() int {
	if a.TimeoutSeconds == 0 {
		return DefaultActionTimeout
	}
	return a.TimeoutSeconds
}

// CompareVersions는 두 SemVer 버전(versionPattern)을 비교한다(a < b면 음수). 사전 배포 표시가 있으면 없는 것보다
// 낮고, 표시끼리는 점으로 나눈 조각을 차례로 비교한다(숫자는 수로, 나머지는 문자열로, 숫자가 문자열보다 낮다).
func CompareVersions(a, b string) int {
	ca, pa, _ := strings.Cut(a, "-")
	cb, pb, _ := strings.Cut(b, "-")
	na, nb := strings.Split(ca, "."), strings.Split(cb, ".")
	for i := 0; i < 3 && i < len(na) && i < len(nb); i++ {
		x, _ := strconv.Atoi(na[i])
		y, _ := strconv.Atoi(nb[i])
		if x != y {
			return x - y
		}
	}
	switch {
	case pa == pb:
		return 0
	case pa == "":
		return 1
	case pb == "":
		return -1
	}
	sa, sb := strings.Split(pa, "."), strings.Split(pb, ".")
	for i := 0; i < len(sa) && i < len(sb); i++ {
		x, ex := strconv.Atoi(sa[i])
		y, ey := strconv.Atoi(sb[i])
		switch {
		case ex == nil && ey == nil:
			if x != y {
				return x - y
			}
		case ex == nil:
			return -1
		case ey == nil:
			return 1
		default:
			if c := strings.Compare(sa[i], sb[i]); c != 0 {
				return c
			}
		}
	}
	return len(sa) - len(sb)
}
