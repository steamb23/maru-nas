package worker

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"onmaru/backend/internal/helper/proto"
)

func TestParseMountInfo(t *testing.T) {
	f, err := os.Open("testdata/mountinfo")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := ParseMountInfo(f)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/", "/proc", "/dev", "/srv",
		"/srv/dev-disk-by-uuid-00000000-1111-2222-3333-444444444444",
		"/srv/mergerfs/pool",
		"/srv/dev-disk-by-label-my disk",
		"/home", "/srvdata",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("mounts = %q, want %q", got, want)
	}
}

func TestBasesFromMountInfo(t *testing.T) {
	cfg := Config{MountInfo: func() (io.ReadCloser, error) { return os.Open("testdata/mountinfo") }}.withDefaults()
	got, err := bases(Job{Home: "/home/alice"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	// /srv 자신, /srv 밖의 마운트(/srvdata 포함)는 넣지 않는다.
	want := []string{
		"/home/alice",
		"/srv/dev-disk-by-uuid-00000000-1111-2222-3333-444444444444",
		"/srv/mergerfs/pool",
		"/srv/dev-disk-by-label-my disk",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("bases = %q, want %q", got, want)
	}
	// 홈이 루트나 상대 경로면 기준으로 쓰지 않는다.
	for _, home := range []string{"/", "", "home/alice", "/home/alice/"} {
		got, _ := bases(Job{Home: home}, cfg)
		if slices.Contains(got, home) {
			t.Errorf("home %q used as a base", home)
		}
	}
}

func TestResolve(t *testing.T) {
	bs := []string{"/home/alice", "/srv/disk", "/srv/disk/nested"}
	cases := []struct {
		path, base, rel, code string
	}{
		{"/home/alice", "/home/alice", ".", ""},
		{"/home/alice/a/b", "/home/alice", "a/b", ""},
		{"/srv/disk/nested/x", "/srv/disk/nested", "x", ""},
		{"/srv/diskette", "", "", proto.CodeOutsideRoots},
		{"/etc/passwd", "", "", proto.CodeOutsideRoots},
		{"/home/alice/../bob", "", "", proto.CodeBadRequest},
		{"/home/alice/", "", "", proto.CodeBadRequest},
		{"home/alice", "", "", proto.CodeBadRequest},
		{"/home//alice", "", "", proto.CodeBadRequest},
		{"/home/alice/a\x00b", "", "", proto.CodeBadRequest},
		{"/home/alice/" + strings.Repeat("a", maxPath), "", "", proto.CodeBadRequest},
	}
	for _, c := range cases {
		got, err := resolve(c.path, bs)
		if code := proto.CodeOf(err); code != c.code {
			t.Errorf("resolve(%q) code = %q, want %q", c.path, code, c.code)
			continue
		}
		if c.code == "" && (got.base != c.base || got.rel != c.rel) {
			t.Errorf("resolve(%q) = %s + %s, want %s + %s", c.path, got.base, got.rel, c.base, c.rel)
		}
	}
}

func TestCheckPrivileges(t *testing.T) {
	if checkPrivileges(proto.ScopeUser, 1000, 1000) != nil {
		t.Error("user scope as uid 1000 rejected")
	}
	if checkPrivileges(proto.ScopeUser, 0, 0) == nil || checkPrivileges(proto.ScopeUser, 1000, 0) == nil {
		t.Error("user scope as root accepted")
	}
	if checkPrivileges(proto.ScopeRoot, 0, 0) != nil {
		t.Error("root scope as root rejected")
	}
	if checkPrivileges(proto.ScopeRoot, 1000, 1000) == nil {
		t.Error("root scope as uid 1000 accepted")
	}
	if checkPrivileges("other", 1000, 1000) == nil {
		t.Error("unknown scope accepted")
	}
}

// env는 임시 디렉터리에 홈과 /srv 아래 디스크 두 개를 흉내 낸 worker 환경이다.
type env struct {
	t     *testing.T
	home  string
	disk  string
	disk2 string
	cfg   Config
	job   Job
}

func newEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	srv := filepath.Join(root, "srv")
	e := &env{
		t:     t,
		home:  filepath.Join(root, "home", "alice"),
		disk:  filepath.Join(srv, "disk1"),
		disk2: filepath.Join(srv, "disk2"),
	}
	for _, d := range []string{e.home, e.disk, e.disk2, filepath.Join(srv, "salt")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mountinfo := fmt.Sprintf("1 0 8:1 / / rw - ext4 /dev/sda1 rw\n"+
		"2 1 8:1 /srv %s ro - ext4 /dev/sda1 rw\n"+
		"3 2 8:17 / %s rw - ext4 /dev/sdb1 rw\n"+
		"4 2 8:33 / %s rw - ext4 /dev/sdc1 rw\n", srv, e.disk, e.disk2)
	e.cfg = Config{
		SrvDir:      srv,
		MountInfo:   func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(mountinfo)), nil },
		IdleTimeout: 5 * time.Second,
	}
	e.job = Job{Scope: proto.ScopeUser, User: "alice", Home: e.home}
	return e
}

