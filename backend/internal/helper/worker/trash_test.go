package worker

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"maru/backend/internal/helper/proto"
)

func (e *env) at(t time.Time) {
	e.cfg.Now = func() time.Time { return t }
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func lmode(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode()
}

func TestTrashQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/home/a/문서 1.txt": "/home/a/%EB%AC%B8%EC%84%9C%201.txt",
		"a-b_c.d~e/f":      "a-b_c.d~e/f",
		"100%+a":           "100%25%2Ba",
	} {
		if got := trashQuote(in); got != want {
			t.Fatalf("trashQuote(%q) = %q, want %q", in, got, want)
		}
		if got := trashUnquote(want); got != in {
			t.Fatalf("trashUnquote(%q) = %q, want %q", want, got, in)
		}
	}
	// trash-cli가 쓴 파일(다른 절과 CRLF가 섞여도 [Trash Info] 절의 값만 본다)
	p, at, ok := parseTrashInfo([]byte("[Other]\nPath=/nope\n[Trash Info]\r\nPath=media/%EC%98%81%ED%99%94/a.mkv\r\nDeletionDate=2026-10-20T14:02:03\r\n"))
	if !ok || p != "media/영화/a.mkv" || !at.Equal(time.Date(2026, 10, 20, 14, 2, 3, 0, time.Local)) {
		t.Fatalf("parse = %q %v %t", p, at, ok)
	}
	if _, _, ok := parseTrashInfo([]byte("[Trash Info]\nDeletionDate=2026-10-20T14:02:03\n")); ok {
		t.Fatal("info without Path parsed")
	}
}

func TestTrashHome(t *testing.T) {
	e := newEnv(t)
	e.at(time.Date(2026, 10, 21, 12, 34, 56, 0, time.Local))
	e.file(filepath.Join(e.home, "문서 1.txt"), "a")
	var res proto.TrashResult
	e.ok(&res, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.home, "문서 1.txt")}, nil)
	trashDir := filepath.Join(e.home, ".local/share/Trash")
	if res.Trash != trashDir || res.Name != "문서 1.txt" {
		t.Fatalf("trash = %+v", res)
	}
	// trash-put과 같은 형식(홈 휴지통은 절대 경로)
	want := "[Trash Info]\nPath=" + e.home + "/%EB%AC%B8%EC%84%9C%201.txt\nDeletionDate=2026-10-21T12:34:56\n"
	if got := readFile(t, filepath.Join(trashDir, "info", "문서 1.txt.trashinfo")); got != want {
		t.Fatalf("trashinfo = %q, want %q", got, want)
	}
	if readFile(t, filepath.Join(trashDir, "files", "문서 1.txt")) != "a" {
		t.Fatal("file not moved")
	}
	if _, err := os.Lstat(filepath.Join(e.home, "문서 1.txt")); !os.IsNotExist(err) {
		t.Fatalf("original left: %v", err)
	}
	for _, d := range []string{trashDir, filepath.Join(trashDir, "files"), filepath.Join(trashDir, "info")} {
		if m := lmode(t, d).Perm(); m != 0o700 {
			t.Fatalf("%s mode = %o", d, m)
		}
	}
	// 같은 이름은 _1을 붙인다.
	e.file(filepath.Join(e.home, "문서 1.txt"), "b")
	e.ok(&res, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.home, "문서 1.txt")}, nil)
	if res.Name != "문서 1.txt_1" || readFile(t, filepath.Join(trashDir, "files", "문서 1.txt_1")) != "b" {
		t.Fatalf("second trash = %+v", res)
	}
	// 휴지통 자신과 그것을 품은 폴더, 기준 디렉터리는 넣지 않는다.
	e.fail(proto.CodeBadRequest, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.home, ".local")}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionTrash, proto.PathParams{Path: e.home}, nil)
	e.fail(proto.CodeNotFound, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.home, "none")}, nil)
	e.fail(proto.CodeOutsideRoots, proto.ActionTrash, proto.PathParams{Path: "/etc/passwd"}, nil)
}

