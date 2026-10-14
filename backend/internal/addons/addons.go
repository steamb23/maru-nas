// Package addons는 애드온 계약(manifest, provider, RPC 허용 목록)을 정의한다.
//
// 애드온은 manifest(api.Manifest)와 우선순위 순서의 provider 목록이다. 내장 애드온은
// internal/addons/<id>/에 두고 internal/addons/builtin의 정적 목록에 등록한다.
// 런타임 로딩이나 Go plugin 패키지는 쓰지 않는다(docs/addons.md).
package addons

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"omv-desktop-dashboard/backend/internal/api"
)

// WritesEnabled는 상태를 바꾸는 OMV RPC(manifest의 rpc.write)를 중계할지다.
// 쓰기 허용 여부는 이 상수 한 곳에서 정한다. 3단계(읽기 전용)에서는 꺼 두고 4단계에서 켠다.
const WritesEnabled = false

// Provider는 한 항목의 데이터를 한 출처에서 가져오는 구현이다.
// 데이터 메서드는 애드온마다 다르므로 여기에는 capability 계산에 필요한 것만 둔다.
type Provider interface {
	// Name은 출처 이름이다(proc, hwmon 등). 응답과 capability의 sources에 담긴다.
	Name() string
	// Requires는 이 provider를 쓰는 데 필요한 플러그인과 역할이다.
	Requires() api.Requirements
}

// Addon은 내장 애드온 하나다.
type Addon struct {
	Manifest api.Manifest
	// Providers는 우선순위 순서다. 쓸 수 있는 것 중 앞의 것을 먼저 쓴다.
	Providers []Provider
}

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
	if m.Title == "" || m.Icon == "" {
		return fmt.Errorf("addon %q: title and icon are required", m.ID)
	}
	if m.Views.Window == nil {
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
	if err := validateRequires(m.Requires); err != nil {
		return fmt.Errorf("addon %q: %w", m.ID, err)
	}
	seen := make(map[string]bool)
	for _, method := range slices.Concat(m.RPC.Read, m.RPC.Write) {
		if !methodPattern.MatchString(method) {
			return fmt.Errorf("addon %q: invalid rpc method %q (want Service.method)", m.ID, method)
		}
		if seen[method] {
			return fmt.Errorf("addon %q: rpc %s declared twice", m.ID, method)
		}
		seen[method] = true
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
		if err := validateRequires(p.Requires()); err != nil {
			return fmt.Errorf("addon %q: provider %q: %w", m.ID, p.Name(), err)
		}
	}
	return nil
}

func validateRequires(req api.Requirements) error {
	for _, p := range req.Plugins {
		if _, ok := knownPlugins[p]; !ok {
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

// Classify는 OMV RPC 메서드가 허용 목록에서 어떤 종류인지와 그 메서드를 선언한 애드온 id를 반환한다.
// 서비스·메서드 이름은 대소문자까지 정확히 같아야 한다.
func (r *Registry) Classify(service, method string) (RPCKind, []string) {
	e, ok := r.methods[service+"."+method]
	if !ok {
		return RPCNone, nil
	}
	return e.kind, slices.Clone(e.addons)
}
