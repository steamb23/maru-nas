package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
)

// 애드온 패키지의 데이터·조작은 패키지 백엔드로 프록시한다(.claude/docs/addon-packages.md의 "maru ↔ 백엔드").
// 백엔드는 OMV를 호스트 소켓(addon_host.go)과 이 요청에 묶인 호출 토큰으로만 부른다.

// maxPackageActionBody는 패키지 조작 요청 본문(브라우저)의 상한이다. input은 addonpkg.MaxActionInput까지다.
const maxPackageActionBody = 16 << 10

// backendsEnabled는 이 설치에서 패키지 백엔드가 켜져 있는지다(Config.AddonHost).
func (h *handler) backendsEnabled() bool { return h.backends != nil }

// backendDisabled는 id가 백엔드 패키지인데 백엔드가 꺼져 있는지다(capability backend_unavailable).
func (h *handler) backendDisabled(id string) bool {
	if h.backendsEnabled() || h.Packages == nil {
		return false
	}
	p, ok := h.Packages.Get(id)
	return ok && p.Manifest.HasBackend()
}

// packageCapability는 addons.Capability에 패키지 백엔드 상태를 더한다. 백엔드가 꺼진 설치에서 백엔드 패키지는
// backend_unavailable이다(역할·플러그인 조건을 먼저 본다).
func (h *handler) packageCapability(a addons.Addon, admin bool) api.AddonCapability {
	c := addons.Capability(a, h.Plugins.Plugins(), admin)
	if c.Available && a.Kind == addons.KindPackage && h.backendDisabled(a.Manifest.ID) {
		c.Available, c.Reason, c.Sources = false, api.ReasonBackendUnavailable, []string{}
	}
	return c
}

// packageBackend는 데이터·조작을 보낼 백엔드 패키지다. 백엔드가 없으면 404 addon_no_backend, 이 설치에서 백엔드가
// 꺼져 있으면 503 addon_backend_disabled를 쓴다.
func (h *handler) packageBackend(w http.ResponseWriter, a addons.Addon) (*addonpkg.Installed, bool) {
	var p *addonpkg.Installed
	if h.Packages != nil {
		p, _ = h.Packages.Get(a.Manifest.ID)
	}
	if p == nil {
		writeError(w, http.StatusNotFound, api.ErrNotFound, "not found")
		return nil, false
	}
	if !p.Manifest.HasBackend() {
		writeError(w, http.StatusNotFound, api.ErrAddonNoBackend, "addon package has no backend")
		return nil, false
	}
	if !h.backendsEnabled() {
		writeError(w, http.StatusServiceUnavailable, api.ErrAddonBackendDisabled, "addon backends are disabled on this installation")
		return nil, false
	}
	return p, true
}

// packageData는 애드온 패키지의 데이터 요청(GET /api/addons/{id})이다. 쿼리 문자열을 그대로 백엔드의 GET /v1/data로
// 넘긴다(2KiB까지). 백엔드는 읽기 토큰으로 선언한 읽기 메서드만 부를 수 있다. 백엔드가 없는 패키지는 OMV 읽기를
// POST /api/addons/{id}/rpc로 한다.
func (h *handler) packageData(w http.ResponseWriter, r *http.Request, s auth.Session, a addons.Addon) {
	if _, ok := h.packageBackend(w, a); !ok {
		return
	}
	id := a.Manifest.ID
	if c := h.packageCapability(a, s.Admin); !c.Available {
		writeError(w, http.StatusForbidden, c.Reason, "addon is not available")
		return
	}
	if !addonpkg.ValidBackendQuery(r.URL.RawQuery) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid query")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.backendDataTimeout)
	defer cancel()
	caller := &rpcCaller{h: h, s: s, addon: id, clientIP: clientIP(r)}
	call, done, ok := h.packageCall(ctx, r, s, id, hostRead, caller)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, api.ErrAddonBackendUnavailable, "too many addon backend calls")
		return
	}
	rep := h.backends.Data(ctx, id, r.URL.RawQuery, call)
	done()
	if caller.unauthorized.Load() {
		h.expire(w, r, s)
		return
	}
	if !rep.OK() {
		log.Printf("addon %s: backend data: %s: %v", id, rep.Code, rep.Err)
	}
	writeBackendReply(w, rep)
}

