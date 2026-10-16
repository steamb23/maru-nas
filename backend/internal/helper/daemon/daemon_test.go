package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"onmaru/backend/internal/helper/proto"
	"onmaru/backend/internal/helper/worker"
)

// TestMain은 ExecSpawner 시험에서 이 테스트 실행 파일이 worker로 실행될 때 worker.Main을 부른다.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "worker" {
		os.Exit(worker.Main())
	}
	os.Exit(m.Run())
}

// 가짜 계정(모두 합성 값). gid 50 = openmediavault-admin, 27 = sudo, 100 = users.
const (
	gidUsers = 100
	gidAdmin = 50
	gidSudo  = 27
)

type fakeAccounts struct {
	mu     sync.Mutex
	users  map[string]Account
	groups map[string]int
}

func newFakeAccounts() *fakeAccounts {
	return &fakeAccounts{
		users: map[string]Account{
			"alice": {Name: "alice", UID: 1001, GID: gidUsers, Groups: []int{gidUsers}, Home: "/home/alice"},
			"bob":   {Name: "bob", UID: 1002, GID: gidUsers, Groups: []int{gidUsers, gidAdmin, gidSudo}, Home: "/home/bob"},
			"carol": {Name: "carol", UID: 1003, GID: gidUsers, Groups: []int{gidUsers, gidAdmin}, Home: "/home/carol"},
			// OMV 내장 admin처럼 시스템 uid지만 관리자 그룹이다.
			"admin":  {Name: "admin", UID: 998, GID: gidUsers, Groups: []int{gidUsers, gidAdmin}, Home: "/home/admin"},
			"daemon": {Name: "daemon", UID: 1, GID: 1, Groups: []int{1}, Home: "/usr/sbin"},
			"root":   {Name: "root", UID: 0, GID: 0, Groups: []int{0, gidAdmin, gidSudo}, Home: "/root"},
		},
		groups: map[string]int{"users": gidUsers, DefaultAdminGroup: gidAdmin, DefaultElevateGroup: gidSudo},
	}
}

func (f *fakeAccounts) Lookup(name string) (Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.users[name]
	if !ok {
		return Account{}, ErrNoAccount
	}
	a.Groups = slices.Clone(a.Groups)
	return a, nil
}

func (f *fakeAccounts) GroupID(name string) (int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.groups[name]
	return g, ok, nil
}

func (f *fakeAccounts) UIDRange() (int, int) { return 1000, 60000 }

func (f *fakeAccounts) set(a Account) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[a.Name] = a
}

type fakeVerifier struct {
	mu    sync.Mutex
	calls int
}

func (v *fakeVerifier) Verify(_ context.Context, user, password string) error {
	v.mu.Lock()
	v.calls++
	v.mu.Unlock()
	if password == user+"-pw" {
		return nil
	}
	return ErrAuth
}

// localSpawner는 프로세스 없이 worker.Serve를 부르고, 받은 권한·작업을 기록한다.
type localSpawner struct {
	mu    sync.Mutex
	creds []*Credential
	jobs  []worker.Job
	cfg   worker.Config
	// block이 있으면 닫힐 때까지 기다린다.
	block chan struct{}
}

func (s *localSpawner) Spawn(_ context.Context, cred *Credential, job worker.Job, conn *net.UnixConn) (string, bool, error) {
	s.mu.Lock()
	s.creds = append(s.creds, cred)
	s.jobs = append(s.jobs, job)
	block := s.block
	s.mu.Unlock()
	if block != nil {
		<-block
	}
	if err := worker.Serve(job, conn, s.cfg); err != nil {
		return worker.Code(err), true, nil
	}
	return "", true, nil
}

func (s *localSpawner) last() (*Credential, worker.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.creds[len(s.creds)-1], s.jobs[len(s.jobs)-1]
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type env struct {
	t        *testing.T
	sock     string
	srv      *Server
	accounts *fakeAccounts
	verifier *fakeVerifier
	spawner  *localSpawner
	clock    *clock
	logs     *logBuf
}

type logBuf struct {
	mu    sync.Mutex
	lines []string
}

func (l *logBuf) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logBuf) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

