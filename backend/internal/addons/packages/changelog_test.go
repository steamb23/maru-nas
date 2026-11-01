package packages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
	"maru/backend/internal/omv"
	"maru/backend/internal/omv/omvfake"
)

// 합성 변경 기록(Debian 형식, 최신 항목이 위).
const sampleChangelog = `openssl (3.5.4-1~deb13u1) trixie-security; urgency=medium

  * Example security update.
    - CVE-0000-0001 (example).

 -- Example Maintainer <maint@example.org>  Mon, 13 Oct 2026 10:00:00 +0000

openssl (3.5.1-1) unstable; urgency=medium

  * New upstream release.

 -- Example Maintainer <maint@example.org>  Tue, 01 Jul 2026 10:00:00 +0000

openssl (3.5.0-2) unstable; urgency=medium

  * Example fix.

 -- Example Maintainer <maint@example.org>  Sun, 01 Jun 2026 10:00:00 +0000
`

func TestValidDebFile(t *testing.T) {
	good := []string{
		"pool/main/o/openssl/openssl_3.5.4-1~deb13u1_amd64.deb",
		"pool/main/b/bind9/bind9-host_1%3a9.20.9-1_amd64.deb",
		"pool/main/l/linux-signed-amd64/linux-image-6.12.41+deb13-amd64_6.12.41-1_amd64.deb",
		"./openmediavault-example_1.0_all.deb",
	}
	for _, f := range good {
		if !validDebFile(f) {
			t.Errorf("%q rejected", f)
		}
	}
	bad := []string{
		"",
		"openssl.deb",
		"pool/main/o/openssl/openssl_3.5.4.deb",
		"pool/main/o/x;reboot;_1_all.deb",
		"pool/main/o/x$(id)_1_all.deb",
		"pool/main/o/X_1_all.deb",
		"pool/../../etc/x_1_all.deb",
		"/var/cache/x_1_all.deb",
		"pool/main/o/x_1_all.deb\n",
		"pool/main/o/x 1_1_all.deb",
		"pool/main/o/x__all.deb",
	}
	for _, f := range bad {
		if validDebFile(f) {
			t.Errorf("%q accepted", f)
		}
	}
}

func TestSplitChangelog(t *testing.T) {
	pkg := aptindex.Package{Name: "openssl", Version: "3.5.4-1~deb13u1", OldVersion: "3.5.1-1"}
	got := SplitChangelog(pkg, strings.TrimRight(sampleChangelog, "\n"), false)
	if !got.Matched || got.Truncated || got.Name != "openssl" || got.Version != pkg.Version {
		t.Fatalf("got %+v", got)
	}
	if !strings.HasPrefix(got.Recent, "openssl (3.5.4-1~deb13u1)") || !strings.HasSuffix(got.Recent, "+0000") {
		t.Errorf("recent = %q", got.Recent)
	}
	if !strings.HasPrefix(got.Earlier, "openssl (3.5.1-1)") || !strings.Contains(got.Earlier, "3.5.0-2") {
		t.Errorf("earlier = %q", got.Earlier)
	}

	// 설치된 버전이 맨 위면 새 항목이 없다.
	got = SplitChangelog(aptindex.Package{Name: "openssl", Version: "x", OldVersion: "3.5.4-1~deb13u1"}, sampleChangelog, false)
	if !got.Matched || got.Recent != "" || !strings.HasPrefix(got.Earlier, "openssl (3.5.4-1~deb13u1)") {
		t.Errorf("top = %+v", got)
	}
	// 바이너리 재빌드(+b1)는 소스 버전으로 찾는다.
	got = SplitChangelog(aptindex.Package{Name: "openssl", OldVersion: "3.5.1-1+b2"}, sampleChangelog, false)
	if !got.Matched || !strings.HasPrefix(got.Earlier, "openssl (3.5.1-1)") {
		t.Errorf("binNMU = %+v", got)
	}
	// 새로 설치, 못 찾음, 본문 속 괄호는 나누지 않는다.
	for _, old := range []string{"", "2.0-1", "example"} {
		got = SplitChangelog(aptindex.Package{Name: "openssl", OldVersion: old}, sampleChangelog+"  * mention (example)\n", true)
		if got.Matched || got.Earlier != "" || !got.Truncated || !strings.HasPrefix(got.Recent, "openssl (3.5.4") {
			t.Errorf("old %q = %+v", old, got)
		}
	}
}

