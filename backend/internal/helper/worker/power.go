package worker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"maru/backend/internal/helper/proto"
)

// Logind는 systemd-logind의 예약된 종료를 다룬다. 운영은 busctl로 D-Bus를 부르고(BusctlLogind), 가짜 도우미는
// 파일만 쓴다.
type Logind interface {
	// ScheduleReboot은 at에 다시 시작하도록 예약한다. 이미 있는 예약은 바뀐다.
	ScheduleReboot(at time.Time) error
	// CancelShutdown은 예약된 종료를 취소한다. 예약이 없어도 성공이다.
	CancelShutdown() error
}

// busctlPath는 systemd의 D-Bus 도구다. 셸을 거치지 않고 인자를 직접 넘긴다.
const busctlPath = "/usr/bin/busctl"

// busctlTimeout은 logind 호출 하나를 기다리는 시간이다.
const busctlTimeout = 20 * time.Second

// logindDest·logindPath·logindManager는 logind의 D-Bus 이름, 객체, 인터페이스다.
const (
	logindDest    = "org.freedesktop.login1"
	logindPath    = "/org/freedesktop/login1"
	logindManager = "org.freedesktop.login1.Manager"
)

// BusctlLogind는 busctl로 logind의 ScheduleShutdown·CancelScheduledShutdown을 부른다. root가 부르면 logind는
// polkit 없이 허용한다(호출자 uid가 logind와 같다). 도우미 유닛의 하드닝(CAP_SYS_BOOT 없음, PrivateNetwork)에서도
// 시스템 버스 소켓에 닿는다.
type BusctlLogind struct{}

func (BusctlLogind) ScheduleReboot(at time.Time) error {
	_, err := runBusctl(scheduleArgs(at))
	return err
}

func (BusctlLogind) CancelShutdown() error {
	// 예약이 없으면 "b false"를 돌려준다. 취소는 멱등이므로 결과는 보지 않는다.
	_, err := runBusctl(cancelArgs())
	return err
}

// scheduleArgs는 at에 다시 시작하는 busctl 인자다. 시각은 CLOCK_REALTIME 마이크로초다.
func scheduleArgs(at time.Time) []string {
	return []string{"call", logindDest, logindPath, logindManager, "ScheduleShutdown", "st", "reboot",
		strconv.FormatInt(at.UnixMicro(), 10)}
}

func cancelArgs() []string {
	return []string{"call", logindDest, logindPath, logindManager, "CancelScheduledShutdown"}
}

func runBusctl(args []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), busctlTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, busctlPath, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("busctl %s: %w: %s", args[4], err, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// powerSchedule은 정한 시각에 다시 시작하도록 예약한다(root 범위). 시각만 받고 다시 확인한다.
func powerSchedule(job Job, cfg Config) error {
	var p proto.PowerScheduleParams
	if err := proto.DecodeParams(job.Params, &p); err != nil {
		return err
	}
	at := time.Unix(p.At, 0)
	if !proto.ValidPowerSchedule(at, cfg.Now()) {
		return proto.NewError(proto.CodeBadRequest)
	}
	return cfg.Logind.ScheduleReboot(at)
}

// powerCancel은 예약된 종료를 취소한다(root 범위).
func powerCancel(cfg Config) error {
	return cfg.Logind.CancelShutdown()
}