func TestTrashDisk(t *testing.T) {
	e := newEnv(t)
	e.at(time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local))
	uid := strconv.Itoa(os.Getuid())
	e.file(filepath.Join(e.disk, "media", "b.txt"), "b")
	var res proto.TrashResult
	// .Trash가 없으면 디스크 맨 위의 .Trash-<uid>(Path는 맨 위 기준 상대 경로)
	e.ok(&res, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.disk, "media", "b.txt")}, nil)
	own := filepath.Join(e.disk, ".Trash-"+uid)
	if res.Trash != own || res.Name != "b.txt" {
		t.Fatalf("trash = %+v", res)
	}
	if got := readFile(t, filepath.Join(own, "info", "b.txt.trashinfo")); got != "[Trash Info]\nPath=media/b.txt\nDeletionDate=2026-10-21T09:00:00\n" {
		t.Fatalf("trashinfo = %q", got)
	}

	var vols proto.TrashDirsResult
	e.ok(&vols, proto.ScopeUser, proto.ActionTrashDirs, nil, nil)
	kinds := map[string]string{}
	for _, v := range vols.Volumes {
		kinds[v.Top] = v.Kind
	}
	if kinds[e.home] != proto.TrashHome || kinds[e.disk] != proto.TrashOwn || kinds[e.disk2] != proto.TrashOwn {
		t.Fatalf("volumes = %+v", vols)
	}

	// 관리자가 .Trash(1777)를 만들면 .Trash/<uid>를 쓴다(root 범위 연산. 테스트는 소유자로 실행한다).
	e.ok(nil, proto.ScopeRoot, proto.ActionTrashPrepare, proto.TrashPrepareParams{Disk: e.disk}, nil)
	if m := lmode(t, filepath.Join(e.disk, ".Trash")); m&os.ModeSticky == 0 || m.Perm() != 0o777 || !m.IsDir() {
		t.Fatalf(".Trash mode = %v", m)
	}
	e.file(filepath.Join(e.disk, "c.txt"), "c")
	e.ok(&res, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.disk, "c.txt")}, nil)
	if res.Trash != filepath.Join(e.disk, ".Trash", uid) {
		t.Fatalf("shared trash = %+v", res)
	}
	if m := lmode(t, res.Trash).Perm(); m != 0o700 {
		t.Fatalf(".Trash/<uid> mode = %o", m)
	}
	e.ok(&vols, proto.ScopeUser, proto.ActionTrashDirs, nil, nil)
	for _, v := range vols.Volumes {
		if v.Top == e.disk && (v.Kind != proto.TrashShared || !v.Shared) {
			t.Fatalf("disk after prepare = %+v", v)
		}
	}
}

func TestTrashPrepare(t *testing.T) {
	e := newEnv(t)
	run := func(disk string) (proto.Response, error) {
		resp, _, err := e.run(proto.ScopeRoot, proto.ActionTrashPrepare, proto.TrashPrepareParams{Disk: disk}, nil)
		return resp, err
	}
	// 디스크(/srv 아래 마운트 지점)만 받는다.
	for _, d := range []string{e.home, filepath.Join(e.disk, "sub"), filepath.Dir(e.disk), "/"} {
		if resp, _ := run(d); resp.OK || resp.Code != proto.CodeOutsideRoots {
			t.Fatalf("prepare %s = %+v", d, resp)
		}
	}
	// 있으면 권한만 고친다.
	if err := os.Mkdir(filepath.Join(e.disk, ".Trash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if resp, err := run(e.disk); !resp.OK || err != nil {
		t.Fatalf("repair = %+v %v", resp, err)
	}
	if m := lmode(t, filepath.Join(e.disk, ".Trash")); m&os.ModeSticky == 0 || m.Perm() != 0o777 {
		t.Fatalf("repaired mode = %v", m)
	}
	// 미리 둔 심볼릭 링크는 따라가지 않는다(대상 폴더의 권한을 바꾸지 않는다).
	target := filepath.Join(e.disk2, "private")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(e.disk2, ".Trash")); err != nil {
		t.Fatal(err)
	}
	if resp, _ := run(e.disk2); resp.OK || resp.Code != proto.CodeNotDir {
		t.Fatalf("symlink prepare = %+v", resp)
	}
	if m := lmode(t, target).Perm(); m != 0o700 {
		t.Fatalf("symlink target mode changed to %o", m)
	}
}

func TestTrashNone(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	e := newEnv(t)
	e.file(filepath.Join(e.disk2, "x.txt"), "x")
	if err := os.Chmod(e.disk2, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(e.disk2, 0o755) })
	// 맨 위에 쓸 수 없고 .Trash도 없으면 휴지통이 없다.
	e.fail(proto.CodeNoTrash, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.disk2, "x.txt")}, nil)
	var vols proto.TrashDirsResult
	e.ok(&vols, proto.ScopeUser, proto.ActionTrashDirs, nil, nil)
	for _, v := range vols.Volumes {
		if v.Top == e.disk2 && v.Kind != proto.TrashNone {
			t.Fatalf("read-only disk = %+v", v)
		}
	}
}

