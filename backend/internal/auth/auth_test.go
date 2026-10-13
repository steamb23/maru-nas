package auth

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time        { return c.t }
func (c *fakeClock) Add(d time.Duration)   { c.t = c.t.Add(d) }
func newClock() *fakeClock                 { return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }
func newTestStore(c *fakeClock) *Store     { s := NewStore(); s.Now = c.Now; return s }
func newTestLimiter(c *fakeClock) *Limiter { l := NewLimiter(); l.Now = c.Now; return l }

func TestStoreSession(t *testing.T) {
	clk := newClock()
	s := newTestStore(clk)
	a := s.Create("admin", true, "omv-a")
	b := s.Create("user", false, "omv-b")
	if a.ID == b.ID || len(a.ID) < 40 {
		t.Fatalf("ids %q %q", a.ID, b.ID)
	}
	got, ok := s.Get(a.ID)
	if !ok || got.Username != "admin" || !got.Admin || got.OMVSessionID != "omv-a" {
		t.Fatalf("get %+v %v", got, ok)
	}

	if _, ok := got.ExpiresAt(); ok {
		t.Error("expiry known before timeout is known")
	}
	clk.Add(time.Minute)
	upd, ok := s.Update(a.ID, func(ss *Session) {
		ss.LastOMVCheck = clk.Now()
		ss.Timeout, ss.TimeoutKnown = 5*time.Minute, true
		ss.ID = "hijack"
	})
	if !ok || upd.ID != a.ID || !upd.LastOMVCheck.Equal(clk.Now()) {
		t.Errorf("update %+v %v", upd, ok)
	}
	if exp, ok := upd.ExpiresAt(); !ok || !exp.Equal(a.Created.Add(5*time.Minute)) {
		t.Errorf("expires at %v %v", exp, ok)
	}
	if _, ok := s.Update("nope", func(*Session) {}); ok {
		t.Error("update of unknown session")
	}

	if _, ok := s.Delete(a.ID); !ok {
		t.Error("delete")
	}
	if _, ok := s.Get(a.ID); ok {
		t.Error("deleted session still found")
	}
	if _, ok := s.Get("nope"); ok {
		t.Error("unknown id found")
	}
}

func TestStoreIdleExpiry(t *testing.T) {
	clk := newClock()
	s := newTestStore(clk)
	a := s.Create("admin", true, "x")
	clk.Add(IdleTTL + time.Second)
	if _, ok := s.Get(a.ID); ok {
		t.Error("idle session not expired")
	}
}

func TestStoreCap(t *testing.T) {
	clk := newClock()
	s := newTestStore(clk)
	first := s.Create("u", false, "x")
	for range MaxSessions {
		clk.Add(time.Millisecond)
		s.Create("u", false, "x")
	}
	if s.Len() != MaxSessions {
		t.Errorf("len %d", s.Len())
	}
	if _, ok := s.Get(first.ID); ok {
		t.Error("oldest session not evicted")
	}
}

func TestStorePending(t *testing.T) {
	clk := newClock()
	s := newTestStore(clk)
	p := s.CreatePending("totp", "omv-p")
	got, ok := s.GetPending(p.ID)
	if !ok || got.Username != "totp" || got.OMVSessionID != "omv-p" {
		t.Fatalf("pending %+v %v", got, ok)
	}
	clk.Add(PendingTTL + time.Second)
	if _, ok := s.GetPending(p.ID); ok {
		t.Error("expired pending found")
	}
	q := s.CreatePending("totp", "omv-q")
	s.DeletePending(q.ID)
	if _, ok := s.GetPending(q.ID); ok {
		t.Error("deleted pending found")
	}
}

func fail(t *testing.T, l *Limiter, ip, user string) {
	t.Helper()
	done, wait := l.Begin(ip, user)
	if done == nil {
		t.Fatalf("blocked unexpectedly (%v)", wait)
	}
	done(Failure)
}

func TestLimiterLockout(t *testing.T) {
	clk := newClock()
	l := newTestLimiter(clk)
	for range FreeFailures {
		fail(t, l, "192.0.2.1", "admin")
	}
	fail(t, l, "192.0.2.1", "admin") // 이번 실패부터 잠긴다.
	if done, wait := l.Begin("192.0.2.1", "admin"); done != nil || wait != BaseLockout {
		t.Fatalf("want lockout %v, got %v", BaseLockout, wait)
	}
	// 같은 IP의 다른 사용자, 다른 IP의 같은 사용자도 막힌다.
	if done, _ := l.Begin("192.0.2.1", "user"); done != nil {
		t.Error("same ip not blocked")
	}
	if done, _ := l.Begin("192.0.2.2", "ADMIN "); done != nil {
		t.Error("same user (normalized) not blocked")
	}

	clk.Add(BaseLockout)
	fail(t, l, "192.0.2.1", "admin")
	if _, wait := l.Begin("192.0.2.1", "admin"); wait != 2*BaseLockout {
		t.Errorf("second lockout %v", wait)
	}
}

func TestLimiterSuccessResets(t *testing.T) {
	clk := newClock()
	l := newTestLimiter(clk)
	for range FreeFailures {
		fail(t, l, "192.0.2.1", "admin")
	}
	done, _ := l.Begin("192.0.2.1", "admin")
	done(Success)
	for range FreeFailures {
		fail(t, l, "192.0.2.1", "admin")
	}
	if done, _ := l.Begin("192.0.2.1", "admin"); done == nil {
		t.Error("blocked after success reset")
	}
}

func TestLimiterWindow(t *testing.T) {
	clk := newClock()
	l := newTestLimiter(clk)
	for range FreeFailures {
		fail(t, l, "192.0.2.1", "admin")
	}
	clk.Add(FailWindow + time.Second)
	fail(t, l, "192.0.2.1", "admin")
	if done, _ := l.Begin("192.0.2.1", "admin"); done == nil {
		t.Error("old failures should have been forgotten")
	}
}

func TestLimiterInflight(t *testing.T) {
	l := newTestLimiter(newClock())
	var dones []func(Outcome)
	for range MaxInflight {
		done, _ := l.Begin("192.0.2.1", "admin")
		if done == nil {
			t.Fatal("blocked below inflight limit")
		}
		dones = append(dones, done)
	}
	if done, wait := l.Begin("192.0.2.1", "other"); done != nil || wait <= 0 {
		t.Fatal("inflight limit not enforced")
	}
	dones[0](Neutral)
	dones[0](Neutral) // 두 번 불러도 한 번만 반영된다.
	if done, _ := l.Begin("192.0.2.1", "other"); done == nil {
		t.Error("slot not released")
	}
}

func TestLimiterNeutral(t *testing.T) {
	l := newTestLimiter(newClock())
	for range FreeFailures + 3 {
		done, _ := l.Begin("192.0.2.1", "admin")
		if done == nil {
			t.Fatal("neutral outcomes must not lock")
		}
		done(Neutral)
	}
}

func TestLockoutCap(t *testing.T) {
	if got := lockout(100); got != MaxLockout {
		t.Errorf("lockout(100) = %v", got)
	}
}
