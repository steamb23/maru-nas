package server

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/addons/packages"
	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
	"maru/backend/internal/auth"
	"maru/backend/internal/jobs"
	"maru/backend/internal/omv"
	"maru/backend/internal/omv/omvfake"
)

const jobsIndex = `[
  {"name": "libc6", "version": "2.41-99", "oldversion": "2.41-12", "repository": "Debian/trixie", "size": 2867200},
  {"name": "openmediavault", "version": "8.5.12-1", "oldversion": "8.5.11-1", "repository": "openmediavault/synchrony", "size": 4096000}
]`

// jobsEnv는 시스템 업데이트 애드온과 store를 쓰는 작업 관리자를 둔 환경이다.
func jobsEnv(t *testing.T, store jobs.Store) (*env, *jobs.Manager) {
	t.Helper()
	reader := aptindex.NewReaderFS(fstest.MapFS{aptindex.IndexPath: &fstest.MapFile{Data: []byte(jobsIndex)}})
	if store == nil {
		store = jobs.NewMemoryStore()
	}
	m := jobs.New(jobs.Config{Store: store, Index: reader, BootID: func() string { return "boot" }})
	t.Cleanup(m.Close)
	e := newEnv(t, envOptions{addons: testRegistry(t, packages.New(reader)), jobs: m})
	return e, m
}

// slowApt는 출력 조각이 n개인 가짜 작업 설정이다(폴링마다 하나씩 나온다).
func slowApt(n int) omvfake.AptConfig {
	cfg := omvfake.DefaultAptConfig()
	var chunks []string
	for i := range n {
		chunks = append(chunks, fmt.Sprintf("line %03d\n", i))
	}
	cfg.Update.Chunks, cfg.Upgrade.Chunks = chunks, chunks
	return cfg
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func getJobs(t *testing.T, b *browser) api.JobsResponse {
	t.Helper()
	rec := b.do(http.MethodGet, "/api/jobs", nil)
	wantStatus(t, rec, http.StatusOK)
	return decode[api.JobsResponse](t, rec)
}

// readOutput은 작업이 끝날 때까지 출력 API를 커서로 따라 읽는다.
func readOutput(t *testing.T, b *browser, id string) (string, string) {
	t.Helper()
	var text strings.Builder
	from := int64(0)
	deadline := time.Now().Add(10 * time.Second)
	for {
		rec := b.do(http.MethodGet, fmt.Sprintf("/api/jobs/%s/output?from=%d", id, from), nil)
		wantStatus(t, rec, http.StatusOK)
		out := decode[api.JobOutput](t, rec)
		if out.From != from {
			t.Fatalf("output from %d, asked %d", out.From, from)
		}
		text.WriteString(out.Text)
		from = out.Next
		if out.State != api.JobRunning && !out.More {
			return text.String(), out.State
		}
		if time.Now().After(deadline) {
			t.Fatal("job did not finish")
		}
		if !out.More {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func jobNotice(ns []api.Notice) *api.Notice {
	for i := range ns {
		if ns[i].Job != nil {
			return &ns[i]
		}
	}
	return nil
}

func TestJobsCheckEndToEnd(t *testing.T) {
	e, _ := jobsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)

	wantStatus(t, act(b, "packages", "check", ""), http.StatusNoContent)
	cur := getJobs(t, b).Current
	if cur == nil || cur.Kind != api.JobAptUpdate || !cur.Mine || cur.Owner != "admin" {
		t.Fatalf("current = %+v", cur)
	}
	text, state := readOutput(t, b, cur.ID)
	if state != api.JobSucceeded || text != strings.Join(omvfake.DefaultAptConfig().Update.Chunks, "") {
		t.Errorf("output %q state %s", text, state)
	}
	waitUntil(t, "result", func() bool { return getJobs(t, b).Current == nil })
	recent := getJobs(t, b).Recent
	if len(recent) != 1 || recent[0].State != api.JobSucceeded || recent[0].Count == nil || *recent[0].Count != 2 {
		t.Fatalf("recent = %+v", recent)
	}

	got := decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil))
	n := jobNotice(got.Notices)
	if n == nil || n.Code != api.NoticeJobSucceeded || !n.Dismissible || n.Job.ID != cur.ID || len(got.Tasks) != 0 {
		t.Fatalf("notifications = %+v", got)
	}
	wantStatus(t, b.do(http.MethodDelete, "/api/notifications/"+n.ID, nil), http.StatusNoContent)
	got = decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil))
	if jobNotice(got.Notices) != nil {
		t.Error("dismissed notice still listed")
	}
	wantCode(t, b.do(http.MethodDelete, "/api/notifications/apt-updates", nil), http.StatusNotFound, api.ErrNotFound)
	wantCode(t, b.do(http.MethodDelete, "/api/notifications/job-20260101-000000-00000000", nil), http.StatusNotFound, api.ErrNotFound)

	for _, c := range e.fake.Calls() {
		if c.Service == "Exec" && c.Touch {
			t.Errorf("Exec.%s extended the session", c.Method)
		}
		if c.Service == "Exec" && c.Method != "getOutput" {
			t.Errorf("unexpected Exec.%s", c.Method)
		}
	}
}