func newEnv(t *testing.T, mod func(*Config)) *env {
	t.Helper()
	e := &env{
		t:        t,
		sock:     filepath.Join(t.TempDir(), "helper.sock"),
		accounts: newFakeAccounts(),
		verifier: &fakeVerifier{},
		spawner:  &localSpawner{cfg: worker.Config{IdleTimeout: 5 * time.Second}},
		clock:    &clock{t: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)},
		logs:     &logBuf{},
	}
	cfg := Config{
		ClientUID: os.Getuid(),
		Accounts:  e.accounts,
		Verifier:  e.verifier,
		Spawner:   e.spawner,
		Now:       e.clock.now,
		FailDelay: -1,
		Logf:      e.logs.logf,
	}
	if mod != nil {
		mod(&cfg)
	}
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.srv = srv
	l, err := ListenUnix(e.sock)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		srv.Serve(l)
		close(done)
	}()
	t.Cleanup(func() {
		l.Close()
		<-done
	})
	return e
}

// call은 요청 하나를 보내고 응답 머리와 뒤따른 본문을 받는다.
func (e *env) call(req proto.Request, body []byte) (proto.Response, []byte) {
	e.t.Helper()
	c, err := net.Dial("unix", e.sock)
	if err != nil {
		e.t.Fatal(err)
	}
	defer c.Close()
	req.V = proto.Version
	req.Size = int64(len(body))
	if err := proto.WriteRequest(c, req); err != nil {
		e.t.Fatal(err)
	}
	if len(body) > 0 {
		c.Write(body)
	}
	resp, err := proto.ReadResponse(c)
	if err != nil {
		e.t.Fatalf("%s: read response: %v", req.Op, err)
	}
	data, _ := io.ReadAll(io.LimitReader(c, resp.Size))
	return resp, data
}

func (e *env) expect(code string, req proto.Request) proto.Response {
	e.t.Helper()
	resp, _ := e.call(req, nil)
	if code == "" && !resp.OK {
		e.t.Fatalf("%s %s: failed with %q", req.Op, req.Action, resp.Code)
	}
	if code != "" && (resp.OK || resp.Code != code) {
		e.t.Fatalf("%s %s: ok=%t code=%q, want %q", req.Op, req.Action, resp.OK, resp.Code, code)
	}
	return resp
}

func (e *env) open(user string, pending bool) proto.OpenResult {
	e.t.Helper()
	var res proto.OpenResult
	resp := e.expect("", proto.Request{Op: proto.OpOpen, User: user, Password: user + "-pw", Pending: pending})
	if err := resp.Decode(&res); err != nil || res.Ticket == "" {
		e.t.Fatalf("open %s: %+v %v", user, res, err)
	}
	return res
}

func (e *env) status(tok string) proto.StatusResult {
	e.t.Helper()
	var st proto.StatusResult
	if err := e.expect("", proto.Request{Op: proto.OpStatus, Ticket: tok}).Decode(&st); err != nil {
		e.t.Fatal(err)
	}
	return st
}

func runID(tok, scope string) proto.Request {
	return proto.Request{Op: proto.OpRun, Ticket: tok, Scope: scope, Action: proto.ActionID}
}

func TestRejectsOtherPeer(t *testing.T) {
	e := newEnv(t, func(c *Config) { c.ClientUID = os.Getuid() + 1 })
	e.expect(proto.CodeForbiddenPeer, proto.Request{Op: proto.OpReset})
	if !strings.Contains(e.logs.text(), "rejected connection") {
		t.Fatal("rejected connection not logged")
	}
}