func TestSplitChangelogEarlierLimit(t *testing.T) {
	entry := func(v string, body int) string {
		return "pkg (" + v + ") unstable; urgency=low\n\n" + strings.Repeat("  * line\n", body) + "\n -- M <m@example.org>  Mon, 01 Jan 2026 00:00:00 +0000\n\n"
	}
	// 설치된 버전 항목 뒤로 한도를 넘는 항목들: 한도 안에서 항목 경계로 자른다.
	text := entry("3", 10) + entry("2", 10) + entry("1", earlierLimit/9) + entry("0", 10)
	got := SplitChangelog(aptindex.Package{Name: "pkg", OldVersion: "2"}, text, false)
	if !got.Truncated || !strings.HasPrefix(got.Earlier, "pkg (2)") || strings.Contains(got.Earlier, "pkg (1)") {
		t.Errorf("entry cut: truncated=%v earlier head %q", got.Truncated, got.Earlier[:20])
	}
	// 설치된 버전 항목 하나가 한도보다 길면 줄 경계에서 자른다.
	text = entry("3", 10) + entry("2", earlierLimit/9+10) + entry("1", 10)
	got = SplitChangelog(aptindex.Package{Name: "pkg", OldVersion: "2"}, text, false)
	if !got.Truncated || len(got.Earlier) > earlierLimit || !strings.HasSuffix(got.Earlier, "* line") {
		t.Errorf("line cut: len=%d tail %q", len(got.Earlier), got.Earlier[len(got.Earlier)-10:])
	}
}

func TestCleanChangelog(t *testing.T) {
	in := "a\x1b[31m\tb\r\n\xff\u0085c\npartial"
	if got := cleanChangelog(in, true); got != "a[31m\tb\n�c" {
		t.Errorf("cut = %q", got)
	}
	if got := cleanChangelog("x\n\n", false); got != "x" {
		t.Errorf("trailing = %q", got)
	}
}

// changelogOMV는 Apt.getChangeLog와 Exec.getOutput을 흉내 내는 가짜 RPCCaller다.
type changelogOMV struct {
	mu       sync.Mutex
	text     string
	err      error
	status   string
	calls    []string
	files    []string
	inflight int
	maxConc  int
	gate     chan struct{}
}

func (c *changelogOMV) Call(_ context.Context, service, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.calls = append(c.calls, service+"."+method)
	c.mu.Unlock()
	switch service + "." + method {
	case "Apt.getChangeLog":
		p, _ := params.(map[string]string)
		c.mu.Lock()
		c.files = append(c.files, p["filename"])
		c.inflight++
		c.maxConc = max(c.maxConc, c.inflight)
		c.mu.Unlock()
		if c.gate != nil {
			<-c.gate
		}
		status := c.status
		if status == "" {
			status = "/tmp/bgstatusAbc123"
		}
		return json.Marshal(status)
	case "Exec.getOutput":
		c.mu.Lock()
		c.inflight--
		c.mu.Unlock()
		if c.err != nil {
			return nil, c.err
		}
		return json.Marshal(map[string]any{"pos": len(c.text), "output": c.text, "pendingOutput": false, "running": false})
	}
	return nil, errors.New("unexpected rpc")
}

type busyJobs struct{ busy bool }

func (busyJobs) Start(context.Context, string) (string, error) { return "", errors.New("no") }
func (b busyJobs) Busy() bool                                  { return b.busy }

const changelogIndex = `[
  {"name": "openssl", "version": "3.5.4-1~deb13u1", "oldversion": "3.5.1-1", "summary": "Secure Sockets Layer toolkit",
   "filename": "pool/main/o/openssl/openssl_3.5.4-1~deb13u1_amd64.deb", "size": 1},
  {"name": "evil", "version": "1", "oldversion": "0", "filename": "pool/main/e/evil;reboot_1_all.deb", "size": 1},
  {"name": "example-omv", "version": "2.0", "oldversion": "1.0", "summary": "Example plugin",
   "extendeddescription": "First paragraph.\n\n\nSecond paragraph.", "filename": "pool/main/e/example-omv_2.0_all.deb", "size": 1},
  {"name": "newpkg", "version": "1.0", "oldversion": "", "summary": "New package", "filename": "pool/main/n/newpkg_1.0_all.deb", "size": 1}
]`

func changelogReader() *aptindex.Reader {
	return aptindex.NewReaderFS(fstest.MapFS{
		aptindex.IndexPath:      {Data: []byte(changelogIndex)},
		aptindex.DpkgStatusPath: {Data: []byte("Package: openssl\nStatus: install ok installed\nArchitecture: amd64\nDescription: old summary\n Installed long text.\n")},
	})
}

