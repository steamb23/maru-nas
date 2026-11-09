package worker

import (
	"errors"
	"slices"
	"testing"
	"time"

	"maru/backend/internal/helper/proto"
)

// recordLogind는 받은 호출을 기록하는 가짜 logind다.
type recordLogind struct {
	scheduled []time.Time
	cancels   int
	err       error
}

func (l *recordLogind) ScheduleReboot(at time.Time) error {
	l.scheduled = append(l.scheduled, at)
	return l.err
}

func (l *recordLogind) CancelShutdown() error {
	l.cancels++
	return l.err
}

func TestPowerSchedule(t *testing.T) {
	e := newEnv(t)
	now := time.Date(2026, 11, 9, 20, 48, 30, 0, time.UTC)
	e.cfg.Now = func() time.Time { return now }
	lg := &recordLogind{}
	e.cfg.Logind = lg
	run := func(params any) (proto.Response, error) {
		resp, _, err := e.run(proto.ScopeRoot, proto.ActionPowerSchedule, params, nil)
		return resp, err
	}
	at := time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)
	if resp, err := run(proto.PowerScheduleParams{At: at.Unix()}); !resp.OK || err != nil {
		t.Fatalf("schedule: %+v %v", resp, err)
	}
	if !slices.Equal(lg.scheduled, []time.Time{time.Unix(at.Unix(), 0)}) {
		t.Fatalf("scheduled %v", lg.scheduled)
	}
	// 지난 시각은 logind가 바로 다시 시작하므로 도우미가 막는다. 7일 넘게 먼 시각, 모르는 인자도 막는다.
	for _, bad := range []any{
		proto.PowerScheduleParams{At: now.Unix()},
		proto.PowerScheduleParams{At: now.Add(-time.Hour).Unix()},
		proto.PowerScheduleParams{At: now.Add(proto.MaxPowerScheduleAhead + time.Minute).Unix()},
		proto.PowerScheduleParams{},
		map[string]any{"at": at.Unix(), "mode": "poweroff"},
	} {
		if resp, _ := run(bad); resp.OK || resp.Code != proto.CodeBadRequest {
			t.Errorf("%+v: %+v", bad, resp)
		}
	}
	if len(lg.scheduled) != 1 {
		t.Fatalf("bad requests reached logind: %v", lg.scheduled)
	}
	// logind가 거절하면(지연 대신 막는 억제 등) 내부 오류다.
	lg.err = errors.New("access denied")
	if resp, _ := run(proto.PowerScheduleParams{At: at.Unix()}); resp.OK || resp.Code != proto.CodeInternal {
		t.Fatalf("logind failure: %+v", resp)
	}
}

func TestPowerCancel(t *testing.T) {
	e := newEnv(t)
	lg := &recordLogind{}
	e.cfg.Logind = lg
	if resp, _, err := e.run(proto.ScopeRoot, proto.ActionPowerCancel, nil, nil); !resp.OK || err != nil {
		t.Fatalf("cancel: %+v %v", resp, err)
	}
	if lg.cancels != 1 {
		t.Fatalf("cancels %d", lg.cancels)
	}
}

// 사용자 범위로는 부를 수 없다(부모가 막지만 worker도 다시 본다).
func TestPowerUserScope(t *testing.T) {
	e := newEnv(t)
	lg := &recordLogind{}
	e.cfg.Logind = lg
	for _, a := range []string{proto.ActionPowerSchedule, proto.ActionPowerCancel} {
		if resp, _, _ := e.run(proto.ScopeUser, a, proto.PowerScheduleParams{At: time.Now().Add(time.Hour).Unix()}, nil); resp.OK {
			t.Errorf("%s ran in user scope", a)
		}
	}
	if len(lg.scheduled) != 0 || lg.cancels != 0 {
		t.Fatal("user scope reached logind")
	}
}

func TestBusctlArgs(t *testing.T) {
	at := time.Unix(1794200000, 0)
	want := []string{"call", "org.freedesktop.login1", "/org/freedesktop/login1", "org.freedesktop.login1.Manager",
		"ScheduleShutdown", "st", "reboot", "1794200000000000"}
	if got := scheduleArgs(at); !slices.Equal(got, want) {
		t.Errorf("schedule args %q", got)
	}
	if got := cancelArgs(); !slices.Equal(got, []string{"call", "org.freedesktop.login1", "/org/freedesktop/login1",
		"org.freedesktop.login1.Manager", "CancelScheduledShutdown"}) {
		t.Errorf("cancel args %q", got)
	}
}
