package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"maru/backend/internal/helper/proto"
)

// copyRun은 fs.copy를 실행하고 진행 응답과 마지막 응답, Serve의 오류를 돌려준다.
func (e *env) copyRun(p proto.CopyParams) ([]proto.CopyProgress, proto.Response, error) {
	e.t.Helper()
	client, server := socketPair(e.t)
	defer client.Close()
	job := e.job
	job.Action = proto.ActionCopy
	b, err := json.Marshal(p)
	if err != nil {
		e.t.Fatal(err)
	}
	job.Params = b
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(job, server, e.cfg)
		server.Close()
	}()
	var progress []proto.CopyProgress
	for {
		resp, err := proto.ReadResponse(client)
		if err != nil {
			e.t.Fatalf("read response: %v", err)
		}
		if !resp.Partial {
			return progress, resp, <-errc
		}
		var pr proto.CopyProgress
		if err := json.Unmarshal(resp.Result, &pr); err != nil {
			e.t.Fatal(err)
		}
		progress = append(progress, pr)
	}
}

// noParts는 dir에 복사 중 임시 이름이 남지 않았는지 확인한다.
func noParts(t *testing.T, dir string) {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, de := range des {
		if strings.HasPrefix(de.Name(), uploadPrefix) {
			t.Fatalf("temporary %s left in %s", de.Name(), dir)
		}
	}
}

func TestCopyAcrossDisks(t *testing.T) {
	e := newEnv(t)
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.Local)
	e.file(filepath.Join(e.home, "a.txt"), "hello")
	e.file(filepath.Join(e.home, "사진", "2024", "b.jpg"), "jpeg data")
	if err := os.Symlink("../a.txt", filepath.Join(e.home, "사진", "link")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(e.home, "사진", "fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.home, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"a.txt", "사진/2024/b.jpg", "사진/2024", "사진"} {
		if err := os.Chtimes(filepath.Join(e.home, p), old, old); err != nil {
			t.Fatal(err)
		}
	}
	to := filepath.Join(e.disk, "백업")
	if err := os.Mkdir(to, 0o755); err != nil {
		t.Fatal(err)
	}
	progress, resp, err := e.copyRun(proto.CopyParams{Dir: e.home, Names: []string{"a.txt", "사진", "run.sh"}, To: to})
	if err != nil || !resp.OK {
		t.Fatalf("copy: code %q, err %v", resp.Code, err)
	}
	var res proto.CopyResult
	if err := resp.Decode(&res); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Copied, []string{"a.txt", "사진", "run.sh"}) || len(res.Failed) != 0 || res.Skipped != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(progress) < 2 || !progress[0].Scanning || progress[len(progress)-1].Scanning {
		t.Fatalf("progress = %+v", progress)
	}
	last := progress[len(progress)-1]
	// a.txt, b.jpg, link, run.sh (fifo는 세지 않는다)
	if last.TotalFiles != 4 || last.Files != 4 || last.TotalBytes != int64(len("hello")+len("jpeg data")+len("#!/bin/sh\n")) ||
		last.Bytes != last.TotalBytes {
		t.Fatalf("last progress = %+v", last)
	}
	if readFile(t, filepath.Join(to, "a.txt")) != "hello" || readFile(t, filepath.Join(to, "사진", "2024", "b.jpg")) != "jpeg data" {
		t.Fatal("content differs")
	}
	if target, err := os.Readlink(filepath.Join(to, "사진", "link")); err != nil || target != "../a.txt" {
		t.Fatalf("link = %q, %v", target, err)
	}
	if _, err := os.Lstat(filepath.Join(to, "사진", "fifo")); !os.IsNotExist(err) {
		t.Fatalf("fifo copied: %v", err)
	}
	for _, p := range []string{"a.txt", "사진/2024/b.jpg", "사진/2024", "사진"} {
		fi, err := os.Stat(filepath.Join(to, p))
		if err != nil {
			t.Fatal(err)
		}
		if !fi.ModTime().Equal(old) {
			t.Fatalf("%s mtime = %v, want %v", p, fi.ModTime(), old)
		}
	}
	if lmode(t, filepath.Join(to, "run.sh"))&0o100 == 0 || lmode(t, filepath.Join(to, "a.txt"))&0o111 != 0 {
		t.Fatal("execute bit not kept")
	}
	// 원본은 그대로다.
	if readFile(t, filepath.Join(e.home, "a.txt")) != "hello" {
		t.Fatal("source changed")
	}
	noParts(t, to)
}

