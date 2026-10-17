package sysinfo

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
)

// pidStat는 /proc/<pid>/stat 한 줄을 만든다. 쓰지 않는 필드는 0이다.
func pidStat(pid, comm, state string, flags, utime, stime, threads, start uint64) string {
	f := make([]string, 50)
	for i := range f {
		f[i] = "0"
	}
	u := func(n uint64) string { return strconv.FormatUint(n, 10) }
	f[0], f[1], f[2] = pid, "("+comm+")", state
	f[8] = u(flags)
	f[13], f[14] = u(utime), u(stime)
	f[19] = u(threads)
	f[21] = u(start)
	return strings.Join(f, " ") + "\n"
}

// requestWith는 sources를 쓸 수 있는 사용자의 요청이다. withProcesses면 ?processes=1이다.
func requestWith(sources []string, withProcesses bool) addons.Request {
	req := addons.Request{Sources: sources, Query: url.Values{}}
	if withProcesses {
		req.Query.Set("processes", "1")
	}
	return req
}

func TestParsePIDStat(t *testing.T) {
	// comm에 공백과 괄호가 들어 있어도 마지막 ')'에서 자른다.
	got, err := ParsePIDStat(strings.NewReader(pidStat("42", "weird) (name", "S", 0x400040, 120, 30, 3, 9000)))
	if err != nil {
		t.Fatal(err)
	}
	want := PIDStat{Comm: "weird) (name", State: "S", Flags: 0x400040, CPUTicks: 150, Threads: 3, StartTicks: 9000}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got.Kernel() {
		t.Error("user process reported as kernel thread")
	}
	k, err := ParsePIDStat(strings.NewReader(pidStat("2", "kthreadd", "S", 0x208040, 0, 0, 1, 1)))
	if err != nil || !k.Kernel() {
		t.Errorf("kthreadd: %+v, %v", k, err)
	}
}