func TestOpenAndRunAsUser(t *testing.T) {
	e := newEnv(t, nil)
	res := e.open("alice", false)
	if res.CanElevate {
		t.Fatal("alice can elevate")
	}
	resp := e.expect("", runID(res.Ticket, proto.ScopeUser))
	var id proto.Identity
	if err := resp.Decode(&id); err != nil {
		t.Fatal(err)
	}
	cred, job := e.spawner.last()
	if cred == nil || cred.UID != 1001 || cred.GID != gidUsers || !slices.Equal(cred.Groups, []int{gidUsers}) {
		t.Fatalf("credential = %+v", cred)
	}
	if job.User != "alice" || job.Home != "/home/alice" || job.Scope != proto.ScopeUser || job.Action != proto.ActionID {
		t.Fatalf("job = %+v", job)
	}
	st := e.status(res.Ticket)
	if st.User != "alice" || st.Pending || st.CanElevate || st.ElevatedSeconds != 0 {
		t.Fatalf("status = %+v", st)
	}
	logs := e.logs.text()
	if strings.Contains(logs, res.Ticket) || strings.Contains(logs, "alice-pw") {
		t.Fatal("ticket or password logged")
	}
	if !strings.Contains(logs, `run: user="alice" scope=user action=id result=ok`) {
		t.Fatalf("audit log missing:\n%s", logs)
	}
}

func TestOpenFailures(t *testing.T) {
	e := newEnv(t, nil)
	e.expect(proto.CodeBadRequest, proto.Request{Op: proto.OpOpen, User: "-alice", Password: "x"})
	e.expect(proto.CodeAuthFailed, proto.Request{Op: proto.OpOpen, User: "nobody", Password: "nobody-pw"})
	// 비밀번호가 맞아도 root와 시스템 계정은 받지 않는다.
	e.expect(proto.CodeAccountNotAllowed, proto.Request{Op: proto.OpOpen, User: "root", Password: "root-pw"})
	e.expect(proto.CodeAccountNotAllowed, proto.Request{Op: proto.OpOpen, User: "daemon", Password: "daemon-pw"})
	// 관리자 그룹의 시스템 uid(OMV 내장 admin)는 받는다.
	e.open("admin", false)
}

func TestOpenRateLimit(t *testing.T) {
	e := newEnv(t, nil)
	bad := proto.Request{Op: proto.OpOpen, User: "alice", Password: "wrong"}
	for range failLimit {
		e.expect(proto.CodeAuthFailed, bad)
	}
	resp := e.expect(proto.CodeRateLimited, proto.Request{Op: proto.OpOpen, User: "alice", Password: "alice-pw"})
	if resp.RetryAfterSeconds != 30 {
		t.Fatalf("retry after = %d", resp.RetryAfterSeconds)
	}
	calls := e.verifier.calls
	e.expect(proto.CodeRateLimited, bad)
	if e.verifier.calls != calls {
		t.Fatal("password checked while locked")
	}
	// 다른 사용자는 영향이 없다.
	e.open("bob", false)
	e.clock.add(31 * time.Second)
	e.open("alice", false)

	// 다시 잠기면 두 배로 늘어난다.
	for range failLimit {
		e.expect(proto.CodeAuthFailed, bad)
	}
	for range failLimit {
		e.clock.add(31 * time.Second)
		e.expect(proto.CodeAuthFailed, bad)
	}
	if resp := e.expect(proto.CodeRateLimited, bad); resp.RetryAfterSeconds != 60 {
		t.Fatalf("second lock = %ds, want 60", resp.RetryAfterSeconds)
	}
}

func TestPendingTicket(t *testing.T) {
	e := newEnv(t, nil)
	res := e.open("alice", true)
	// 2차 인증을 마치기 전에는 실행할 수 없다.
	e.expect(proto.CodeTicketInvalid, runID(res.Ticket, proto.ScopeUser))
	if st := e.status(res.Ticket); !st.Pending {
		t.Fatal("status is not pending")
	}
	e.expect("", proto.Request{Op: proto.OpActivate, Ticket: res.Ticket})
	e.expect("", runID(res.Ticket, proto.ScopeUser))

	late := e.open("alice", true)
	e.clock.add(5*time.Minute + time.Second)
	e.expect(proto.CodeTicketInvalid, proto.Request{Op: proto.OpActivate, Ticket: late.Ticket})
}

