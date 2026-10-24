package omvjob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/jobs"
	"maru/backend/internal/omv"
	"maru/backend/internal/omv/omvfake"
)

const testAgent = "maru-test"

// caller는 OMV 세션 하나로 부르는 RPCCaller다(서버의 rpcCaller처럼 늘 updatelastaccess=false).
type caller struct {
	c   *omv.Client
	sid string
}

func (c caller) Call(ctx context.Context, service, method string, params any) (json.RawMessage, error) {
	if service == "Exec" && method == "stop" {
		return nil, addons.ErrRPCNotAllowed
	}
	res, err := c.c.Call(ctx, omv.Request{SessionID: c.sid, Service: service, Method: method, Params: params, NoTouch: true})
	return res.Result, err
}

// pool은 가짜 세션 묶음이다.
type pool struct {
	mu       sync.Mutex
	sessions []Session
	dead     []string
	prefers  []string
}

func (p *pool) Candidates(prefer, addon string) []Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prefers = append(p.prefers, prefer)
	out := slices.Clone(p.sessions)
	slices.SortStableFunc(out, func(a, b Session) int {
		switch {
		case a.Key == prefer:
			return -1
		case b.Key == prefer:
			return 1
		}
		return 0
	})
	return out
}

func (p *pool) Dead(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dead = append(p.dead, key)
	p.sessions = slices.DeleteFunc(p.sessions, func(s Session) bool { return s.Key == key })
}

func (p *pool) add(s Session) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sessions = append(p.sessions, s)
}

// sink는 출력을 모으는 jobs.Sink다. onOutput은 출력을 받을 때마다 부른다(테스트가 상황을 바꾼다).
type sink struct {
	mu       sync.Mutex
	out      bytes.Buffer
	handle   json.RawMessage
	waiting  bool
	waited   bool
	offers   chan struct{}
	onOutput func(n int)
	outputs  int
}

func newSink() *sink { return &sink{offers: make(chan struct{}, 1)} }

func (s *sink) Output(chunk []byte, h json.RawMessage) {
	s.mu.Lock()
	s.out.Write(chunk)
	s.handle = h
	s.outputs++
	n, f := s.outputs, s.onOutput
	s.mu.Unlock()
	if f != nil {
		f(n)
	}
}

func (s *sink) Waiting(w bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiting = w
	if w {
		s.waited = true
	}
}

func (s *sink) Offers() <-chan struct{} { return s.offers }

func (s *sink) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.String()
}

type env struct {
	fake   *omvfake.Server
	client *omv.Client
	pool   *pool
	r      *Runner
	// indexAt은 업데이트 목록 파일의 수정 시각이다.
	indexAt time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	fake := omvfake.New(omvfake.DefaultUsers(), 0)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	e := &env{fake: fake, client: omv.NewClient(srv.URL, testAgent), pool: &pool{}}
	e.r = New(e.pool, func() (time.Time, error) { return e.indexAt, nil })
	e.r.Interval = time.Millisecond
	e.r.Backoff = []time.Duration{time.Millisecond}
	e.r.WaitRetry = 20 * time.Millisecond
	e.r.FailConfirmFor = 0
	return e
}

func (e *env) login(t *testing.T, user string) Session {
	t.Helper()
	lr, err := e.client.Login(context.Background(), user, user, "")
	if err != nil {
		t.Fatal(err)
	}
	return Session{Key: "key-" + user + "-" + lr.SessionID[:6], User: user, RPC: caller{e.client, lr.SessionID}}
}

func (e *env) start(t *testing.T, kind jobs.Kind, s Session) jobs.WatchJob {
	t.Helper()
	h, err := e.r.Start(context.Background(), kind, jobs.StartRequest{Owner: s.User, SessionKey: s.Key, RPC: s.RPC})
	if err != nil {
		t.Fatal(err)
	}
	return jobs.WatchJob{ID: "20261024-101500-1a2b3c4d", Kind: kind, Handle: h, StartedAt: time.Now(), Prefer: s.Key}
}

