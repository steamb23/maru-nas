// Package addons는 애드온 계약(manifest, provider, RPC 허용 목록)을 정의한다.
//
// 애드온은 manifest(api.Manifest)와 우선순위 순서의 provider 목록이다. 내장 애드온은
// internal/addons/<id>/에 두고 internal/addons/builtin의 정적 목록에 등록한다. 내장 목록에는
// 런타임 로딩이나 Go plugin 패키지를 쓰지 않는다(.claude/docs/addons.md).
//
// 저장소 밖 애드온(애드온 패키지, internal/addonpkg)은 Extend로 내장 레지스트리에 한 건씩 더한다.
// 패키지는 Kind가 KindPackage이고 Go 구현(Fetch, Actions)이 없다(.claude/docs/addon-packages.md).
package addons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"maru/backend/internal/api"
)

// WritesEnabled는 상태를 바꾸는 OMV RPC(manifest의 rpc.write)를 부를 수 있는지다.
// 쓰기 허용 여부는 이 상수 한 곳에서 정한다. 끄면 조작에서도 쓰기 RPC를 부를 수 없다.
//
// 켜져 있어도 쓰기는 애드온 조작(Addon.Actions)으로만 부른다. /api/rpc 중계와 Fetch는 쓰기를
// 거부한다. Compose.doContainerCommand처럼 파라미터를 검사하지 않고 셸 명령을 만드는 OMV 메서드가
// 있어서, 파라미터는 브라우저가 아니라 애드온 코드가 만들어야 하기 때문이다.
const WritesEnabled = true

// Provider는 한 항목의 데이터를 한 출처에서 가져오는 구현이다.
// 데이터 메서드는 애드온마다 다르므로 여기에는 capability 계산에 필요한 것만 둔다.
type Provider interface {
	// Name은 출처 이름이다(proc, hwmon 등). 응답과 capability의 sources에 담긴다.
	Name() string
	// Requires는 이 provider를 쓰는 데 필요한 플러그인과 역할이다.
	Requires() api.Requirements
}

// Kind는 애드온의 출처다.
type Kind int

const (
	// KindBuiltin은 저장소 안에서 함께 빌드한 애드온이다(0값).
	KindBuiltin Kind = iota
	// KindPackage는 관리자가 설치한 애드온 패키지다. 보기는 샌드박스 iframe이고, 데이터·조작은
	// 패키지 백엔드가 맡으므로 Fetch·Actions가 없다.
	KindPackage
)

// Addon은 애드온 하나다.
type Addon struct {
	Kind     Kind
	Manifest api.Manifest
	// Plugins는 패키지가 requires.plugins에 밝힌 플러그인 패키지 → RPC 파일 이름이다(knownPlugins에 없는
	// 플러그인도 쓸 수 있게). 내장 애드온은 비워 두고 knownPlugins만 쓴다.
	Plugins map[string]string
	// Providers는 우선순위 순서다. 쓸 수 있는 것 중 앞의 것을 먼저 쓴다.
	Providers []Provider
	// Fetch는 GET /api/addons/{id}의 응답을 만든다. 없으면 그 경로는 404다.
	// 서버는 capability를 먼저 확인하므로 Fetch는 쓸 수 있는 사용자에게만 불린다.
	Fetch func(ctx context.Context, req Request) (any, error)
	// Actions는 manifest의 actions에 선언한 조작의 구현이다(이름 → 구현). 선언과 1:1로 맞아야 한다.
	Actions map[string]Action
}

