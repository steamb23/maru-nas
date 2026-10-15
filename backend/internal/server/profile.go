package server

import (
	"context"
	"log"
	"net/http"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
)

// 프로필: 로그인한 사용자 자신의 OMV 계정 정보. 애드온이 아니므로 manifest 허용 목록 대신 아래 고정 메서드만
// 부른다(profileMethods). 브라우저는 메서드나 파라미터를 고를 수 없고, 서버가 자기 사용자 이름으로 만든다.

// profileMethods는 프로필이 부르는 OMV RPC와 그 종류다.
var profileMethods = map[string]addons.RPCKind{
	"UserMgmt.getUserByContext": addons.RPCRead,
	"UserMgmt.getUser":          addons.RPCRead,
}

// omvUser는 UserMgmt.getUser(관리자)·getUserByContext(누구나) 응답에서 쓰는 값이다.
type omvUser struct {
	Name    string   `json:"name"`
	Comment string   `json:"comment"`
	Email   string   `json:"email"`
	Groups  []string `json:"groups"`
	System  bool     `json:"system"`
	// Disallow(getUser)와 ReadOnly(getUserByContext)는 같은 값(disallowusermod)이다.
	Disallow bool `json:"disallowusermod"`
	ReadOnly bool `json:"_readonly"`
}

// loadProfile은 로그인한 사용자의 OMV 계정 정보를 읽는다. 관리자는 getUser로 그룹과 시스템 계정 여부까지,
// 일반 사용자는 getUserByContext로 읽는다.
func loadProfile(ctx context.Context, rpc addons.RPCCaller, s auth.Session) (omvUser, error) {
	var u omvUser
	if s.Admin {
		err := addons.CallInto(ctx, rpc, "UserMgmt", "getUser", map[string]any{"name": s.Username}, &u)
		return u, err
	}
	err := addons.CallInto(ctx, rpc, "UserMgmt", "getUserByContext", map[string]any{}, &u)
	u.Disallow = u.ReadOnly
	return u, err
}

// response는 브라우저에 보낼 값이다.
func (u omvUser) response(username string) api.ProfileResponse {
	p := api.ProfileResponse{
		Username:      username,
		Email:         u.Email,
		Comment:       u.Comment,
		Groups:        u.Groups,
		SystemAccount: u.System,
	}
	switch {
	case u.System:
		p.ReadOnlyReason = api.ProfileReadOnlySystem
	case u.Disallow:
		p.ReadOnlyReason = api.ProfileReadOnlyLocked
	}
	return p
}

// profileCaller는 이 사용자의 OMV 세션으로 프로필 고정 메서드만 부르는 호출기다.
func (h *handler) profileCaller(r *http.Request, s auth.Session, writes bool) *rpcCaller {
	return &rpcCaller{h: h, s: s, addon: "profile", clientIP: clientIP(r), writes: writes, fixed: profileMethods}
}

// profile은 로그인한 사용자 자신의 OMV 계정 정보를 돌려준다.
func (h *handler) profile(w http.ResponseWriter, r *http.Request, s auth.Session) {
	caller := h.profileCaller(r, s, false)
	u, err := loadProfile(r.Context(), caller, s)
	if caller.unauthorized.Load() {
		h.expire(w, r, s)
		return
	}
	if err != nil {
		log.Printf("profile: %v", err)
		if caller.failed.Load() {
			writeError(w, http.StatusBadGateway, api.ErrOMVError, "rpc call failed")
		} else {
			writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to read profile"})
		}
		return
	}
	writeJSON(w, http.StatusOK, u.response(s.Username))
}
