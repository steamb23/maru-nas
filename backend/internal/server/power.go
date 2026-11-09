package server

import (
	"context"
	"log"
	"net/http"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/auth"
)

// powerHelperTimeout은 다시 시작 예약·취소에서 권한 도우미를 기다리는 시간이다. 요청이 끊겨도 끝까지 기다려
// logind에 한 예약과 기록이 어긋나지 않게 한다.
const powerHelperTimeout = 30 * time.Second

// powerSchedule은 지금 예약된 종료를 준다(관리자). 예약 파일을 매번 새로 읽으므로 대시보드 밖에서 한 취소도 바로 보인다
// (서버 알림은 30초 보관한다).
func (h *handler) powerSchedule(w http.ResponseWriter, _ *http.Request, _ auth.Session) {
	h.writePowerSchedule(w)
}

// setPowerSchedule은 정한 시각에 다시 시작하도록 예약한다(관리자, root 권한 상승 필요). 이미 있는 예약(대시보드 밖에서 한
// 시스템 종료 포함)은 이 예약으로 바뀐다. 상승하지 않았으면 도우미의 elevation_required를 그대로 돌려준다.
func (h *handler) setPowerSchedule(w http.ResponseWriter, r *http.Request, s auth.Session) {
	var req api.PowerScheduleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	at, err := time.Parse(time.RFC3339, req.At)
	if err != nil {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "at must be an RFC 3339 time")
		return
	}
	at = at.Truncate(time.Second)
	if d := at.Sub(h.Now()); d < api.PowerScheduleMinAheadSeconds*time.Second || d > api.PowerScheduleMaxAheadSeconds*time.Second {
		writeError(w, http.StatusBadRequest, api.ErrPowerTimeInvalid, "the time must be between 1 minute and 7 days from now")
		return
	}
	if !h.powerReady(w) {
		return
	}
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), powerHelperTimeout)
	defer cancel()
	if err := h.Helper.PowerSchedule(ctx, ticket, at); err != nil {
		h.writeHelperError(w, s, err)
		return
	}
	log.Printf("power: reboot scheduled at=%s by=%q", at.UTC().Format(time.RFC3339), s.Username)
	// logind는 이미 예약했다. 기록을 남기지 못하면 예약한 사람만 모르는 예약이 되므로 오류로 돌려주지 않는다.
	if err := h.Power.Scheduled(at, s.Username); err != nil {
		log.Printf("power: record schedule: %v", err)
	}
	h.Notify.Invalidate()
	h.writePowerSchedule(w)
}

// cancelPowerSchedule은 예약된 종료를 취소한다(관리자, root 권한 상승 필요). 대시보드 밖에서 한 예약도 취소한다.
func (h *handler) cancelPowerSchedule(w http.ResponseWriter, r *http.Request, s auth.Session) {
	if !h.powerReady(w) {
		return
	}
	ticket, ok := h.helperTicket(w, s)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), powerHelperTimeout)
	defer cancel()
	if err := h.Helper.PowerCancel(ctx, ticket); err != nil {
		h.writeHelperError(w, s, err)
		return
	}
	log.Printf("power: scheduled shutdown cancelled by=%q", s.Username)
	if err := h.Power.Cancelled(); err != nil {
		log.Printf("power: clear schedule record: %v", err)
	}
	h.Notify.Invalidate()
	h.writePowerSchedule(w)
}

// powerReady는 예약을 읽을 수 있는지다. 아니면 503을 쓴다.
func (h *handler) powerReady(w http.ResponseWriter) bool {
	if h.Power == nil {
		writeError(w, http.StatusServiceUnavailable, api.ErrHelperUnavailable, "restart scheduling is not configured")
		return false
	}
	return true
}

func (h *handler) writePowerSchedule(w http.ResponseWriter) {
	resp := api.PowerScheduleResponse{Now: h.Now().Format(time.RFC3339), TimeZone: h.TimeZone}
	if h.Power != nil {
		sc, err := h.Power.Get()
		if err != nil {
			log.Printf("power: read schedule: %v", err)
			writeError(w, http.StatusInternalServerError, "", "failed to read the shutdown schedule")
			return
		}
		if sc != nil {
			resp.Schedule = &api.PowerSchedule{At: sc.At.UTC().Format(time.RFC3339), Mode: sc.Mode, By: sc.By}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