// Action은 조작 하나의 구현이다(POST /api/addons/{id}/actions/{name}). Run과 Query 중 하나만 둔다.
type Action struct {
	// Timeout은 Run·Query 전체에 주는 시간이다. 0이면 DefaultActionTimeout이다.
	Timeout time.Duration
	// Run은 조작을 실행한다. 대상(Target)은 브라우저가 보낸 값이므로 OMV에 넘기기 전에 반드시 검사한다.
	// 잘못된 대상은 ErrInvalidTarget, 사용자에게 이유를 알려야 하는 거절은 *ActionError로 돌려준다.
	// 성공하면 서버가 본문 없이 204로 답한다.
	Run func(ctx context.Context, req ActionRequest) error
	// Query는 결과를 돌려주는 조작이다(시스템 업데이트의 변경 기록). 쓰기 RPC로 선언해야 하는 메서드
	// (파라미터로 셸 명령을 만드는 메서드 등)를 불러 데이터를 읽을 때 쓴다. 대상 검사와 오류는 Run과 같고,
	// 성공하면 서버가 결과를 JSON 200으로 답한다.
	Query func(ctx context.Context, req ActionRequest) (any, error)
}

// DefaultActionTimeout은 Action.Timeout이 0일 때 쓰는 시간이다.
const DefaultActionTimeout = time.Minute

// ActionRequest는 조작 호출 하나의 입력이다.
type ActionRequest struct {
	// Admin은 요청한 사용자가 OMV 관리자인지다.
	Admin bool
	// Target은 조작 대상(컨테이너 이름, 서비스 이름 등)이다. 검사하지 않은 값이다.
	Target string
	// RPC는 그 애드온이 선언한 읽기·쓰기 메서드를 부를 수 있다.
	RPC RPCCaller
	// Jobs는 요청한 세션으로 서버 백그라운드 작업을 시작한다(시스템 업데이트). 작업을 쓰지 않는 조작은 무시한다.
	Jobs JobStarter
}

// JobStarter는 조작이 서버 백그라운드 작업(internal/jobs)을 시작하는 입구다. 서버가 요청한 세션에 묶어 만든다
// (애드온은 세션 ID를 받지 않는다). 작업을 시작하고 바로 돌아오며, 거절은 *ActionError다(409 job_busy,
// apt_locked, config_dirty).
type JobStarter interface {
	Start(ctx context.Context, kind string) (id string, err error)
	// Busy는 서버 백그라운드 작업이 진행 중인지(시작을 준비하는 중 포함)다. apt 잠금을 함께 쓰는 조작이
	// 작업과 겹치지 않게 거절할 때 쓴다.
	Busy() bool
}

var (
	// ErrInvalidTarget은 조작 대상이 형식에 맞지 않거나 없다는 뜻이다(400 bad_request).
	ErrInvalidTarget = errors.New("invalid action target")
	// ErrActionFailed는 OMV 백그라운드 작업이 실패했다는 뜻이다(502 action_failed).
	ErrActionFailed = errors.New("action failed")
	// ErrActionTimeout은 Timeout 안에 작업이 끝나지 않았다는 뜻이다(504 action_timeout).
	// OMV 쪽 작업은 멈추지 않고 계속 돌 수 있다.
	ErrActionTimeout = errors.New("action timed out")
)

// ActionError는 조작을 시작하지 않고 거절한 이유다. 서버가 Status와 Code를 그대로 응답한다.
type ActionError struct {
	Status int
	// Code는 api.ErrorResponse.Code 값이다.
	Code    string
	Message string
}

func (e *ActionError) Error() string { return e.Message }

// Request는 Fetch 호출 하나의 입력이다.
type Request struct {
	// Admin은 요청한 사용자가 OMV 관리자인지다.
	Admin bool
	// Sources는 이 사용자가 쓸 수 있는 provider 이름이다(우선순위 순서, Capability의 sources).
	Sources []string
	// Refresh는 사용자가 새로고침을 눌렀는지다(?refresh=1). 캐시를 쓰는 애드온만 본다.
	Refresh bool
	// Query는 요청의 조회 파라미터다. 브라우저가 보낸 값이므로 애드온이 자기가 정한 이름·값만 읽는다
	// (시스템 정보의 processes=1). OMV에 그대로 넘기지 않는다.
	Query url.Values
	// RPC는 요청한 사용자의 OMV 세션으로 OMV RPC를 부른다.
	RPC RPCCaller
}

// Has는 name provider를 쓸 수 있는지다.
func (r Request) Has(name string) bool { return slices.Contains(r.Sources, name) }

