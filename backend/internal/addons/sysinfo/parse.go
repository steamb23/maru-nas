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
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"maru/backend/internal/api"
)

// CPUTimes는 /proc/stat의 합산 "cpu" 줄이며 단위는 clock tick이다.
type CPUTimes struct {
	Idle   uint64 // idle + iowait
	Kernel uint64 // system + irq + softirq
	Total  uint64 // guest/guest_nice를 뺀 모든 필드의 합
}

// ParseCPUTimes는 /proc/stat에서 합산 "cpu" 줄을 파싱한다.
func ParseCPUTimes(r io.Reader) (CPUTimes, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		return parseCPUFields(fields[1:])
	}
	if err := sc.Err(); err != nil {
		return CPUTimes{}, err
	}
	return CPUTimes{}, errors.New("cpu line not found")
}

// parseCPUFields는 /proc/stat의 cpu 줄에서 이름 뒤의 값을 합산한다.
func parseCPUFields(vals []string) (CPUTimes, error) {
	if len(vals) < 4 {
		return CPUTimes{}, fmt.Errorf("cpu line has %d fields, want >= 4", len(vals))
	}
	// 필드 순서: user nice system idle iowait irq softirq steal guest guest_nice.
	// guest와 guest_nice는 이미 user, nice에 포함되어 있으므로 제외한다.
	// 커널 시간은 system, irq, softirq다. steal은 하이퍼바이저가 가져간 시간이라 넣지 않는다.
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
		switch i {
		case 3, 4: // idle, iowait
			t.Idle += n
		case 2, 5, 6: // system, irq, softirq
			t.Kernel += n
		}
	}
	return t, nil
}

// CoreTimes는 /proc/stat의 "cpuN" 줄 하나다.
type CoreTimes struct {
	ID int
	CPUTimes
}

// CPUStat은 /proc/stat의 합산 "cpu" 줄과 켜져 있는 코어별 "cpuN" 줄이다.
type CPUStat struct {
	All   CPUTimes
	Cores []CoreTimes
}

// ParseCPUStat은 /proc/stat에서 합산 줄과 코어별 줄을 파싱한다. 코어는 나온 순서대로 둔다.
func ParseCPUStat(r io.Reader) (CPUStat, error) {
	var st CPUStat
	found := false
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		t, err := parseCPUFields(fields[1:])
		if err != nil {
			return CPUStat{}, fmt.Errorf("%s: %w", fields[0], err)
		}
		if fields[0] == "cpu" {
			st.All, found = t, true
			continue
		}
		id, err := strconv.Atoi(fields[0][len("cpu"):])
		if err != nil || id < 0 {
			return CPUStat{}, fmt.Errorf("bad cpu line name %q", fields[0])
		}
		st.Cores = append(st.Cores, CoreTimes{ID: id, CPUTimes: t})
	}
	if err := sc.Err(); err != nil {
		return CPUStat{}, err
	}
	if !found {
		return CPUStat{}, errors.New("cpu line not found")
	}
	return st, nil
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
// Total과 Available 말고는 커널에 따라 줄이 없을 수 있고(Zswap 등), 없으면 0이다.
type MemInfo struct {
	Total        uint64
	Available    uint64
	Free         uint64
	Buffers      uint64
	Cached       uint64 // Shmem을 포함한다
	Shmem        uint64
	SReclaimable uint64
	Zswap        uint64 // zswap 풀이 실제로 쓰는 크기(압축 뒤). Zswapped는 압축 전 크기라 쓰지 않는다
	SwapTotal    uint64
	SwapFree     uint64
}

// Used는 Total - Available을 반환한다.
func (m MemInfo) Used() uint64 {
	if m.Available > m.Total {
		return 0
	}
	return m.Total - m.Available
}

// Detail은 RAM 구성(htop 방식)과 스왑이다. zramBytes는 zram 장치가 쓰는 메모리 합계다.
// Cached에 Shmem이 포함되므로 캐시는 Cached + SReclaimable - Shmem이다. 항목의 합이 늘 Total이 되게
// 여유, 버퍼, 공유, 캐시, 압축 순으로 남은 양 안에서만 떼고 나머지를 앱으로 둔다.
func (m MemInfo) Detail(zramBytes uint64) api.MemoryDetail {
	rest := m.Total
	take := func(n uint64) uint64 {
		n = min(n, rest)
		rest -= n
		return n
	}
	var cache uint64
	if c := m.Cached + m.SReclaimable; c > m.Shmem {
		cache = c - m.Shmem
	}
	d := api.MemoryDetail{SwapTotalBytes: m.SwapTotal}
	d.FreeBytes = take(m.Free)
	d.BuffersBytes = take(m.Buffers)
	d.SharedBytes = take(m.Shmem)
	d.CacheBytes = take(cache)
	d.CompressedBytes = take(m.Zswap + zramBytes)
	d.AppsBytes = rest
	if m.SwapFree < m.SwapTotal {
		d.SwapUsedBytes = m.SwapTotal - m.SwapFree
	}
	return d
}

