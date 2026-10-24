package server

import (
	"log"
	"maps"
	"net/http"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/notify"
)

// maxSeenEntries는 읽음 상태 항목 수의 상한이다. 프론트는 지금 목록에 있는 알림만 보내므로 넉넉하다.
const maxSeenEntries = 128

// notifications는 이 사용자가 볼 서버 알림과 그중 읽은 것(id → 리비전)을 돌려준다.
// 읽음 상태가 함께 오므로 다른 탭·기기에서 읽은 것도 다음 요청에 맞춰진다.
func (h *handler) notifications(w http.ResponseWriter, r *http.Request, s auth.Session) {
	notices := h.Notify.Notices(r.Context(), s.Admin)
	p, err := h.Prefs.Get(s.Username)
	if err != nil {
		log.Printf("notifications: read preferences: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to read preferences")
		return
	}
	var seen map[string]string
	if p.Notifications != nil {
		seen = p.Notifications.Seen
	}
	writeJSON(w, http.StatusOK, api.NotificationsResponse{Notices: notices, Seen: notify.SeenFor(seen, notices), Tasks: []api.Job{}})
}

// setNotificationPrefs는 알림 읽음 상태 전체를 바꾼다. 다른 설정은 그대로 둔다.
func (h *handler) setNotificationPrefs(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.NotificationPrefsRequest
	if !decodeJSONLimit(w, r, &req, maxPreferencesBodyBytes) {
		return
	}
	if req.Notifications == nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "notifications is required")
		return
	}
	seen := req.Notifications.Seen
	if len(seen) > maxSeenEntries {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "too many seen entries")
		return
	}
	for id, rev := range seen {
		if !notify.ValidID(id) || !notify.ValidRevision(rev) {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "bad seen entry")
			return
		}
	}
	if err := h.Prefs.SetNotifications(s.Username, api.NotificationPrefs{Seen: maps.Clone(seen)}); err != nil {
		log.Printf("preferences: write: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to save preferences")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