// RPCCaller는 provider가 OMV RPC를 부를 때 쓴다. 서버가 요청마다 사용자 세션으로 만든다.
//
// 그 애드온이 manifest에 선언한 메서드만 부를 수 있다(Registry.Classify). Fetch에 넘기는 호출기는
// 읽기만, 조작(Action.Run·Query)에 넘기는 호출기는 WritesEnabled가 켜져 있으면 쓰기도 부를 수 있다.
// OMV 세션의 마지막 접근 시각은 갱신하지 않는다.
type RPCCaller interface {
	Call(ctx context.Context, service, method string, params any) (json.RawMessage, error)
}

// ErrRPCNotAllowed는 애드온이 선언하지 않은 RPC를 부르거나, 쓰기를 부를 수 없는 곳(Fetch, WritesEnabled가
// 꺼져 있을 때)에서 쓰기 RPC를 부르려 했다는 뜻이다.
var ErrRPCNotAllowed = errors.New("rpc method is not allowed for this addon")

// RPCKind는 OMV RPC 메서드가 허용 목록에서 어떤 종류인지다.
type RPCKind int

const (
	// RPCNone은 어떤 애드온도 선언하지 않은 메서드다.
	RPCNone RPCKind = iota
	RPCRead
	RPCWrite
)

// Registry는 등록된 애드온 목록이다. 만든 뒤에는 바꾸지 않으므로 여러 고루틴에서 함께 써도 된다.
type Registry struct {
	addons []Addon
	byID   map[string]int
	// methods는 "Service.method" → 종류와 그 메서드를 선언한 애드온 id 목록이다.
	methods map[string]methodEntry
}

type methodEntry struct {
	kind   RPCKind
	addons []string
}

