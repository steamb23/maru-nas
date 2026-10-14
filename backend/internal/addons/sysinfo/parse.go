// Package sysinfo는 /proc에서 시스템 통계를 읽는다.
//
// 파서는 샘플 데이터로 테스트할 수 있도록 io.Reader를 입력으로 받는다.
// 실제 /proc 파일과의 연결은 reader.go의 Reader 타입이 맡는다.
package sysinfo

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CPUTimes는 /proc/stat의 합산 "cpu" 줄이며 단위는 clock tick이다.
type CPUTimes struct {
	Idle  uint64 // idle + iowait
	Total uint64 // guest/guest_nice를 뺀 모든 필드의 합
}

// ParseCPUTimes는 /proc/stat에서 합산 "cpu" 줄을 파싱한다.
func ParseCPUTimes(r io.Reader) (CPUTimes, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		vals := fields[1:]
		if len(vals) < 4 {
			return CPUTimes{}, fmt.Errorf("cpu line has %d fields, want >= 4", len(vals))
		}
		// 필드 순서: user nice system idle iowait irq softirq steal guest guest_nice.
		// guest와 guest_nice는 이미 user, nice에 포함되어 있으므로 제외한다.
		if len(vals) > 8 {
			vals = vals[:8]
		}
		var t CPUTimes
		for i, v := range vals {
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return CPUTimes{}, fmt.Errorf("cpu field %d: %w", i, err)
			}
			t.Total += n
			if i == 3 || i == 4 { // idle, iowait
				t.Idle += n
			}
		}
		return t, nil
	}
	if err := sc.Err(); err != nil {
		return CPUTimes{}, err
	}
	return CPUTimes{}, errors.New("cpu line not found")
}

// CPUUsagePercent는 두 샘플 사이의 사용 시간 비율을 0~100으로 반환한다.
func CPUUsagePercent(prev, cur CPUTimes) float64 {
	if cur.Total <= prev.Total {
		return 0
	}
	total := float64(cur.Total - prev.Total)
	idle := float64(cur.Idle - prev.Idle)
	if cur.Idle < prev.Idle {
		idle = 0
	}
	p := (total - idle) / total * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// MemInfo는 /proc/meminfo에서 사용하는 값이며 단위는 바이트다.
type MemInfo struct {
	Total     uint64
	Available uint64
}

// Used는 Total - Available을 반환한다.
func (m MemInfo) Used() uint64 {
	if m.Available > m.Total {
		return 0
	}
	return m.Total - m.Available
}

// ParseMemInfo는 /proc/meminfo에서 MemTotal과 MemAvailable을 파싱한다.
func ParseMemInfo(r io.Reader) (MemInfo, error) {
	var m MemInfo
	var haveTotal, haveAvail bool
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		var dst *uint64
		switch key {
		case "MemTotal":
			dst, haveTotal = &m.Total, true
		case "MemAvailable":
			dst, haveAvail = &m.Available, true
		default:
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return MemInfo{}, fmt.Errorf("%s: missing value", key)
		}
		n, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return MemInfo{}, fmt.Errorf("%s: %w", key, err)
		}
		if len(fields) > 1 && fields[1] == "kB" {
			n *= 1024
		}
		*dst = n
	}
	if err := sc.Err(); err != nil {
		return MemInfo{}, err
	}
	if !haveTotal || !haveAvail {
		return MemInfo{}, errors.New("MemTotal or MemAvailable not found")
	}
	return m, nil
}

// ParseUptime은 /proc/uptime의 첫 번째 값(부팅 후 경과 초)을 파싱한다.
func ParseUptime(r io.Reader) (float64, error) {
	b, err := io.ReadAll(io.LimitReader(r, 256))
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, errors.New("empty uptime")
	}
	return strconv.ParseFloat(fields[0], 64)
}
