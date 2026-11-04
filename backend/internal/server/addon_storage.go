package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/prefs"
)

// addonStorage는 애드온 패키지의 이 사용자 저장값이다(GET /api/preferences/addons/{id}). 저장한 적 없거나 지금 설치본과
// 설치 ID가 다르면(지웠다 다시 설치함) data가 null이다.
func (h *handler) addonStorage(w http.ResponseWriter, r *http.Request, s auth.Session) {
	p, ok := h.visiblePackage(r.PathValue("id"), s)
	if !ok {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	d, found, err := h.AddonData.GetAddonData(s.Username, p.ID)
	if err != nil {
		log.Printf("addon %s: read storage: user=%q: %v", p.ID, s.Username, err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to read addon data"})
		return
	}
	var data json.RawMessage
	if found && d.InstallUID == p.InstallUID {
		data = d.Data
	}
	writeJSON(w, http.StatusOK, struct {
		Data json.RawMessage `json:"data"`
	}{data})
}

// setAddonStorage는 애드온 패키지의 이 사용자 저장값을 바꾼다(PUT /api/preferences/addons/{id}, 본문 {data}).
// 지웠거나 다시 설치한 패키지의 남은 값은 이때 함께 지운다.
func (h *handler) setAddonStorage(w http.ResponseWriter, r *http.Request, s auth.Session) {
	p, ok := h.visiblePackage(r.PathValue("id"), s)
	if !ok {
		writeJSON(w, http.StatusNotFound, api.ErrorResponse{Error: "not found"})
		return
	}
	var req struct {
		Data json.RawMessage `json:"data"`
	}
	if !decodeJSONLimit(w, r, &req, prefs.MaxAddonData+1024) {
		return
	}
	if len(req.Data) == 0 || !json.Valid(req.Data) || len(req.Data) > prefs.MaxAddonData {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "data must be a JSON value of at most 64 KiB")
		return
	}
	keep := func(id, uid string) bool {
		if st, ok := h.Packages.Status(id); ok && st.Record != nil {
			return st.Record.InstallUID == uid
		}
		if q, ok := h.Packages.Get(id); ok {
			return q.InstallUID == uid
		}
		return false
	}
	err := h.AddonData.SetAddonData(s.Username, p.ID, prefs.AddonData{InstallUID: p.InstallUID, Data: req.Data}, keep)
	switch {
	case errors.Is(err, prefs.ErrAddonDataFull):
		writeError(w, http.StatusRequestEntityTooLarge, api.ErrBadRequest, "addon data is full")
	case err != nil:
		log.Printf("addon %s: write storage: user=%q: %v", p.ID, s.Username, err)
		writeJSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to save addon data"})
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