func TestDismissAllNotices(t *testing.T) {
	store := jobs.NewMemoryStore()
	// 결과는 최신이 앞이다.
	ids := []string{"20261024-100200-00000002", "20261024-100100-00000001", "20261024-100000-00000000"}
	var rs []jobs.Result
	for i, id := range ids {
		end := time.Now().Add(-time.Duration(i) * time.Minute)
		rs = append(rs, jobs.Result{ID: id, Kind: jobs.KindAptUpdate, Owner: "admin", State: api.JobSucceeded, StartedAt: end, EndedAt: end})
	}
	if err := store.SaveResults("admin", rs); err != nil {
		t.Fatal(err)
	}
	e, _ := jobsEnv(t, store)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	jobNotices := func() []string {
		var out []string
		for _, n := range decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil)).Notices {
			if n.Job != nil {
				out = append(out, n.ID)
			}
		}
		return out
	}
	if got := jobNotices(); len(got) != len(ids) {
		t.Fatalf("job notices = %v", got)
	}

	wantCode(t, b.do(http.MethodDelete, "/api/notifications", nil), http.StatusBadRequest, api.ErrBadRequest)
	wantCode(t, b.do(http.MethodDelete, "/api/notifications?through=apt-updates", nil), http.StatusNotFound, api.ErrNotFound)
	other := e.browser()
	wantStatus(t, other.login("user", "user"), http.StatusOK)
	wantCode(t, other.do(http.MethodDelete, "/api/notifications?through="+jobs.NoticeID(ids[0]), nil), http.StatusNotFound, api.ErrNotFound)

	// 기준과 그보다 먼저 끝난 결과를 지우고, 나중에 끝난 결과는 남긴다.
	wantStatus(t, b.do(http.MethodDelete, "/api/notifications?through="+jobs.NoticeID(ids[1]), nil), http.StatusNoContent)
	if got := jobNotices(); !slices.Equal(got, []string{jobs.NoticeID(ids[0])}) {
		t.Errorf("job notices after dismissing through %s = %v", ids[1], got)
	}
	wantStatus(t, b.do(http.MethodDelete, "/api/notifications?through="+jobs.NoticeID(ids[0]), nil), http.StatusNoContent)
	if got := jobNotices(); len(got) != 0 {
		t.Errorf("job notices after dismissing all = %v", got)
	}
}

