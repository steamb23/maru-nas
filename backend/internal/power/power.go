// Package power는 systemd-logind에 예약된 종료(다시 시작, 시스템 종료)를 읽고, 대시보드에서 예약한 사람을 기록한다.
//
// 예약과 취소는 권한 도우미의 root 연산(power.schedule, power.cancel)이 logind에 한다. 이 패키지는 logind가 쓰는 예약
// 파일(/run/systemd/shutdown/scheduled, 0644)을 직접 읽으므로 대시보드 밖(SSH의 shutdown 등)에서 한 예약도 보인다.
// logind는 예약한 사람을 남기지 않으므로(도우미도 sudo도 UID는 root다) 대시보드가 예약할 때 남긴 기록(Record)이 파일의
// 시각(마이크로초)·종류와 정확히 같을 때만 예약한 사람을 알린다.
package power

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"strconv"
	"strings"
	"time"
)

// ScheduleFile은 logind의 예약 파일이다(fs.FS 경로라 앞 슬래시가 없다).
const ScheduleFile = "run/systemd/shutdown/scheduled"

// maxScheduleBytes는 예약 파일을 읽는 크기 상한이다(벽 메시지가 들어갈 수 있다).
const maxScheduleBytes = 64 << 10

// 종료 종류(api.PowerSchedule.Mode).
const (
	ModeReboot   = "reboot"
	ModePoweroff = "poweroff"
	ModeOther    = "other"
)

// logindReboot는 도우미가 예약하는 logind 종류 이름이다(ScheduleShutdown의 첫 인자).
const logindReboot = "reboot"

// Scheduled는 logind 예약 파일의 내용이다.
type Scheduled struct {
	// USEC는 예약 시각(CLOCK_REALTIME 마이크로초)이다.
	USEC int64
	// Mode는 logind의 종류 이름 그대로다(reboot, poweroff, halt, kexec, soft-reboot 등).
	Mode string
	// DryRun이면 시각이 되어도 아무 일도 하지 않는다(shutdown --dry-run 등).
	DryRun bool
}

// At은 예약 시각이다.
func (s Scheduled) At() time.Time { return time.UnixMicro(s.USEC) }

// ParseScheduled는 예약 파일(KEY=VALUE 줄)을 읽는다. USEC와 MODE만 보고 나머지(WARN_WALL, UID, TTY, WALL_MESSAGE)는
// 무시한다. USEC가 없거나 양수가 아니면 오류다.
func ParseScheduled(r io.Reader) (Scheduled, error) {
	var s Scheduled
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxScheduleBytes)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		switch key {
		case "USEC":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n <= 0 {
				return Scheduled{}, fmt.Errorf("power: bad USEC %q", value)
			}
			s.USEC = n
		case "MODE":
			s.Mode = value
		case "DRY_RUN":
			s.DryRun = value == "1" || value == "yes" || value == "true"
		}
	}
	if err := sc.Err(); err != nil {
		return Scheduled{}, err
	}
	if s.USEC == 0 {
		return Scheduled{}, errors.New("power: no USEC")
	}
	// 옛 logind는 dry-run을 종류 이름 앞에 붙였다.
	if m, ok := strings.CutPrefix(s.Mode, "dry-"); ok {
		s.Mode, s.DryRun = m, true
	}
	return s, nil
}

// ReadScheduled는 fsys에서 예약 파일을 읽는다. 없으면 ok가 false다.
func ReadScheduled(fsys fs.FS) (s Scheduled, ok bool, err error) {
	f, err := fsys.Open(ScheduleFile)
	if errors.Is(err, fs.ErrNotExist) {
		return Scheduled{}, false, nil
	}
	if err != nil {
		return Scheduled{}, false, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxScheduleBytes+1))
	if err != nil {
		return Scheduled{}, false, err
	}
	if len(b) > maxScheduleBytes {
		return Scheduled{}, false, errors.New("power: schedule file is too large")
	}
	s, err = ParseScheduled(bytes.NewReader(b))
	if err != nil {
		return Scheduled{}, false, err
	}
	return s, true, nil
}

// modeOf는 logind 종류 이름을 화면의 종류로 바꾼다. 커널부터 다시 올라오는 종류는 다시 시작, 전원이 꺼지거나 멈추는
// 종류는 시스템 종료로 본다.
func modeOf(logind string) string {
	switch logind {
	case "reboot", "kexec", "soft-reboot":
		return ModeReboot
	case "poweroff", "halt":
		return ModePoweroff
	}
	return ModeOther
}

// Schedule은 지금 예약이다.
type Schedule struct {
	At time.Time
	// Mode는 ModeReboot, ModePoweroff, ModeOther 중 하나다.
	Mode string
	// By는 대시보드에서 예약한 사용자 이름이다. 대시보드 밖에서 예약했으면 비어 있다.
	By string
	// USEC는 logind 예약 시각 그대로다(알림 리비전에 쓴다).
	USEC int64
}

// Schedules는 예약 파일과 예약 기록을 함께 읽는다.
type Schedules struct {
	// FS는 예약 파일을 읽는 루트다(운영은 /, 개발 서버는 가짜 루트).
	FS    fs.FS
	Store Store
	Now   func() time.Time
}

// New는 fsys의 예약 파일과 store의 기록을 읽는 Schedules다. store가 nil이면 메모리에 둔다.
func New(fsys fs.FS, store Store) *Schedules {
	if store == nil {
		store = NewMemoryStore()
	}
	return &Schedules{FS: fsys, Store: store, Now: time.Now}
}

// Get은 지금 예약이다. 없거나, 시각이 지났거나(종료가 진행 중이거나 가짜 logind), dry-run이면 nil이다. 예약이 없으면
// 남은 기록을 지운다(다시 시작하면 /run이 비므로 기록만 남는다).
func (s *Schedules) Get() (*Schedule, error) {
	sc, ok, err := ReadScheduled(s.FS)
	if err != nil {
		return nil, err
	}
	if !ok || sc.DryRun || !sc.At().After(s.Now()) {
		s.clearStale()
		return nil, nil
	}
	out := &Schedule{At: sc.At(), Mode: modeOf(sc.Mode), USEC: sc.USEC}
	rec, ok, err := s.Store.PowerRecord()
	switch {
	case err != nil:
		log.Printf("power: schedule record: %v", err)
	case ok && rec.USEC == sc.USEC && rec.Mode == sc.Mode:
		out.By = rec.By
	}
	return out, nil
}

func (s *Schedules) clearStale() {
	if _, ok, err := s.Store.PowerRecord(); err == nil && ok {
		if err := s.Store.ClearPowerRecord(); err != nil {
			log.Printf("power: schedule record: %v", err)
		}
	}
}

// Scheduled는 by가 at에 다시 시작하도록 예약했다고 기록한다. 도우미가 logind에 예약한 뒤 부른다. 도우미는 초 단위로
// 예약하므로 기록도 초를 마이크로초로 바꾼 값이다.
func (s *Schedules) Scheduled(at time.Time, by string) error {
	return s.Store.SetPowerRecord(Record{USEC: at.Unix() * 1_000_000, Mode: logindReboot, By: by})
}

// Cancelled는 예약 기록을 지운다. 도우미가 logind의 예약을 취소한 뒤 부른다.
func (s *Schedules) Cancelled() error { return s.Store.ClearPowerRecord() }