func TestElevation(t *testing.T) {
	e := newEnv(t, nil)
	bob := e.open("bob", false)
	if !bob.CanElevate {
		t.Fatal("bob cannot elevate")
	}
	e.expect(proto.CodeElevationRequired, runID(bob.Ticket, proto.ScopeRoot))
	e.expect(proto.CodeAuthFailed, proto.Request{Op: proto.OpElevate, Ticket: bob.Ticket, Password: "wrong"})
	var el proto.ElevateResult
	if err := e.expect("", proto.Request{Op: proto.OpElevate, Ticket: bob.Ticket, Password: "bob-pw"}).Decode(&el); err != nil || el.ElevatedSeconds != 300 {
		t.Fatalf("elevate = %+v %v", el, err)
	}
	e.expect("", runID(bob.Ticket, proto.ScopeRoot))
	if cred, job := e.spawner.last(); cred != nil || job.Scope != proto.ScopeRoot {
		t.Fatalf("root run credential = %+v job = %+v", cred, job)
	}
	// 파일 연산은 root 범위에 없다.
	e.expect(proto.CodeBadRequest, proto.Request{Op: proto.OpRun, Ticket: bob.Ticket, Scope: proto.ScopeRoot, Action: proto.ActionList, Params: json.RawMessage(`{"path":"/"}`)})

	e.clock.add(4 * time.Minute)
	if st := e.status(bob.Ticket); st.ElevatedSeconds != 60 {
		t.Fatalf("elevated seconds = %d", st.ElevatedSeconds)
	}
	// 연산을 해도 늘어나지 않는다.
	e.expect("", runID(bob.Ticket, proto.ScopeRoot))
	e.clock.add(time.Minute + time.Second)
	e.expect(proto.CodeElevationRequired, runID(bob.Ticket, proto.ScopeRoot))

	// 해제
	e.expect("", proto.Request{Op: proto.OpElevate, Ticket: bob.Ticket, Password: "bob-pw"})
	e.expect("", proto.Request{Op: proto.OpDrop, Ticket: bob.Ticket})
	e.expect(proto.CodeElevationRequired, runID(bob.Ticket, proto.ScopeRoot))

	// 상승은 티켓마다다. 같은 사용자의 다른 티켓은 상승하지 않는다.
	e.expect("", proto.Request{Op: proto.OpElevate, Ticket: bob.Ticket, Password: "bob-pw"})
	other := e.open("bob", false)
	e.expect(proto.CodeElevationRequired, runID(other.Ticket, proto.ScopeRoot))

	// 상승한 뒤 sudo 그룹에서 빠지면 상승을 거둔다.
	b, _ := e.accounts.Lookup("bob")
	b.Groups = []int{gidUsers, gidAdmin}
	e.accounts.set(b)
	e.expect(proto.CodeElevationNotAllowed, runID(bob.Ticket, proto.ScopeRoot))
	b.Groups = []int{gidUsers, gidAdmin, gidSudo}
	e.accounts.set(b)
	e.expect(proto.CodeElevationRequired, runID(bob.Ticket, proto.ScopeRoot))
}

func TestElevationNotAllowed(t *testing.T) {
	e := newEnv(t, nil)
	for _, user := range []string{"alice", "carol", "admin"} {
		res := e.open(user, false)
		if res.CanElevate {
			t.Errorf("%s can elevate", user)
		}
		calls := e.verifier.calls
		e.expect(proto.CodeElevationNotAllowed, proto.Request{Op: proto.OpElevate, Ticket: res.Ticket, Password: user + "-pw"})
		if e.verifier.calls != calls {
			t.Errorf("%s: password checked although elevation is not allowed", user)
		}
	}
}

func TestElevateGroupConfig(t *testing.T) {
	// 상승 그룹을 OMV 관리자 그룹으로 두면 OMV 관리자는 모두 상승할 수 있다.
	e := newEnv(t, func(c *Config) { c.ElevateGroup = DefaultAdminGroup })
	if !e.open("carol", false).CanElevate || e.open("alice", false).CanElevate {
		t.Fatal("elevate group config ignored")
	}
}

