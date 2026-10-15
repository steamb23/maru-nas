package server

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
)

// 프로필: 로그인한 사용자 자신의 OMV 계정 정보. 애드온이 아니므로 manifest 허용 목록 대신 아래 고정 메서드만
// 부른다(profileMethods). 브라우저는 메서드나 파라미터를 고를 수 없고, 서버가 자기 사용자 이름으로 만든다.

// profileMethods는 프로필이 부르는 OMV RPC와 그 종류다. 쓰기는 설명 저장(setProfile)만 부른다.
var profileMethods = map[string]addons.RPCKind{
	"UserMgmt.getUserByContext": addons.RPCRead,
	"UserMgmt.getUser":          addons.RPCRead,
	"UserMgmt.setUserByContext": addons.RPCWrite,
	"Config.isDirty":            addons.RPCRead,
	"Config.applyChangesBg":     addons.RPCWrite,
	"Exec.isRunning":            addons.RPCRead,
}

const (
	// maxProfileBodyBytes는 PUT /api/profile 본문의 상한이다.
	maxProfileBodyBytes = 4 << 10
	// maxCommentBytes는 계정 설명(GECOS)의 상한이다.
	maxCommentBytes = 255
	// profileSaveTimeout은 설명 저장과 OMV 적용(관리자)을 기다리는 시간이다. 서비스 켜기·끄기와 같다.
	profileSaveTimeout = 3 * time.Minute
)

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

// validComment는 계정 설명으로 쓸 수 있는지다. /etc/passwd의 한 칸이므로 ':'와 줄바꿈 등 제어 문자를 받지 않는다.
func validComment(c string) bool {
	if !utf8.ValidString(c) || len(c) > maxCommentBytes || strings.Contains(c, ":") {
		return false
	}
	return !strings.ContainsFunc(c, unicode.IsControl)
}

// setProfile은 로그인한 사용자 자신의 계정 설명을 바꾼다.
//
// OMV의 UserMgmt.setUserByContext는 전자 우편도 필수라 서버가 지금 값을 다시 읽어 그대로 함께 보낸다(브라우저는 설명만
// 보낸다). 저장하면 OMV가 postfix 등을 적용 대기로 표시하므로, 관리자는 서비스 켜기·끄기처럼 적용까지 하고
// (addons.ApplyChange, 적용 대기가 있으면 409 config_dirty), 일반 사용자는 적용 권한이 없어 적용 대기로 남긴다.
// 실제 OMV로는 시스템 상태를 바꾸는 호출이라 사용자가 확인한다.
func (h *handler) setProfile(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.ProfileRequest
	if !decodeJSONLimit(w, r, &req, maxProfileBodyBytes) {
		return
	}
	if req.Comment == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "comment is required")
		return
	}
	comment := strings.TrimSpace(*req.Comment)
	if !validComment(comment) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid comment")
		return
	}
	if !addons.WritesEnabled {
		writeError(w, http.StatusForbidden, api.ErrRPCForbidden, "omv writes are disabled")
		return
	}
	key := "profile\x00" + s.Username
	if !h.actions.start(key) {
		writeError(w, http.StatusConflict, api.ErrActionBusy, "profile change already in progress")
		return
	}
	defer h.actions.done(key)

	// 브라우저 연결이 끊겨도 OMV 쪽 적용은 계속되므로 끝날 때까지 기다린다(애드온 조작과 같다).
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), profileSaveTimeout)
	defer cancel()
	caller := h.profileCaller(r, s, true)
	applied, err := saveComment(ctx, caller, s, comment)
	result, ae := actionOutcome(ctx, err, caller)
	// 설명 내용은 로그에 남기지 않는다.
	log.Printf("profile: user=%q change=comment result=%s", s.Username, result)
	if err != nil {
		log.Printf("profile: %v", err)
	}
	if result != "ok" {
		h.writeActionFailure(w, r, s, result, ae)
		return
	}
	writeJSON(w, http.StatusOK, api.ProfileSaveResponse{Applied: applied})
}

// saveComment는 설명을 저장하고, 관리자면 OMV 적용까지 한다. applied는 적용 대기를 남기지 않았는지다.
// 저장이 거절되면 OMV 오류(502 omv_error), 저장은 됐지만 적용이 실패하면 ErrActionFailed(502 action_failed)다.
func saveComment(ctx context.Context, rpc addons.RPCCaller, s auth.Session, comment string) (bool, error) {
	u, err := loadProfile(ctx, rpc, s)
	if err != nil {
		return false, err
	}
	if u.System || u.Disallow {
		return false, &addons.ActionError{
			Status:  http.StatusConflict,
			Code:    api.ErrProfileReadOnly,
			Message: "this account cannot be modified",
		}
	}
	set := func(ctx context.Context) (bool, error) {
		if u.Comment == comment {
			return false, nil
		}
		params := map[string]any{"email": u.Email, "comment": comment}
		if _, err := rpc.Call(ctx, "UserMgmt", "setUserByContext", params); err != nil {
			return false, err
		}
		return true, nil
	}
	if s.Admin {
		return true, addons.ApplyChange(ctx, rpc, set)
	}
	changed, err := set(ctx)
	return !changed, err
}
