package addons

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
)

// 패키지를 받지 않은 이유(Rejection.Code). 관리자 페이지가 이유별 문구를 보인다.
const (
	// RejectConflictBuiltin은 내장 애드온과 id가 같다는 뜻이다. 내장이 나중에 같은 id를 쓰게 되면 내장이 이긴다.
	RejectConflictBuiltin = "conflict_builtin"
	// RejectConflictPackage는 먼저 받은 다른 패키지와 id가 같다는 뜻이다.
	RejectConflictPackage = "conflict_package"
	// RejectRPCDenied는 금지 표의 메서드를 선언했다는 뜻이다(DeniedForPackage).
	RejectRPCDenied = "rpc_denied"
	// RejectRPCKind는 다른 애드온이 읽기로 선언한 메서드를 쓰기로(또는 그 반대로) 선언했다는 뜻이다.
	RejectRPCKind = "rpc_kind"
	// RejectReservedID는 셸이 쓰거나 쓸 id라는 뜻이다(ReservedAppIDs).
	RejectReservedID = "reserved_id"
	// RejectInvalid는 그 밖의 검사(크기, 형식, 플러그인 표 등)에 걸렸다는 뜻이다.
	RejectInvalid = "invalid"
)

// Rejection은 Extend가 받지 않은 패키지와 그 이유다. Detail은 로그·관리자 화면용 영어 문장이다.
type Rejection struct {
	ID     string
	Code   string
	Detail string
}

// rpcFilePattern은 패키지가 밝히는 플러그인 RPC 파일 이름 형식이다.
var rpcFilePattern = regexp.MustCompile(`^[a-z0-9_-]+\.inc$`)

// Extend는 내장 레지스트리 base에 애드온 패키지 후보를 순서대로 하나씩 더한 새 레지스트리를 만든다.
// 검사에 걸린 후보는 이유와 함께 돌려주고 나머지와 내장은 그대로 둔다. base는 바꾸지 않는다.
// 후보 순서가 중요하다: 같은 id나 메서드 종류가 겹치면 앞의 것이 이긴다(시작할 때는 설치 순서로 넘긴다).
func Extend(base *Registry, cands []Addon) (*Registry, []Rejection) {
	accepted := base.All()
	builtinIDs := make(map[string]bool, len(accepted))
	for _, a := range accepted {
		builtinIDs[a.Manifest.ID] = true
	}
	packageIDs := make(map[string]bool)
	// kinds는 지금까지 받은 애드온이 선언한 메서드의 종류다(rpcKey 기준).
	kinds := make(map[string]RPCKind)
	addKinds := func(a Addon) {
		for _, m := range a.Manifest.RPC.Read {
			kinds[rpcKey(m)] = RPCRead
		}
		for _, m := range a.Manifest.RPC.Write {
			kinds[rpcKey(m)] = RPCWrite
		}
	}
	for _, a := range accepted {
		addKinds(a)
	}

	var rejected []Rejection
	reject := func(id, code, format string, args ...any) {
		rejected = append(rejected, Rejection{ID: id, Code: code, Detail: fmt.Sprintf(format, args...)})
	}
	for _, c := range cands {
		id := c.Manifest.ID
		switch {
		case c.Kind != KindPackage:
			reject(id, RejectInvalid, "not a package addon")
			continue
		case builtinIDs[id]:
			reject(id, RejectConflictBuiltin, "id %q is used by a built-in addon", id)
			continue
		case packageIDs[id]:
			reject(id, RejectConflictPackage, "id %q is used by another package", id)
			continue
		}
		if code, detail := validatePackage(c); code != "" {
			reject(id, code, "%s", detail)
			continue
		}
		if m, ok := kindConflict(c, kinds); ok {
			reject(id, RejectRPCKind, "rpc %s is declared with a different kind by another addon", m)
			continue
		}
		if _, err := NewRegistry(append(slices.Clone(accepted), c)...); err != nil {
			reject(id, RejectInvalid, "%v", err)
			continue
		}
		accepted = append(accepted, c)
		packageIDs[id] = true
		addKinds(c)
	}
	r, err := NewRegistry(accepted...)
	if err != nil {
		// 위에서 하나씩 통과시켰으므로 일어나지 않는다. 일어나면 내장만 둔다.
		for _, c := range cands {
			if packageIDs[c.Manifest.ID] {
				reject(c.Manifest.ID, RejectInvalid, "%v", err)
			}
		}
		return base, rejected
	}
	return r, rejected
}

// validatePackage는 내장 애드온에는 없는 패키지 규칙을 본다. 통과하면 code가 빈 문자열이다.
func validatePackage(a Addon) (code, detail string) {
	m := a.Manifest
	if !idPattern.MatchString(m.ID) || len(m.ID) > MaxPackageIDLen {
		return RejectInvalid, fmt.Sprintf("invalid package id %q (lowercase letters, digits and '-', at most %d characters)", m.ID, MaxPackageIDLen)
	}
	if IsReservedAppID(m.ID) {
		return RejectReservedID, fmt.Sprintf("id %q is reserved", m.ID)
	}
	for _, method := range slices.Concat(m.RPC.Read, m.RPC.Write) {
		if reason, denied := DeniedForPackage(method); denied {
			return RejectRPCDenied, fmt.Sprintf("rpc %s is not allowed for packages: %s", method, reason)
		}
	}
	// 쓰기는 조작으로만 부르므로, 조작이 없는 쓰기 선언은 실수다.
	if len(m.RPC.Write) > 0 && len(m.Actions) == 0 {
		return RejectInvalid, "write rpc requires at least one action"
	}
	// 같은 메서드를 서비스 이름의 대소문자만 바꿔 두 번 선언하지 못하게 한다(OMV에는 같은 메서드다).
	seen := make(map[string]bool)
	for _, method := range slices.Concat(m.RPC.Read, m.RPC.Write) {
		k := rpcKey(method)
		if seen[k] {
			return RejectInvalid, fmt.Sprintf("rpc %s declared twice", method)
		}
		seen[k] = true
	}
	for _, pkg := range slices.Sorted(maps.Keys(a.Plugins)) {
		file := a.Plugins[pkg]
		if !rpcFilePattern.MatchString(file) {
			return RejectInvalid, fmt.Sprintf("plugin %q: invalid rpc file %q", pkg, file)
		}
		if known, ok := knownPlugins[pkg]; ok && known != file {
			return RejectInvalid, fmt.Sprintf("plugin %q installs %s, not %s", pkg, known, file)
		}
	}
	return "", ""
}

// kindConflict는 a가 선언한 메서드 중 지금까지 받은 애드온과 종류가 다른 것을 찾는다.
func kindConflict(a Addon, kinds map[string]RPCKind) (string, bool) {
	check := func(methods []string, kind RPCKind) (string, bool) {
		for _, m := range methods {
			if k, ok := kinds[rpcKey(m)]; ok && k != kind {
				return m, true
			}
		}
		return "", false
	}
	if m, ok := check(a.Manifest.RPC.Read, RPCRead); ok {
		return m, true
	}
	return check(a.Manifest.RPC.Write, RPCWrite)
}