func (e *env) file(p, content string) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// socketPair는 연결된 Unix 소켓 두 개다(부모가 worker에 넘기는 연결과 같은 종류).
func socketPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	conns := make([]net.Conn, 2)
	for i, fd := range fds {
		f := os.NewFile(uintptr(fd), "sock")
		c, err := net.FileConn(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		conns[i] = c
	}
	return conns[0], conns[1]
}

// run은 연산 하나를 실행하고 응답 머리, 뒤따른 본문, Serve의 오류를 돌려준다.
func (e *env) run(scope, action string, params any, body []byte) (proto.Response, []byte, error) {
	e.t.Helper()
	client, server := socketPair(e.t)
	defer client.Close()
	job := e.job
	job.Scope, job.Action, job.Size = scope, action, int64(len(body))
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			e.t.Fatal(err)
		}
		job.Params = b
	}
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(job, server, e.cfg)
		server.Close()
	}()
	if body != nil {
		go client.Write(body)
	}
	resp, err := proto.ReadResponse(client)
	if err != nil {
		e.t.Fatalf("%s: read response: %v", action, err)
	}
	data, _ := io.ReadAll(io.LimitReader(client, resp.Size))
	return resp, data, <-errc
}

// ok는 성공 응답의 결과를 v로 읽는다.
func (e *env) ok(v any, scope, action string, params any, body []byte) []byte {
	e.t.Helper()
	resp, data, err := e.run(scope, action, params, body)
	if err != nil || !resp.OK {
		e.t.Fatalf("%s %v: code %q, err %v", action, params, resp.Code, err)
	}
	if v != nil {
		if err := resp.Decode(v); err != nil {
			e.t.Fatal(err)
		}
	}
	return data
}

// fail은 연산이 code로 실패하는지 확인한다.
func (e *env) fail(code, action string, params any, body []byte) {
	e.t.Helper()
	resp, _, err := e.run(proto.ScopeUser, action, params, body)
	if resp.OK || resp.Code != code || proto.CodeOf(err) != code {
		e.t.Fatalf("%s %v: ok=%t code=%q err=%v, want %q", action, params, resp.OK, resp.Code, err, code)
	}
}

func TestIdentity(t *testing.T) {
	e := newEnv(t)
	var id proto.Identity
	e.ok(&id, proto.ScopeUser, proto.ActionID, nil, nil)
	if id.UID != os.Getuid() || id.GID != os.Getgid() || len(id.Groups) == 0 || id.Groups[0].ID != os.Getgid() {
		t.Fatalf("identity = %+v", id)
	}
}

func TestActionNotAllowedInScope(t *testing.T) {
	e := newEnv(t)
	resp, _, err := e.run(proto.ScopeRoot, proto.ActionList, proto.PathParams{Path: e.home}, nil)
	if resp.OK || resp.Code != proto.CodeBadRequest || err == nil {
		t.Fatalf("root fs.list: %+v %v", resp, err)
	}
}

