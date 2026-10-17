package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"onmaru/backend/internal/terminal/sshfake"
)

func TestManagerSharesClient(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{NoKeyboardInteractive: true})
	m := NewManager(e.dialer)
	ask := &answers{next: fixed("user", "user")}
	a := Auth{Ask: ask.ask}
	w1, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	w2, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	// 같은 대시보드 세션의 둘째 창은 같은 연결을 쓰고 다시 묻지 않는다.
	if ask.count() != 1 || e.sshd.Conns() != 1 || m.Clients() != 1 {
		t.Fatalf("asked %d, conns %d, clients %d", ask.count(), e.sshd.Conns(), m.Clients())
	}

	readUntil(t, w1, "$ ")
	w1.Write([]byte("whoami\r"))
	readUntil(t, w1, "user\r\n")
	readUntil(t, w2, "$ ")
	w2.Write([]byte("size\r"))
	readUntil(t, w2, "100x30")
	if err := w2.Resize(Size{Cols: 5000, Rows: 0}); err != nil {
		t.Fatal(err)
	}
	sizeEventually(t, w2, "1000x1")

	// 셸이 끝나면 종료 코드를 준다. 남은 창이 있으면 연결은 그대로다.
	w2.Write([]byte("exit 3\r"))
	if code, reason := w2.Wait(); code != 3 || reason != "" {
		t.Fatalf("wait = %d %q", code, reason)
	}
	w2.Close()
	if m.Clients() != 1 {
		t.Fatal("client closed while a window is open")
	}
	// 마지막 창을 닫으면 연결도 닫는다.
	w1.Close()
	if m.Clients() != 0 {
		t.Fatal("client kept after the last window")
	}
	waitFor(t, func() bool { return e.sshd.Conns() == 0 })

	// 다음 창은 다시 접속한다.
	w3, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	w3.Close()
	if ask.count() != 2 {
		t.Fatalf("asked %d times, want 2", ask.count())
	}
}

func TestManagerCloseSession(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{})
	m := NewManager(e.dialer)
	a := Auth{Keys: e.keys("user")}
	w, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.Open(context.Background(), "s2", "user", a, Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	m.CloseSession("s1", ReasonSessionEnded)
	if code, reason := w.Wait(); code != -1 || reason != ReasonSessionEnded {
		t.Fatalf("wait = %d %q", code, reason)
	}
	w.Close()
	if m.Clients() != 1 {
		t.Fatalf("clients = %d, want only the other session", m.Clients())
	}
	m.CloseAll(ReasonDisabled)
	if _, reason := other.Wait(); reason != ReasonDisabled {
		t.Fatalf("reason = %q", reason)
	}
}

func TestManagerCloseSessionWhileAsking(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{NoKeyboardInteractive: true})
	m := NewManager(e.dialer)
	asking := make(chan struct{})
	a := Auth{Ask: func(ctx context.Context, _ Challenge) ([]string, error) {
		close(asking)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	errc := make(chan error, 1)
	go func() {
		_, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
		errc <- err
	}()
	<-asking
	m.CloseSession("s1", ReasonSessionEnded)
	select {
	case err := <-errc:
		var de *Error
		if !errors.As(err, &de) || de.Code != CodeCanceled {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dial not canceled when the session ended")
	}
	if m.Clients() != 0 {
		t.Fatal("client kept")
	}
}

func TestManagerWindowLimit(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{})
	m := NewManager(e.dialer)
	a := Auth{Keys: e.keys("user")}
	var ws []*Window
	for range MaxWindows {
		w, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		ws = append(ws, w)
	}
	_, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeTooMany {
		t.Fatalf("err = %v, want too many windows", err)
	}
	for _, w := range ws {
		w.Close()
	}
}

func TestManagerReconnectsAfterServerDrop(t *testing.T) {
	e := newTestEnv(t, sshfake.Options{})
	m := NewManager(e.dialer)
	a := Auth{Keys: e.keys("user")}
	w, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	// sshd가 연결을 끊으면(재시작) 창은 connection_lost로 끝나고 클라이언트를 잊는다.
	e.sshd.DropConns()
	if _, reason := w.Wait(); reason != ReasonConnectionLost {
		t.Fatalf("reason = %q", reason)
	}
	waitFor(t, func() bool { return m.Clients() == 0 })
	w.Close()
	w2, err := m.Open(context.Background(), "s1", "user", a, Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	w2.Close()
}

// sizeEventually는 셸의 size 출력이 want가 될 때까지 다시 묻는다. 가짜 sshd는 window-change 요청과
// 입력을 다른 고루틴에서 처리하므로 바로 뒤의 입력이 먼저 처리될 수 있다.
func sizeEventually(t *testing.T, w *Window, want string) {
	t.Helper()
	for range 100 {
		w.Write([]byte("size\r"))
		if strings.Contains(readUntil(t, w, "$ "), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("size never became %s", want)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
