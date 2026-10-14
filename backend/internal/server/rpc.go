package server

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
	"omv-desktop-dashboard/backend/internal/auth"
	"omv-desktop-dashboard/backend/internal/omv"
)

// maxRPCBodyBytes는 /api/rpc 요청 본문의 최대 크기다.
const maxRPCBodyBytes = 64 << 10

// rpc는 브라우저의 OMV RPC 요청을 그 사용자의 OMV 세션으로 중계한다.
//
//   - 등록된 애드온이 선언한 메서드만 통과시킨다. 쓰기 메서드는 addons.WritesEnabled가 켜져야 한다.
//   - 그 메서드를 선언한 애드온 중 이 사용자가 쓸 수 있는 것이 있어야 한다(역할, 플러그인).
//     최종 역할 검사는 OMV가 하며, 여기서는 불필요한 호출을 미리 막는다.
//   - OMV 세션의 마지막 접근 시각은 갱신하지 않는다. 창·위젯의 주기 요청이 세션을 연장하면
//     안 되기 때문이다. 세션 연장은 /api/session/activity만 한다.
func (h *handler) rpc(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !requireJSON(w, r) {
		return
	}
	var req api.RPCRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid request body")
		return
	}
	name := req.Service + "." + req.Method

	kind, ids := h.Addons.Classify(req.Service, req.Method)
	if kind == addons.RPCNone || (kind == addons.RPCWrite && !addons.WritesEnabled) {
		log.Printf("rpc forbidden: user=%q method=%q", s.Username, name)
		writeError(w, http.StatusForbidden, api.ErrRPCForbidden, "rpc method is not allowed")
		return
	}
	if reason := h.rpcUnavailable(ids, s.Admin); reason != "" {
		writeError(w, http.StatusForbidden, reason, "no addon using this rpc method is available")
		return
	}

	var params any
	if p := bytes.TrimSpace(req.Params); len(p) > 0 && !bytes.Equal(p, []byte("null")) {
		params = json.RawMessage(p)
	}
	res, err := h.OMV.Call(r.Context(), omv.Request{
		SessionID: s.OMVSessionID,
		Service:   req.Service,
		Method:    req.Method,
		Params:    params,
		NoTouch:   true,
		ClientIP:  clientIP(r),
	})
	switch {
	case omv.IsUnauthorized(err):
		h.expire(w, r, s)
		return
	case omv.HTTPStatus(err) == http.StatusNotFound:
		// RPC 서비스가 없다(플러그인이 지워졌을 수 있음). 다음 capability 요청 때 다시 감지한다.
		h.Plugins.Invalidate()
		log.Printf("rpc %s: %v", name, err)
		writeError(w, http.StatusBadGateway, api.ErrOMVError, "rpc call failed")
		return
	case err != nil:
		log.Printf("rpc %s: %v", name, err)
		writeError(w, http.StatusBadGateway, api.ErrOMVError, "rpc call failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	result := res.Result
	if len(result) == 0 {
		result = json.RawMessage("null")
	}
	w.Write(result)
}

// rpcUnavailable은 ids의 애드온 중 이 사용자가 쓸 수 있는 것이 없으면 첫 애드온의 이유를 반환한다.
func (h *handler) rpcUnavailable(ids []string, admin bool) string {
	plugins := h.Plugins.Plugins()
	reason := ""
	for _, id := range ids {
		a, ok := h.Addons.Get(id)
		if !ok {
			continue
		}
		c := addons.Capability(a, plugins, admin)
		if c.Available {
			return ""
		}
		if reason == "" {
			reason = c.Reason
		}
	}
	if reason == "" {
		reason = api.ErrRPCForbidden
	}
	return reason
}