func TestListAndStat(t *testing.T) {
	e := newEnv(t)
	e.file(filepath.Join(e.home, "b.txt"), "hello")
	e.file(filepath.Join(e.home, "dir", "x"), "")
	if err := os.Symlink("b.txt", filepath.Join(e.home, "a-link")); err != nil {
		t.Fatal(err)
	}
	// 기준 밖을 가리키는 링크는 대상 종류를 알려 주지 않는다.
	if err := os.Symlink("/etc", filepath.Join(e.home, "z-out")); err != nil {
		t.Fatal(err)
	}
	var res proto.ListResult
	e.ok(&res, proto.ScopeUser, proto.ActionList, proto.PathParams{Path: e.home}, nil)
	names := []string{}
	for _, en := range res.Entries {
		names = append(names, en.Name)
	}
	if !slices.Equal(names, []string{"a-link", "b.txt", "dir", "z-out"}) || res.Truncated {
		t.Fatalf("list = %q truncated=%t", names, res.Truncated)
	}
	byName := map[string]proto.Entry{}
	for _, en := range res.Entries {
		byName[en.Name] = en
	}
	if f := byName["b.txt"]; f.Type != proto.TypeFile || f.Size != 5 || f.Mode != 0o644 {
		t.Errorf("b.txt = %+v", f)
	}
	if d := byName["dir"]; d.Type != proto.TypeDir || d.Size != 0 {
		t.Errorf("dir = %+v", d)
	}
	if l := byName["a-link"]; l.Type != proto.TypeSymlink || l.Target != "b.txt" || l.LinkType != proto.TypeFile {
		t.Errorf("a-link = %+v", l)
	}
	if l := byName["z-out"]; l.Type != proto.TypeSymlink || l.Target != "/etc" || l.LinkType != "" {
		t.Errorf("z-out = %+v", l)
	}

	var st proto.Entry
	e.ok(&st, proto.ScopeUser, proto.ActionStat, proto.PathParams{Path: filepath.Join(e.home, "b.txt")}, nil)
	if st.Name != "b.txt" || st.Size != 5 {
		t.Errorf("stat = %+v", st)
	}
	e.ok(&st, proto.ScopeUser, proto.ActionStat, proto.PathParams{Path: e.disk}, nil)
	if st.Name != "disk1" || st.Type != proto.TypeDir {
		t.Errorf("stat base = %+v", st)
	}

	e.fail(proto.CodeNotDir, proto.ActionList, proto.PathParams{Path: filepath.Join(e.home, "b.txt")}, nil)
	e.fail(proto.CodeNotFound, proto.ActionStat, proto.PathParams{Path: filepath.Join(e.home, "nope")}, nil)
	e.fail(proto.CodeOutsideRoots, proto.ActionList, proto.PathParams{Path: "/etc"}, nil)
	// /srv 자신과 루트 파일시스템 디렉터리(/srv/salt)는 기준이 아니다.
	e.fail(proto.CodeOutsideRoots, proto.ActionList, proto.PathParams{Path: e.cfg.SrvDir}, nil)
	e.fail(proto.CodeOutsideRoots, proto.ActionList, proto.PathParams{Path: filepath.Join(e.cfg.SrvDir, "salt")}, nil)
	// 기준 밖을 가리키는 링크를 거쳐 나갈 수 없다.
	e.fail(proto.CodeOutsideRoots, proto.ActionList, proto.PathParams{Path: filepath.Join(e.home, "z-out")}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionList, proto.PathParams{Path: e.home + "/../x"}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionList, map[string]any{"path": e.home, "extra": 1}, nil)
}

func TestListTruncated(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(e.disk, "many")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range MaxList + 1 {
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("f%05d", i)))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	var res proto.ListResult
	e.ok(&res, proto.ScopeUser, proto.ActionList, proto.PathParams{Path: dir}, nil)
	if len(res.Entries) != MaxList || !res.Truncated {
		t.Fatalf("entries = %d truncated = %t", len(res.Entries), res.Truncated)
	}
}

