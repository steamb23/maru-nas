package client

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"onmaru/backend/internal/helper/helperfake"
	"onmaru/backend/internal/helper/proto"
)

func start(t *testing.T) (*Client, *helperfake.Server) {
	t.Helper()
	dir := t.TempDir()
	fake, err := helperfake.Start(filepath.Join(dir, "helper.sock"), filepath.Join(dir, "root"), helperfake.DefaultUsers(),
		helperfake.Options{FailDelay: -1, Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fake.Close)
	return New(fake.Socket), fake
}

func TestSessionFlow(t *testing.T) {
	c, _ := start(t)
	ctx := context.Background()
	if err := c.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Open(ctx, "totp", "wrong", false); proto.CodeOf(err) != proto.CodeAuthFailed {
		t.Fatalf("wrong password: %v", err)
	}
	res, err := c.Open(ctx, "totp", "totp", true)
	if err != nil || !res.CanElevate {
		t.Fatalf("open: %+v %v", res, err)
	}
	if _, err := c.Identity(ctx, res.Ticket, proto.ScopeUser); proto.CodeOf(err) != proto.CodeTicketInvalid {
		t.Fatalf("pending ticket ran: %v", err)
	}
	if err := c.Activate(ctx, res.Ticket); err != nil {
		t.Fatal(err)
	}
	id, err := c.Identity(ctx, res.Ticket, proto.ScopeUser)
	if err != nil || id.UID != 1002 || id.User != "totp" {
		t.Fatalf("identity: %+v %v", id, err)
	}
	if _, err := c.Identity(ctx, res.Ticket, proto.ScopeRoot); proto.CodeOf(err) != proto.CodeElevationRequired {
		t.Fatalf("root without elevation: %v", err)
	}
	el, err := c.Elevate(ctx, res.Ticket, "totp")
	if err != nil || el.ElevatedSeconds != 300 {
		t.Fatalf("elevate: %+v %v", el, err)
	}
	if id, err := c.Identity(ctx, res.Ticket, proto.ScopeRoot); err != nil || id.UID != 0 {
		t.Fatalf("root identity: %+v %v", id, err)
	}
	st, err := c.Status(ctx, res.Ticket)
	if err != nil || st.ElevatedSeconds != 300 || !st.CanElevate {
		t.Fatalf("status: %+v %v", st, err)
	}
	if err := c.Drop(ctx, res.Ticket); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(ctx, res.Ticket); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Status(ctx, res.Ticket); proto.CodeOf(err) != proto.CodeTicketInvalid {
		t.Fatalf("closed ticket: %v", err)
	}
}

func TestFileOps(t *testing.T) {
	c, fake := start(t)
	ctx := context.Background()
	res, err := c.Open(ctx, "user", "user", false)
	if err != nil {
		t.Fatal(err)
	}
	tk := res.Ticket
	home := fake.Home("user")
	content := bytes.Repeat([]byte("onmaru "), 100000)
	e, err := c.Write(ctx, tk, home+"/big.txt", false, bytes.NewReader(content), int64(len(content)))
	if err != nil || e.Size != int64(len(content)) {
		t.Fatalf("write: %+v %v", e, err)
	}
	if _, err := c.Write(ctx, tk, home+"/big.txt", false, strings.NewReader("x"), 1); proto.CodeOf(err) != proto.CodeExists {
		t.Fatalf("write existing: %v", err)
	}
	// 도우미가 내용을 받기 전에 거절해도 응답을 받는다.
	if _, err := c.Write(ctx, tk, "/etc/x", false, bytes.NewReader(content), int64(len(content))); proto.CodeOf(err) != proto.CodeOutsideRoots {
		t.Fatalf("write outside: %v", err)
	}
	// 내용이 모자라면 기다리지 않고 거절된다.
	start := time.Now()
	if _, err := c.Write(ctx, tk, home+"/short", false, strings.NewReader("abc"), 10); err == nil {
		t.Fatal("short upload accepted")
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("short upload waited for the idle timeout")
	}

	rr, body, err := c.Read(ctx, tk, home+"/big.txt", 7, 14)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	body.Close()
	if err != nil || string(got) != "onmaru onmaru " || rr.FileSize != int64(len(content)) || body.Size != 14 {
		t.Fatalf("read: %q %+v %v", got, rr, err)
	}
	if _, _, err := c.Read(ctx, tk, home+"/missing", 0, 0); proto.CodeOf(err) != proto.CodeNotFound {
		t.Fatalf("read missing: %v", err)
	}

	if _, err := c.Mkdir(ctx, tk, home+"/d"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Rename(ctx, tk, home+"/big.txt", home+"/d/moved.txt"); err != nil {
		t.Fatal(err)
	}
	if st, err := c.Stat(ctx, tk, home+"/d/moved.txt"); err != nil || st.Name != "moved.txt" {
		t.Fatalf("stat: %+v %v", st, err)
	}
	list, err := c.List(ctx, tk, home+"/d")
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := c.Remove(ctx, tk, home+"/d", false); proto.CodeOf(err) != proto.CodeNotEmpty {
		t.Fatalf("remove non-empty: %v", err)
	}
	if err := c.Remove(ctx, tk, home+"/d", true); err != nil {
		t.Fatal(err)
	}
	// 다른 사용자의 홈은 기준 밖이다.
	if _, err := c.List(ctx, tk, fake.Home("admin")); proto.CodeOf(err) != proto.CodeOutsideRoots {
		t.Fatalf("other home: %v", err)
	}
	if _, err := c.List(ctx, tk, fake.Disk()); err != nil {
		t.Fatalf("disk: %v", err)
	}
}

func TestUnavailable(t *testing.T) {
	c := New(filepath.Join(t.TempDir(), "missing.sock"))
	if _, err := c.Status(context.Background(), "x"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if proto.CodeOf(c.Reset(context.Background())) != "" {
		t.Fatal("transport error has a helper code")
	}
}
