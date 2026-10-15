package omvfake

import (
	"encoding/json"
	"net/http"
	"slices"
)

// 사용자 관리 RPC. 형식은 OMV 8의 engined/rpc/usermgmt.inc를 따른다.
//
//   - UserMgmt.getUserByContext (모두): 로그인한 사용자의 name, comment, email, sshpubkeys, _readonly
//   - UserMgmt.getUser (admin): name으로 찾은 사용자. 위에 더해 uid, gid, dir, shell, groups, system, disallowusermod
//   - UserMgmt.setUserByContext (모두): email·comment(필수)를 바꾼다. 시스템 계정과 수정 금지 계정은 거부하고,
//     저장하면 실제 OMV처럼 postfix 모듈을 적용 대기(dirty)로 표시한다

// User는 username의 지금 계정 정보다(setUserByContext로 바뀐 값 포함). 테스트에서 저장 결과를 확인할 때 쓴다.
func (s *Server) User(username string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[username]
	return u, ok
}

// handleUserMgmt는 UserMgmt RPC를 처리한다. 모르는 메서드면 ok가 false다. s.mu를 쥔 채로 부른다.
func (s *Server) handleUserMgmt(sess *session, method string, params json.RawMessage) (any, *rpcError, bool) {
	switch method {
	case "getUserByContext":
		u := s.users[sess.username]
		return map[string]any{
			"name":       sess.username,
			"comment":    u.Comment,
			"email":      u.Email,
			"sshpubkeys": []string{},
			"_readonly":  u.ReadOnly,
		}, nil, true
	case "getUser":
		if !sess.admin {
			return nil, errStatus(http.StatusForbidden, "Invalid context role."), true
		}
		var p struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(params, &p) != nil || p.Name == "" {
			return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters."), true
		}
		u, ok := s.users[p.Name]
		if !ok {
			return nil, errStatus(http.StatusInternalServerError, "The user '"+p.Name+"' does not exist."), true
		}
		groups := u.Groups
		if groups == nil {
			groups = []string{}
		}
		return map[string]any{
			"name":            u.Name,
			"uid":             1000,
			"gid":             100,
			"comment":         u.Comment,
			"dir":             "/home/" + u.Name,
			"shell":           "/usr/bin/sh",
			"groups":          groups,
			"system":          u.System,
			"email":           u.Email,
			"disallowusermod": u.ReadOnly,
			"sshpubkeys":      []string{},
		}, nil, true
	case "setUserByContext":
		var p struct {
			Email   *string `json:"email"`
			Comment *string `json:"comment"`
		}
		if json.Unmarshal(params, &p) != nil || p.Email == nil || p.Comment == nil {
			return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters."), true
		}
		u := s.users[sess.username]
		if u.ReadOnly {
			return nil, errStatus(http.StatusInternalServerError,
				"Unauthorized attempt to modify the user account '"+u.Name+"'"), true
		}
		if u.System {
			return nil, errStatus(http.StatusInternalServerError,
				"Unauthorized attempt to modify the system user account '"+u.Name+"'"), true
		}
		u.Email, u.Comment = *p.Email, *p.Comment
		s.users[u.Name] = u
		if !slices.Contains(s.plugin.Dirty, "postfix") {
			s.plugin.Dirty = append(slices.Clone(s.plugin.Dirty), "postfix")
		}
		return map[string]any{"name": u.Name, "email": u.Email, "comment": u.Comment}, nil, true
	}
	return nil, nil, false
}
