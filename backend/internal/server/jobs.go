package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/auth"
	"maru/backend/internal/jobs"
	"maru/backend/internal/jobs/omvjob"
)

// 서버 백그라운드 작업(시스템 업데이트의 확인·설치). 작업은 internal/jobs가 기록하고 omvjob이 관리자 OMV 세션으로
// 지켜본다. 여기서는 감시자가 쓸 세션 묶음, 조작이 작업을 시작하는 입구, 작업 API를 둔다.

// jobSessions는 감시자가 쓸 관리자 대시보드 세션이다(omvjob.Sessions).
type jobSessions struct{ h *handler }

// Candidates는 관리자 세션을 prefer 먼저, 나머지는 최근에 쓴 순서로 돌려준다. 호출기는 그 애드온의 허용 목록을
// 따르고 OMV 세션을 연장하지 않는다(rpcCaller는 늘 updatelastaccess=false).
func (j jobSessions) Candidates(prefer, addon string) []omvjob.Session {
	var list []auth.Session
	for _, s := range j.h.Sessions.List() {
		if s.Admin {
			list = append(list, s)
		}
	}
	slices.SortFunc(list, func(a, b auth.Session) int {
		switch {
		case a.ID == prefer && b.ID != prefer:
			return -1
		case b.ID == prefer && a.ID != prefer:
			return 1
		}
		return b.LastSeen.Compare(a.LastSeen)
	})
	out := make([]omvjob.Session, len(list))
	for i, s := range list {
		out[i] = omvjob.Session{Key: s.ID, User: s.Username, RPC: &rpcCaller{h: j.h, s: s, addon: addon}}
	}
	return out
}

// Dead는 OMV가 파기한 세션(401)의 대시보드 세션을 지운다. 그 브라우저의 다음 요청도 어차피 401이다.
func (j jobSessions) Dead(key string) {
	if s, ok := j.h.Sessions.Delete(key); ok {
		log.Printf("omv session expired: user=%q (seen by job watcher)", s.Username)
	}
}

// jobStarter는 조작(addons.ActionRequest.Jobs)이 요청한 세션으로 작업을 시작하는 입구다.
type jobStarter struct {
	h      *handler
	s      auth.Session
	caller *rpcCaller
}

func (j jobStarter) Start(ctx context.Context, kind string) (string, error) {
	id, err := j.h.Jobs.Start(ctx, jobs.Kind(kind), jobs.StartRequest{Owner: j.s.Username, SessionKey: j.s.ID, RPC: j.caller})
	switch {
	case errors.Is(err, jobs.ErrBusy):
		return "", &addons.ActionError{Status: http.StatusConflict, Code: api.ErrJobBusy, Message: "a job is already running"}
	case errors.Is(err, jobs.ErrLocked):
		return "", &addons.ActionError{Status: http.StatusConflict, Code: api.ErrAptLocked, Message: "package database is locked"}
	case errors.Is(err, jobs.ErrConfigDirty):
		return "", &addons.ActionError{Status: http.StatusConflict, Code: api.ErrConfigDirty, Message: "configuration changes are pending"}
	}
	return id, err
}

// jobsList는 진행 중인 작업과 이 사용자의 결과다(GET /api/jobs).
func (h *handler) jobsList(w http.ResponseWriter, _ *http.Request, s auth.Session) {
	h.Jobs.Offer()
	writeJSON(w, http.StatusOK, h.Jobs.Jobs(s.Username))
}

// jobGet은 작업 하나다(GET /api/jobs/{id}).
func (h *handler) jobGet(w http.ResponseWriter, r *http.Request, s auth.Session) {
	h.Jobs.Offer()
	j, err := h.Jobs.Get(r.PathValue("id"), s.Username)
	if err != nil {
		writeError(w, http.StatusNotFound, api.ErrNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, j)
}

// jobOutput은 작업 출력을 from(논리 위치)부터 준다(GET /api/jobs/{id}/output?from=N).
func (h *handler) jobOutput(w http.ResponseWriter, r *http.Request, s auth.Session) {
	h.Jobs.Offer()
	from := int64(0)
	if v := r.URL.Query().Get("from"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid from")
			return
		}
		from = n
	}
	out, err := h.Jobs.Output(r.PathValue("id"), s.Username, from)
	if err != nil {
		writeError(w, http.StatusNotFound, api.ErrNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// dismissNotice는 이 사용자의 지울 수 있는 알림(작업 결과) 하나를 지운다(DELETE /api/notifications/{id}).
func (h *handler) dismissNotice(w http.ResponseWriter, r *http.Request, s auth.Session) {
	id := r.PathValue("id")
	if !strings.HasPrefix(id, "job-") {
		writeError(w, http.StatusNotFound, api.ErrNotFound, "notice cannot be dismissed")
		return
	}
	switch err := h.Jobs.Dismiss(s.Username, id); {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, api.ErrNotFound, "notice cannot be dismissed")
	case err != nil:
		log.Printf("notifications: dismiss: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to dismiss notice")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// dismissNotices는 알림 센터의 "모두 지우기"다(DELETE /api/notifications?through=job-…). through는 화면에 보이는
// 가장 최근 작업 결과 알림이고, 그 알림과 그보다 먼저 끝난 이 사용자의 작업 결과를 모두 지운다. 출처 알림은 지울 수 없다.
func (h *handler) dismissNotices(w http.ResponseWriter, r *http.Request, s auth.Session) {
	through := r.URL.Query().Get("through")
	if through == "" {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "through is required")
		return
	}
	switch err := h.Jobs.DismissThrough(s.Username, through); {
	case errors.Is(err, jobs.ErrNotFound):
		writeError(w, http.StatusNotFound, api.ErrNotFound, "notice cannot be dismissed")
	case err != nil:
		log.Printf("notifications: dismiss all: %v", err)
		writeError(w, http.StatusInternalServerError, "", "failed to dismiss notices")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
