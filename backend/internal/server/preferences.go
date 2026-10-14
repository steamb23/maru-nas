package server

import (
	"log"
	"net/http"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
)

// maxPreferencesBodyBytes는 PUT /api/preferences/* 본문의 상한이다. 위젯 64개도 넉넉히 들어간다.
const maxPreferencesBodyBytes = 64 << 10

// preferences는 로그인한 사용자의 대시보드 설정을 돌려준다.
func (h *handler) preferences(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	p, err := h.Prefs.Get(s.Username)
	if err != nil {
		log.Printf("preferences: read: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to read preferences")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// setWidgets는 위젯 배치 전체를 바꾼다. 다른 설정은 그대로 둔다.
func (h *handler) setWidgets(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.WidgetsRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Widgets == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "widgets is required")
		return
	}
	if err := h.Addons.ValidateWidgets(req.Widgets); err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, err.Error())
		return
	}
	if err := h.Prefs.SetWidgets(s.Username, req.Widgets); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