func TestChangelogAction(t *testing.T) {
	a := New(changelogReader())
	act := a.Actions["changelog"]
	if act.Query == nil || act.Run != nil || act.Timeout != changelogTimeout {
		t.Fatalf("action = %+v", act)
	}
	ctx := context.Background()
	omvc := &changelogOMV{text: sampleChangelog}
	req := addons.ActionRequest{Admin: true, Target: "openssl", RPC: omvc, Jobs: busyJobs{}}

	out, err := act.Query(ctx, req)
	cl, ok := out.(api.PackageChangelog)
	if err != nil || !ok || !cl.Matched || !strings.HasPrefix(cl.Recent, "openssl (3.5.4") {
		t.Fatalf("changelog = %+v, %v", out, err)
	}
	// OMV에는 목록의 파일 이름만 넘긴다.
	if len(omvc.files) != 1 || omvc.files[0] != "pool/main/o/openssl/openssl_3.5.4-1~deb13u1_amd64.deb" {
		t.Errorf("files = %v", omvc.files)
	}
	// 두 번째는 기억해 둔 것을 쓴다.
	if _, err := act.Query(ctx, req); err != nil || len(omvc.files) != 1 {
		t.Errorf("cached: files=%v err=%v", omvc.files, err)
	}

	errCode := func(target string, jobs addons.JobStarter) (string, int) {
		_, err := act.Query(ctx, addons.ActionRequest{Admin: true, Target: target, RPC: omvc, Jobs: jobs})
		var ae *addons.ActionError
		if errors.As(err, &ae) {
			return ae.Code, ae.Status
		}
		if errors.Is(err, addons.ErrInvalidTarget) {
			return "invalid", 0
		}
		return "other", 0
	}
	if code, status := errCode("nope", busyJobs{}); code != api.ErrNotFound || status != http.StatusNotFound {
		t.Errorf("missing = %s %d", code, status)
	}
	if code, _ := errCode("", busyJobs{}); code != "invalid" {
		t.Errorf("empty target = %s", code)
	}
	// 셸에 들어갈 수 있는 파일 이름은 OMV에 보내지 않는다.
	if code, _ := errCode("evil", busyJobs{}); code != "invalid" || len(omvc.files) != 1 {
		t.Errorf("evil = %s, files %v", code, omvc.files)
	}
	if code, status := errCode("newpkg", busyJobs{busy: true}); code != api.ErrJobBusy || status != http.StatusConflict {
		t.Errorf("busy = %s %d", code, status)
	}
}

// omvCaller는 가짜 OMV에 실제 클라이언트로 부르는 RPCCaller다(서버의 rpcCaller처럼 updatelastaccess=false).
type omvCaller struct {
	c   *omv.Client
	sid string
}

func (c omvCaller) Call(ctx context.Context, service, method string, params any) (json.RawMessage, error) {
	res, err := c.c.Call(ctx, omv.Request{SessionID: c.sid, Service: service, Method: method, Params: params, NoTouch: true})
	return res.Result, err
}

