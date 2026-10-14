package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
	"omv-desktop-dashboard/backend/internal/auth"
)

// maxActionBodyBytes는 조작 요청 본문의 최대 크기다.
const maxActionBodyBytes = 4 << 10

// addonAction은 애드온 조작 하나를 실행한다(POST /api/addons/{id}/actions/{action}).
//
// capability(역할, 플러그인)와 조작의 admin 조건을 먼저 확인하고, 같은 (애드온, 대상)에 진행 중인
// 조작이 있으면 거절한다. 조작은 브라우저 연결이 끊겨도 Timeout까지 계속 기다린다. OMV 쪽 작업은
// 어차피 계속 돌므로, 끝날 때까지 진행 중 표시를 유지해 같은 조작이 겹치지 않게 하기 위해서다.
func (h *handler) addonAction(w http.ResponseWriter, r *http.Request, s auth.Session) {
	id, name := r.PathValue("id"), r.PathValue("action")
	a, ok := h.Addons.Get(id)
	decl, act, found := h.Addons.ActionDecl(id, name)
	if !ok || !found {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	var req api.ActionRequest
	if !decodeJSONLimit(w, r, &req, maxActionBodyBytes) {
		return
	}
	if c := addons.Capability(a, h.Plugins.Plugins(), s.Admin); !c.Available {
		writeError(w, http.StatusForbidden, c.Reason, "addon is not available")
		return
	}
	if decl.Admin && !s.Admin {
		writeError(w, http.StatusForbidden, api.ErrAdminRequired, "action requires an administrator")
		return
	}

	key := id + "\x00" + req.Target
	if !h.actions.start(key) {
		writeError(w, http.StatusConflict, api.ErrActionBusy, "action already in progress")
		return
	}
	defer h.actions.done(key)

	timeout := act.Timeout
	if timeout <= 0 {
		timeout = addons.DefaultActionTimeout
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), timeout)
	defer cancel()
	caller := &rpcCaller{h: h, s: s, addon: id, clientIP: clientIP(r), writes: true}
	err := act.Run(ctx, addons.ActionRequest{Admin: s.Admin, Target: req.Target, RPC: caller})

	var ae *addons.ActionError
	result := "ok"
	switch {
	case caller.unauthorized.Load():
		result = "unauthorized"
	case err == nil:
	case errors.As(err, &ae):
		result = ae.Code
	case errors.Is(err, addons.ErrInvalidTarget):
		result = api.ErrBadRequest
	case errors.Is(err, addons.ErrActionTimeout), errors.Is(ctx.Err(), context.DeadlineExceeded):
		result = api.ErrActionTimeout
	case errors.Is(err, addons.ErrActionFailed):
		result = api.ErrActionFailed
	case caller.failed.Load():
		result = api.ErrOMVError
	default:
		result = "error"
	}
	// 대상은 브라우저가 보낸 값이므로 %q로 남긴다(로그 줄 바꿈 주입 방지).
	log.Printf("action: user=%q addon=%s action=%s target=%q result=%s", s.Username, id, name, req.Target, result)
	if err != nil {
		log.Printf("action %s/%s: %v", id, name, err)
	}

	switch result {
	case "unauthorized":
		h.expire(w, r, s)
	case "ok":
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	case api.ErrBadRequest:
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid action target")
	case api.ErrActionTimeout:
		writeError(w, http.StatusGatewayTimeout, api.ErrActionTimeout, "action timed out")
	case api.ErrActionFailed:
		writeError(w, http.StatusBadGateway, api.ErrActionFailed, "action failed")
	case api.ErrOMVError:
		writeError(w, http.StatusBadGateway, api.ErrOMVError, "rpc call failed")
	case "error":
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "action failed"})
	default:
		writeError(w, ae.Status, ae.Code, ae.Message)
	}
}

// inflight는 진행 중인 조작의 (애드온, 대상) 집합이다.
type inflight struct {
	mu   sync.Mutex
	keys map[string]bool
}

func newInflight() *inflight { return &inflight{keys: make(map[string]bool)} }

// start는 key가 진행 중이 아니면 표시하고 true를 반환한다.
func (f *inflight) start(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.keys[key] {
		return false
	}
	f.keys[key] = true
	return true
}

func (f *inflight) done(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.keys, key)
}