var (
	idPattern     = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	methodPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*\.[A-Za-z][A-Za-z0-9]*$`)
)

// NewRegistry는 애드온 목록을 검사해 Registry를 만든다. 순서는 그대로 유지한다.
func NewRegistry(list ...Addon) (*Registry, error) {
	r := &Registry{byID: make(map[string]int), methods: make(map[string]methodEntry)}
	var errs []error
	for _, a := range list {
		if err := validate(a); err != nil {
			errs = append(errs, err)
			continue
		}
		id := a.Manifest.ID
		if _, dup := r.byID[id]; dup {
			errs = append(errs, fmt.Errorf("addon %q: duplicate id", id))
			continue
		}
		if err := r.addMethods(id, a.Manifest.RPC); err != nil {
			errs = append(errs, err)
			continue
		}
		r.byID[id] = len(r.addons)
		r.addons = append(r.addons, a)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return r, nil
}

// addMethods는 애드온의 RPC 메서드를 등록한다. 한 메서드가 읽기와 쓰기로 함께 선언되면 거부한다.
func (r *Registry) addMethods(id string, rpc api.RPCMethods) error {
	for kind, methods := range map[RPCKind][]string{RPCRead: rpc.Read, RPCWrite: rpc.Write} {
		for _, m := range methods {
			if e, ok := r.methods[m]; ok && e.kind != kind {
				return fmt.Errorf("addon %q: rpc %s declared as both read and write", id, m)
			}
		}
	}
	for kind, methods := range map[RPCKind][]string{RPCRead: rpc.Read, RPCWrite: rpc.Write} {
		for _, m := range methods {
			e := r.methods[m]
			e.kind = kind
			if !slices.Contains(e.addons, id) {
				e.addons = append(e.addons, id)
			}
			r.methods[m] = e
		}
	}
	return nil
}

func validate(a Addon) error {
	m := a.Manifest
	if !idPattern.MatchString(m.ID) {
		return fmt.Errorf("addon %q: invalid id", m.ID)
	}
	// 패키지의 이름·아이콘은 번역 키·아이콘 이름이 아니라 패키지 파일에 있다(addonpkg).
	if a.Kind == KindBuiltin && (m.Title == "" || m.Icon == "") {
		return fmt.Errorf("addon %q: title and icon are required", m.ID)
	}
	if m.Views.Window == nil && m.Views.Widget == nil {
		return fmt.Errorf("addon %q: at least one view is required", m.ID)
	}
	if w := m.Views.Window; w != nil {
		if w.DefaultSize.Width <= 0 || w.DefaultSize.Height <= 0 {
			return fmt.Errorf("addon %q: window default size must be positive", m.ID)
		}
		if mn := w.MinSize; mn != nil && (mn.Width <= 0 || mn.Height <= 0 ||
			mn.Width > w.DefaultSize.Width || mn.Height > w.DefaultSize.Height) {
			return fmt.Errorf("addon %q: window min size must be positive and not exceed the default size", m.ID)
		}
	}
	if w := m.Views.Widget; w != nil {
		if err := validateWidget(w); err != nil {
			return fmt.Errorf("addon %q: %w", m.ID, err)
		}
	}
	if err := validateRequires(m.Requires, a.Plugins); err != nil {
		return fmt.Errorf("addon %q: %w", m.ID, err)
	}
	seen := make(map[string]bool)
	for _, method := range slices.Concat(m.RPC.Read, m.RPC.Write) {
		if !methodPattern.MatchString(method) {
			return fmt.Errorf("addon %q: invalid rpc method %q (want Service.method)", m.ID, method)
		}
		// 로그인·로그아웃 같은 Session 서비스는 백엔드만 부른다.
		if strings.HasPrefix(method, "Session.") {
			return fmt.Errorf("addon %q: rpc %s is reserved for the backend", m.ID, method)
		}
		if seen[method] {
			return fmt.Errorf("addon %q: rpc %s declared twice", m.ID, method)
		}
		seen[method] = true
	}
	if err := validateActions(a); err != nil {
		return fmt.Errorf("addon %q: %w", m.ID, err)
	}
	if len(a.Providers) == 0 {
		return fmt.Errorf("addon %q: at least one provider is required", m.ID)
	}
	names := make(map[string]bool)
	for _, p := range a.Providers {
		if p.Name() == "" || names[p.Name()] {
			return fmt.Errorf("addon %q: provider names must be unique and non-empty", m.ID)
		}
		names[p.Name()] = true
		if err := validateRequires(p.Requires(), a.Plugins); err != nil {
			return fmt.Errorf("addon %q: provider %q: %w", m.ID, p.Name(), err)
		}
	}
	return nil
}

// validateActions는 manifest의 actions 선언과 구현이 1:1로 맞는지 확인한다.
// 패키지는 선언만 있고 구현은 패키지 백엔드에 있으므로 Actions가 비어 있어야 한다.
// "내장 조작이 있으면 쓰기 RPC 1개 이상"은 builtin의 테스트가 본다(패키지 조작은 OMV 쓰기 없이도 된다).
func validateActions(a Addon) error {
	decl := a.Manifest.Actions
	seen := make(map[string]bool)
	for _, d := range decl {
		if !idPattern.MatchString(d.Name) {
			return fmt.Errorf("invalid action name %q", d.Name)
		}
		if seen[d.Name] {
			return fmt.Errorf("action %q declared twice", d.Name)
		}
		seen[d.Name] = true
		if a.Kind == KindPackage {
			continue
		}
		act, ok := a.Actions[d.Name]
		if !ok || (act.Run == nil && act.Query == nil) {
			return fmt.Errorf("action %q has no implementation", d.Name)
		}
		if act.Run != nil && act.Query != nil {
			return fmt.Errorf("action %q has both Run and Query", d.Name)
		}
	}
	if a.Kind == KindPackage && (len(a.Actions) > 0 || a.Fetch != nil) {
		return errors.New("package addons must not have Go implementations")
	}
	for name := range a.Actions {
		if !seen[name] {
			return fmt.Errorf("action %q is not declared in the manifest", name)
		}
	}
	return nil
}

// MaxWidgetSpan은 위젯 한 변의 최대 칸 수다.
const MaxWidgetSpan = 12

// validateWidget은 위젯 크기가 1~MaxWidgetSpan 칸이고 최소 ≤ 기본 ≤ 최대인지 확인한다.
func validateWidget(w *api.WidgetView) error {
	inRange := func(s api.GridSize) bool {
		return s.Cols >= 1 && s.Rows >= 1 && s.Cols <= MaxWidgetSpan && s.Rows <= MaxWidgetSpan
	}
	if !inRange(w.DefaultSize) {
		return fmt.Errorf("widget default size must be 1..%d cells", MaxWidgetSpan)
	}
	if mn := w.MinSize; mn != nil && (!inRange(*mn) || mn.Cols > w.DefaultSize.Cols || mn.Rows > w.DefaultSize.Rows) {
		return fmt.Errorf("widget min size must be 1..%d cells and not exceed the default size", MaxWidgetSpan)
	}
	if mx := w.MaxSize; mx != nil && (!inRange(*mx) || mx.Cols < w.DefaultSize.Cols || mx.Rows < w.DefaultSize.Rows) {
		return fmt.Errorf("widget max size must be 1..%d cells and not be below the default size", MaxWidgetSpan)
	}
	return nil
}

// validateRequires는 요구한 플러그인이 knownPlugins나 그 애드온이 밝힌 표(extra, 패키지만)에 있는지 본다.
func validateRequires(req api.Requirements, extra map[string]string) error {
	for _, p := range req.Plugins {
		if _, ok := knownPlugins[p]; ok {
			continue
		}
		if _, ok := extra[p]; !ok {
			return fmt.Errorf("unknown plugin %q (add it to knownPlugins)", p)
		}
	}
	return nil
}

// All은 등록 순서대로 애드온을 반환한다.
func (r *Registry) All() []Addon { return slices.Clone(r.addons) }

// Get은 id의 애드온을 반환한다.
func (r *Registry) Get(id string) (Addon, bool) {
	i, ok := r.byID[id]
	if !ok {
		return Addon{}, false
	}
	return r.addons[i], true
}

// ActionDecl은 id 애드온의 name 조작 선언과 구현을 반환한다.
func (r *Registry) ActionDecl(id, name string) (api.ActionDecl, Action, bool) {
	a, ok := r.Get(id)
	if !ok {
		return api.ActionDecl{}, Action{}, false
	}
	for _, d := range a.Manifest.Actions {
		if d.Name == name {
			return d, a.Actions[name], true
		}
	}
	return api.ActionDecl{}, Action{}, false
}

// Classify는 OMV RPC 메서드가 허용 목록에서 어떤 종류인지와 그 메서드를 선언한 애드온 id를 반환한다.
// 서비스·메서드 이름은 대소문자까지 정확히 같아야 한다.
func (r *Registry) Classify(service, method string) (RPCKind, []string) {
	e, ok := r.methods[service+"."+method]
	if !ok {
		return RPCNone, nil
	}
	return e.kind, slices.Clone(e.addons)
}

// CallInto는 RPC를 부르고 결과 JSON을 out에 디코딩한다.
func CallInto(ctx context.Context, c RPCCaller, service, method string, params, out any) error {
	raw, err := c.Call(ctx, service, method, params)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("rpc %s.%s: decode response: %w", service, method, err)
	}
	return nil
}

// BgPollInterval은 WaitBg가 OMV 백그라운드 작업 상태를 묻는 간격이다.
const BgPollInterval = 500 * time.Millisecond

// WaitBg는 OMV 백그라운드 작업이 끝날 때까지 기다린다. filename은 execBgProc·callMethodBg를 쓰는
// 메서드(Compose.doContainerCommand, Config.applyChangesBg 등)가 돌려준 상태 파일 이름이다.
//
// Exec.isRunning(관리자 전용)을 interval마다 부르며, running이 false가 되면 성공이다. 작업이
// 실패하면 OMV가 Exec.isRunning에서 예외를 던지므로 ErrActionFailed로 감싼다(오류 내용은
// 로그용이며 브라우저에 넘기지 않는다). Exec.isRunning은 끝난 작업의 상태 파일을 지운다.
// ctx가 끝나면 ErrActionTimeout을 돌려준다. 이때 OMV 쪽 작업은 계속 돌고 상태 파일도 남는다.
func WaitBg(ctx context.Context, c RPCCaller, filename string, interval time.Duration) error {
	if filename == "" {
		return fmt.Errorf("%w: empty background status file name", ErrActionFailed)
	}
	for {
		var res struct {
			Running bool `json:"running"`
		}
		err := CallInto(ctx, c, "Exec", "isRunning", map[string]string{"filename": filename}, &res)
		switch {
		case ctx.Err() != nil:
			return ErrActionTimeout
		case err != nil:
			return fmt.Errorf("%w: %w", ErrActionFailed, err)
		case !res.Running:
			return nil
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return ErrActionTimeout
		case <-t.C:
		}
	}
}

// bgOutputChunk는 ReadBg가 Exec.getOutput 한 번에 읽는 길이다(OMV 스키마의 최소 1024 이상).
const bgOutputChunk = 512 << 10

// bgOutput은 Exec.getOutput 응답이다. pos는 다음에 읽을 위치(출력 파일의 바이트 위치)다.
type bgOutput struct {
	Pos           int64  `json:"pos"`
	Output        string `json:"output"`
	PendingOutput bool   `json:"pendingOutput"`
	Running       bool   `json:"running"`
}

// ReadBg는 OMV 백그라운드 작업(execBgProc)의 출력을 끝까지 읽고 앞에서부터 limit 바이트까지 돌려준다.
// cut은 limit을 넘어 뒤를 버렸는지다(UTF-8 글자 경계는 지키지 않는다).
//
// Exec.getOutput(관리자 전용)을 읽을 출력이 남아 있으면 바로, 작업이 진행 중이면 interval마다 부른다.
// OMV는 작업이 끝나고 출력을 다 읽어야 상태·출력 파일을 지우므로 limit을 넘은 뒤에도 끝까지 읽고 버린다.
// 작업이 실패하면 OMV가 Exec.getOutput에서 예외를 던지며, 그 오류를 감싸지 않고 돌려준다(호출한 쪽이
// 문구로 원인을 가리고, 문구는 로그에만 남긴다). ctx가 끝나면 ErrActionTimeout이고 이때 파일은 남는다.
func ReadBg(ctx context.Context, c RPCCaller, filename string, interval time.Duration, limit int) (string, bool, error) {
	if filename == "" {
		return "", false, fmt.Errorf("%w: empty background status file name", ErrActionFailed)
	}
	var (
		buf strings.Builder
		cut bool
		pos int64
	)
	for {
		var res bgOutput
		err := CallInto(ctx, c, "Exec", "getOutput",
			map[string]any{"filename": filename, "pos": pos, "length": bgOutputChunk}, &res)
		switch {
		case ctx.Err() != nil:
			return "", false, ErrActionTimeout
		case err != nil:
			return "", false, err
		case res.Pos < pos || (res.PendingOutput && res.Pos == pos):
			// 위치가 나아가지 않으면 같은 요청을 끝없이 되풀이하게 된다.
			return "", false, fmt.Errorf("%w: output position did not advance (%d -> %d)", ErrActionFailed, pos, res.Pos)
		}
		pos = res.Pos
		if room := limit - buf.Len(); len(res.Output) > room {
			buf.WriteString(res.Output[:max(room, 0)])
			cut = true
		} else {
			buf.WriteString(res.Output)
		}
		if res.PendingOutput {
			continue
		}
		if !res.Running {
			return buf.String(), cut, nil
		}
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return "", false, ErrActionTimeout
		case <-t.C:
		}
	}
}

// ValidID는 id가 애드온·조작 이름 형식(소문자·숫자·'-')인지다. 애드온 패키지 형식 검사가 같은 규칙을 쓴다.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// ValidMethod는 method가 "Service.method" 형식인지다.
func ValidMethod(method string) bool { return methodPattern.MatchString(method) }

// ValidateWidgetView는 위젯 크기가 1~MaxWidgetSpan 칸이고 최소 ≤ 기본 ≤ 최대인지 확인한다.
func ValidateWidgetView(w *api.WidgetView) error { return validateWidget(w) }