func TestRead(t *testing.T) {
	e := newEnv(t)
	p := filepath.Join(e.disk, "data.bin")
	e.file(p, "0123456789")
	var res proto.ReadResult
	if data := e.ok(&res, proto.ScopeUser, proto.ActionRead, proto.ReadParams{Path: p}, nil); string(data) != "0123456789" || res.FileSize != 10 {
		t.Fatalf("read = %q %+v", data, res)
	}
	if data := e.ok(nil, proto.ScopeUser, proto.ActionRead, proto.ReadParams{Path: p, Offset: 3, Length: 4}, nil); string(data) != "3456" {
		t.Fatalf("read range = %q", data)
	}
	if data := e.ok(nil, proto.ScopeUser, proto.ActionRead, proto.ReadParams{Path: p, Offset: 50}, nil); len(data) != 0 {
		t.Fatalf("read past end = %q", data)
	}
	e.fail(proto.CodeBadRequest, proto.ActionRead, proto.ReadParams{Path: p, Offset: -1}, nil)
	e.fail(proto.CodeIsDir, proto.ActionRead, proto.ReadParams{Path: e.disk}, nil)

	// FIFO는 쓰는 쪽이 없어도 멈추지 않고 거절한다.
	fifo := filepath.Join(e.disk, "pipe")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		e.fail(proto.CodeNotRegular, proto.ActionRead, proto.ReadParams{Path: fifo}, nil)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reading a FIFO blocked")
	}
}

func TestWrite(t *testing.T) {
	e := newEnv(t)
	p := filepath.Join(e.disk, "up.txt")
	var en proto.Entry
	e.ok(&en, proto.ScopeUser, proto.ActionWrite, proto.WriteParams{Path: p}, []byte("first"))
	if got, _ := os.ReadFile(p); string(got) != "first" || en.Name != "up.txt" || en.Size != 5 {
		t.Fatalf("write = %q %+v", got, en)
	}
	e.fail(proto.CodeExists, proto.ActionWrite, proto.WriteParams{Path: p}, []byte("second"))

	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	e.ok(&en, proto.ScopeUser, proto.ActionWrite, proto.WriteParams{Path: p, Overwrite: true}, []byte("second!"))
	if got, _ := os.ReadFile(p); string(got) != "second!" || en.Mode != 0o600 {
		t.Fatalf("overwrite = %q mode %o", got, en.Mode)
	}

	// 빈 파일도 올릴 수 있다.
	e.ok(nil, proto.ScopeUser, proto.ActionWrite, proto.WriteParams{Path: filepath.Join(e.disk, "empty")}, []byte{})

	e.fail(proto.CodeIsDir, proto.ActionWrite, proto.WriteParams{Path: e.home + "/sub", Overwrite: true}, func() []byte {
		os.Mkdir(e.home+"/sub", 0o755)
		return []byte("x")
	}())
	e.fail(proto.CodeBadRequest, proto.ActionWrite, proto.WriteParams{Path: e.disk}, []byte("x"))
	e.fail(proto.CodeBadRequest, proto.ActionWrite, proto.WriteParams{Path: filepath.Join(e.disk, uploadPrefix+"x")}, []byte("x"))
	e.fail(proto.CodeNotFound, proto.ActionWrite, proto.WriteParams{Path: filepath.Join(e.disk, "no", "dir")}, []byte("x"))
	e.fail(proto.CodeOutsideRoots, proto.ActionWrite, proto.WriteParams{Path: "/tmp/x"}, []byte("x"))

	// 남은 임시 파일이 없어야 한다.
	ents, _ := os.ReadDir(e.disk)
	for _, d := range ents {
		if strings.HasPrefix(d.Name(), uploadPrefix) {
			t.Errorf("leftover temp file %s", d.Name())
		}
	}
}

