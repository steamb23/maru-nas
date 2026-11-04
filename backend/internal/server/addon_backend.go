package server

import (
	"net/http"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
)

// packageData는 애드온 패키지의 데이터 요청(GET /api/addons/{id})이다. 데이터는 패키지 백엔드가 만든다.
// 백엔드가 없는 패키지는 OMV 읽기를 POST /api/addons/{id}/rpc로 한다.
func (h *handler) packageData(w http.ResponseWriter, _ *http.Request, s auth.Session, a addons.Addon) {
	if c := addons.Capability(a, h.Plugins.Plugins(), s.Admin); !c.Available {
		writeError(w, http.StatusForbidden, c.Reason, "addon is not available")
		return
	}
	writeError(w, http.StatusNotFound, api.ErrAddonNoBackend, "addon package has no backend")
}

// packageAction은 애드온 패키지의 조작(POST /api/addons/{id}/actions/{action})이다. 조작은 패키지 백엔드가 맡는다.
func (h *handler) packageAction(w http.ResponseWriter, _ *http.Request, s auth.Session, a addons.Addon, _ api.ActionDecl) {
	if c := addons.Capability(a, h.Plugins.Plugins(), s.Admin); !c.Available {
		writeError(w, http.StatusForbidden, c.Reason, "addon is not available")
		return
	}
	writeError(w, http.StatusNotFound, api.ErrAddonNoBackend, "addon package has no backend")
}