func TestTrashListRestorePurgeEmpty(t *testing.T) {
	e := newEnv(t)
	e.file(filepath.Join(e.home, "old.txt"), "old")
	e.file(filepath.Join(e.disk, "media", "sub", "new.txt"), "new")
	e.file(filepath.Join(e.disk, "media", "dir", "inner"), "i")
	e.at(time.Date(2026, 10, 1, 8, 0, 0, 0, time.Local))
	e.ok(nil, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.home, "old.txt")}, nil)
	e.at(time.Date(2026, 10, 2, 8, 0, 0, 0, time.Local))
	e.ok(nil, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.disk, "media", "sub", "new.txt")}, nil)
	e.at(time.Date(2026, 10, 3, 8, 0, 0, 0, time.Local))
	e.ok(nil, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.disk, "media", "dir")}, nil)
	own := filepath.Join(e.disk, ".Trash-"+strconv.Itoa(os.Getuid()))
	// 짝이 없는 trashinfo와 파일은 목록에서 뺀다.
	e.file(filepath.Join(own, "info", "orphan.trashinfo"), "[Trash Info]\nPath=orphan\nDeletionDate=2026-10-04T08:00:00\n")
	e.file(filepath.Join(own, "files", "lonely"), "")

	var vols proto.TrashDirsResult
	e.ok(&vols, proto.ScopeUser, proto.ActionTrashDirs, nil, nil)
	if vols.Items != 4 {
		t.Fatalf("trashinfo count = %d", vols.Items)
	}

	var list proto.TrashListResult
	e.ok(&list, proto.ScopeUser, proto.ActionTrashList, nil, nil)
	if len(list.Items) != 3 || list.Truncated {
		t.Fatalf("list = %+v", list)
	}
	dir, sub, old := list.Items[0], list.Items[1], list.Items[2]
	if dir.Name != "dir" || dir.Type != proto.TypeDir || dir.Path != filepath.Join(e.disk, "media", "dir") || dir.Trash != own ||
		dir.DeletedAt != time.Date(2026, 10, 3, 8, 0, 0, 0, time.Local).Unix() {
		t.Fatalf("dir item = %+v", dir)
	}
	if sub.Path != filepath.Join(e.disk, "media", "sub", "new.txt") || sub.Size != 3 || sub.Type != proto.TypeFile {
		t.Fatalf("sub item = %+v", sub)
	}
	if old.Path != filepath.Join(e.home, "old.txt") || old.Trash != filepath.Join(e.home, ".local/share/Trash") {
		t.Fatalf("home item = %+v", old)
	}
	MaxTrashItems = 2
	t.Cleanup(func() { MaxTrashItems = 10000 })
	e.ok(&list, proto.ScopeUser, proto.ActionTrashList, nil, nil)
	if len(list.Items) != 2 || !list.Truncated {
		t.Fatalf("capped list = %d %t", len(list.Items), list.Truncated)
	}
	MaxTrashItems = 10000

	// 되돌리기: 없어진 폴더는 만든다. 같은 이름이 있으면 exists, 다른 기준이면 cross_device다.
	var en proto.Entry
	e.ok(&en, proto.ScopeUser, proto.ActionTrashRestore, proto.TrashRestoreParams{Trash: own, Name: "new.txt", To: sub.Path}, nil)
	if en.Name != "new.txt" || readFile(t, sub.Path) != "new" {
		t.Fatalf("restored = %+v", en)
	}
	if _, err := os.Lstat(filepath.Join(own, "info", "new.txt.trashinfo")); !os.IsNotExist(err) {
		t.Fatalf("info left: %v", err)
	}
	e.file(filepath.Join(e.disk, "media", "dir"), "taken")
	e.fail(proto.CodeExists, proto.ActionTrashRestore, proto.TrashRestoreParams{Trash: own, Name: "dir", To: dir.Path}, nil)
	e.fail(proto.CodeCrossDevice, proto.ActionTrashRestore, proto.TrashRestoreParams{Trash: own, Name: "dir", To: filepath.Join(e.home, "dir")}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionTrashRestore, proto.TrashRestoreParams{Trash: own, Name: "dir", To: filepath.Join(own, "x")}, nil)
	e.fail(proto.CodeNotFound, proto.ActionTrashRestore, proto.TrashRestoreParams{Trash: filepath.Join(e.disk, "media"), Name: "dir", To: filepath.Join(e.disk, "d2")}, nil)
	e.fail(proto.CodeBadRequest, proto.ActionTrashRestore, proto.TrashRestoreParams{Trash: own, Name: "../x", To: filepath.Join(e.disk, "d2")}, nil)
	e.ok(&en, proto.ScopeUser, proto.ActionTrashRestore, proto.TrashRestoreParams{Trash: own, Name: "dir", To: filepath.Join(e.disk, "media", "dir 2")}, nil)
	if readFile(t, filepath.Join(e.disk, "media", "dir 2", "inner")) != "i" {
		t.Fatal("restore under a new name failed")
	}

	// 영구 삭제
	e.ok(nil, proto.ScopeUser, proto.ActionTrashPurge, proto.TrashItemParams{Trash: old.Trash, Name: "old.txt"}, nil)
	e.fail(proto.CodeNotFound, proto.ActionTrashPurge, proto.TrashItemParams{Trash: old.Trash, Name: "old.txt"}, nil)

	// 비우기는 모든 휴지통의 files·info를 비우고 휴지통 폴더는 둔다.
	e.file(filepath.Join(e.home, "again.txt"), "a")
	e.ok(nil, proto.ScopeUser, proto.ActionTrash, proto.PathParams{Path: filepath.Join(e.home, "again.txt")}, nil)
	e.ok(nil, proto.ScopeUser, proto.ActionTrashEmpty, nil, nil)
	e.ok(&list, proto.ScopeUser, proto.ActionTrashList, nil, nil)
	if len(list.Items) != 0 {
		t.Fatalf("after empty = %+v", list.Items)
	}
	for _, d := range []string{filepath.Join(own, "files"), filepath.Join(own, "info"), filepath.Join(e.home, ".local/share/Trash/files")} {
		ents, err := os.ReadDir(d)
		if err != nil || len(ents) != 0 {
			t.Fatalf("%s after empty: %v %v", d, ents, err)
		}
	}
}
