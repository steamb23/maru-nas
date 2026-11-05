package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
)

// addonRPC는 애드온 패키지의 OMV 읽기 중계다(POST /api/addons/{id}/rpc). 호스트(대시보드)가 패키지 프레임 대신 부른다.
//
//   - 경로의 패키지가 manifest rpc.read에 선언한 메서드만 부른다(rpcCaller{addon: id}). 다른 애드온이 선언한
//     메서드와 쓰기는 거부한다. 쓰기는 패키지 조작으로만 부른다
//   - 금지 표(addons.DeniedForPackage)를 부를 때 한 번 더 본다(등록할 때 이미 막았다)
//   - capability(역할, 플러그인)를 먼저 확인하고 OMV 세션의 마지막 접근 시각은 갱신하지 않는다
func (h *handler) addonRPC(w http.ResponseWriter, r *http.Request, s auth.Session) {
	id := r.PathValue("id")
	a, ok := h.addons().Get(id)
	if !ok || a.Kind != addons.KindPackage {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	if !requireJSON(w, r) {
		return
	}
	var req api.RPCRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid request body")
		return
	}
	if c := h.packageCapability(a, s.Admin); !c.Available {
		writeError(w, http.StatusForbidden, c.Reason, "addon is not available")
		return
	}
	name := req.Service + "." + req.Method
	if _, denied := addons.DeniedForPackage(name); denied || !addons.ValidMethod(name) {
		log.Printf("addon %s: rpc forbidden: user=%q method=%q", id, s.Username, name)
		writeError(w, http.StatusForbidden, api.ErrRPCForbidden, "rpc method is not allowed")
		return
	}
	var params any
	if p := bytes.TrimSpace(req.Params); len(p) > 0 && !bytes.Equal(p, []byte("null")) {
		params = json.RawMessage(p)
	}
	caller := &rpcCaller{h: h, s: s, addon: id, clientIP: clientIP(r)}
	res, err := caller.Call(r.Context(), req.Service, req.Method, params)
	switch {
	case caller.unauthorized.Load():
		h.expire(w, r, s)
		return
	case errors.Is(err, addons.ErrRPCNotAllowed):
		writeError(w, http.StatusForbidden, api.ErrRPCForbidden, "rpc method is not allowed")
		return
	case err != nil:
		log.Printf("addon %s: rpc %q: %v", id, name, err)
		writeError(w, http.StatusBadGateway, api.ErrOMVError, "rpc call failed")
		return
	}
	writeRPCResult(w, res)
}