func TestTicketLifecycle(t *testing.T) {
	e := newEnv(t, nil)
	a := e.open("alice", false)
	e.expect("", proto.Request{Op: proto.OpClose, Ticket: a.Ticket})
	e.expect(proto.CodeTicketInvalid, runID(a.Ticket, proto.ScopeUser))
	// 없는 티켓을 닫아도 성공이다.
	e.expect("", proto.Request{Op: proto.OpClose, Ticket: a.Ticket})

	b := e.open("alice", false)
	c := e.open("bob", false)
	e.expect("", proto.Request{Op: proto.OpReset})
	e.expect(proto.CodeTicketInvalid, runID(b.Ticket, proto.ScopeUser))
	e.expect(proto.CodeTicketInvalid, runID(c.Ticket, proto.ScopeUser))

	d := e.open("alice", false)
	e.clock.add(23 * time.Hour)
	e.expect("", runID(d.Ticket, proto.ScopeUser))
	e.clock.add(24*time.Hour + time.Second)
	e.expect(proto.CodeTicketInvalid, runID(d.Ticket, proto.ScopeUser))

	e.expect(proto.CodeTicketInvalid, runID("not-a-ticket", proto.ScopeUser))
	e.expect(proto.CodeTicketInvalid, runID(strings.Repeat("A", 43), proto.ScopeUser))
}

func TestTicketDroppedWhenAccountChanges(t *testing.T) {
	e := newEnv(t, nil)
	res := e.open("alice", false)
	a, _ := e.accounts.Lookup("alice")
	a.UID = 2001
	e.accounts.set(a)
	e.expect(proto.CodeTicketInvalid, runID(res.Ticket, proto.ScopeUser))
	// uid를 되돌려도 지운 티켓은 살아나지 않는다.
	a.UID = 1001
	e.accounts.set(a)
	e.expect(proto.CodeTicketInvalid, runID(res.Ticket, proto.ScopeUser))
}

func TestBadRequests(t *testing.T) {
	e := newEnv(t, nil)
	res := e.open("alice", false)
	e.expect(proto.CodeBadRequest, proto.Request{Op: "nope"})
	e.expect(proto.CodeBadRequest, proto.Request{Op: proto.OpRun, Ticket: res.Ticket, Scope: proto.ScopeUser, Action: "exec"})
	e.expect(proto.CodeBadRequest, proto.Request{Op: proto.OpRun, Ticket: res.Ticket, Scope: "admin", Action: proto.ActionID})

	// 버전이 다르거나 프레임이 깨지면 거절한다.
	c, err := net.Dial("unix", e.sock)
	if err != nil {
		t.Fatal(err)
	}
	proto.WriteRequest(c, proto.Request{V: 99, Op: proto.OpReset})
	if resp, err := proto.ReadResponse(c); err != nil || resp.Code != proto.CodeBadRequest {
		t.Fatalf("wrong version: %+v %v", resp, err)
	}
	c.Close()
	c, _ = net.Dial("unix", e.sock)
	c.Write([]byte{0xff, 0xff, 0xff, 0xff})
	if resp, err := proto.ReadResponse(c); err != nil || resp.Code != proto.CodeBadRequest {
		t.Fatalf("broken frame: %+v %v", resp, err)
	}
	c.Close()
}

func TestRunFileOps(t *testing.T) {
	e := newEnv(t, nil)
	home := t.TempDir()
	a, _ := e.accounts.Lookup("alice")
	a.Home = home
	e.accounts.set(a)
	e.spawner.cfg.MountInfo = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("")), nil }
	res := e.open("alice", false)
	params := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

	resp, _ := e.call(proto.Request{Op: proto.OpRun, Ticket: res.Ticket, Scope: proto.ScopeUser, Action: proto.ActionWrite,
		Params: params(proto.WriteParams{Path: home + "/a.txt"})}, []byte("hello"))
	if !resp.OK {
		t.Fatalf("write: %q", resp.Code)
	}
	resp, data := e.call(proto.Request{Op: proto.OpRun, Ticket: res.Ticket, Scope: proto.ScopeUser, Action: proto.ActionRead,
		Params: params(proto.ReadParams{Path: home + "/a.txt"})}, nil)
	if !resp.OK || string(data) != "hello" {
		t.Fatalf("read: %+v %q", resp, data)
	}
	e.expect(proto.CodeOutsideRoots, proto.Request{Op: proto.OpRun, Ticket: res.Ticket, Scope: proto.ScopeUser, Action: proto.ActionList,
		Params: params(proto.PathParams{Path: "/etc"})})
	logs := e.logs.text()
	if !strings.Contains(logs, `action=fs.write path="`+home+`/a.txt" result=ok`) {
		t.Fatalf("write not audited with its path:\n%s", logs)
	}
	if strings.Contains(logs, `action=fs.read path=`) {
		t.Fatalf("read path logged:\n%s", logs)
	}
}

