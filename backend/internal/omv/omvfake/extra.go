package omvfake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// 개발용 추가 RPC(개발 서버의 FAKEOMV_EXTRA_RPC). 애드온 패키지 개발자가 가짜 OMV에 없는 메서드(주로 플러그인 RPC)에
// 정해진 응답을 받게 한다. 파일 형식(패키지 개발 디렉터리의 dev/fake-rpc.json):
//
//	{
//	  "Example.getStatus": { "result": { "running": true }, "admin": true },
//	  "Example.getList": { "result": [] }
//	}
//
//   - 키는 "Service.method"다. 서비스 이름은 실제 OMV처럼 대소문자를 가리지 않고 메서드 이름은 그대로 비교한다
//   - result: 그 메서드의 결과(필수, null도 된다). 파라미터와 상관없이 늘 같은 값이다
//   - admin: true면 관리자 세션만 받는다(아니면 OMV의 validateMethodContext처럼 403 "Invalid context role.")
//   - 가짜 OMV가 이미 구현한 메서드에는 쓰이지 않는다(구현이 이긴다). Session 서비스는 둘 수 없고, 세션 검사(인증,
//     만료, User-Agent)는 다른 메서드와 같다

// ExtraRPC는 추가 메서드 하나의 응답이다.
type ExtraRPC struct {
	// Result는 결과 JSON이다.
	Result json.RawMessage
	// Admin이면 관리자 세션만 받는다.
	Admin bool
}

// extraName은 추가 메서드 키의 형식이다(OMV 서비스·메서드 이름은 글자와 숫자다).
var extraName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*\.[A-Za-z][A-Za-z0-9]*$`)

// extraKey는 추가 메서드를 찾는 키다. 서비스 이름만 소문자로 맞춘다(rpc/servicemanager.inc의 mb_strtolower).
func extraKey(service, method string) string {
	return strings.ToLower(service) + "." + method
}

// ParseExtraRPC는 추가 RPC 파일 하나를 읽는다. 모르는 필드, 형식이 틀린 키, result가 없는 항목은 오류다.
// 돌려주는 맵의 키는 파일에 적힌 이름 그대로다.
func ParseExtraRPC(data []byte) (map[string]ExtraRPC, error) {
	var raw map[string]map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("want a JSON object of \"Service.method\": {\"result\": …, \"admin\": bool}: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("trailing data after the JSON object")
	}
	if raw == nil {
		return nil, fmt.Errorf("want a JSON object, got null")
	}
	out := make(map[string]ExtraRPC, len(raw))
	seen := map[string]string{}
	for name, entry := range raw {
		if !extraName.MatchString(name) {
			return nil, fmt.Errorf("%q: want \"Service.method\"", name)
		}
		service, method, _ := strings.Cut(name, ".")
		if strings.EqualFold(service, "Session") {
			return nil, fmt.Errorf("%q: Session methods cannot be faked", name)
		}
		key := extraKey(service, method)
		if prev, ok := seen[key]; ok {
			return nil, fmt.Errorf("%q and %q are the same method", prev, name)
		}
		seen[key] = name
		if entry == nil {
			return nil, fmt.Errorf("%q: want an object with \"result\"", name)
		}
		var e ExtraRPC
		for k, v := range entry {
			switch k {
			case "result":
				e.Result = v
			case "admin":
				if err := json.Unmarshal(v, &e.Admin); err != nil {
					return nil, fmt.Errorf("%q: admin must be true or false", name)
				}
			default:
				return nil, fmt.Errorf("%q: unknown field %q (want result, admin)", name, k)
			}
		}
		if e.Result == nil {
			return nil, fmt.Errorf("%q: result is required (use null for no result)", name)
		}
		out[name] = e
	}
	return out, nil
}

// SetExtraRPC는 추가 메서드를 바꾼다. 키는 "Service.method"다(ParseExtraRPC의 결과).
func (s *Server) SetExtraRPC(m map[string]ExtraRPC) {
	extra := make(map[string]ExtraRPC, len(m))
	for name, e := range m {
		service, method, _ := strings.Cut(name, ".")
		extra[extraKey(service, method)] = e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extra = extra
}

// ExtraRPCNames는 추가 메서드 키(소문자 서비스.메서드)다. 시험이 쓴다.
func (s *Server) ExtraRPCNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Sorted(maps.Keys(s.extra))
}

// handleExtra는 가짜 OMV가 구현하지 않은 메서드를 추가 메서드로 답한다. 없으면 ok가 false다. s.mu를 쥔 채로 부른다.
func (s *Server) handleExtra(sess *session, service, method string) (any, *rpcError, bool) {
	e, ok := s.extra[extraKey(service, method)]
	if !ok {
		return nil, nil, false
	}
	if e.Admin && !sess.admin {
		return nil, errStatus(http.StatusForbidden, "Invalid context role."), true
	}
	return e.Result, nil, true
}