func TestCopyFailures(t *testing.T) {
	e := newEnv(t)
	e.file(filepath.Join(e.disk, "src", "a.txt"), "new")
	e.file(filepath.Join(e.disk, "src", "b.txt"), "b")
	e.file(filepath.Join(e.disk, "src", "d", "c.txt"), "c")
	e.file(filepath.Join(e.disk2, "dst", "a.txt"), "old")
	if err := syscall.Mkfifo(filepath.Join(e.disk, "src", "fifo"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, dst := filepath.Join(e.disk, "src"), filepath.Join(e.disk2, "dst")
	progress, resp, err := e.copyRun(proto.CopyParams{Dir: src, Names: []string{"a.txt", "missing", "fifo", "b.txt"}, To: dst})
	if err != nil || !resp.OK {
		t.Fatalf("copy: code %q, err %v", resp.Code, err)
	}
	// 대상에 이미 있는 a.txt는 세지 않는다.
	if last := progress[len(progress)-1]; last.TotalFiles != 1 || last.TotalBytes != 1 || last.Bytes != 1 {
		t.Fatalf("last progress = %+v", last)
	}
	var res proto.CopyResult
	if err := resp.Decode(&res); err != nil {
		t.Fatal(err)
	}
	want := []proto.CopyFailure{{Name: "a.txt", Code: proto.CodeExists}, {Name: "missing", Code: proto.CodeNotFound}, {Name: "fifo", Code: proto.CodeNotRegular}}
	if !slices.Equal(res.Copied, []string{"b.txt"}) || !slices.Equal(res.Failed, want) {
		t.Fatalf("result = %+v", res)
	}
	// 같은 이름은 덮어쓰지 않는다.
	if readFile(t, filepath.Join(dst, "a.txt")) != "old" {
		t.Fatal("existing file overwritten")
	}
	noParts(t, dst)

	// 시작하기 전의 실패는 오류 응답이다.
	for _, c := range []struct {
		code string
		p    proto.CopyParams
	}{
		{proto.CodeBadRequest, proto.CopyParams{Dir: src, To: dst}},
		{proto.CodeBadRequest, proto.CopyParams{Dir: src, Names: []string{"../x"}, To: dst}},
		// 폴더를 자기 안으로 복사하지 않는다.
		{proto.CodeBadRequest, proto.CopyParams{Dir: src, Names: []string{"d"}, To: filepath.Join(src, "d")}},
		{proto.CodeNotDir, proto.CopyParams{Dir: src, Names: []string{"b.txt"}, To: filepath.Join(dst, "a.txt")}},
		{proto.CodeNotFound, proto.CopyParams{Dir: src, Names: []string{"b.txt"}, To: filepath.Join(dst, "none")}},
		{proto.CodeOutsideRoots, proto.CopyParams{Dir: src, Names: []string{"b.txt"}, To: "/etc"}},
	} {
		progress, resp, err := e.copyRun(c.p)
		if len(progress) != 0 || resp.OK || resp.Code != c.code || proto.CodeOf(err) != c.code {
			t.Fatalf("%+v: progress %d, ok=%t code=%q err=%v, want %q", c.p, len(progress), resp.OK, resp.Code, err, c.code)
		}
	}
}

// 받는 쪽이 끊으면 하던 항목의 임시 이름을 지우고 멈춘다.
func TestCopyCancel(t *testing.T) {
	every, chunk := copyReportEvery, copyChunk
	copyReportEvery, copyChunk = 0, 1
	t.Cleanup(func() { copyReportEvery, copyChunk = every, chunk })
	e := newEnv(t)
	e.file(filepath.Join(e.home, "big.bin"), strings.Repeat("x", 1<<20))
	client, server := socketPair(t)
	job := e.job
	job.Action = proto.ActionCopy
	job.Params, _ = json.Marshal(proto.CopyParams{Dir: e.home, Names: []string{"big.bin"}, To: e.disk})
	errc := make(chan error, 1)
	go func() {
		errc <- Serve(job, server, e.cfg)
		server.Close()
	}()
	for {
		resp, err := proto.ReadResponse(client)
		if err != nil || !resp.Partial {
			t.Fatalf("response %+v, %v", resp, err)
		}
		var pr proto.CopyProgress
		json.Unmarshal(resp.Result, &pr)
		if pr.Bytes > 0 {
			break
		}
	}
	client.Close()
	if err := <-errc; Code(err) != proto.CodeCanceled {
		t.Fatalf("copy after the connection closed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(e.disk, "big.bin")); !os.IsNotExist(err) {
		t.Fatalf("partial copy visible: %v", err)
	}
	noParts(t, e.disk)
}