// ParseMemInfo는 /proc/meminfo를 파싱한다. MemTotal과 MemAvailable은 반드시 있어야 한다.
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
		case "MemFree":
			dst = &m.Free
		case "Buffers":
			dst = &m.Buffers
		case "Cached":
			dst = &m.Cached
		case "Shmem":
			dst = &m.Shmem
		case "SReclaimable":
			dst = &m.SReclaimable
		case "Zswap":
			dst = &m.Zswap
		case "SwapTotal":
			dst = &m.SwapTotal
		case "SwapFree":
			dst = &m.SwapFree
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

// LoadAvg는 /proc/loadavg의 값이다.
type LoadAvg struct {
	// One, Five, Fifteen은 1·5·15분 부하 평균이다.
	One, Five, Fifteen float64
	// Running은 지금 실행할 수 있는 스케줄링 단위 수, Threads는 전체 스케줄링 단위(스레드) 수다(넷째 값 "실행 중/전체").
	Running int
	Threads int
}

// ParseLoadavg는 /proc/loadavg를 파싱한다.
func ParseLoadavg(r io.Reader) (LoadAvg, error) {
	b, err := io.ReadAll(io.LimitReader(r, 256))
	if err != nil {
		return LoadAvg{}, err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 4 {
		return LoadAvg{}, fmt.Errorf("loadavg has %d fields, want >= 4", len(fields))
	}
	var l LoadAvg
	for i, dst := range []*float64{&l.One, &l.Five, &l.Fifteen} {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
			return LoadAvg{}, fmt.Errorf("loadavg field %d: %q", i, fields[i])
		}
		*dst = v
	}
	running, total, ok := strings.Cut(fields[3], "/")
	if !ok {
		return LoadAvg{}, fmt.Errorf("loadavg tasks %q", fields[3])
	}
	if l.Running, err = strconv.Atoi(running); err != nil {
		return LoadAvg{}, fmt.Errorf("loadavg running: %w", err)
	}
	if l.Threads, err = strconv.Atoi(total); err != nil {
		return LoadAvg{}, fmt.Errorf("loadavg threads: %w", err)
	}
	return l, nil
}

// ParseZramMemUsed는 /sys/block/zramN/mm_stat의 셋째 값(mem_used_total, 압축한 데이터와 관리 정보가 쓰는 메모리, 바이트)을 읽는다.
func ParseZramMemUsed(r io.Reader) (uint64, error) {
	b, err := io.ReadAll(io.LimitReader(r, 512))
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 3 {
		return 0, fmt.Errorf("mm_stat has %d fields, want >= 3", len(fields))
	}
	return strconv.ParseUint(fields[2], 10, 64)
}

// pfKthread는 /proc/<pid>/stat flags의 커널 스레드 표시(PF_KTHREAD)다.
const pfKthread = 0x00200000

// PIDStat은 /proc/<pid>/stat에서 쓰는 값이다. 시간 값의 단위는 clock tick이다.
type PIDStat struct {
	Comm  string
	State string
	Flags uint64
	// CPUTicks는 utime + stime이다(끝난 자식 프로세스의 시간은 넣지 않는다).
	CPUTicks   uint64
	Threads    int
	StartTicks uint64
}

// Kernel은 커널 스레드인지다. 좀비도 명령줄이 비어 있으므로 명령줄이 아니라 flags로 판단한다.
func (s PIDStat) Kernel() bool { return s.Flags&pfKthread != 0 }