func (e *env) watch(t *testing.T, job jobs.WatchJob, s *sink) jobs.Outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out := e.r.Watch(ctx, job, s)
	if ctx.Err() != nil {
		t.Fatal("watch did not end")
	}
	return out
}

func joined(chunks []string) string { return strings.Join(chunks, "") }

func TestStartAndWatchSuccess(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	e.pool.add(admin)
	job := e.start(t, jobs.KindAptUpdate, admin)
	var h handle
	if json.Unmarshal(job.Handle, &h) != nil || !strings.HasPrefix(h.Filename, "/tmp/bgstatus") || h.Pos != 0 {
		t.Fatalf("handle = %s", job.Handle)
	}
	s := newSink()
	out := e.watch(t, job, s)
	if out.State != api.JobSucceeded || out.Incomplete {
		t.Fatalf("outcome = %+v", out)
	}
	if want := joined(omvfake.DefaultAptConfig().Update.Chunks); s.text() != want {
		t.Errorf("output = %q, want %q", s.text(), want)
	}
	if e.fake.BgJobs() != 0 {
		t.Error("status file not cleaned up")
	}
	var last handle
	json.Unmarshal(s.handle, &last)
	if last.Pos != int64(len(s.text())) {
		t.Errorf("last handle pos = %d", last.Pos)
	}
	for _, c := range e.fake.Calls() {
		if c.Service == "Exec" && c.Touch {
			t.Errorf("%s.%s extended the session", c.Service, c.Method)
		}
		if c.Service == "Exec" && c.Method == "isRunning" {
			t.Error("Exec.isRunning would delete the status file")
		}
	}
	if e.pool.prefers[0] != admin.Key {
		t.Errorf("prefer = %v", e.pool.prefers)
	}
}

func TestStartRefusals(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	req := jobs.StartRequest{Owner: "admin", RPC: admin.RPC}

	cfg := omvfake.DefaultAptConfig()
	cfg.Locked = true
	e.fake.SetAptConfig(cfg)
	for _, kind := range []jobs.Kind{jobs.KindAptUpdate, jobs.KindAptUpgrade} {
		if _, err := e.r.Start(context.Background(), kind, req); !errors.Is(err, jobs.ErrLocked) {
			t.Errorf("%s locked err = %v", kind, err)
		}
	}

	e.fake.SetAptConfig(omvfake.DefaultAptConfig())
	d := omvfake.DefaultPluginData()
	d.Dirty = []string{"apt"}
	e.fake.SetPluginData(d)
	if _, err := e.r.Start(context.Background(), jobs.KindAptUpdate, req); !errors.Is(err, jobs.ErrConfigDirty) {
		t.Errorf("update dirty err = %v", err)
	}
	// 적용 대기는 update만 확인한다.
	if _, err := e.r.Start(context.Background(), jobs.KindAptUpgrade, req); err != nil {
		t.Errorf("upgrade with dirty apt: %v", err)
	}

	// 관리자가 아니면 OMV가 거절한다(잠금·적용 대기가 아닌 오류).
	user := e.login(t, "user")
	_, err := e.r.Start(context.Background(), jobs.KindAptUpgrade, jobs.StartRequest{Owner: "user", RPC: user.RPC})
	if err == nil || errors.Is(err, jobs.ErrLocked) || errors.Is(err, jobs.ErrConfigDirty) {
		t.Errorf("non-admin err = %v", err)
	}
}

func TestStartDoesNotRetry(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	e.fake.SetGlitch("Apt", "upgrade", 1, omvfake.GlitchEngine)
	if _, err := e.r.Start(context.Background(), jobs.KindAptUpgrade, jobs.StartRequest{RPC: admin.RPC}); err == nil {
		t.Fatal("start succeeded through an engine error")
	}
	n := 0
	for _, c := range e.fake.Calls() {
		if c.Service == "Apt" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Apt.upgrade called %d times", n)
	}
}

