package worker

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"onmaru/backend/internal/helper/proto"
)

// dirent는 linux_dirent64 레코드 하나를 만든다(8바이트 정렬).
func dirent(ino uint64, typ uint8, name string) []byte {
	n := direntHeader + len(name) + 1
	n = (n + 7) &^ 7
	b := make([]byte, n)
	binary.NativeEndian.PutUint64(b[0:8], ino)
	binary.NativeEndian.PutUint16(b[16:18], uint16(n))
	b[18] = typ
	copy(b[direntHeader:], name)
	return b
}

func TestParseDirents(t *testing.T) {
	var buf []byte
	for _, d := range [][]byte{
		dirent(1, syscall.DT_DIR, "."),
		dirent(2, syscall.DT_DIR, ".."),
		dirent(3, syscall.DT_DIR, "사진"),
		dirent(0, syscall.DT_REG, "deleted"),
		dirent(4, syscall.DT_REG, "a.txt"),
		dirent(5, syscall.DT_LNK, "link"),
		dirent(6, syscall.DT_FIFO, "pipe"),
		dirent(7, syscall.DT_UNKNOWN, "fuse"),
	} {
		buf = append(buf, d...)
	}
	var got []proto.NameEntry
	more, err := parseDirents(buf, func(name string, typ byte) bool {
		got = append(got, proto.NameEntry{Type: typ, Name: name})
		return true
	})
	want := []proto.NameEntry{{Type: 'd', Name: "사진"}, {Type: 'f', Name: "a.txt"}, {Type: 'l', Name: "link"}, {Type: 'o', Name: "pipe"}, {Type: '?', Name: "fuse"}}
	if err != nil || !more || !slices.Equal(got, want) {
		t.Fatalf("parse = %v %v %v", got, more, err)
	}

	// fn이 멈추면 더 읽지 않는다.
	n := 0
	more, err = parseDirents(buf, func(string, byte) bool { n++; return n < 2 })
	if err != nil || more || n != 2 {
		t.Fatalf("stop: n=%d more=%v err=%v", n, more, err)
	}

	// 레코드 길이가 버퍼를 넘거나 너무 짧으면 오류다.
	bad := dirent(9, syscall.DT_REG, "x")
	binary.NativeEndian.PutUint16(bad[16:18], 200)
	if _, err := parseDirents(bad, func(string, byte) bool { return true }); err == nil {
		t.Fatal("oversized reclen accepted")
	}
	binary.NativeEndian.PutUint16(bad[16:18], 4)
	if _, err := parseDirents(bad, func(string, byte) bool { return true }); err == nil {
		t.Fatal("short reclen accepted")
	}
}

