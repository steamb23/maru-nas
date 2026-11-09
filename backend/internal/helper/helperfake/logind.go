package helperfake

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// fakeLogind는 systemd-logind 대신 예약 파일만 쓰고 지운다. 시각이 지나도 다시 시작하지 않으므로 파일은 지난
// 시각으로 남는다(읽는 쪽은 지난 예약을 없는 것으로 본다). 형식은 logind의 update_schedule_file과 같다.
type fakeLogind struct {
	file string
}

func (l fakeLogind) ScheduleReboot(at time.Time) error {
	if err := os.MkdirAll(filepath.Dir(l.file), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("USEC=%d\nWARN_WALL=1\nMODE=reboot\nUID=0\n", at.UnixMicro())
	tmp := l.file + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, l.file)
}

func (l fakeLogind) CancelShutdown() error {
	if err := os.Remove(l.file); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
