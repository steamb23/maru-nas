package sysinfo

import (
	"io"
	"io/fs"
	"os"
	"sync"
	"time"

	"omv-desktop-dashboard/backend/internal/api"
)

const (
	// 직전 CPU 샘플이 이보다 오래됐으면 긴 구간의 평균 대신 새로 두 번 샘플링한다.
	maxSampleAge = 10 * time.Second
	// 의미 있는 CPU 차이를 얻기 위한 최소 측정 간격.
	minSampleGap = 250 * time.Millisecond
)

// Reader는 /proc 형태의 파일 시스템에서 시스템 정보를 읽는다.
type Reader struct {
	proc  fs.FS
	sleep func(time.Duration)
	now   func() time.Time

	mu       sync.Mutex
	prev     CPUTimes
	prevTime time.Time
}

// NewReader는 호스트의 /proc를 읽는 Reader를 반환한다.
func NewReader() *Reader {
	return NewReaderFS(os.DirFS("/proc"))
}

// NewReaderFS는 proc를 읽는 Reader를 반환한다.
// proc에는 "stat", "meminfo", "uptime" 파일이 있어야 한다.
func NewReaderFS(proc fs.FS) *Reader {
	return &Reader{proc: proc, sleep: time.Sleep, now: time.Now}
}

// System은 현재 CPU, 메모리, 가동 시간 값을 반환한다.
func (r *Reader) System() (api.SystemResponse, error) {
	cpu, err := r.cpuUsage()
	if err != nil {
		return api.SystemResponse{}, err
	}
	mem, err := readFile(r.proc, "meminfo", ParseMemInfo)
	if err != nil {
		return api.SystemResponse{}, err
	}
	up, err := readFile(r.proc, "uptime", ParseUptime)
	if err != nil {
		return api.SystemResponse{}, err
	}
	return api.SystemResponse{
		CPU: api.CPUInfo{UsagePercent: cpu},
		Memory: api.MemoryInfo{
			TotalBytes:     mem.Total,
			UsedBytes:      mem.Used(),
			AvailableBytes: mem.Available,
		},
		UptimeSeconds: up,
	}, nil
}

// cpuUsage는 직전 호출 이후의 사용률을 측정한다. 쓸 만한 직전 샘플이 없으면
// minSampleGap 간격으로 두 번 샘플링한다.
func (r *Reader) cpuUsage() (float64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cur, err := readFile(r.proc, "stat", ParseCPUTimes)
	if err != nil {
		return 0, err
	}
	prev := r.prev
	age := r.now().Sub(r.prevTime)
	if r.prevTime.IsZero() || age > maxSampleAge || age < minSampleGap {
		r.sleep(minSampleGap)
		prev = cur
		if cur, err = readFile(r.proc, "stat", ParseCPUTimes); err != nil {
			return 0, err
		}
	}
	r.prev, r.prevTime = cur, r.now()
	return CPUUsagePercent(prev, cur), nil
}

func readFile[T any](fsys fs.FS, name string, parse func(io.Reader) (T, error)) (T, error) {
	f, err := fsys.Open(name)
	if err != nil {
		var zero T
		return zero, err
	}
	defer f.Close()
	return parse(f)
}