func (e *env) names(p string) (proto.NamesResult, []proto.NameEntry) {
	e.t.Helper()
	var res proto.NamesResult
	body := e.ok(&res, proto.ScopeUser, proto.ActionNames, proto.PathParams{Path: p}, nil)
	list, err := proto.ParseNames(body, res.Count)
	if err != nil {
		e.t.Fatalf("names body: %v", err)
	}
	slices.SortFunc(list, func(a, b proto.NameEntry) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return res, list
}

func TestNames(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(e.disk, "media")
	e.file(filepath.Join(dir, "b.mkv"), "video")
	e.file(filepath.Join(dir, "sub", "x"), "x")
	if err := os.Symlink("b.mkv", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, list := e.names(dir)
	want := []proto.NameEntry{{Type: 'f', Name: "b.mkv"}, {Type: 'o', Name: "fifo"}, {Type: 'l', Name: "link"}, {Type: 'd', Name: "sub"}}
	if !slices.Equal(list, want) || res.Count != 4 || res.Truncated || !res.Writable {
		t.Fatalf("names = %+v %v", res, list)
	}
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.DirModTimeNs != st.ModTime().UnixNano() {
		t.Fatalf("dirModTimeNs = %d, want %d", res.DirModTimeNs, st.ModTime().UnixNano())
	}

	// 빈 폴더, 기준 디렉터리 자신
	empty := filepath.Join(e.disk, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if res, list := e.names(empty); res.Count != 0 || len(list) != 0 {
		t.Fatalf("empty = %+v %v", res, list)
	}
	if res, _ := e.names(e.disk); res.Count != 2 {
		t.Fatalf("disk = %+v", res)
	}

	e.fail(proto.CodeNotDir, proto.ActionNames, proto.PathParams{Path: filepath.Join(dir, "b.mkv")}, nil)
	e.fail(proto.CodeNotFound, proto.ActionNames, proto.PathParams{Path: filepath.Join(dir, "missing")}, nil)
	e.fail(proto.CodeOutsideRoots, proto.ActionNames, proto.PathParams{Path: "/etc"}, nil)
	// 심볼릭 링크로 기준 밖에 나갈 수 없다.
	if err := os.Symlink("/etc", filepath.Join(e.disk, "etc")); err != nil {
		t.Fatal(err)
	}
	e.fail(proto.CodeOutsideRoots, proto.ActionNames, proto.PathParams{Path: filepath.Join(e.disk, "etc")}, nil)
}

func TestNamesTruncated(t *testing.T) {
	e := newEnv(t)
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		e.file(filepath.Join(e.disk, "many", n), "")
	}
	limit := namesLimit
	namesLimit = 3
	t.Cleanup(func() { namesLimit = limit })
	res, list := e.names(filepath.Join(e.disk, "many"))
	if !res.Truncated || res.Count != 3 || len(list) != 3 {
		t.Fatalf("truncated = %+v %v", res, list)
	}
	namesLimit = limit
	bytesLimit := namesBytesLimit
	namesBytesLimit = 2 * proto.NameSize("a")
	t.Cleanup(func() { namesBytesLimit = bytesLimit })
	if res, _ := e.names(filepath.Join(e.disk, "many")); !res.Truncated || res.Count != 2 {
		t.Fatalf("byte limit = %+v", res)
	}
}

func TestResolveUnknown(t *testing.T) {
	e := newEnv(t)
	e.file(filepath.Join(e.disk, "f"), "")
	if err := os.Mkdir(filepath.Join(e.disk, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := os.OpenRoot(e.disk)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	in := func() []proto.NameEntry {
		return []proto.NameEntry{{Type: '?', Name: "f"}, {Type: '?', Name: "d"}, {Type: '?', Name: "gone"}, {Type: 'f', Name: "known"}}
	}
	got := resolveUnknown(r, ".", in(), time.Now().Add(time.Minute))
	if want := []proto.NameEntry{{Type: 'f', Name: "f"}, {Type: 'd', Name: "d"}, {Type: 'f', Name: "known"}}; !slices.Equal(got, want) {
		t.Fatalf("resolved = %v", got)
	}
	// 시간이 다 되면 나머지는 그대로 둔다.
	got = resolveUnknown(r, ".", in(), time.Now().Add(-time.Second))
	if !slices.Equal(got, in()) {
		t.Fatalf("expired budget = %v", got)
	}
}

func TestStatMany(t *testing.T) {
	e := newEnv(t)
	dir := filepath.Join(e.disk, "media")
	e.file(filepath.Join(dir, "a.txt"), "hello")
	if err := os.Mkdir(filepath.Join(dir, "open"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	var res proto.StatManyResult
	e.ok(&res, proto.ScopeUser, proto.ActionStatMany, proto.StatManyParams{Dir: dir, Names: []string{"a.txt", "missing", "open", "link"}}, nil)
	if len(res.Items) != 4 {
		t.Fatalf("items = %+v", res.Items)
	}
	a, missing, open, link := res.Items[0], res.Items[1], res.Items[2], res.Items[3]
	if a.Name != "a.txt" || a.Type != proto.TypeFile || a.Size != 5 || a.Code != "" || a.ModTimeNs == 0 {
		t.Fatalf("a.txt = %+v", a)
	}
	if missing.Name != "missing" || missing.Code != proto.CodeNotFound {
		t.Fatalf("missing = %+v", missing)
	}
	if open.Type != proto.TypeDir || open.Locked {
		t.Fatalf("open = %+v", open)
	}
	if link.Type != proto.TypeSymlink || link.Target != "a.txt" || link.LinkType != proto.TypeFile {
		t.Fatalf("link = %+v", link)
	}

	// 이름은 그 폴더 바로 안만 받는다.
	for _, bad := range []string{"", ".", "..", "a/b", "../x", "a\x00b", strings.Repeat("a", 256)} {
		e.fail(proto.CodeBadRequest, proto.ActionStatMany, proto.StatManyParams{Dir: dir, Names: []string{bad}}, nil)
	}
	tooMany := make([]string, proto.MaxStatMany+1)
	for i := range tooMany {
		tooMany[i] = "a.txt"
	}
	e.fail(proto.CodeBadRequest, proto.ActionStatMany, proto.StatManyParams{Dir: dir, Names: tooMany}, nil)
	e.fail(proto.CodeOutsideRoots, proto.ActionStatMany, proto.StatManyParams{Dir: "/etc", Names: []string{"passwd"}}, nil)
	e.fail(proto.CodeNotDir, proto.ActionStatMany, proto.StatManyParams{Dir: filepath.Join(dir, "a.txt"), Names: []string{"x"}}, nil)
}

func TestStatManyLocked(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	e := newEnv(t)
	locked := filepath.Join(e.disk, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	var res proto.StatManyResult
	e.ok(&res, proto.ScopeUser, proto.ActionStatMany, proto.StatManyParams{Dir: e.disk, Names: []string{"locked"}}, nil)
	if it := res.Items[0]; it.Type != proto.TypeDir || !it.Locked || it.Code != "" {
		t.Fatalf("locked = %+v", it)
	}
	// 목록을 볼 수 없는 폴더(x만)는 다루지 않는다. os.Root가 폴더를 읽기로 열기 때문이고, 탐색기는 목록을
	// 볼 수 있는 폴더 안의 이름만 묻는다.
	searchOnly := filepath.Join(e.disk, "searchonly")
	e.file(filepath.Join(searchOnly, "f"), "x")
	if err := os.Chmod(searchOnly, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(searchOnly, 0o755) })
	e.fail(proto.CodePermissionDenied, proto.ActionStatMany, proto.StatManyParams{Dir: searchOnly, Names: []string{"f"}}, nil)
}

func TestInfo(t *testing.T) {
	e := newEnv(t)
	f := filepath.Join(e.disk, "docs", "a.txt")
	e.file(f, "hello")
	var res proto.InfoResult
	e.ok(&res, proto.ScopeUser, proto.ActionInfo, proto.PathParams{Path: f}, nil)
	if res.Name != "a.txt" || res.Type != proto.TypeFile || res.Size != 5 || res.UID != os.Getuid() || res.GID != os.Getgid() {
		t.Fatalf("info = %+v", res)
	}
	if res.Access == nil || !res.Access.Read || !res.Access.Write {
		t.Fatalf("access = %+v", res.Access)
	}
	if res.ACL != nil && *res.ACL {
		t.Fatalf("acl on a plain file = %v", *res.ACL)
	}

	// 폴더와 기준 디렉터리 자신
	e.ok(&res, proto.ScopeUser, proto.ActionInfo, proto.PathParams{Path: filepath.Join(e.disk, "docs")}, nil)
	if res.Type != proto.TypeDir || res.Access == nil || !res.Access.Read || !res.Access.Write {
		t.Fatalf("dir info = %+v", res)
	}
	e.ok(&res, proto.ScopeUser, proto.ActionInfo, proto.PathParams{Path: e.disk}, nil)
	if res.Name != "disk1" || res.Type != proto.TypeDir || res.Access == nil || !res.Access.Read {
		t.Fatalf("base info = %+v", res)
	}

	// 심볼릭 링크는 권한·ACL을 보지 않는다.
	link := filepath.Join(e.disk, "passwd")
	if err := os.Symlink("/etc/passwd", link); err != nil {
		t.Fatal(err)
	}
	res = proto.InfoResult{}
	e.ok(&res, proto.ScopeUser, proto.ActionInfo, proto.PathParams{Path: link}, nil)
	if res.Type != proto.TypeSymlink || res.Target != "/etc/passwd" || res.LinkType != "" || res.Access != nil || res.ACL != nil {
		t.Fatalf("link info = %+v", res)
	}

	if os.Getuid() != 0 {
		ro := filepath.Join(e.disk, "ro.txt")
		e.file(ro, "x")
		if err := os.Chmod(ro, 0o444); err != nil {
			t.Fatal(err)
		}
		e.ok(&res, proto.ScopeUser, proto.ActionInfo, proto.PathParams{Path: ro}, nil)
		if res.Access == nil || !res.Access.Read || res.Access.Write {
			t.Fatalf("read-only access = %+v", res.Access)
		}
	}
	e.fail(proto.CodeNotFound, proto.ActionInfo, proto.PathParams{Path: filepath.Join(e.disk, "missing")}, nil)
	e.fail(proto.CodeOutsideRoots, proto.ActionInfo, proto.PathParams{Path: "/etc/passwd"}, nil)
}
