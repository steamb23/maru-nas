package server

import (
	"context"
	"log"
	"net/http"
	"net/mail"
	"slices"
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

// profileMethods는 프로필이 부르는 OMV RPC와 그 종류다. 쓰기는 전자 우편·태그 저장(setProfile)만 부른다.
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
	// maxCommentBytes는 태그를 이은 계정 설명 칸(GECOS)의 상한이다.
	maxCommentBytes = 255
	// maxTags는 태그 수의 상한이다.
	maxTags = 32
	// maxEmailBytes는 전자 우편 주소의 상한이다.
	maxEmailBytes = 254
	// profileSaveTimeout은 저장과 OMV 적용(관리자)을 기다리는 시간이다. 서비스 켜기·끄기와 같다.
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
		Tags:          parseTags(u.Comment),
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

// OMV 워크벤치는 계정 설명 칸(comment, GECOS)을 "Tags"로 보여 주며 태그를 쉼표로 이어 저장한다.

// parseTags는 설명 칸을 태그로 나눈다. 앞뒤 공백과 빈 태그는 뺀다.
func parseTags(comment string) []string {
	var tags []string
	for t := range strings.SplitSeq(comment, ",") {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// cleanTags는 받은 태그의 앞뒤 공백을 지우고 빈 태그와 중복을 뺀다. 쓸 수 없는 태그가 있으면 false다.
// 설명 칸은 /etc/passwd의 한 칸이므로 ':'와 줄바꿈 등 제어 문자를 받지 않고, ','는 태그 구분자라 받지 않는다.
func cleanTags(in []string) ([]string, bool) {
	out := []string{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || slices.Contains(out, t) {
			continue
		}
		if !utf8.ValidString(t) || strings.ContainsAny(t, ",:") || strings.ContainsFunc(t, unicode.IsControl) {
			return nil, false
		}
		out = append(out, t)
	}
	return out, len(out) <= maxTags && len(strings.Join(out, ",")) <= maxCommentBytes
}

// validEmail은 전자 우편 주소인지다. 빈 값은 지우는 것이라 받는다. 최종 검사는 OMV가 한다.
func validEmail(e string) bool {
	if e == "" {
		return true
	}
	if len(e) > maxEmailBytes || strings.ContainsFunc(e, unicode.IsSpace) {
		return false
	}
	a, err := mail.ParseAddress(e)
	return err == nil && a.Name == "" && a.Address == e
}

// setProfile은 로그인한 사용자 자신의 전자 우편과 태그를 바꾼다. 보내지 않은 값은 그대로 둔다.
//
// OMV의 UserMgmt.setUserByContext는 전자 우편과 설명 칸이 모두 필수라 서버가 지금 값을 다시 읽어 보내지 않은 값을
// 채운다(OMV 자체의 사용자 프로필 폼과 같은 범위). 저장하면 OMV가 postfix 등을 적용 대기로 표시하므로, 관리자는
// 서비스 켜기·끄기처럼 적용까지 하고(addons.ApplyChange, 적용 대기가 있으면 409 config_dirty), 일반 사용자는
// 적용 권한이 없어 적용 대기로 남긴다. 실제 OMV로는 시스템 상태를 바꾸는 호출이라 사용자가 확인한다.
func (h *handler) setProfile(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.ProfileRequest
	if !decodeJSONLimit(w, r, &req, maxProfileBodyBytes) {
		return
	}
	if req.Email == nil && req.Tags == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "email or tags is required")
		return
	}
	var change profileChange
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if !validEmail(email) {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid email")
			return
		}
		change.email = &email
	}
	if req.Tags != nil {
		tags, ok := cleanTags(*req.Tags)
		if !ok {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid tags")
			return
		}
		change.tags = tags
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
	applied, err := saveProfile(ctx, caller, s, change)
	result, ae := actionOutcome(ctx, err, caller)
	// 전자 우편·태그 내용은 로그에 남기지 않는다.
	log.Printf("profile: user=%q email=%t tags=%t result=%s", s.Username, change.email != nil, change.tags != nil, result)
	if err != nil {
		log.Printf("profile: %v", err)
	}
	if result != "ok" {
		h.writeActionFailure(w, r, s, result, ae)
		return
	}
	writeJSON(w, http.StatusOK, api.ProfileSaveResponse{Applied: applied})
}

// profileChange는 바꿀 값이다. nil이면 그대로 둔다.
type profileChange struct {
	email *string
	tags  []string
}

// saveProfile은 전자 우편·태그를 저장하고, 관리자면 OMV 적용까지 한다. applied는 적용 대기를 남기지 않았는지다.
// 저장이 거절되면 OMV 오류(502 omv_error), 저장은 됐지만 적용이 실패하면 ErrActionFailed(502 action_failed)다.
func saveProfile(ctx context.Context, rpc addons.RPCCaller, s auth.Session, c profileChange) (bool, error) {
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
	email, comment := u.Email, u.Comment
	if c.email != nil {
		email = *c.email
	}
	// 태그가 같으면 설명 칸을 그대로 둔다(OMV가 저장한 공백 등 모양을 바꾸지 않게).
	if c.tags != nil && !slices.Equal(c.tags, parseTags(u.Comment)) {
		comment = strings.Join(c.tags, ",")
	}
	set := func(ctx context.Context) (bool, error) {
		if email == u.Email && comment == u.Comment {
			return false, nil
		}
		params := map[string]any{"email": email, "comment": comment}
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
