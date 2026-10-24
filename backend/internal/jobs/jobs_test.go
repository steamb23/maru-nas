package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
	"maru/backend/internal/notify"
)

// step은 가짜 Runner의 Watch가 할 일 하나다. outcome이 있으면 그 결과로 끝난다.
type step struct {
	out     string
	pos     int
	waiting *bool
	outcome *Outcome
}

// fakeRunner는 정해진 순서로 출력을 내는 Runner다. steps가 끝나면 ctx가 끝날 때까지 기다린다.
type fakeRunner struct {
	name     string
	startErr error
	// gate가 있으면 Start가 그 채널이 닫힐 때까지 기다린다.
	gate    chan struct{}
	steps   chan step
	mu      sync.Mutex
	watched []WatchJob
	starts  int
}

func newFakeRunner() *fakeRunner { return &fakeRunner{name: "fake", steps: make(chan step, 16)} }

func (r *fakeRunner) Name() string { return r.name }

func (r *fakeRunner) Start(ctx context.Context, kind Kind, req StartRequest) (json.RawMessage, error) {
	r.mu.Lock()
	r.starts++
	gate, err := r.gate, r.startErr
	r.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(`{"pos":0}`), nil
}

func (r *fakeRunner) Watch(ctx context.Context, job WatchJob, sink Sink) Outcome {
	r.mu.Lock()
	r.watched = append(r.watched, job)
	r.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return Outcome{}
		case s := <-r.steps:
			if s.waiting != nil {
				sink.Waiting(*s.waiting)
			}
			if s.out != "" || s.pos > 0 {
				h, _ := json.Marshal(map[string]int{"pos": s.pos})
				sink.Output([]byte(s.out), h)
			}
			if s.outcome != nil {
				return *s.outcome
			}
		}
	}
}

type fakeIndex struct {
	mu   sync.Mutex
	pkgs []aptindex.Package
}

func (f *fakeIndex) Index() (aptindex.Index, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return aptindex.Index{Packages: append([]aptindex.Package(nil), f.pkgs...)}, nil
}

func (f *fakeIndex) set(pkgs ...aptindex.Package) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pkgs = pkgs
}

type env struct {
	m        *Manager
	r        *fakeRunner
	store    *MemoryStore
	index    *fakeIndex
	finished chan struct{}
	boot     string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{r: newFakeRunner(), store: NewMemoryStore(), index: &fakeIndex{}, finished: make(chan struct{}, 8), boot: "boot-a"}
	e.m = New(Config{
		Store:    e.store,
		Index:    e.index,
		BootID:   func() string { return e.boot },
		OnFinish: func() { e.finished <- struct{}{} },
	})
	e.m.Register(e.r)
	t.Cleanup(e.m.Close)
	return e
}

func (e *env) waitFinish(t *testing.T) {
	t.Helper()
	select {
	case <-e.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("job did not finish")
	}
}

func outcome(state, code string) *Outcome { return &Outcome{State: state, Code: code} }

func pkg(name, version string) aptindex.Package {
	return aptindex.Package{Name: name, Version: version}
}

func TestStartBusyAndRelease(t *testing.T) {
	e := newEnv(t)
	e.r.gate = make(chan struct{})
	e.r.startErr = ErrLocked
	done := make(chan error)
	go func() {
		_, err := e.m.Start(context.Background(), KindAptUpdate, StartRequest{Owner: "admin"})
		done <- err
	}()
	// 첫 시작이 OMV를 부르는 동안 다른 시작은 겹침이다.
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.r.mu.Lock()
		started := e.r.starts
		e.r.mu.Unlock()
		if started == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := e.m.Start(context.Background(), KindAptUpgrade, StartRequest{Owner: "bob"}); !errors.Is(err, ErrBusy) {
		t.Fatalf("second start err = %v, want ErrBusy", err)
	}
	close(e.r.gate)
	if err := <-done; !errors.Is(err, ErrLocked) {
		t.Fatalf("first start err = %v", err)
	}
	// 거절되면 슬롯을 푼다.
	e.r.mu.Lock()
	e.r.gate, e.r.startErr = nil, nil
	e.r.mu.Unlock()
	if _, err := e.m.Start(context.Background(), KindAptUpdate, StartRequest{Owner: "admin"}); err != nil {
		t.Fatalf("start after refusal: %v", err)
	}
	if cur, _ := e.store.LoadCurrent(); cur == nil || cur.Owner != "admin" || cur.BootID != "boot-a" {
		t.Errorf("current = %+v", cur)
	}
	if _, err := e.m.Start(context.Background(), KindAptUpdate, StartRequest{Owner: "admin"}); !errors.Is(err, ErrBusy) {
		t.Errorf("start while running err = %v", err)
	}
	if _, err := e.m.Start(context.Background(), "bogus", StartRequest{}); !errors.Is(err, ErrUnknownKind) {
		t.Errorf("unknown kind err = %v", err)
	}
}