func TestJobsUpgradeFailures(t *testing.T) {
	cases := map[string]struct{ fail, code string }{
		"apt":          {omvfake.AptExecFailedMessage, api.JobCodeAptFailed},
		"core removal": {omvfake.AptCoreRemovalMessage, api.JobCodeAptCoreRemoval},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e, _ := jobsEnv(t, nil)
			cfg := omvfake.DefaultAptConfig()
			cfg.Upgrade.Fail = c.fail
			e.fake.SetAptConfig(cfg)
			b := e.browser()
			wantStatus(t, b.login("admin", "admin"), http.StatusOK)
			wantStatus(t, act(b, "packages", "upgrade", ""), http.StatusNoContent)
			id := getJobs(t, b).Current.ID
			text, state := readOutput(t, b, id)
			if state != api.JobFailed || text != strings.Join(cfg.Upgrade.Chunks, "") {
				t.Errorf("output %q state %s", text, state)
			}
			waitUntil(t, "result", func() bool { return getJobs(t, b).Current == nil })
			r := getJobs(t, b).Recent[0]
			if r.State != api.JobFailed || r.Code != c.code || r.Count != nil {
				t.Errorf("result = %+v", r)
			}
			n := jobNotice(decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil)).Notices)
			if n == nil || n.Code != api.NoticeJobFailed || n.Level != api.NoticeError || n.Job.Code != c.code {
				t.Errorf("notice = %+v", n)
			}
		})
	}
}

func TestJobsStartRefusals(t *testing.T) {
	e, _ := jobsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantCode(t, act(b, "packages", "check", "x"), http.StatusBadRequest, api.ErrBadRequest)

	cfg := omvfake.DefaultAptConfig()
	cfg.Locked = true
	e.fake.SetAptConfig(cfg)
	wantCode(t, act(b, "packages", "upgrade", ""), http.StatusConflict, api.ErrAptLocked)
	if getJobs(t, b).Current != nil {
		t.Fatal("refused job recorded")
	}

	e.fake.SetAptConfig(slowApt(200))
	d := omvfake.DefaultPluginData()
	d.Dirty = []string{"apt"}
	e.fake.SetPluginData(d)
	wantCode(t, act(b, "packages", "check", ""), http.StatusConflict, api.ErrConfigDirty)

	// 적용 대기는 업데이트 확인만 막는다. 설치가 도는 동안은 다른 관리자도 시작할 수 없다.
	wantStatus(t, act(b, "packages", "upgrade", ""), http.StatusNoContent)
	other := e.browser()
	wantStatus(t, other.login("admin", "admin"), http.StatusOK)
	wantCode(t, act(other, "packages", "upgrade", ""), http.StatusConflict, api.ErrJobBusy)
	cur := getJobs(t, other).Current
	if cur == nil || cur.Mine != true || cur.Kind != api.JobAptUpgrade {
		t.Errorf("current for the other session = %+v", cur)
	}
	readOutput(t, b, cur.ID)
}

func TestJobsNonAdmin(t *testing.T) {
	e, _ := jobsEnv(t, nil)
	e.fake.SetAptConfig(slowApt(200))
	admin := e.browser()
	wantStatus(t, admin.login("admin", "admin"), http.StatusOK)
	wantStatus(t, act(admin, "packages", "upgrade", ""), http.StatusNoContent)

	user := e.browser()
	wantStatus(t, user.login("user", "user"), http.StatusOK)
	wantCode(t, user.do(http.MethodGet, "/api/jobs", nil), http.StatusForbidden, api.ErrAdminRequired)
	id := getJobs(t, admin).Current.ID
	wantCode(t, user.do(http.MethodGet, "/api/jobs/"+id+"/output", nil), http.StatusForbidden, api.ErrAdminRequired)
	wantCode(t, act(user, "packages", "check", ""), http.StatusForbidden, api.ErrAdminRequired)
	got := decode[api.NotificationsResponse](t, user.do(http.MethodGet, "/api/notifications", nil))
	if len(got.Tasks) != 0 {
		t.Errorf("tasks for a non-admin = %+v", got.Tasks)
	}
	if got := decode[api.NotificationsResponse](t, admin.do(http.MethodGet, "/api/notifications", nil)); len(got.Tasks) != 1 {
		t.Errorf("tasks for the admin = %+v", got.Tasks)
	}
	readOutput(t, admin, id)
}

