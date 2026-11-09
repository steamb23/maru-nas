package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/helper/proto"
	"maru/backend/internal/power"
)

func powerNotice(t *testing.T, b *browser) (*api.Notice, api.NotificationsResponse) {
	t.Helper()
	resp := decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil))
	for _, n := range resp.Notices {
		if n.ID == power.NoticeID {
			return &n, resp
		}
	}
	return nil, resp
}

// writeLogind는 대시보드 밖(SSH의 shutdown 등)에서 한 예약처럼 logind 형식의 예약 파일을 쓴다.
func writeLogind(t *testing.T, e *env, at time.Time, mode string) {
	t.Helper()
	body := fmt.Sprintf("USEC=%d\nWARN_WALL=1\nMODE=%s\nUID=0\nTTY=pts/0\n", at.UnixMicro(), mode)
	if err := os.MkdirAll(filepath.Dir(e.helper.ShutdownFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(e.helper.ShutdownFile, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPowerSchedule(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, powerNotices: true, clockStart: time.Now().Truncate(time.Second)})
	now := e.clk.Now()
	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, user.do(http.MethodGet, "/api/power/schedule", nil), http.StatusForbidden, api.ErrAdminRequired)
	wantCode(t, user.do(http.MethodPut, "/api/power/schedule", api.PowerScheduleRequest{At: now.Add(time.Hour).Format(time.RFC3339)}),
		http.StatusForbidden, api.ErrAdminRequired)
	if n, resp := powerNotice(t, user); n != nil || resp.Now != now.Format(time.RFC3339) {
		t.Fatalf("before schedule: notice %+v now %q", n, resp.Now)
	}

	b := e.browser()
	loginTOTP(t, b)
	got := decode[api.PowerScheduleResponse](t, b.do(http.MethodGet, "/api/power/schedule", nil))
	if got.Schedule != nil || got.Now != now.Format(time.RFC3339) || got.TimeZone != "Asia/Seoul" {
		t.Fatalf("empty schedule = %+v", got)
	}
	set := func(at string) api.PowerScheduleResponse {
		t.Helper()
		return decode[api.PowerScheduleResponse](t, b.do(http.MethodPut, "/api/power/schedule", api.PowerScheduleRequest{At: at}))
	}

	// 형식과 범위는 도우미에 가기 전에 거른다.
	wantCode(t, b.do(http.MethodPut, "/api/power/schedule", api.PowerScheduleRequest{At: "tomorrow 3am"}), http.StatusBadRequest, api.ErrBadRequest)
	for _, at := range []time.Time{now.Add(-time.Minute), now.Add(30 * time.Second), now.Add(7*24*time.Hour + time.Minute)} {
		wantCode(t, b.do(http.MethodPut, "/api/power/schedule", api.PowerScheduleRequest{At: at.Format(time.RFC3339)}),
			http.StatusBadRequest, api.ErrPowerTimeInvalid)
	}
	// root 연산이라 권한 상승이 필요하다.
	at := now.Add(6 * time.Hour)
	wantCode(t, b.do(http.MethodPut, "/api/power/schedule", api.PowerScheduleRequest{At: at.Format(time.RFC3339)}),
		http.StatusForbidden, api.ErrElevationRequired)
	wantCode(t, b.do(http.MethodDelete, "/api/power/schedule", nil), http.StatusForbidden, api.ErrElevationRequired)
	if _, err := os.Stat(e.helper.ShutdownFile); !os.IsNotExist(err) {
		t.Fatalf("scheduled before elevation: %v", err)
	}
	wantStatus(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}), http.StatusOK)

	// 초 아래는 버린다. 다른 시간대로 보내도 같은 시각이다.
	got = set(at.In(time.FixedZone("", -5*3600)).Add(900 * time.Millisecond).Format(time.RFC3339Nano))
	want := api.PowerSchedule{At: at.UTC().Format(time.RFC3339), Mode: power.ModeReboot, By: "totp"}
	if got.Schedule == nil || *got.Schedule != want {
		t.Fatalf("scheduled = %+v", got.Schedule)
	}
	data, err := os.ReadFile(e.helper.ShutdownFile)
	if err != nil || !strings.Contains(string(data), fmt.Sprintf("USEC=%d\n", at.Unix()*1_000_000)) {
		t.Fatalf("logind file %q: %v", data, err)
	}
	// 예약은 모든 사용자에게 알린다(그 시각에 모든 세션이 끊긴다).
	n, _ := powerNotice(t, user)
	if n == nil || n.Code != api.NoticeRebootScheduled || n.At != want.At || !slices.Equal(n.Items, []string{"totp"}) ||
		n.Action == nil || n.Action.Kind != api.NoticeActionPower || n.Level != api.NoticeWarning {
		t.Fatalf("notice = %+v", n)
	}
	rev := n.Revision

	// 대시보드 밖에서 시스템 종료로 바꿨다: 기록과 맞지 않으므로 예약한 사람을 모른다.
	elsewhere := now.Add(2 * time.Hour)
	writeLogind(t, e, elsewhere, "poweroff")
	got = decode[api.PowerScheduleResponse](t, b.do(http.MethodGet, "/api/power/schedule", nil))
	if got.Schedule == nil || *got.Schedule != (api.PowerSchedule{At: elsewhere.UTC().Format(time.RFC3339), Mode: power.ModePoweroff}) {
		t.Fatalf("elsewhere = %+v", got.Schedule)
	}
	// 같은 시각이라도 마이크로초가 다르면 대시보드의 예약이 아니다.
	writeLogind(t, e, at.Add(time.Microsecond), "reboot")
	if got = decode[api.PowerScheduleResponse](t, b.do(http.MethodGet, "/api/power/schedule", nil)); got.Schedule == nil || got.Schedule.By != "" {
		t.Fatalf("microsecond mismatch = %+v", got.Schedule)
	}
	// 알림은 30초 보관한다. 지난 뒤에는 새 예약을 알리고 리비전이 바뀐다.
	e.clk.Add(31 * time.Second)
	writeLogind(t, e, elsewhere, "poweroff")
	if n, _ = powerNotice(t, user); n == nil || n.Code != api.NoticeShutdownScheduled || len(n.Items) != 0 || n.Revision == rev {
		t.Fatalf("elsewhere notice = %+v", n)
	}

	// 시각이 지난 예약은 없는 것으로 보고 남은 기록을 지운다.
	set(at.Format(time.RFC3339))
	writeLogind(t, e, e.clk.Now().Add(-time.Second), "reboot")
	if got = decode[api.PowerScheduleResponse](t, b.do(http.MethodGet, "/api/power/schedule", nil)); got.Schedule != nil {
		t.Fatalf("past schedule = %+v", got.Schedule)
	}
	if _, ok, _ := e.power.Store.PowerRecord(); ok {
		t.Fatal("record kept after the schedule passed")
	}

	// 취소하면 파일과 알림이 사라진다(취소는 알림을 바로 다시 읽게 한다).
	set(at.Format(time.RFC3339))
	if n, _ = powerNotice(t, user); n == nil {
		t.Fatal("no notice after rescheduling")
	}
	got = decode[api.PowerScheduleResponse](t, b.do(http.MethodDelete, "/api/power/schedule", nil))
	if got.Schedule != nil {
		t.Fatalf("after cancel = %+v", got.Schedule)
	}
	if _, err := os.Stat(e.helper.ShutdownFile); !os.IsNotExist(err) {
		t.Fatalf("logind file after cancel: %v", err)
	}
	if n, _ = powerNotice(t, user); n != nil {
		t.Fatalf("notice after cancel = %+v", n)
	}
	// 예약이 없어도 취소는 성공한다.
	wantStatus(t, b.do(http.MethodDelete, "/api/power/schedule", nil), http.StatusOK)

	logs := e.logs.String()
	if !strings.Contains(logs, `power: reboot scheduled at=`+at.UTC().Format(time.RFC3339)+` by="totp"`) ||
		!strings.Contains(logs, `power: scheduled shutdown cancelled by="totp"`) {
		t.Fatalf("logs:\n%s", logs)
	}
}

