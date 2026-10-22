package sysinfo

import (
	"math"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/api"
)

func TestParseCPUTimes(t *testing.T) {
	f, err := os.Open("testdata/stat")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseCPUTimes(f)
	if err != nil {
		t.Fatal(err)
	}
	// 앞 8개 필드의 합, idle + iowait, system + irq + softirq.
	want := CPUTimes{
		Idle:   46828483 + 16683,
		Kernel: 3084719 + 0 + 25195,
		Total:  10132153 + 290696 + 3084719 + 46828483 + 16683 + 0 + 25195 + 0,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseCPUTimesErrors(t *testing.T) {
	for name, in := range map[string]string{
		"missing":   "intr 1 2 3\n",
		"short":     "cpu 1 2 3\n",
		"nonnumber": "cpu 1 x 3 4\n",
	} {
		if _, err := ParseCPUTimes(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestCPUUsagePercent(t *testing.T) {
	prev := CPUTimes{Idle: 100, Total: 200}
	cases := []struct {
		name string
		cur  CPUTimes
		want float64
	}{
		{"quarter busy", CPUTimes{Idle: 175, Total: 300}, 25},
		{"fully busy", CPUTimes{Idle: 100, Total: 300}, 100},
		{"idle", CPUTimes{Idle: 200, Total: 300}, 0},
		{"no change", prev, 0},
		{"counter went back", CPUTimes{Idle: 50, Total: 100}, 0},
	}
	for _, c := range cases {
		if got := CPUUsagePercent(prev, c.cur); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestParseMemInfo(t *testing.T) {
	f, err := os.Open("testdata/meminfo")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseMemInfo(f)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 16303428*1024 || got.Available != 8151714*1024 {
		t.Errorf("got %+v", got)
	}
	if got.Used() != (16303428-8151714)*1024 {
		t.Errorf("Used() = %d", got.Used())
	}
}

func TestParseCPUTimesShortLine(t *testing.T) {
	// irq·softirq가 없는 옛 형식이면 커널 시간은 system뿐이다.
	got, err := ParseCPUTimes(strings.NewReader("cpu 10 2 30 400\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != (CPUTimes{Idle: 400, Kernel: 30, Total: 442}) {
		t.Errorf("got %+v", got)
	}
}

func TestParseMemInfoDetailFields(t *testing.T) {
	f, err := os.Open("testdata/meminfo")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseMemInfo(f)
	if err != nil {
		t.Fatal(err)
	}
	want := MemInfo{
		Total: 16303428 * 1024, Available: 8151714 * 1024, Free: 1234567 * 1024,
		Buffers: 123456 * 1024, Cached: 5678901 * 1024, Shmem: 234567 * 1024,
		SReclaimable: 345678 * 1024, Zswap: 45678 * 1024,
		SwapTotal: 2097148 * 1024, SwapFree: 1048574 * 1024,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestMemInfoDetail(t *testing.T) {
	cases := []struct {
		name string
		m    MemInfo
		zram uint64
		want api.MemoryDetail
	}{
		{
			name: "typical",
			m: MemInfo{Total: 1000, Free: 100, Buffers: 20, Cached: 400, Shmem: 50, SReclaimable: 30, Zswap: 10,
				SwapTotal: 500, SwapFree: 400},
			zram: 5,
			want: api.MemoryDetail{FreeBytes: 100, BuffersBytes: 20, SharedBytes: 50, CacheBytes: 380,
				CompressedBytes: 15, AppsBytes: 435, SwapTotalBytes: 500, SwapUsedBytes: 100},
		},
		{
			name: "only required fields",
			m:    MemInfo{Total: 1000, Available: 600},
			want: api.MemoryDetail{AppsBytes: 1000},
		},
		{
			// Shmem이 Cached + SReclaimable보다 크면 캐시는 0이다.
			name: "shmem above cached",
			m:    MemInfo{Total: 1000, Free: 100, Cached: 40, Shmem: 60},
			want: api.MemoryDetail{FreeBytes: 100, SharedBytes: 60, AppsBytes: 840},
		},
		{
			// 값이 Total보다 많아도 합은 Total이고 앞 항목부터 채운다.
			name: "overflow",
			m:    MemInfo{Total: 100, Free: 70, Buffers: 20, Cached: 50, SwapTotal: 10, SwapFree: 20},
			want: api.MemoryDetail{FreeBytes: 70, BuffersBytes: 20, CacheBytes: 10, SwapTotalBytes: 10},
		},
	}
	for _, c := range cases {
		got := c.m.Detail(c.zram)
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
		sum := got.AppsBytes + got.SharedBytes + got.CompressedBytes + got.BuffersBytes + got.CacheBytes + got.FreeBytes
		if sum != c.m.Total {
			t.Errorf("%s: sum %d, want total %d", c.name, sum, c.m.Total)
		}
	}
}

func TestParseMemInfoMissingAvailable(t *testing.T) {
	if _, err := ParseMemInfo(strings.NewReader("MemTotal: 100 kB\n")); err == nil {
		t.Error("expected error")
	}
}

func TestParseUptime(t *testing.T) {
	got, err := ParseUptime(strings.NewReader("351234.56 2709876.12\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got != 351234.56 {
		t.Errorf("got %v", got)
	}
	if _, err := ParseUptime(strings.NewReader("")); err == nil {
		t.Error("expected error for empty input")
	}
}

func TestReaderSystem(t *testing.T) {
	stats := []string{
		"cpu  100 0 100 800 0 0 0 0 0 0\n",
		"cpu  200 0 200 1400 0 0 0 0 0 0\n", // 사용 +200, 유휴 +600 -> 25%
		"cpu  700 0 200 1400 0 0 0 0 0 0\n", // 사용 +500, 유휴 +0 -> 100%
	}
	fsys := fstest.MapFS{
		"meminfo": {Data: []byte("MemTotal: 1000 kB\nMemAvailable: 250 kB\n")},
		"uptime":  {Data: []byte("42.5 10.0\n")},
	}
	setStat := func(i int) { fsys["stat"] = &fstest.MapFile{Data: []byte(stats[i])} }

	clock := time.Unix(0, 0)
	statIdx := 0
	setStat(0)
	r := NewReaderFS(fsys, fstest.MapFS{}, nil)
	r.now = func() time.Time { return clock }
	r.sleep = func(d time.Duration) {
		clock = clock.Add(d)
		statIdx++
		setStat(statIdx)
	}

	// 첫 호출: 직전 샘플이 없으므로 두 번 샘플링한다.
	got, err := r.System()
	if err != nil {
		t.Fatal(err)
	}
	if got.CPU.UsagePercent != 25 {
		t.Errorf("first CPU = %v, want 25", got.CPU.UsagePercent)
	}
	if got.Memory.TotalBytes != 1000*1024 || got.Memory.UsedBytes != 750*1024 || got.Memory.AvailableBytes != 250*1024 {
		t.Errorf("memory = %+v", got.Memory)
	}
	if got.UptimeSeconds != 42.5 {
		t.Errorf("uptime = %v", got.UptimeSeconds)
	}

	// 2초 뒤 두 번째 호출: 저장된 샘플을 쓰고 대기하지 않는다.
	clock = clock.Add(2 * time.Second)
	setStat(2)
	r.sleep = func(time.Duration) { t.Error("unexpected sleep") }
	got, err = r.System()
	if err != nil {
		t.Fatal(err)
	}
	if got.CPU.UsagePercent != 100 {
		t.Errorf("second CPU = %v, want 100", got.CPU.UsagePercent)
	}
}