func TestParsePIDStatErrors(t *testing.T) {
	for name, in := range map[string]string{
		"no comm":   "42 S 1 2 3\n",
		"short":     "42 (x) S 1 2 3\n",
		"nonnumber": strings.Replace(pidStat("42", "x", "S", 0, 1, 2, 1, 5), " 1 2 ", " x 2 ", 1),
	} {
		if _, err := ParsePIDStat(strings.NewReader(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParsePIDStatus(t *testing.T) {
	in := "Name:\tdemo\nState:\tS (sleeping)\nUid:\t1001\t1001\t1001\t1001\nGid:\t100\t100\t100\t100\nVmRSS:\t   2048 kB\nThreads:\t1\n"
	got, err := ParsePIDStatus(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got != (PIDStatus{UID: "1001", RSSBytes: 2048 * 1024}) {
		t.Errorf("got %+v", got)
	}
	// 커널 스레드에는 VmRSS가 없다.
	got, err = ParsePIDStatus(strings.NewReader("Name:\tkworker/0:1\nUid:\t0\t0\t0\t0\n"))
	if err != nil || got != (PIDStatus{UID: "0"}) {
		t.Errorf("kernel thread: %+v, %v", got, err)
	}
	if _, err := ParsePIDStatus(strings.NewReader("Name:\tdemo\n")); err == nil {
		t.Error("expected error without Uid")
	}
}

func TestParseCmdline(t *testing.T) {
	got, err := ParseCmdline(strings.NewReader("/usr/bin/demo\x00--flag\x00value\x00"))
	if err != nil || got != "/usr/bin/demo --flag value" {
		t.Errorf("got %q, %v", got, err)
	}
	// 길면 글자 경계에서 자르고 말줄임표를 붙인다. 'ㅎ'는 UTF-8 3바이트다.
	long := strings.Repeat("a", MaxCommandBytes-1) + "ㅎㅎ"
	got, err = ParseCmdline(strings.NewReader(long))
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.Repeat("a", MaxCommandBytes-1)+"…" {
		t.Errorf("truncated to %q", got[len(got)-8:])
	}
	if got, _ := ParseCmdline(strings.NewReader("")); got != "" {
		t.Errorf("empty cmdline = %q", got)
	}
}

func processFS() fstest.MapFS {
	return fstest.MapFS{
		"1/stat":      {Data: []byte(pidStat("1", "systemd", "S", 0x400100, 500, 300, 1, 2))},
		"1/status":    {Data: []byte("Uid:\t0\t0\t0\t0\nVmRSS:\t 12000 kB\n")},
		"1/cmdline":   {Data: []byte("/sbin/init\x00")},
		"2/stat":      {Data: []byte(pidStat("2", "kthreadd", "S", 0x208040, 0, 0, 1, 2))},
		"2/status":    {Data: []byte("Uid:\t0\t0\t0\t0\n")},
		"2/cmdline":   {Data: []byte("")},
		"812/stat":    {Data: []byte(pidStat("812", "demo-daemon", "R", 0x400040, 9000, 1000, 12, 4000))},
		"812/status":  {Data: []byte("Uid:\t1001\t1001\t1001\t1001\nVmRSS:\t 64000 kB\n")},
		"812/cmdline": {Data: []byte("/opt/demo/bin/demo-daemon\x00--config\x00/etc/demo.conf\x00")},
		// 좀비: 명령줄이 비고 VmRSS가 없지만 커널 스레드는 아니다.
		"900/stat":    {Data: []byte(pidStat("900", "defunct-child", "Z", 0x40044, 3, 1, 1, 6000))},
		"900/status":  {Data: []byte("Uid:\t1002\t1002\t1002\t1002\n")},
		"900/cmdline": {Data: []byte("")},
		// 읽는 사이에 끝난 프로세스는 디렉터리만 남는다.
		"950/oom_score": {Data: []byte("0\n")},
		// 숫자가 아닌 항목은 프로세스가 아니다.
		"self/stat": {Data: []byte(pidStat("812", "demo-daemon", "R", 0, 0, 0, 1, 1))},
		"stat":      {Data: []byte("cpu 1 2 3 4\n")},
	}
}

func fakeLookup(uid string) string {
	switch uid {
	case "0":
		return "root"
	case "1001":
		return "demo"
	}
	return uid
}

func TestProcessesList(t *testing.T) {
	looked := map[string]int{}
	p := NewProcessesFS(processFS(), func(uid string) string {
		looked[uid]++
		return fakeLookup(uid)
	})
	got, err := p.List()
	if err != nil {
		t.Fatal(err)
	}
	want := []api.Process{
		{PID: 1, StartTicks: 2, Name: "systemd", Command: "/sbin/init", User: "root", State: "S", CPUTicks: 800, RSSBytes: 12000 * 1024, Threads: 1},
		{PID: 2, StartTicks: 2, Name: "kthreadd", User: "root", State: "S", Threads: 1, Kernel: true},
		{PID: 812, StartTicks: 4000, Name: "demo-daemon", Command: "/opt/demo/bin/demo-daemon --config /etc/demo.conf", User: "demo", State: "R", CPUTicks: 10000, RSSBytes: 64000 * 1024, Threads: 12},
		{PID: 900, StartTicks: 6000, Name: "defunct-child", User: "1002", State: "Z", CPUTicks: 4, Threads: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d processes: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("process %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// 같은 uid는 한 번만 찾는다.
	if looked["0"] != 1 || looked["1001"] != 1 {
		t.Errorf("lookups %v", looked)
	}
}

func TestProcessesListBroken(t *testing.T) {
	// 모든 프로세스를 읽지 못하면 오류다.
	p := NewProcessesFS(fstest.MapFS{
		"1/stat":   {Data: []byte("garbage\n")},
		"1/status": {Data: []byte("Uid:\t0\t0\t0\t0\n")},
	}, fakeLookup)
	if _, err := p.List(); err == nil {
		t.Error("expected error")
	}
	// 사라진 프로세스만 있으면 빈 목록이다.
	p = NewProcessesFS(fstest.MapFS{"5/oom_score": {Data: []byte("0\n")}}, fakeLookup)
	if got, err := p.List(); err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestAddonFetchProcessesOnlyForAdmin(t *testing.T) {
	proc, block := statsFS()
	a := New(NewReaderFS(proc, block, nil), NewProcessesFS(processFS(), fakeLookup))
	admin := []string{"proc", SourceProcesses}
	for _, c := range []struct {
		name    string
		sources []string
		query   bool
		want    int
		used    []string
	}{
		// 일반 사용자는 요청해도 받지 못한다.
		{"user", []string{"proc"}, true, 0, []string{"proc"}},
		// 관리자도 processes=1일 때만 목록을 읽는다(성능 페이지는 목록이 필요 없다).
		{"admin without query", admin, false, 0, []string{"proc"}},
		{"admin", admin, true, 4, admin},
	} {
		res, err := a.Fetch(t.Context(), requestWith(c.sources, c.query))
		if err != nil {
			t.Fatal(err)
		}
		got := res.(api.SystemStatsResponse)
		if len(got.Processes) != c.want || !slices.Equal(got.Sources, c.used) {
			t.Errorf("%s: %d processes, sources %v", c.name, len(got.Processes), got.Sources)
		}
	}

	// 목록을 읽지 못하면 요약만 주고 실패한 출처로 알린다.
	broken := NewProcessesFS(fstest.MapFS{"1/stat": {Data: []byte("garbage")}, "1/status": {Data: []byte("Uid:\t0\n")}}, fakeLookup)
	a = New(NewReaderFS(proc, block, nil), broken)
	res, err := a.Fetch(t.Context(), requestWith(admin, true))
	if err != nil {
		t.Fatal(err)
	}
	got := res.(api.SystemStatsResponse)
	if got.Processes != nil || len(got.FailedSources) != 1 || got.FailedSources[0] != SourceProcesses {
		t.Errorf("processes %v, failed %v", got.Processes, got.FailedSources)
	}
}