func TestWriteShortBody(t *testing.T) {
	e := newEnv(t)
	client, server := socketPair(t)
	job := e.job
	job.Action, job.Size = proto.ActionWrite, 10
	job.Params, _ = json.Marshal(proto.WriteParams{Path: filepath.Join(e.disk, "short")})
	errc := make(chan error, 1)
	go func() { errc <- Serve(job, server, e.cfg); server.Close() }()
	client.Write([]byte("abc"))
	client.(*net.UnixConn).CloseWrite()
	resp, err := proto.ReadResponse(client)
	client.Close()
	if err != nil || resp.OK || resp.Code != proto.CodeBadRequest {
		t.Fatalf("short body: %+v %v", resp, err)
	}
	<-errc
	ents, _ := os.ReadDir(e.disk)
	if len(ents) != 0 {
		t.Fatalf("files left after a short upload: %v", ents)
	}
}

func TestMkdirRenameRemove(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(e.disk, "new")
	e.ok(nil, proto.ScopeUser, proto.ActionMkdir, proto.PathParams{Path: dir}, nil)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("mkdir: %v", err)
	}
	e.fail(proto.CodeExists, proto.ActionMkdir, proto.PathParams{Path: dir}, nil)
	e.fail(proto.CodeExists, proto.ActionMkdir, proto.PathParams{Path: e.disk}, nil)

	e.file(filepath.Join(dir, "a"), "a")
	e.file(filepath.Join(e.disk, "b"), "b")
	var en proto.Entry
	e.ok(&en, proto.ScopeUser, proto.ActionRename, proto.RenameParams{From: filepath.Join(dir, "a"), To: filepath.Join(dir, "c")}, nil)
	if en.Name != "c" {
		t.Fatalf("rename = %+v", en)
	}
	e.fail(proto.CodeExists, proto.ActionRename, proto.RenameParams{From: filepath.Join(dir, "c"), To: filepath.Join(e.disk, "b")}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionRename, proto.RenameParams{From: filepath.Join(e.disk, "b"), To: filepath.Join(e.disk2, "b")}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionRename, proto.RenameParams{From: e.disk, To: filepath.Join(e.disk, "x")}, nil)
	e.fail(proto.CodeNotFound, proto.ActionRename, proto.RenameParams{From: filepath.Join(e.disk, "zz"), To: filepath.Join(e.disk, "yy")}, nil)

	e.fail(proto.CodeNotEmpty, proto.ActionRemove, proto.RemoveParams{Path: dir}, nil)
	e.ok(nil, proto.ScopeUser, proto.ActionRemove, proto.RemoveParams{Path: dir, Recursive: true}, nil)
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("recursive remove left %s: %v", dir, err)
	}
	e.ok(nil, proto.ScopeUser, proto.ActionRemove, proto.RemoveParams{Path: filepath.Join(e.disk, "b")}, nil)
	e.fail(proto.CodeNotFound, proto.ActionRemove, proto.RemoveParams{Path: filepath.Join(e.disk, "b")}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionRemove, proto.RemoveParams{Path: e.disk, Recursive: true}, nil)
	if _, err := os.Stat(e.disk); err != nil {
		t.Fatalf("base removed: %v", err)
	}
}

func TestRemoveDoesNotFollowLinks(t *testing.T) {
	e := newEnv(t)
	keep := filepath.Join(e.disk2, "keep")
	e.file(keep, "x")
	dir := filepath.Join(e.disk, "d")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(e.disk2, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	e.ok(nil, proto.ScopeUser, proto.ActionRemove, proto.RemoveParams{Path: dir, Recursive: true}, nil)
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("link target was removed: %v", err)
	}
}

func TestPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	e := newEnv(t)
	locked := filepath.Join(e.disk, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	e.fail(proto.CodePermissionDenied, proto.ActionList, proto.PathParams{Path: locked}, nil)
	e.fail(proto.CodePermissionDenied, proto.ActionWrite, proto.WriteParams{Path: filepath.Join(locked, "x")}, []byte("x"))
}
