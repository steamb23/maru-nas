package sysinfo

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"onmaru/backend/internal/api"
)

const (
	// 직전 CPU 샘플이 이보다 오래됐으면 긴 구간의 평균 대신 새로 두 번 샘플링한다.
	maxSampleAge = 10 * time.Second
	// 의미 있는 CPU 차이를 얻기 위한 최소 측정 간격.
	minSampleGap = 250 * time.Millisecond
)

// NetSource는 네트워크 인터페이스 카운터를 준다. *network.Reader가 구현한다.
type NetSource interface {
	Network() (api.NetworkResponse, error)
}

// Reader는 /proc 형태의 파일 시스템에서 시스템 정보를 읽는 provider다(이름 proc).
type Reader struct {
	proc  fs.FS
	block fs.FS
	net   NetSource
	sleep func(time.Duration)
	now   func() time.Time
	// start는 sampledAtMs의 기준 시각이다.
	start time.Time

	mu       sync.Mutex
	prev     CPUTimes
	prevTime time.Time
}

// NewReader는 호스트의 /proc, /sys/block과 net을 읽는 Reader를 반환한다.
func NewReader(net NetSource) *Reader {
	return NewReaderFS(os.DirFS("/proc"), os.DirFS("/sys/block"), net)
}

// NewReaderFS는 proc와 block(/sys/block 형태)을 읽는 Reader를 반환한다.
// proc에는 "stat", "meminfo", "uptime", "diskstats", "loadavg" 파일이 있어야 한다. net이 nil이면 네트워크 합계가 없다.
func NewReaderFS(proc, block fs.FS, net NetSource) *Reader {
	return &Reader{proc: proc, block: block, net: net, sleep: time.Sleep, now: time.Now, start: time.Now()}
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

// Stats는 시스템 정보 창이 쓰는 누적 카운터를 반환한다. 사용률·속도는 프론트가 두 응답의 차이로 계산한다.
// 네트워크·디스크 합계, 부하 평균, 프로세스·스레드 수는 읽지 못하면 빼고 나머지로 응답한다.
func (r *Reader) Stats() (api.SystemStatsResponse, error) {
	cpu, err := readFile(r.proc, "stat", ParseCPUStat)
	if err != nil {
		return api.SystemStatsResponse{}, err
	}
	mem, err := readFile(r.proc, "meminfo", ParseMemInfo)
	if err != nil {
		return api.SystemStatsResponse{}, err
	}
	up, err := readFile(r.proc, "uptime", ParseUptime)
	if err != nil {
		return api.SystemStatsResponse{}, err
	}
	res := api.SystemStatsResponse{
		AddonSources: api.AddonSources{Sources: []string{r.Name()}},
		SampledAtMs:  r.now().Sub(r.start).Milliseconds(),
		CPU: api.CPUCounters{
			TotalTicks:  cpu.All.Total,
			IdleTicks:   cpu.All.Idle,
			KernelTicks: cpu.All.Kernel,
			Cores:       make([]api.CoreTicks, 0, len(cpu.Cores)),
		},
		Memory: api.MemoryInfo{
			TotalBytes:     mem.Total,
			UsedBytes:      mem.Used(),
			AvailableBytes: mem.Available,
		},
		MemoryDetail:  mem.Detail(r.zramMemUsed()),
		UptimeSeconds: up,
	}
	for _, c := range cpu.Cores {
		res.CPU.Cores = append(res.CPU.Cores, api.CoreTicks{ID: c.ID, TotalTicks: c.Total, IdleTicks: c.Idle, KernelTicks: c.Kernel})
	}
	if t, err := r.netTotals(); err == nil {
		res.Network = &t
	}
	if t, err := r.diskTotals(); err == nil {
		res.Disk = &t
	}
	if load, err := readFile(r.proc, "loadavg", ParseLoadavg); err == nil {
		res.Load = &api.LoadAverage{One: load.One, Five: load.Five, Fifteen: load.Fifteen, Running: load.Running}
		if n, err := r.processCount(); err == nil {
			res.Tasks = &api.TaskCounts{Processes: n, Threads: load.Threads}
		}
	}
	return res, nil
}

// processCount는 /proc의 PID 디렉터리 수다. 프로세스 목록을 읽지 않고 센다.
func (r *Reader) processCount() (int, error) {
	entries, err := fs.ReadDir(r.proc, ".")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if pid, err := strconv.Atoi(e.Name()); err == nil && pid > 0 && e.IsDir() {
			n++
		}
	}
	return n, nil
}

// zramMemUsed는 zram 장치(/sys/block/zramN)가 쓰는 메모리 합계다. 읽지 못한 장치는 건너뛴다.
func (r *Reader) zramMemUsed() uint64 {
	entries, err := fs.ReadDir(r.block, ".")
	if err != nil {
		return 0
	}
	var total uint64
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "zram") {
			continue
		}
		if n, err := readFile(r.block, e.Name()+"/mm_stat", ParseZramMemUsed); err == nil {
			total += n
		}
	}
	return total
}

// netTotals는 물리 인터페이스의 누적 바이트 합계다. 브리지·본딩 같은 가상 인터페이스는
// 아래 물리 인터페이스와 같은 트래픽을 다시 세므로 뺀다.
func (r *Reader) netTotals() (api.NetTotals, error) {
	if r.net == nil {
		return api.NetTotals{}, errors.New("no network source")
	}
	n, err := r.net.Network()
	if err != nil {
		return api.NetTotals{}, err
	}
	var t api.NetTotals
	for _, i := range n.Interfaces {
		if !i.Virtual {
			t.RxBytes += i.RxBytes
			t.TxBytes += i.TxBytes
		}
	}
	return t, nil
}

// diskTotals는 물리 디스크 전체 장치의 누적 바이트 합계다. /sys/block에 device 링크가 있는 장치만 센다.
// 파티션은 /sys/block에 없고, loop·md·dm·zram은 device 링크가 없어 빠진다(이중 계산 방지).
func (r *Reader) diskTotals() (api.DiskTotals, error) {
	disks, err := readFile(r.proc, "diskstats", ParseDiskStats)
	if err != nil {
		return api.DiskTotals{}, err
	}
	var t api.DiskTotals
	for _, d := range disks {
		// sysfs는 장치 이름의 '/'를 '!'로 쓴다(cciss/c0d0 → cciss!c0d0).
		name := strings.ReplaceAll(d.Name, "/", "!")
		if !fs.ValidPath(name) {
			continue
		}
		if _, err := fs.ReadLink(r.block, name+"/device"); err != nil {
			continue
		}
		t.ReadBytes += d.ReadSectors * SectorBytes
		t.WriteBytes += d.WriteSectors * SectorBytes
	}
	return t, nil
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