// ParsePIDStat은 /proc/<pid>/stat 한 줄을 파싱한다.
// comm에는 공백과 괄호가 들어갈 수 있으므로 마지막 ')'에서 자른 뒤 나머지를 나눈다.
func ParsePIDStat(r io.Reader) (PIDStat, error) {
	b, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return PIDStat{}, err
	}
	line := string(b)
	open := strings.IndexByte(line, '(')
	end := strings.LastIndexByte(line, ')')
	if open < 0 || end < open {
		return PIDStat{}, errors.New("stat: comm not found")
	}
	// rest[0]이 proc(5)의 3번째 필드(state)다.
	rest := strings.Fields(line[end+1:])
	if len(rest) < 20 {
		return PIDStat{}, fmt.Errorf("stat has %d fields after comm, want >= 20", len(rest))
	}
	nums := make(map[int]uint64, 5)
	for _, field := range []int{9, 14, 15, 20, 22} { // flags, utime, stime, num_threads, starttime
		n, err := strconv.ParseUint(rest[field-3], 10, 64)
		if err != nil {
			return PIDStat{}, fmt.Errorf("stat field %d: %w", field, err)
		}
		nums[field] = n
	}
	return PIDStat{
		Comm:       line[open+1 : end],
		State:      rest[0],
		Flags:      nums[9],
		CPUTicks:   nums[14] + nums[15],
		Threads:    int(nums[20]),
		StartTicks: nums[22],
	}, nil
}

// PIDStatus는 /proc/<pid>/status에서 쓰는 값이다.
type PIDStatus struct {
	// UID는 실제 사용자 uid다(Uid 줄의 첫 값).
	UID string
	// RSSBytes는 VmRSS다. 커널 스레드에는 그 줄이 없어 0이다.
	RSSBytes uint64
}

// ParsePIDStatus는 /proc/<pid>/status에서 Uid와 VmRSS를 파싱한다.
func ParsePIDStatus(r io.Reader) (PIDStatus, error) {
	var st PIDStatus
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		switch key {
		case "Uid":
			if len(fields) == 0 {
				return PIDStatus{}, errors.New("Uid: missing value")
			}
			if _, err := strconv.ParseUint(fields[0], 10, 32); err != nil {
				return PIDStatus{}, fmt.Errorf("Uid: %w", err)
			}
			st.UID = fields[0]
		case "VmRSS":
			if len(fields) == 0 {
				return PIDStatus{}, errors.New("VmRSS: missing value")
			}
			n, err := strconv.ParseUint(fields[0], 10, 64)
			if err != nil {
				return PIDStatus{}, fmt.Errorf("VmRSS: %w", err)
			}
			if len(fields) > 1 && fields[1] == "kB" {
				n *= 1024
			}
			st.RSSBytes = n
		}
	}
	if err := sc.Err(); err != nil {
		return PIDStatus{}, err
	}
	if st.UID == "" {
		return PIDStatus{}, errors.New("Uid not found")
	}
	return st, nil
}

// MaxCommandBytes는 명령줄을 자르는 길이다. 응답 크기를 제한한다(긴 Java 명령줄 등).
const MaxCommandBytes = 512

// ParseCmdline은 /proc/<pid>/cmdline(NUL로 나눈 인자)을 공백으로 이은 문자열로 만든다.
// MaxCommandBytes보다 길면 UTF-8 글자 경계에서 자르고 "…"를 붙인다.
func ParseCmdline(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxCommandBytes+1))
	if err != nil {
		return "", err
	}
	cut := len(b) > MaxCommandBytes
	if cut {
		end := MaxCommandBytes
		// 글자 중간에서 자르지 않게 글자 시작 바이트까지 물러난다(UTF-8 글자는 4바이트 이하).
		for i := end; i > end-utf8.UTFMax && i > 0; i-- {
			if utf8.RuneStart(b[i]) {
				end = i
				break
			}
		}
		b = b[:end]
	}
	s := strings.TrimRight(strings.ReplaceAll(string(b), "\x00", " "), " ")
	if cut {
		s += "…"
	}
	return s, nil
}

// DiskStat은 /proc/diskstats 한 줄이다. 섹터는 장치와 상관없이 512바이트 단위다.
type DiskStat struct {
	Name         string
	ReadSectors  uint64
	WriteSectors uint64
}

// SectorBytes는 /proc/diskstats의 섹터 크기다.
const SectorBytes = 512

// ParseDiskStats는 /proc/diskstats를 파싱한다. 필드가 모자란 줄은 건너뛴다.
func ParseDiskStats(r io.Reader) ([]DiskStat, error) {
	var out []DiskStat
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		// major minor name reads merged sectors_read ms writes merged sectors_written ...
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		rd, err := strconv.ParseUint(fields[5], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s sectors read: %w", fields[2], err)
		}
		wr, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s sectors written: %w", fields[2], err)
		}
		out = append(out, DiskStat{Name: fields[2], ReadSectors: rd, WriteSectors: wr})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