// TestChangelogFromOMV는 가짜 OMV의 Apt.getChangeLog로 성공과 실패 문구의 분류를 확인한다.
func TestChangelogFromOMV(t *testing.T) {
	fake := omvfake.New(omvfake.DefaultUsers(), 0)
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	client := omv.NewClient(srv.URL, "maru-test")
	lr, err := client.Login(context.Background(), "admin", "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	cfg := omvfake.DefaultAptConfig()
	cfg.Changelogs = map[string]omvfake.AptChangelog{
		"openssl":     {Text: sampleChangelog, Download: 2},
		"example-omv": {},
	}
	fake.SetAptConfig(cfg)
	rpc := omvCaller{client, lr.SessionID}

	cl := newChangelogs()
	cl.interval = time.Millisecond
	text, cut, err := cl.get(context.Background(), rpc, "pool/main/o/openssl/openssl_3.5.4-1~deb13u1_amd64.deb")
	if err != nil || cut || text != strings.TrimRight(sampleChangelog, "\n") {
		t.Fatalf("text=%q cut=%v err=%v", text, cut, err)
	}
	// 끝까지 읽어 OMV가 상태 파일을 지웠다.
	if fake.BgJobs() != 0 {
		t.Errorf("background jobs left: %d", fake.BgJobs())
	}

	code := func(filename string) string {
		_, _, err := newChangelogs().get(context.Background(), rpc, filename)
		var ae *addons.ActionError
		if errors.As(err, &ae) {
			return ae.Code
		}
		if errors.Is(err, addons.ErrActionFailed) {
			return api.ErrActionFailed
		}
		return fmt.Sprint(err)
	}
	if got := code("pool/main/e/example-omv_2.0_all.deb"); got != api.ErrNoChangelog {
		t.Errorf("no changelog = %s", got)
	}
	if got := code("pool/main/n/newpkg_1.0_all.deb"); got != api.ErrDownloadFailed {
		t.Errorf("download failed = %s", got)
	}
	cfg.Changelogs["openssl"] = omvfake.AptChangelog{Fail: "Failed to execute command 'tar': broken"}
	fake.SetAptConfig(cfg)
	if got := code("pool/main/o/openssl/openssl_9_amd64.deb"); got != api.ErrActionFailed {
		t.Errorf("other failure = %s", got)
	}
	if got := changelogError("f", addons.ErrActionTimeout); !errors.Is(got, addons.ErrActionTimeout) {
		t.Errorf("timeout = %v", got)
	}
}

func TestChangelogOneAtATime(t *testing.T) {
	cl := newChangelogs()
	omvc := &changelogOMV{text: "x", gate: make(chan struct{})}
	var wg sync.WaitGroup
	for _, f := range []string{"pool/a_1_all.deb", "pool/b_1_all.deb"} {
		wg.Go(func() {
			if _, _, err := cl.get(context.Background(), omvc, f); err != nil {
				t.Error(err)
			}
		})
	}
	// 첫 호출이 OMV에서 기다리는 동안 둘째는 들어오지 못한다.
	close(omvc.gate)
	wg.Wait()
	if omvc.maxConc != 1 || len(omvc.files) != 2 {
		t.Errorf("max concurrent = %d, files %v", omvc.maxConc, omvc.files)
	}

	bad := &changelogOMV{text: "x", status: "/etc/passwd"}
	if _, _, err := newChangelogs().get(context.Background(), bad, "pool/a_1_all.deb"); !errors.Is(err, addons.ErrActionFailed) {
		t.Errorf("bad status file err = %v", err)
	}
	failing := &changelogOMV{err: errors.New("boom")}
	c2 := newChangelogs()
	if _, _, err := c2.get(context.Background(), failing, "pool/a_1_all.deb"); err == nil {
		t.Error("failure not reported")
	}
	if _, ok := c2.cache.get("pool/a_1_all.deb"); ok {
		t.Error("failure cached")
	}
}

func TestChangelogCacheEvicts(t *testing.T) {
	c := &changelogCache{}
	for i := range changelogCacheSize + 2 {
		c.put(cachedChangelog{filename: string(rune('a' + i))})
	}
	if _, ok := c.get("a"); ok {
		t.Error("oldest kept")
	}
	if _, ok := c.get(string(rune('a' + changelogCacheSize + 1))); !ok {
		t.Error("newest dropped")
	}
	if len(c.entries) != changelogCacheSize {
		t.Errorf("len = %d", len(c.entries))
	}
}

func TestDetail(t *testing.T) {
	a := New(changelogReader())
	fetch := func(name string) api.PackagesResponse {
		t.Helper()
		out, err := a.Fetch(context.Background(), addons.Request{Admin: true, Query: url.Values{"detail": {name}}})
		if err != nil {
			t.Fatal(err)
		}
		return out.(api.PackagesResponse)
	}
	// 설치될 버전의 긴 설명이 있으면 그것을 쓴다.
	if d := fetch("example-omv").Detail; d == nil || d.Description != "First paragraph.\n\nSecond paragraph." || d.DescriptionFrom != api.DescriptionCandidate || d.Summary != "Example plugin" {
		t.Errorf("candidate = %+v", d)
	}
	// 없으면 설치된 버전의 것을 dpkg 상태에서 읽는다. 요약은 설치될 버전의 것이다.
	if d := fetch("openssl").Detail; d == nil || d.Description != "Installed long text." || d.DescriptionFrom != api.DescriptionInstalled || d.Summary != "Secure Sockets Layer toolkit" {
		t.Errorf("installed = %+v", d)
	}
	// 새로 설치하는 패키지는 요약뿐이다.
	if d := fetch("newpkg").Detail; d == nil || d.Description != "" || d.DescriptionFrom != "" {
		t.Errorf("new = %+v", d)
	}
	if res := fetch("nope"); res.Detail != nil || len(res.Packages) != 4 {
		t.Errorf("missing = %+v", res.Detail)
	}
	// 묻지 않으면 담지 않는다.
	out, _ := a.Fetch(context.Background(), addons.Request{Admin: true})
	if out.(api.PackagesResponse).Detail != nil {
		t.Error("detail without query")
	}

	// dpkg 상태를 읽지 못하면 요약만 주고 failedSources에 담는다.
	broken := New(aptindex.NewReaderFS(fstest.MapFS{
		aptindex.IndexPath:             {Data: []byte(changelogIndex)},
		aptindex.DpkgStatusPath + "/x": {Data: []byte("x")},
	}))
	out, err := broken.Fetch(context.Background(), addons.Request{Admin: true, Query: url.Values{"detail": {"openssl"}}})
	res := out.(api.PackagesResponse)
	if err != nil || res.Detail == nil || res.Detail.Description != "" || len(res.FailedSources) != 1 || res.FailedSources[0] != SourceDpkg {
		t.Errorf("broken = %+v, %v", res, err)
	}
}