// 시스템 종료처럼 다시 시작이 아닌 예약을 대시보드에서 바꾸면 다시 시작 예약이 된다.
func TestPowerScheduleReplacesElsewhere(t *testing.T) {
	e := newEnv(t, envOptions{helper: true, clockStart: time.Now().Truncate(time.Second)})
	b := e.browser()
	loginTOTP(t, b)
	wantStatus(t, b.do(http.MethodPost, "/api/helper/elevation", api.ElevationRequest{Password: "totp"}), http.StatusOK)
	writeLogind(t, e, e.clk.Now().Add(time.Hour), "poweroff")
	at := e.clk.Now().Add(3 * time.Hour)
	got := decode[api.PowerScheduleResponse](t, b.do(http.MethodPut, "/api/power/schedule", api.PowerScheduleRequest{At: at.Format(time.RFC3339)}))
	if got.Schedule == nil || got.Schedule.Mode != power.ModeReboot || got.Schedule.By != "totp" {
		t.Fatalf("replaced = %+v", got.Schedule)
	}
}

// 도우미가 없거나 예약을 읽지 못하는 설치.
func TestPowerScheduleUnavailable(t *testing.T) {
	e := newEnv(t, envOptions{helperSocket: filepath.Join(t.TempDir(), "missing.sock")})
	b := e.browser()
	loginTOTP(t, b)
	got := decode[api.PowerScheduleResponse](t, b.do(http.MethodGet, "/api/power/schedule", nil))
	if got.Schedule != nil {
		t.Fatalf("schedule = %+v", got.Schedule)
	}
	wantCode(t, b.do(http.MethodPut, "/api/power/schedule", api.PowerScheduleRequest{At: e.clk.Now().Add(time.Hour).Format(time.RFC3339)}),
		http.StatusServiceUnavailable, api.ErrHelperUnavailable)
}

// 화면이 쓰는 상한과 도우미가 다시 확인하는 상한이 같다.
func TestPowerScheduleLimits(t *testing.T) {
	if time.Duration(api.PowerScheduleMaxAheadSeconds)*time.Second != proto.MaxPowerScheduleAhead {
		t.Fatalf("api max %ds, helper max %v", api.PowerScheduleMaxAheadSeconds, proto.MaxPowerScheduleAhead)
	}
}