func TestJobsOutputValidation(t *testing.T) {
	e, _ := jobsEnv(t, nil)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, act(b, "packages", "check", ""), http.StatusNoContent)
	id := getJobs(t, b).Current.ID
	for _, from := range []string{"-1", "abc"} {
		wantCode(t, b.do(http.MethodGet, "/api/jobs/"+id+"/output?from="+from, nil), http.StatusBadRequest, api.ErrBadRequest)
	}
	wantCode(t, b.do(http.MethodGet, "/api/jobs/20260101-000000-00000000/output", nil), http.StatusNotFound, api.ErrNotFound)
	wantCode(t, b.do(http.MethodGet, "/api/jobs/nope", nil), http.StatusNotFound, api.ErrNotFound)
	if rec := b.do(http.MethodGet, "/api/jobs/"+id, nil); rec.Code != http.StatusOK || decode[api.Job](t, rec).ID != id {
		t.Errorf("get job = %d", rec.Code)
	}
	readOutput(t, b, id)
}

// 시작한 관리자가 로그아웃하면(OMV가 그 세션을 파기) 다른 관리자 세션으로 이어 지켜본다.
func TestJobsSessionHop(t *testing.T) {
	e, _ := jobsEnv(t, nil)
	e.fake.SetAptConfig(slowApt(60))
	starter := e.browser()
	wantStatus(t, starter.login("admin", "admin"), http.StatusOK)
	watcher := e.browser()
	wantStatus(t, watcher.login("totp", "totp"), http.StatusOK)
	wantStatus(t, watcher.do(http.MethodPost, "/api/login/verify", api.VerifyRequest{Response: "123456"}), http.StatusOK)

	wantStatus(t, act(starter, "packages", "upgrade", ""), http.StatusNoContent)
	id := getJobs(t, starter).Current.ID
	wantStatus(t, starter.do(http.MethodPost, "/api/logout", nil), http.StatusNoContent)

	// 시작하지 않은 관리자도 진행 중 출력을 읽는다.
	text, state := readOutput(t, watcher, id)
	if state != api.JobSucceeded || text != strings.Join(slowApt(60).Upgrade.Chunks, "") {
		t.Errorf("output %q state %s", text, state)
	}
	// 결과는 시작한 사용자에게 남는다.
	if r := getJobs(t, watcher).Recent; len(r) != 0 {
		t.Errorf("results for another admin = %+v", r)
	}
	wantStatus(t, starter.login("admin", "admin"), http.StatusOK)
	if r := getJobs(t, starter).Recent; len(r) != 1 || r[0].ID != id || r[0].State != api.JobSucceeded {
		t.Errorf("results for the starter = %+v", r)
	}
	n := jobNotice(decode[api.NotificationsResponse](t, starter.do(http.MethodGet, "/api/notifications", nil)).Notices)
	if n == nil || n.Code != api.NoticeJobSucceeded {
		t.Errorf("starter notice = %+v", n)
	}
}

// 관리자가 아무도 없으면 기다렸다가, 관리자가 다시 로그인하면 이어 지켜본다.
func TestJobsWaitForAdmin(t *testing.T) {
	e, m := jobsEnv(t, nil)
	e.fake.SetAptConfig(slowApt(60))
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, act(b, "packages", "upgrade", ""), http.StatusNoContent)
	wantStatus(t, b.do(http.MethodPost, "/api/logout", nil), http.StatusNoContent)
	waitUntil(t, "waiting", func() bool { j := m.Tasks(""); return len(j) == 1 && j[0].Waiting })
	calls := len(e.fake.Calls())
	time.Sleep(30 * time.Millisecond)
	if len(e.fake.Calls()) != calls {
		t.Error("polled without an admin session")
	}

	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	got := decode[api.NotificationsResponse](t, b.do(http.MethodGet, "/api/notifications", nil))
	if len(got.Tasks) != 1 {
		t.Fatalf("tasks = %+v", got.Tasks)
	}
	text, state := readOutput(t, b, got.Tasks[0].ID)
	if state != api.JobSucceeded || text != strings.Join(slowApt(60).Upgrade.Chunks, "") {
		t.Errorf("output %q state %s", text, state)
	}
}