func TestWatchFailures(t *testing.T) {
	cases := map[string]struct {
		fail  string
		state string
		code  string
	}{
		"apt failed":   {omvfake.AptExecFailedMessage, api.JobFailed, api.JobCodeAptFailed},
		"core removal": {omvfake.AptCoreRemovalMessage, api.JobFailed, api.JobCodeAptCoreRemoval},
		"other":        {"Something unexpected happened.", api.JobFailed, api.JobCodeFailed},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			cfg := omvfake.DefaultAptConfig()
			cfg.Upgrade.Fail = c.fail
			e.fake.SetAptConfig(cfg)
			admin := e.login(t, "admin")
			e.pool.add(admin)
			s := newSink()
			out := e.watch(t, e.start(t, jobs.KindAptUpgrade, admin), s)
			if out.State != c.state || out.Code != c.code {
				t.Errorf("outcome = %+v", out)
			}
			// 실패 전까지의 출력은 남는다.
			if s.text() != joined(cfg.Upgrade.Chunks) {
				t.Errorf("output = %q", s.text())
			}
			// 실제 OMV처럼 실패한 작업의 상태 파일은 남는다.
			if e.fake.BgJobs() != 1 {
				t.Errorf("bg jobs = %d", e.fake.BgJobs())
			}
		})
	}
}

func TestWatchTransientErrors(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	e.pool.add(admin)
	job := e.start(t, jobs.KindAptUpgrade, admin)
	e.fake.SetGlitch("Exec", "getOutput", 3, omvfake.GlitchProxy)
	s := newSink()
	s.onOutput = func(n int) {
		if n == 2 {
			e.fake.SetGlitch("Exec", "getOutput", 4, omvfake.GlitchEngine)
		}
	}
	out := e.watch(t, job, s)
	if out.State != api.JobSucceeded {
		t.Errorf("outcome = %+v", out)
	}
	if s.text() != joined(omvfake.DefaultAptConfig().Upgrade.Chunks) {
		t.Errorf("output = %q", s.text())
	}
}

func TestWatchOutage(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	e.pool.add(admin)
	job := e.start(t, jobs.KindAptUpgrade, admin)
	e.fake.SetGlitch("Exec", "getOutput", 1000, omvfake.GlitchProxy)
	e.r.OutageLimit = 20 * time.Millisecond
	if out := e.watch(t, job, newSink()); out.State != api.JobUnknown || out.Code != api.JobCodeTimeout {
		t.Errorf("outcome = %+v", out)
	}
}

func TestWatchGone(t *testing.T) {
	for _, refreshed := range []bool{true, false} {
		e := newEnv(t)
		admin := e.login(t, "admin")
		e.pool.add(admin)
		job := e.start(t, jobs.KindAptUpgrade, admin)
		if refreshed {
			e.indexAt = job.StartedAt.Add(time.Second)
		} else {
			e.indexAt = job.StartedAt.Add(-time.Hour)
		}
		s := newSink()
		// 워크벤치의 작업 패널이 끝까지 읽어 상태 파일을 지운 상황이다.
		s.onOutput = func(n int) {
			if n == 2 {
				e.fake.DropBg()
			}
		}
		out := e.watch(t, job, s)
		want := jobs.Outcome{State: api.JobSucceeded, Incomplete: true}
		if !refreshed {
			want = jobs.Outcome{State: api.JobUnknown, Code: api.JobCodeLost, Incomplete: true}
		}
		if out != want {
			t.Errorf("refreshed=%v outcome = %+v, want %+v", refreshed, out, want)
		}
	}
}

func TestSessionHop(t *testing.T) {
	e := newEnv(t)
	first := e.login(t, "admin")
	second := e.login(t, "admin")
	e.pool.add(first)
	e.pool.add(second)
	job := e.start(t, jobs.KindAptUpgrade, first)
	s := newSink()
	s.onOutput = func(n int) {
		if n == 1 {
			// 시작한 세션이 로그아웃했다. OMV가 그 세션을 파기한다.
			e.client.Logout(context.Background(), first.RPC.(caller).sid)
		}
	}
	out := e.watch(t, job, s)
	if out.State != api.JobSucceeded {
		t.Fatalf("outcome = %+v", out)
	}
	if !slices.Equal(e.pool.dead, []string{first.Key}) {
		t.Errorf("dead = %v", e.pool.dead)
	}
	if s.text() != joined(omvfake.DefaultAptConfig().Upgrade.Chunks) {
		t.Errorf("output after hop = %q", s.text())
	}
}

