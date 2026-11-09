package power

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/api"
)

// 합성한 예약 파일. 형식은 systemd-logind(257)의 update_schedule_file과 같다.
const sample = "USEC=1794231164000000\nWARN_WALL=1\nMODE=reboot\nUID=0\nTTY=pts/0\nWALL_MESSAGE=maintenance\\nwindow\n"

func TestParseScheduled(t *testing.T) {
	got, err := ParseScheduled(strings.NewReader(sample))
	if err != nil || got != (Scheduled{USEC: 1794231164000000, Mode: "reboot"}) {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got.At() != time.UnixMicro(1794231164000000) {
		t.Fatalf("at %v", got.At())
	}
	for name, body := range map[string]string{
		"dry prefix": "USEC=5\nMODE=dry-poweroff\n",
		"dry field":  "USEC=5\nMODE=poweroff\nDRY_RUN=1\n",
	} {
		got, err := ParseScheduled(strings.NewReader(body))
		if err != nil || got != (Scheduled{USEC: 5, Mode: "poweroff", DryRun: true}) {
			t.Errorf("%s: %+v, %v", name, got, err)
		}
	}
	for _, bad := range []string{"", "MODE=reboot\n", "USEC=abc\nMODE=reboot\n", "USEC=0\n", "USEC=-5\n"} {
		if _, err := ParseScheduled(strings.NewReader(bad)); err == nil {
			t.Errorf("%q: no error", bad)
		}
	}
}

func TestReadScheduled(t *testing.T) {
	if _, ok, err := ReadScheduled(fstest.MapFS{}); ok || err != nil {
		t.Fatalf("missing: %v %v", ok, err)
	}
	fsys := fstest.MapFS{ScheduleFile: {Data: []byte(sample)}}
	if s, ok, err := ReadScheduled(fsys); !ok || err != nil || s.Mode != "reboot" {
		t.Fatalf("read: %+v %v %v", s, ok, err)
	}
	fsys[ScheduleFile] = &fstest.MapFile{Data: []byte("USEC=1\nWALL_MESSAGE=" + strings.Repeat("x", maxScheduleBytes))}
	if _, _, err := ReadScheduled(fsys); err == nil {
		t.Fatal("oversized file accepted")
	}
}

func TestModeOf(t *testing.T) {
	for in, want := range map[string]string{"reboot": ModeReboot, "kexec": ModeReboot, "soft-reboot": ModeReboot,
		"poweroff": ModePoweroff, "halt": ModePoweroff, "": ModeOther, "suspend": ModeOther} {
		if got := modeOf(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func file(at time.Time, mode string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("USEC=" + strconv.FormatInt(at.UnixMicro(), 10) + "\nWARN_WALL=1\nMODE=" + mode + "\nUID=0\n")}
}

func TestSchedulesGet(t *testing.T) {
	now := time.Date(2026, 11, 9, 20, 48, 0, 0, time.UTC)
	at := now.Add(6 * time.Hour)
	fsys := fstest.MapFS{}
	s := New(fsys, nil)
	s.Now = func() time.Time { return now }

	if got, err := s.Get(); got != nil || err != nil {
		t.Fatalf("none: %+v %v", got, err)
	}
	if err := s.Scheduled(at, "bob"); err != nil {
		t.Fatal(err)
	}
	// 파일이 없으면(다시 시작해 /run이 비었다) 기록을 지운다.
	if got, _ := s.Get(); got != nil {
		t.Fatalf("no file: %+v", got)
	}
	if _, ok, _ := s.Store.PowerRecord(); ok {
		t.Fatal("record kept without a schedule")
	}

	s.Scheduled(at, "bob")
	fsys[ScheduleFile] = file(at, "reboot")
	if got, err := s.Get(); err != nil || got == nil || !got.At.Equal(at) || got.Mode != ModeReboot || got.By != "bob" ||
		got.USEC != at.UnixMicro() {
		t.Fatalf("match: %+v %v", got, err)
	}
	// 마이크로초나 종류가 다르면 대시보드의 예약이 아니다.
	for _, f := range []*fstest.MapFile{file(at.Add(time.Microsecond), "reboot"), file(at, "poweroff"), file(at, "kexec")} {
		fsys[ScheduleFile] = f
		if got, _ := s.Get(); got == nil || got.By != "" {
			t.Errorf("%s: %+v", f.Data, got)
		}
	}
	// dry-run과 지난 시각은 예약이 없는 것이다.
	fsys[ScheduleFile] = file(at, "dry-reboot")
	if got, _ := s.Get(); got != nil {
		t.Fatalf("dry run: %+v", got)
	}
	s.Scheduled(at, "bob")
	fsys[ScheduleFile] = file(now, "reboot")
	if got, _ := s.Get(); got != nil {
		t.Fatalf("past: %+v", got)
	}
	if _, ok, _ := s.Store.PowerRecord(); ok {
		t.Fatal("record kept after the schedule passed")
	}
	// 읽을 수 없는 파일은 오류다(알림은 직전 결과를 유지한다).
	fsys[ScheduleFile] = &fstest.MapFile{Data: []byte("MODE=reboot\n")}
	if _, err := s.Get(); err == nil {
		t.Fatal("bad file: no error")
	}
}

func TestSource(t *testing.T) {
	now := time.Date(2026, 11, 9, 20, 48, 0, 0, time.UTC)
	at := now.Add(6 * time.Hour)
	fsys := fstest.MapFS{}
	s := New(fsys, nil)
	s.Now = func() time.Time { return now }
	src := Source{Schedules: s}
	if src.Admin() {
		t.Fatal("schedule notices must reach every user")
	}
	if got, err := src.Collect(context.Background()); len(got) != 0 || err != nil {
		t.Fatalf("none: %+v %v", got, err)
	}
	fsys[ScheduleFile] = file(at, "reboot")
	got, err := src.Collect(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("collect: %+v %v", got, err)
	}
	n := got[0]
	if n.ID != NoticeID || n.Code != api.NoticeRebootScheduled || n.At != "2026-11-10T02:48:00Z" || len(n.Items) != 0 ||
		n.Action == nil || n.Action.Kind != api.NoticeActionPower || n.Action.Page != api.NoticePowerSchedule {
		t.Fatalf("notice %+v", n)
	}
	// 예약한 사람이 정해지면 내용이 바뀐 것이다.
	s.Scheduled(at, "bob")
	got, _ = src.Collect(context.Background())
	if got[0].Revision == n.Revision || len(got[0].Items) != 1 || got[0].Items[0] != "bob" {
		t.Fatalf("with by: %+v", got[0])
	}
	fsys[ScheduleFile] = file(at, "poweroff")
	if got, _ = src.Collect(context.Background()); got[0].Code != api.NoticeShutdownScheduled {
		t.Fatalf("poweroff: %+v", got[0])
	}
}

func TestZoneName(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/share/zoneinfo/Asia/Seoul":                       "Asia/Seoul",
		"../usr/share/zoneinfo/America/Argentina/Buenos_Aires": "America/Argentina/Buenos_Aires",
		"Etc/GMT+9":      "Etc/GMT+9",
		"UTC":            "UTC",
		"/etc/localtime": "",
		"Asia/../../etc": "",
		"Asia/Seoul;rm":  "",
		"":               "",
	} {
		if got := zoneName(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