// maru가 다시 시작해도 상태 디렉터리의 기록으로 같은 작업을 이어 지켜본다(출력이 겹치지 않는다).
func TestJobsResumeAfterRestart(t *testing.T) {
	store := jobs.NewMemoryStore()
	e, m := jobsEnv(t, store)
	cfg := slowApt(80)
	e.fake.SetAptConfig(cfg)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, act(b, "packages", "upgrade", ""), http.StatusNoContent)
	id := getJobs(t, b).Current.ID
	waitUntil(t, "some output", func() bool { j := m.Tasks(""); return len(j) == 1 && j[0].OutputEnd > 0 })
	m.Close()

	// 새 프로세스: 같은 OMV, 같은 상태 디렉터리, 새 세션 저장소.
	reader := aptindex.NewReaderFS(fstest.MapFS{aptindex.IndexPath: &fstest.MapFile{Data: []byte(jobsIndex)}})
	m2 := jobs.New(jobs.Config{Store: store, Index: reader, BootID: func() string { return "boot" }})
	t.Cleanup(m2.Close)
	e2 := *e
	e2.store = auth.NewStore()
	e2.h = New(Config{
		System:      fakeSource{resp: api.SystemResponse{UptimeSeconds: 1}},
		Static:      http.NotFoundHandler(),
		OMV:         omv.NewClient(e.omvTS.URL, "maru-test/1"),
		Sessions:    e2.store,
		Addons:      testRegistry(t, packages.New(reader)),
		Jobs:        m2,
		FailDelay:   -1,
		JobInterval: 5 * time.Millisecond,
		Now:         e.clk.Now,
	})
	b2 := e2.browser()
	wantStatus(t, b2.login("admin", "admin"), http.StatusOK)
	wantStatus(t, b2.do(http.MethodGet, "/api/notifications", nil), http.StatusOK)
	text, state := readOutput(t, b2, id)
	if state != api.JobSucceeded || text != strings.Join(cfg.Upgrade.Chunks, "") {
		t.Errorf("output after restart %q state %s", text, state)
	}
}

func TestJobsTransientErrors(t *testing.T) {
	e, _ := jobsEnv(t, nil)
	e.fake.SetGlitch("Exec", "getOutput", 3, omvfake.GlitchProxy)
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, act(b, "packages", "upgrade", ""), http.StatusNoContent)
	e.fake.SetGlitch("Exec", "getOutput", 3, omvfake.GlitchEngine)
	_, state := readOutput(t, b, getJobs(t, b).Current.ID)
	if state != api.JobSucceeded {
		t.Errorf("state %s", state)
	}
}

// 다른 곳(워크벤치의 작업 패널)이 먼저 끝까지 읽어 상태 파일이 사라지면 결과를 모른다.
func TestJobsStatusFileGone(t *testing.T) {
	e, m := jobsEnv(t, nil)
	e.fake.SetAptConfig(slowApt(200))
	b := e.browser()
	wantStatus(t, b.login("admin", "admin"), http.StatusOK)
	wantStatus(t, act(b, "packages", "upgrade", ""), http.StatusNoContent)
	id := getJobs(t, b).Current.ID
	waitUntil(t, "some output", func() bool { j := m.Tasks(""); return len(j) == 1 && j[0].OutputEnd > 0 })
	e.fake.DropBg()
	_, state := readOutput(t, b, id)
	if state != api.JobUnknown {
		t.Fatalf("state %s", state)
	}
	r := getJobs(t, b).Recent[0]
	if r.Code != api.JobCodeLost || !r.OutputIncomplete {
		t.Errorf("result = %+v", r)
	}
}

func TestJobsExecStopNotAllowed(t *testing.T) {
	reg := testRegistry(t, packages.New(aptindex.NewReaderFS(fstest.MapFS{})))
	if kind, ids := reg.Classify("Exec", "stop"); kind != 0 || len(ids) != 0 {
		t.Errorf("Exec.stop is declared: %v %v", kind, ids)
	}
	if kind, ids := reg.Classify("Exec", "getOutput"); !slices.Contains(ids, packages.ID) || kind == 0 {
		t.Errorf("Exec.getOutput = %v %v", kind, ids)
	}
}