func TestWaitForSession(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	job := e.start(t, jobs.KindAptUpdate, admin)
	e.r.WaitRetry = time.Hour // 신호로만 깨운다
	s := newSink()
	done := make(chan jobs.Outcome)
	go func() { done <- e.watch(t, job, s) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		w := s.waiting
		s.mu.Unlock()
		if w {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not waiting without sessions")
		}
		time.Sleep(time.Millisecond)
	}
	// 기다리는 동안은 OMV를 부르지 않는다.
	calls := len(e.fake.Calls())
	time.Sleep(20 * time.Millisecond)
	if len(e.fake.Calls()) != calls {
		t.Error("polled without a session")
	}
	e.pool.add(admin)
	s.offers <- struct{}{}
	if out := <-done; out.State != api.JobSucceeded {
		t.Errorf("outcome = %+v", out)
	}
	if s.waiting || !s.waited {
		t.Errorf("waiting = %v, waited = %v", s.waiting, s.waited)
	}
}

func TestSkipNonAdminSession(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	user := e.login(t, "user")
	e.pool.add(user)
	e.pool.add(admin)
	job := e.start(t, jobs.KindAptUpdate, admin)
	job.Prefer = user.Key
	if out := e.watch(t, job, newSink()); out.State != api.JobSucceeded {
		t.Errorf("outcome = %+v", out)
	}
	if len(e.pool.dead) != 0 {
		t.Errorf("non-admin session marked dead: %v", e.pool.dead)
	}
}

func TestMaxRun(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	e.pool.add(admin)
	job := e.start(t, jobs.KindAptUpdate, admin)
	job.StartedAt = time.Now().Add(-jobs.Kinds[jobs.KindAptUpdate].MaxRun - time.Minute)
	if out := e.watch(t, job, newSink()); out.State != api.JobUnknown || out.Code != api.JobCodeTimeout {
		t.Errorf("outcome = %+v", out)
	}
}

func TestBadHandleAndNotAllowed(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	e.pool.add(admin)
	job := jobs.WatchJob{ID: "x", Kind: jobs.KindAptUpdate, Handle: json.RawMessage(`{"filename":"/etc/passwd","pos":0}`)}
	if out := e.watch(t, job, newSink()); out.Code != api.JobCodeLost {
		t.Errorf("bad handle outcome = %+v", out)
	}
	notAllowed := Session{Key: "k", User: "admin", RPC: rpcFunc(func() error { return addons.ErrRPCNotAllowed })}
	e.pool.sessions = []Session{notAllowed}
	job.Handle = json.RawMessage(`{"filename":"/tmp/bgstatusAbc","pos":0}`)
	if out := e.watch(t, job, newSink()); out.State != api.JobUnknown || out.Code != api.JobCodeLost {
		t.Errorf("not allowed outcome = %+v", out)
	}
}

type rpcFunc func() error

func (f rpcFunc) Call(context.Context, string, string, any) (json.RawMessage, error) { return nil, f() }

func TestStopsOnContext(t *testing.T) {
	e := newEnv(t)
	admin := e.login(t, "admin")
	e.pool.add(admin)
	e.fake.SetBgPolls(1 << 20)
	cfg := omvfake.DefaultAptConfig()
	e.fake.SetAptConfig(cfg)
	job := e.start(t, jobs.KindAptUpdate, admin)
	ctx, cancel := context.WithCancel(context.Background())
	s := newSink()
	s.onOutput = func(n int) {
		if n == 1 {
			cancel()
		}
	}
	if out := e.r.Watch(ctx, job, s); out.State != "" {
		t.Errorf("outcome after cancel = %+v", out)
	}
}
