package sysinfo

import (
	"math"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
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
	// 앞 8개 필드의 합, idle + iowait.
	want := CPUTimes{
		Idle:  46828483 + 16683,
		Total: 10132153 + 290696 + 3084719 + 46828483 + 16683 + 0 + 25195 + 0,
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