func TestWorkerLimit(t *testing.T) {
	e := newEnv(t, nil)
	e.spawner.block = make(chan struct{})
	res := e.open("alice", false)
	var wg sync.WaitGroup
	for range maxUserWorkers {
		wg.Go(func() { e.call(runID(res.Ticket, proto.ScopeUser), nil) })
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		e.spawner.mu.Lock()
		n := len(e.spawner.jobs)
		e.spawner.mu.Unlock()
		if n == maxUserWorkers {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d workers started", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.expect(proto.CodeBusy, runID(res.Ticket, proto.ScopeUser))
	// 다른 사용자는 실행할 수 있다(전체 자리는 남아 있다).
	bob := e.open("bob", false)
	go e.call(runID(bob.Ticket, proto.ScopeUser), nil)
	close(e.spawner.block)
	wg.Wait()
}

func TestIdleExit(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "h.sock")
	srv, err := New(Config{
		ClientUID: os.Getuid(), Accounts: newFakeAccounts(), Verifier: &fakeVerifier{}, Spawner: &localSpawner{},
		FailDelay: -1, IdleExit: 100 * time.Millisecond, Logf: func(string, ...any) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	l, err := ListenUnix(sock)
	if err != nil {
		t.Fatal(err)
	}
	// 티켓이 있으면 끝나지 않는다.
	srv.tickets.issue("alice", 1001, false)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	select {
	case err := <-done:
		t.Fatalf("exited with a live ticket: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	srv.tickets.reset()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("idle exit returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not exit when idle")
	}
}

// TestExecSpawner는 실제로 자식 프로세스를 띄워 작업 전달(stdin), 연결 전달(fd 3), 결과 코드를 확인한다.
// root가 아니어도 되도록 자기 uid·gid로 실행하고 보조 그룹은 바꾸지 않는다.
func TestExecSpawner(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if os.Getuid() == 0 {
		t.Skip("run as a normal user so the worker's user-scope check passes")
	}
	sp := ExecSpawner{Path: exe, noSetGroups: true}
	cred := &Credential{UID: os.Getuid(), GID: os.Getgid()}
	spawn := func(job worker.Job) (proto.Response, string, bool, error) {
		a, b := unixPair(t)
		defer a.Close()
		type out struct {
			code    string
			started bool
			err     error
		}
		ch := make(chan out, 1)
		go func() {
			code, started, err := sp.Spawn(context.Background(), cred, job, b)
			b.Close()
			ch <- out{code, started, err}
		}()
		resp, rerr := proto.ReadResponse(a)
		o := <-ch
		if rerr != nil && o.started {
			t.Fatalf("read response: %v (spawn err %v)", rerr, o.err)
		}
		return resp, o.code, o.started, o.err
	}

	resp, code, started, err := spawn(worker.Job{Scope: proto.ScopeUser, Action: proto.ActionID, User: "me", Home: "/nonexistent"})
	var id proto.Identity
	if err != nil || code != "" || !started || resp.Decode(&id) != nil || id.UID != os.Getuid() {
		t.Fatalf("id: resp=%+v code=%q started=%t err=%v", resp, code, started, err)
	}

	params, _ := json.Marshal(proto.PathParams{Path: "/etc"})
	resp, code, _, err = spawn(worker.Job{Scope: proto.ScopeUser, Action: proto.ActionList, Params: params, Home: "/nonexistent"})
	if err != nil || code != proto.CodeOutsideRoots || resp.Code != proto.CodeOutsideRoots {
		t.Fatalf("outside: resp=%+v code=%q err=%v", resp, code, err)
	}

	// root 범위 worker는 root가 아니면 스스로 거절한다.
	resp, code, _, _ = spawn(worker.Job{Scope: proto.ScopeRoot, Action: proto.ActionID})
	if code != proto.CodeInternal || resp.OK {
		t.Fatalf("root scope as non-root: resp=%+v code=%q", resp, code)
	}
}

func unixPair(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	dir := t.TempDir()
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, "p.sock"), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	ch := make(chan *net.UnixConn, 1)
	go func() {
		c, _ := l.AcceptUnix()
		ch <- c
	}()
	a, err := net.DialUnix("unix", nil, l.Addr().(*net.UnixAddr))
	if err != nil {
		t.Fatal(err)
	}
	return a, <-ch
}

func TestChkPwd(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "unix_chkpwd")
	// 인자와 stdin(NUL로 끝나는 비밀번호)을 확인하는 가짜 unix_chkpwd. 환경 변수가 비었는지도 본다.
	script := `#!/bin/sh
[ -z "$HOME$USER" ] || exit 9
[ "$#" -eq 2 ] || exit 8
if [ "$2" = chkexpiry ]; then
  [ "$1" = expired ] && exit 13
  exit 0
fi
[ "$2" = nonull ] || exit 7
pw=$(tr '\000' '\n' | head -n 1)
[ "$pw" = "s3cret pass" ] && exit 0
exit 7
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	v := ChkPwd{Path: stub}
	ctx := context.Background()
	if err := v.Verify(ctx, "alice", "s3cret pass"); err != nil {
		t.Fatalf("correct password: %v", err)
	}
	for name, c := range map[string][2]string{
		"wrong":    {"alice", "nope"},
		"empty":    {"alice", ""},
		"nul":      {"alice", "s3cret pass\x00x"},
		"long":     {"alice", strings.Repeat("x", maxPassword+1)},
		"bad user": {"-alice", "s3cret pass"},
		"expired":  {"expired", "s3cret pass"},
	} {
		if err := v.Verify(ctx, c[0], c[1]); err != ErrAuth {
			t.Errorf("%s: err = %v, want ErrAuth", name, err)
		}
	}
	if err := (ChkPwd{Path: filepath.Join(dir, "missing")}).Verify(ctx, "alice", "s3cret pass"); err == nil || err == ErrAuth {
		t.Errorf("missing checker: err = %v", err)
	}
}

func TestParseUIDRange(t *testing.T) {
	lo, hi := ParseUIDRange(strings.NewReader("# comment\nUID_MIN\t\t 1500\nUID_MAX 50000\nGID_MIN 1000\n#UID_MIN 1\n"))
	if lo != 1500 || hi != 50000 {
		t.Fatalf("range = %d..%d", lo, hi)
	}
	lo, hi = ParseUIDRange(strings.NewReader(""))
	if lo != defaultUIDMin || hi != defaultUIDMax {
		t.Fatalf("default range = %d..%d", lo, hi)
	}
}

func TestValidNames(t *testing.T) {
	for _, ok := range []string{"alice", "john.doe", "user_1", "a-b", "Admin"} {
		if !ValidUserName(ok) {
			t.Errorf("%q rejected", ok)
		}
	}
	for _, bad := range []string{"", "-x", ".x", "a b", "a/b", "a:b", strings.Repeat("a", 65)} {
		if ValidUserName(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestActivationListenerIgnoresOtherPID(t *testing.T) {
	t.Setenv("LISTEN_PID", "1")
	t.Setenv("LISTEN_FDS", "1")
	l, err := ActivationListener()
	if l != nil || err != nil {
		t.Fatalf("listener = %v, err = %v", l, err)
	}
	if os.Getenv("LISTEN_FDS") != "" {
		t.Fatal("LISTEN_FDS not cleared")
	}
}

func TestNewValidates(t *testing.T) {
	base := Config{ClientUID: 1000, Accounts: newFakeAccounts(), Verifier: &fakeVerifier{}, Spawner: &localSpawner{}}
	if _, err := New(base); err != nil {
		t.Fatal(err)
	}
	for name, mod := range map[string]func(*Config){
		"root client": func(c *Config) { c.ClientUID = 0 },
		"no accounts": func(c *Config) { c.Accounts = nil },
		"no verifier": func(c *Config) { c.Verifier = nil },
		"no spawner":  func(c *Config) { c.Spawner = nil },
	} {
		c := base
		mod(&c)
		if _, err := New(c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