// packageAction은 애드온 패키지의 조작(POST /api/addons/{id}/actions/{action})이다. 본문의 대상·입력을 백엔드의
// POST /v1/actions/{name}으로 넘긴다. 확인 순서는 내장 조작과 같다: 백엔드 있음·켜짐 → capability → decl.Admin →
// 쓰기를 선언한 패키지면 단계 확인 → 진행 중 표시 → WithoutCancel + manifest의 시간 제한 → 감사 로그.
// 백엔드는 쓰기 토큰으로 선언한 읽기·쓰기 메서드와 /v1/apply를 부를 수 있다.
func (h *handler) packageAction(w http.ResponseWriter, r *http.Request, s auth.Session, a addons.Addon, decl api.ActionDecl) {
	p, ok := h.packageBackend(w, a)
	if !ok {
		return
	}
	id, name := a.Manifest.ID, decl.Name
	var req api.ActionRequest
	if !decodeJSONLimit(w, r, &req, maxPackageActionBody) {
		return
	}
	if !addonpkg.ValidActionInput(req.Input) {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "action input must be a JSON object or null (8 KiB at most)")
		return
	}
	if c := h.packageCapability(a, s.Admin); !c.Available {
		writeError(w, http.StatusForbidden, c.Reason, "addon is not available")
		return
	}
	if decl.Admin && !s.Admin {
		writeError(w, http.StatusForbidden, api.ErrAdminRequired, "action requires an administrator")
		return
	}
	// 쓰기를 선언한 패키지의 조작은 정책이 꺼져 있어도 비밀번호 재확인이 필요하다(빼앗은 쿠키만으로 OMV 쓰기를 하지 못하게).
	if len(p.Manifest.RPC.Write) > 0 {
		if level := h.packageWriteLevel(); !satisfies(s, level) {
			writeSecurityRequired(w, level)
			return
		}
	}

	key := id + "\x00" + req.Target
	if !h.actions.start(key) {
		writeError(w, http.StatusConflict, api.ErrActionBusy, "action already in progress")
		return
	}
	defer h.actions.done(key)

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), packageActionTimeout(p, name))
	defer cancel()
	caller := &rpcCaller{h: h, s: s, addon: id, clientIP: clientIP(r), writes: true}
	call, done, ok := h.packageCall(ctx, r, s, id, hostWrite, caller)
	var rep addonpkg.BackendReply
	if ok {
		rep = h.backends.Action(ctx, id, name, req.Target, req.Input, call)
		done()
	} else {
		rep = addonpkg.BackendReply{Status: http.StatusServiceUnavailable, Code: api.ErrAddonBackendUnavailable,
			Message: "too many addon backend calls"}
	}

	result := "ok"
	switch {
	case caller.unauthorized.Load():
		result = "unauthorized"
	case !rep.OK():
		result = rep.Code
	}
	// 대상은 브라우저가 보낸 값이므로 %q로 남긴다(로그 줄 바꿈 주입 방지). 토큰은 남기지 않는다.
	log.Printf("action: user=%q addon=%s action=%s target=%q result=%s", s.Username, id, name, req.Target, result)
	if rep.Err != nil && !rep.OK() {
		log.Printf("action %s/%s: %v", id, name, rep.Err)
	}
	if result == "unauthorized" {
		h.expire(w, r, s)
		return
	}
	writeBackendReply(w, rep)
}

// packageActionTimeout은 manifest의 조작 시간 제한이다(없으면 addons.DefaultActionTimeout).
func packageActionTimeout(p *addonpkg.Installed, name string) time.Duration {
	for _, a := range p.Manifest.Actions {
		if a.Name == name && a.TimeoutSeconds > 0 {
			return time.Duration(a.TimeoutSeconds) * time.Second
		}
	}
	return addons.DefaultActionTimeout
}

// writeBackendReply는 백엔드 응답을 바꾼 값을 쓴다. 백엔드의 헤더는 넘기지 않는다.
func writeBackendReply(w http.ResponseWriter, rep addonpkg.BackendReply) {
	switch rep.Status {
	case http.StatusOK:
		writeRPCResult(w, rep.Body)
	case http.StatusNoContent:
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	default:
		writeJSON(w, rep.Status, api.ErrorResponse{Error: rep.Message, Code: rep.Code, AddonCode: rep.AddonCode})
	}
}
