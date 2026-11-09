package power

import (
	"context"
	"strconv"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/notify"
)

// NoticeID는 예약 알림의 id다. 읽음 상태의 키다.
const NoticeID = "power-schedule"

// Source는 예약된 종료를 알리는 출처다. 그 시각에 모든 세션이 끊기므로 모든 사용자에게 보인다.
type Source struct{ Schedules *Schedules }

func (Source) Name() string { return "power-schedule" }
func (Source) Admin() bool  { return false }

// Collect는 지금 예약을 읽는다. 리비전은 시각·종류·예약한 사람으로 만든다(예약을 바꾸면 다시 읽지 않은 알림이 된다).
func (s Source) Collect(context.Context) ([]api.Notice, error) {
	sc, err := s.Schedules.Get()
	if err != nil || sc == nil {
		return nil, err
	}
	n := api.Notice{
		ID:       NoticeID,
		Code:     api.NoticeShutdownScheduled,
		Level:    api.NoticeWarning,
		Revision: notify.Revision("power", strconv.FormatInt(sc.USEC, 10), "mode="+sc.Mode, "by="+sc.By),
		At:       sc.At.UTC().Format(time.RFC3339),
		Action:   &api.NoticeAction{Kind: api.NoticeActionPower},
	}
	if sc.Mode == ModeReboot {
		n.Code = api.NoticeRebootScheduled
	}
	if sc.By != "" {
		n.Items = []string{sc.By}
	}
	return []api.Notice{n}, nil
}
