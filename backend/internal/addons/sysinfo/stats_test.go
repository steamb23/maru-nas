package sysinfo

import (
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"onmaru/backend/internal/api"
)

func TestParseCPUStat(t *testing.T) {
	f, err := os.Open("testdata/stat")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseCPUStat(f)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ParseCPUTimes(strings.NewReader("cpu  10132153 290696 3084719 46828483 16683 0 25195 0 175628 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got.All != all {
		t.Errorf("all = %+v, want %+v", got.All, all)
	}
	want := []CoreTimes{
		{ID: 0, CPUTimes: CPUTimes{Idle: 13343292 + 6130, Total: 1393280 + 32966 + 572056 + 13343292 + 6130 + 0 + 17875 + 0}},
		{ID: 1, CPUTimes: CPUTimes{Idle: 13404148 + 3683, Total: 1335808 + 31652 + 427683 + 13404148 + 3683 + 0 + 1849 + 0}},
	}
	if len(got.Cores) != len(want) || got.Cores[0] != want[0] || got.Cores[1] != want[1] {
		t.Errorf("cores = %+v, want %+v", got.Cores, want)
	}
}

func TestParseCPUStatOfflineCore(t *testing.T) {
	// 꺼진 코어(cpu1)는 줄이 없으므로 번호가 건너뛴다.
	got, err := ParseCPUStat(strings.NewReader("cpu 4 0 4 8 0\ncpu0 2 0 2 4 0\ncpu2 2 0 2 4 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cores) != 2 || got.Cores[0].ID != 0 || got.Cores[1].ID != 2 {
		t.Errorf("cores = %+v", got.Cores)
	}
}

func TestParseCPUStatErrors(t *testing.T) {
	for name, in := range map[string]string{
		"missing total": "cpu0 1 2 3 4\n",
		"bad core name": "cpu 1 2 3 4\ncpuX 1 2 3 4\n",
		"short core":    "cpu 1 2 3 4\ncpu0 1 2\n",
	} {
		if _, err := ParseCPUStat(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseDiskStats(t *testing.T) {
	f, err := os.Open("testdata/diskstats")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseDiskStats(f)
	if err != nil {
		t.Fatal(err)
	}
	// 필드가 모자란 ram0 줄은 건너뛴다.
	if len(got) != 9 {
		t.Fatalf("got %d disks: %+v", len(got), got)
	}
	if got[1] != (DiskStat{Name: "sda", ReadSectors: 800000, WriteSectors: 400000}) {
		t.Errorf("sda = %+v", got[1])
	}
	if got[8].Name != "cciss/c0d0" {
		t.Errorf("last = %+v", got[8])
	}
	if _, err := ParseDiskStats(strings.NewReader("8 0 sda 1 0 x 0 0 0 0 0\n")); err == nil {
		t.Error("expected error for non-number")
	}
}

type fakeNet struct {
	res api.NetworkResponse
	err error
}

func (f fakeNet) Network() (api.NetworkResponse, error) { return f.res, f.err }

func statsFS() (proc, block fstest.MapFS) {
	diskstats, err := os.ReadFile("testdata/diskstats")
	if err != nil {
		panic(err)
	}
	proc = fstest.MapFS{
		"stat":      {Data: []byte("cpu  100 0 100 800 0 0 0 0 0 0\ncpu0 50 0 50 400 0\ncpu1 50 0 50 400 0\n")},
		"meminfo":   {Data: []byte("MemTotal: 1000 kB\nMemAvailable: 250 kB\n")},
		"uptime":    {Data: []byte("42.5 10.0\n")},
		"diskstats": {Data: diskstats},
	}
	link := func(target string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(target), Mode: os.ModeSymlink}
	}
	block = fstest.MapFS{
		// 물리 디스크는 device 링크가 있다.
		"sda/device":        link("../../../0:0:0:0"),
		"sdb/device":        link("../../../1:0:0:0"),
		"nvme0n1/device":    link("../../nvme0"),
		"cciss!c0d0/device": link("../../../c0d0"),
		// 가상 장치는 /sys/block에 있지만 device 링크가 없다.
		"loop0/size": {Data: []byte("0\n")},
		"md0/size":   {Data: []byte("0\n")},
		"zram0/size": {Data: []byte("0\n")},
	}
	return proc, block
}

func TestReaderStats(t *testing.T) {
	proc, block := statsFS()
	net := fakeNet{res: api.NetworkResponse{Interfaces: []api.NetInterface{
		{Name: "eth0", RxBytes: 1000, TxBytes: 500},
		{Name: "eth1", RxBytes: 30, TxBytes: 20},
		// 브리지는 아래 물리 인터페이스의 트래픽을 다시 센다.
		{Name: "br0", Virtual: true, RxBytes: 1000, TxBytes: 500},
	}}}
	r := NewReaderFS(proc, block, net)
	start := time.Unix(100, 0)
	r.start = start
	r.now = func() time.Time { return start.Add(1500 * time.Millisecond) }

	got, err := r.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 1 || got.Sources[0] != "proc" || got.SampledAtMs != 1500 {
		t.Errorf("sources %v, sampledAtMs %d", got.Sources, got.SampledAtMs)
	}
	if got.CPU.TotalTicks != 1000 || got.CPU.IdleTicks != 800 || len(got.CPU.Cores) != 2 ||
		got.CPU.Cores[1] != (api.CoreTicks{ID: 1, TotalTicks: 500, IdleTicks: 400}) {
		t.Errorf("cpu = %+v", got.CPU)
	}
	if got.Memory.TotalBytes != 1000*1024 || got.Memory.UsedBytes != 750*1024 || got.UptimeSeconds != 42.5 {
		t.Errorf("memory %+v, uptime %v", got.Memory, got.UptimeSeconds)
	}
	if got.Network == nil || *got.Network != (api.NetTotals{RxBytes: 1030, TxBytes: 520}) {
		t.Errorf("network = %+v", got.Network)
	}
	// sda + sdb + nvme0n1 + cciss/c0d0. 파티션·loop·md·zram은 빠진다.
	wantRead := uint64(800000+200000+3000000+80) * SectorBytes
	wantWrite := uint64(400000+100000+2000000+160) * SectorBytes
	if got.Disk == nil || *got.Disk != (api.DiskTotals{ReadBytes: wantRead, WriteBytes: wantWrite}) {
		t.Errorf("disk = %+v, want read %d write %d", got.Disk, wantRead, wantWrite)
	}
	if got.Processes != nil {
		t.Errorf("reader must not list processes")
	}
}

func TestReaderStatsWithoutNetworkAndDisk(t *testing.T) {
	proc, block := statsFS()
	delete(proc, "diskstats")
	r := NewReaderFS(proc, block, fakeNet{err: errors.New("no /proc/net/dev")})
	got, err := r.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if got.Network != nil || got.Disk != nil {
		t.Errorf("network %+v, disk %+v; want both missing", got.Network, got.Disk)
	}
	if _, err := NewReaderFS(proc, block, nil).Stats(); err != nil {
		t.Errorf("nil network source: %v", err)
	}
}

func TestReaderStatsMissingCore(t *testing.T) {
	proc, block := statsFS()
	delete(proc, "meminfo")
	if _, err := NewReaderFS(proc, block, nil).Stats(); err == nil {
		t.Error("expected error without meminfo")
	}
}