func TestRunToSuccess(t *testing.T) {
	e := newEnv(t)
	e.index.set(pkg("libc6", "2"), pkg("openmediavault", "8.5.12-1"))
	id, err := e.m.Start(context.Background(), KindAptUpgrade, StartRequest{Owner: "admin", SessionKey: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(id) {
		t.Fatalf("id %q", id)
	}
	waiting := true
	e.r.steps <- step{out: "Reading package lists...\n", pos: 25}
	e.r.steps <- step{waiting: &waiting}
	waitFor(t, func() bool { j := e.m.Tasks("bob"); return len(j) == 1 && j[0].Waiting && !j[0].Mine })
	if j := e.m.Tasks("bob")[0]; j.LastLine != "Reading package lists..." {
		t.Errorf("last line = %q", j.LastLine)
	}

	// 진행 중 출력은 어느 관리자나 읽는다.
	out, err := e.m.Output(id, "bob", 0)
	if err != nil || out.Text != "Reading package lists...\n" || out.State != api.JobRunning || !out.Waiting || out.More {
		t.Fatalf("running output = %+v, %v", out, err)
	}
	waiting = false
	e.index.set(pkg("libc6", "2")) // openmediavault가 설치되어 목록에서 빠졌다
	e.r.steps <- step{waiting: &waiting, out: "Setting up openmediavault (8.5.12-1) ...\n", pos: 66, outcome: outcome(api.JobSucceeded, "")}
	e.waitFinish(t)

	if cur, _ := e.store.LoadCurrent(); cur != nil {
		t.Errorf("current not cleared: %+v", cur)
	}
	if j := e.m.Tasks("admin"); len(j) != 0 {
		t.Errorf("tasks after finish = %v", j)
	}
	jobs := e.m.Jobs("admin")
	if jobs.Current != nil || len(jobs.Recent) != 1 {
		t.Fatalf("jobs = %+v", jobs)
	}
	r := jobs.Recent[0]
	if r.ID != id || r.State != api.JobSucceeded || !r.Mine || r.Count == nil || *r.Count != 1 || r.Items[0] != "openmediavault" || r.EndedAt == "" {
		t.Errorf("result = %+v", r)
	}
	// 마지막으로 끝난 작업은 시작하지 않은 관리자의 목록에도 last로 온다.
	if j := e.m.Jobs("bob"); j.Last == nil || j.Last.ID != id || j.Last.Mine || len(j.Recent) != 0 {
		t.Errorf("bob jobs = %+v", j)
	}
	// 끝난 직후에는 시작하지 않은 관리자도 출력을 읽는다.
	out, err = e.m.Output(id, "bob", 25)
	if err != nil || out.Text != "Setting up openmediavault (8.5.12-1) ...\n" || out.State != api.JobSucceeded {
		t.Errorf("finished output = %+v, %v", out, err)
	}

	notices := e.m.NoticesFor("admin")
	if len(notices) != 1 {
		t.Fatalf("notices = %v", notices)
	}
	n := notices[0]
	if n.ID != NoticeID(id) || n.Code != api.NoticeJobSucceeded || n.Level != api.NoticeSuccess || !n.Dismissible ||
		n.Job == nil || n.Job.Kind != api.JobAptUpgrade || n.Action == nil || n.Action.App != "packages" || notify.Check(n) != "" {
		t.Errorf("notice = %+v", n)
	}
	if got := e.m.NoticesFor("bob"); len(got) != 0 {
		t.Errorf("other user's notices = %v", got)
	}
	if err := e.m.Dismiss("bob", n.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("dismiss by other user err = %v", err)
	}
	if err := e.m.Dismiss("admin", n.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.m.Dismiss("admin", n.ID); err != nil {
		t.Errorf("second dismiss err = %v", err)
	}
	if got := e.m.NoticesFor("admin"); len(got) != 0 {
		t.Errorf("notices after dismiss = %v", got)
	}
	if got := e.m.Jobs("admin").Recent; len(got) != 1 || !got[0].Dismissed {
		t.Errorf("recent after dismiss = %+v", got)
	}
	if err := e.m.Dismiss("admin", "apt-updates"); !errors.Is(err, ErrNotFound) {
		t.Errorf("dismiss of a non-job notice err = %v", err)
	}
	if _, err := e.m.Output("20260101-000000-00000000", "admin", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown output err = %v", err)
	}
}

func TestFailureNotice(t *testing.T) {
	e := newEnv(t)
	id, _ := e.m.Start(context.Background(), KindAptUpgrade, StartRequest{Owner: "admin"})
	e.r.steps <- step{out: "E: Sub-process /usr/bin/dpkg returned an error code (1)\n", pos: 10, outcome: outcome(api.JobFailed, api.JobCodeAptFailed)}
	e.waitFinish(t)
	n := e.m.NoticesFor("admin")
	if len(n) != 1 || n[0].Code != api.NoticeJobFailed || n[0].Level != api.NoticeError || n[0].Job.Code != api.JobCodeAptFailed || n[0].Count != nil {
		t.Errorf("notices = %+v", n)
	}
	if j, err := e.m.Get(id, "admin"); err != nil || j.State != api.JobFailed {
		t.Errorf("get = %+v, %v", j, err)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestResume(t *testing.T) {
	store := NewMemoryStore()
	rec := &Record{ID: "20261024-101500-1a2b3c4d", Kind: KindAptUpgrade, Owner: "admin", Runner: "fake",
		Handle: json.RawMessage(`{"pos":3}`), BootID: "boot-a", StartedAt: time.Now().Add(-time.Minute),
		Output: OutputRange{End: 3}}
	store.SaveCurrent(rec)
	// 기록보다 뒤에 붙은 출력(기록 전에 멈춤)은 다시 받으므로 잘린다.
	store.AppendOutput(rec.ID, []byte("abcXYZ"))

	r := newFakeRunner()
	finished := make(chan struct{}, 1)
	m := New(Config{Store: store, BootID: func() string { return "boot-a" }, OnFinish: func() { finished <- struct{}{} }})
	m.Register(r)
	defer m.Close()
	m.Resume()
	m.Resume() // 두 번째는 아무것도 하지 않는다
	waitFor(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return len(r.watched) == 1 })
	if r.watched[0].Prefer != "" || string(r.watched[0].Handle) != `{"pos":3}` {
		t.Errorf("watched = %+v", r.watched[0])
	}
	// 다시 시작한 직후에도 저장한 출력의 마지막 줄이 보인다.
	if j := m.Tasks("admin"); len(j) != 1 || j[0].LastLine != "abc" {
		t.Errorf("tasks after resume = %+v", j)
	}
	r.steps <- step{out: "def", pos: 6, outcome: outcome(api.JobSucceeded, "")}
	<-finished
	out, _ := m.Output(rec.ID, "admin", 0)
	if out.Text != "abcdef" {
		t.Errorf("output after resume = %q", out.Text)
	}
}

func TestResumeEnds(t *testing.T) {
	cases := map[string]struct {
		boot, runner string
		code         string
	}{
		"reboot":         {"boot-b", "fake", api.JobCodeReboot},
		"unknown runner": {"boot-a", "service", api.JobCodeLost},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store := NewMemoryStore()
			store.SaveCurrent(&Record{ID: "20261024-101500-1a2b3c4d", Kind: KindAptUpdate, Owner: "admin", Runner: c.runner,
				Handle: json.RawMessage(`{}`), BootID: "boot-a", StartedAt: time.Now()})
			r := newFakeRunner()
			m := New(Config{Store: store, BootID: func() string { return c.boot }})
			m.Register(r)
			defer m.Close()
			m.Resume()
			rs, _ := store.LoadResults("admin")
			if len(rs) != 1 || rs[0].State != api.JobUnknown || rs[0].Code != c.code {
				t.Errorf("results = %+v", rs)
			}
			if cur, _ := store.LoadCurrent(); cur != nil {
				t.Errorf("current = %+v", cur)
			}
			if len(r.watched) != 0 {
				t.Error("runner watched an ended job")
			}
			if n := m.NoticesFor("admin"); len(n) != 1 || n[0].Code != api.NoticeJobUnknown || n[0].Level != api.NoticeWarning {
				t.Errorf("notices = %+v", n)
			}
		})
	}
}

func TestResumeAfterResultSaved(t *testing.T) {
	store := NewMemoryStore()
	id := "20261024-101500-1a2b3c4d"
	store.SaveCurrent(&Record{ID: id, Kind: KindAptUpdate, Owner: "admin", Runner: "fake", Handle: json.RawMessage(`{}`)})
	store.SaveResults("admin", []Result{{ID: id, Kind: KindAptUpdate, Owner: "admin", State: api.JobSucceeded, EndedAt: time.Now()}})
	r := newFakeRunner()
	m := New(Config{Store: store, BootID: func() string { return "" }})
	m.Register(r)
	defer m.Close()
	m.Resume()
	if cur, _ := store.LoadCurrent(); cur != nil || len(r.watched) != 0 {
		t.Errorf("current = %+v, watched = %v", cur, r.watched)
	}
	if rs, _ := store.LoadResults("admin"); len(rs) != 1 {
		t.Errorf("results = %+v", rs)
	}
}

func TestPruneResults(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	var rs []Result
	for i := range 12 {
		rs = append(rs, Result{ID: string(rune('a' + i)), EndedAt: now.Add(-time.Duration(i) * time.Hour)})
	}
	rs[3].EndedAt = now.Add(-31 * 24 * time.Hour)
	kept, dropped := pruneResults(rs, now)
	if len(kept) != 9 || len(dropped) != 3 {
		t.Fatalf("kept %d dropped %d", len(kept), len(dropped))
	}
	for _, r := range kept {
		if r.ID == "d" {
			t.Error("expired result kept")
		}
	}
}

func TestResultsLimitDeletesOutput(t *testing.T) {
	e := newEnv(t)
	var first string
	for i := range maxResults + 1 {
		id, err := e.m.Start(context.Background(), KindAptUpdate, StartRequest{Owner: "admin"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = id
		}
		e.r.steps <- step{out: "x\n", pos: 2, outcome: outcome(api.JobSucceeded, "")}
		e.waitFinish(t)
	}
	if rs := e.m.Jobs("admin").Recent; len(rs) != maxResults {
		t.Errorf("results = %d", len(rs))
	}
	if b, _ := e.store.ReadOutput(first, 0, 10); len(b) != 0 {
		t.Error("output of a pruned result kept")
	}
	if n := e.m.NoticesFor("admin"); len(n) != maxNotices {
		t.Errorf("notices = %d, want %d", len(n), maxNotices)
	}
}

func TestOutputCap(t *testing.T) {
	e := newEnv(t)
	id, _ := e.m.Start(context.Background(), KindAptUpgrade, StartRequest{Owner: "admin"})
	line := strings.Repeat("x", 1023) + "\n"
	big := strings.Repeat(line, 1024) // 1MiB
	for i := range 5 {
		e.r.steps <- step{out: big, pos: (i + 1) << 20}
	}
	waitFor(t, func() bool { j := e.m.Tasks("admin"); return len(j) == 1 && j[0].OutputEnd == 5<<20 })
	j := e.m.Tasks("admin")[0]
	if !j.OutputIncomplete {
		t.Error("truncated output not marked")
	}
	out, err := e.m.Output(id, "admin", 0)
	if err != nil {
		t.Fatal(err)
	}
	// 앞을 버렸으므로 처음부터 요청하면 남은 출력의 시작부터 온다. 한 번에 64KiB까지다.
	if out.From < 5<<20-outputKeep-1024 || out.From > 5<<20-outputKeep+1024 || len(out.Text) != maxReadBytes || !out.More {
		t.Errorf("output from=%d len=%d more=%v", out.From, len(out.Text), out.More)
	}
	if out.From%1024 != 0 {
		t.Errorf("output not cut on a line boundary: from=%d", out.From)
	}
}

func TestLastLine(t *testing.T) {
	for in, want := range map[string]string{
		"a\nb\n":     "b",
		"a\nb  \n\n": "b",
		"only":       "only",
		"\n\n":       "",
	} {
		if got := lastLine([]byte(in)); got != want {
			t.Errorf("lastLine(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("가", 100)
	if got := lastLine([]byte(long)); len(got) > maxLastLine || !strings.HasPrefix(long, got) {
		t.Errorf("long line = %d bytes", len(got))
	}
}

func TestCutPartialRune(t *testing.T) {
	s := []byte("가나")
	for cut := 1; cut <= len(s); cut++ {
		got := string(cutPartialRune(s[:cut]))
		if !strings.HasPrefix("가나", got) || (cut%3 == 0 && len(got) != cut) || (cut%3 != 0 && len(got) != cut/3*3) {
			t.Errorf("cut %d = %q", cut, got)
		}
	}
	if got := cutPartialRune([]byte("ab")); string(got) != "ab" {
		t.Errorf("ascii = %q", got)
	}
}

func TestCloseCheckpoints(t *testing.T) {
	store := NewMemoryStore()
	r := newFakeRunner()
	m := New(Config{Store: store, BootID: func() string { return "" }})
	m.Register(r)
	m.Start(context.Background(), KindAptUpgrade, StartRequest{Owner: "admin"})
	r.steps <- step{out: "abc", pos: 3}
	waitFor(t, func() bool { j := m.Tasks("admin"); return len(j) == 1 && j[0].OutputEnd == 3 })
	m.Close()
	cur, _ := store.LoadCurrent()
	if cur == nil || string(cur.Handle) != `{"pos":3}` || cur.Output.End != 3 {
		t.Errorf("current after close = %+v", cur)
	}
	if _, err := m.Start(context.Background(), KindAptUpdate, StartRequest{}); !errors.Is(err, ErrNoRunner) {
		t.Errorf("start after close err = %v", err)
	}
}
