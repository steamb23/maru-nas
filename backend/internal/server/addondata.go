package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sync/atomic"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/omv"
)

// addonData는 애드온 하나의 현황 데이터를 돌려준다(GET /api/addons/{id}).
//
// capability(역할, 플러그인)를 먼저 확인하고, 애드온의 Fetch에 이 사용자의 OMV 세션으로 만든
// RPC 호출기를 넘긴다. Fetch 안의 OMV 호출이 401을 받으면 대시보드 세션도 지운다.
func (h *handler) addonData(w http.ResponseWriter, r *http.Request, s auth.Session) {
	a, ok := h.Addons.Get(r.PathValue("id"))
	if !ok || a.Fetch == nil {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	c := addons.Capability(a, h.Plugins.Plugins(), s.Admin)
	if !c.Available {
		writeError(w, http.StatusForbidden, c.Reason, "addon is not available")
		return
	}
	caller := &rpcCaller{h: h, s: s, addon: a.Manifest.ID, clientIP: clientIP(r)}
	res, err := a.Fetch(r.Context(), addons.Request{
		Admin:   s.Admin,
		Sources: c.Sources,
		Refresh: r.URL.Query().Get("refresh") == "1",
		Query:   r.URL.Query(),
		RPC:     caller,
	})
	if caller.unauthorized.Load() {
		h.expire(w, r, s)
		return
	}
	if err != nil {
		log.Printf("addon %s: %v", a.Manifest.ID, err)
		if caller.failed.Load() {
			writeError(w, http.StatusBadGateway, api.ErrOMVError, "rpc call failed")
		} else {
			writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to read addon data"})
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// rpcCaller는 provider·조작의 OMV RPC 호출을 한 사용자 세션으로 보낸다(addons.RPCCaller).
// 여러 provider가 동시에 써도 된다.
type rpcCaller struct {
	h        *handler
	s        auth.Session
	addon    string
	clientIP string
	// writes는 쓰기 메서드를 허용하는지다. 조작(Action.Run)에 넘길 때만 켠다.
	writes bool
	// fixed가 있으면 manifest 허용 목록 대신 이 고정 메서드(이름 → 종류)만 부른다. 애드온이 아닌 서버 기능(프로필)이 쓴다.
	fixed map[string]addons.RPCKind
	// unauthorized는 OMV가 세션 만료로 거부했는지다. provider가 오류를 삼켜도 세션을 지우게 한다.
	unauthorized atomic.Bool
	// failed는 OMV 호출이 하나라도 실패했는지다.
	failed atomic.Bool
}

func (c *rpcCaller) Call(ctx context.Context, service, method string, params any) (json.RawMessage, error) {
	name := service + "." + method
	var kind addons.RPCKind
	allowed := false
	if c.fixed != nil {
		kind, allowed = c.fixed[name]
	} else {
		var ids []string
		kind, ids = c.h.Addons.Classify(service, method)
		allowed = kind != addons.RPCNone && slices.Contains(ids, c.addon)
	}
	if !allowed || (kind == addons.RPCWrite && !(c.writes && addons.WritesEnabled)) {
		log.Printf("addon %s: rpc forbidden: %s", c.addon, name)
		return nil, fmt.Errorf("rpc %s: %w", name, addons.ErrRPCNotAllowed)
	}
	res, err := c.h.OMV.Call(ctx, omv.Request{
		SessionID: c.s.OMVSessionID,
		Service:   service,
		Method:    method,
		Params:    params,
		NoTouch:   true,
		ClientIP:  c.clientIP,
	})
	if err != nil {
		c.failed.Store(true)
		switch omv.HTTPStatus(err) {
		case http.StatusUnauthorized:
			c.unauthorized.Store(true)
		case http.StatusNotFound:
			// RPC 서비스가 없다(플러그인이 지워졌을 수 있음). 다음 capability 요청 때 다시 감지한다.
			c.h.Plugins.Invalidate()
		}
		return nil, fmt.Errorf("rpc %s: %w", name, err)
	}
	return res.Result, nil
}
