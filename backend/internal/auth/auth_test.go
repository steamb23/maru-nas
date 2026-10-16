package auth

import (
	"strconv"
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
	p := s.CreatePending("totp", "omv-p", "")
	got, ok := s.GetPending(p.ID)
	if !ok || got.Username != "totp" || got.OMVSessionID != "omv-p" {
		t.Fatalf("pending %+v %v", got, ok)
	}
	clk.Add(PendingTTL + time.Second)
	if _, ok := s.GetPending(p.ID); ok {
		t.Error("expired pending found")
	}
	q := s.CreatePending("totp", "omv-q", "")
	s.DeletePending(q.ID)
	if _, ok := s.GetPending(q.ID); ok {
		t.Error("deleted pending found")
	}
}

// 지워지는 세션·대기는 훅으로 알린다(도우미 티켓 폐기). 인증을 마쳐 꺼낸 대기는 알리지 않는다.
func TestStoreRemoveHooks(t *testing.T) {
	clk := newClock()
	s := newTestStore(clk)
	var sessions, pendings []string
	s.OnRemove = func(ss Session) {
		// 훅은 잠금 밖에서 불리므로 저장소를 다시 써도 된다.
		s.Len()
		sessions = append(sessions, ss.HelperTicket)
	}
	s.OnRemovePending = func(p Pending) { pendings = append(pendings, p.HelperTicket) }

	a := s.Create("a", false, "x")
	s.Update(a.ID, func(ss *Session) { ss.HelperTicket = "t-a" })
	s.Delete(a.ID)
	s.Delete(a.ID)

	b := s.Create("b", false, "x")
	s.Update(b.ID, func(ss *Session) { ss.HelperTicket = "t-b" })
	clk.Add(IdleTTL + time.Second)
	s.Get(b.ID)

	p := s.CreatePending("p", "x", "t-p")
	s.DeletePending(p.ID)
	q := s.CreatePending("q", "x", "t-q")
	clk.Add(PendingTTL + time.Second)
	s.GetPending(q.ID)
	r := s.CreatePending("r", "x", "t-r")
	if got, ok := s.ClaimPending(r.ID); !ok || got.HelperTicket != "t-r" {
		t.Fatalf("claim = %+v %v", got, ok)
	}
	if _, ok := s.ClaimPending(r.ID); ok {
		t.Fatal("claimed twice")
	}

	if want := []string{"t-a", "t-b"}; len(sessions) != 2 || sessions[0] != want[0] || sessions[1] != want[1] {
		t.Errorf("removed sessions = %q, want %q", sessions, want)
	}
	if want := []string{"t-p", "t-q"}; len(pendings) != 2 || pendings[0] != want[0] || pendings[1] != want[1] {
		t.Errorf("removed pendings = %q, want %q", pendings, want)
	}
}

func TestStoreCapNotifies(t *testing.T) {
	clk := newClock()
	s := newTestStore(clk)
	n := 0
	s.OnRemove = func(Session) { n++ }
	for range MaxSessions + 1 {
		clk.Add(time.Millisecond)
		s.Create("u", false, "x")
	}
	if n != 1 {
		t.Fatalf("evictions notified = %d", n)
	}
}

func fail(t *testing.T, l *Limiter, ip, user string) {
	t.Helper()
	done, wait := l.Begin(ip, user, "")
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
	if done, wait := l.Begin("192.0.2.1", "admin", ""); done != nil || wait != BaseLockout {
		t.Fatalf("want lockout %v, got %v", BaseLockout, wait)
	}
	// 같은 IP의 다른 사용자, 다른 IP의 같은 사용자도 막힌다.
	if done, _ := l.Begin("192.0.2.1", "user", ""); done != nil {
		t.Error("same ip not blocked")
	}
	if done, _ := l.Begin("192.0.2.2", "ADMIN ", ""); done != nil {
		t.Error("same user (normalized) not blocked")
	}

	clk.Add(BaseLockout)
	fail(t, l, "192.0.2.1", "admin")
	if _, wait := l.Begin("192.0.2.1", "admin", ""); wait != 2*BaseLockout {
		t.Errorf("second lockout %v", wait)
	}
}

func TestLimiterSuccessResets(t *testing.T) {
	clk := newClock()
	l := newTestLimiter(clk)
	for range FreeFailures {
		fail(t, l, "192.0.2.1", "admin")
	}
	done, _ := l.Begin("192.0.2.1", "admin", "")
	done(Success)
	for range FreeFailures {
		fail(t, l, "192.0.2.1", "admin")
	}
	if done, _ := l.Begin("192.0.2.1", "admin", ""); done == nil {
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
	if done, _ := l.Begin("192.0.2.1", "admin", ""); done == nil {
		t.Error("old failures should have been forgotten")
	}
}

func TestLimiterInflight(t *testing.T) {
	l := newTestLimiter(newClock())
	var dones []func(Outcome)
	for range MaxInflight {
		done, _ := l.Begin("192.0.2.1", "admin", "")
		if done == nil {
			t.Fatal("blocked below inflight limit")
		}
		dones = append(dones, done)
	}
	if done, wait := l.Begin("192.0.2.1", "other", ""); done != nil || wait <= 0 {
		t.Fatal("inflight limit not enforced")
	}
	dones[0](Neutral)
	dones[0](Neutral) // 두 번 불러도 한 번만 반영된다.
	if done, _ := l.Begin("192.0.2.1", "other", ""); done == nil {
		t.Error("slot not released")
	}
}

func TestLimiterNeutral(t *testing.T) {
	l := newTestLimiter(newClock())
	for range FreeFailures + 3 {
		done, _ := l.Begin("192.0.2.1", "admin", "")
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

// lockUser는 공격자 IP에서 username을 잠근다. 공격자 IP도 함께 잠기므로 부를 때마다 다른 IP를 쓴다.
func lockUser(t *testing.T, l *Limiter, username string) {
	t.Helper()
	attackers++
	ip := "198.51.100." + strconv.Itoa(attackers)
	for range FreeFailures + 1 {
		fail(t, l, ip, username)
	}
}

var attackers int

func TestLimiterTrustedDeviceSkipsUserLock(t *testing.T) {
	l := newTestLimiter(newClock())
	token := l.Trust("Admin", "")
	other := l.Trust("user", "")
	lockUser(t, l, "admin")

	if done, _ := l.Begin("192.0.2.1", "admin", ""); done != nil {
		t.Fatal("untrusted attempt not blocked")
	}
	if done, _ := l.Begin("192.0.2.1", "admin", "unknown-token"); done != nil {
		t.Fatal("unknown token treated as trusted")
	}
	// 다른 사용자의 토큰은 신뢰하지 않는다.
	if done, _ := l.Begin("192.0.2.1", "admin", other); done != nil {
		t.Fatal("token of another user treated as trusted")
	}
	done, wait := l.Begin("192.0.2.1", " ADMIN", token)
	if done == nil {
		t.Fatalf("trusted device blocked (%v)", wait)
	}
	done(Neutral)
	// 신뢰 기기라도 IP별 잠금은 받는다.
	if done, _ := l.Begin("198.51.100."+strconv.Itoa(attackers), "admin", token); done != nil {
		t.Fatal("trusted device bypassed ip lock")
	}
}

func TestLimiterTrustedFailuresStayOffUserRecord(t *testing.T) {
	l := newTestLimiter(newClock())
	token := l.Trust("admin", "")
	for i := range MaxDeviceFailures - 1 {
		done, _ := l.Begin("192.0.2."+strconv.Itoa(i+1), "admin", token)
		done(Failure)
	}
	// 신뢰 기기의 실패는 사용자 이름별 기록에 들어가지 않는다.
	fail(t, l, "192.0.2.100", "admin")
	if done, _ := l.Begin("192.0.2.101", "admin", ""); done == nil {
		t.Error("trusted failures counted toward the username lock")
	}
}

func TestLimiterDeviceRevokedAfterFailures(t *testing.T) {
	l := newTestLimiter(newClock())
	token := l.Trust("admin", "")
	lockUser(t, l, "admin")
	attempt := func(i int, o Outcome) {
		t.Helper()
		done, wait := l.Begin("192.0.2."+strconv.Itoa(i+1), "admin", token)
		if done == nil {
			t.Fatalf("attempt %d blocked (%v)", i, wait)
		}
		done(o)
	}
	for i := range MaxDeviceFailures - 1 {
		attempt(i, Failure)
	}
	attempt(10, Success) // 성공하면 기기 실패 횟수가 초기화된다.
	lockUser(t, l, "admin")
	for i := range MaxDeviceFailures {
		attempt(20+i, Failure)
	}
	if done, _ := l.Begin("192.0.2.200", "admin", token); done != nil {
		t.Error("token not revoked after repeated failures")
	}
}

func TestLimiterDeviceExpiryAndReplace(t *testing.T) {
	clk := newClock()
	l := newTestLimiter(clk)
	old := l.Trust("admin", "")
	token := l.Trust("admin", old)
	lockUser(t, l, "admin")
	if done, _ := l.Begin("192.0.2.1", "admin", old); done != nil {
		t.Error("replaced token still trusted")
	}
	if done, _ := l.Begin("192.0.2.1", "admin", token); done == nil {
		t.Fatal("new token not trusted")
	} else {
		done(Neutral)
	}
	clk.Add(DeviceTTL)
	lockUser(t, l, "admin")
	if done, _ := l.Begin("192.0.2.1", "admin", token); done != nil {
		t.Error("expired token still trusted")
	}
}

func TestLimiterDeviceCap(t *testing.T) {
	clk := newClock()
	l := newTestLimiter(clk)
	first := l.Trust("admin", "")
	for range MaxDevices {
		clk.Add(time.Second)
		l.Trust("user", "")
	}
	if len(l.devices) != MaxDevices {
		t.Errorf("devices %d, want %d", len(l.devices), MaxDevices)
	}
	if _, ok := l.devices[first]; ok {
		t.Error("oldest token not evicted")
	}
}

func TestLimiterEntriesBounded(t *testing.T) {
	l := newTestLimiter(newClock())
	lockUser(t, l, "admin")
	for i := range maxEntries {
		fail(t, l, "203.0.113."+strconv.Itoa(i), "spray"+strconv.Itoa(i))
	}
	if n := len(l.fails); n > maxEntries {
		t.Errorf("fail records %d, want <= %d", n, maxEntries)
	}
	// 잠기지 않은 기록부터 지우므로 잠긴 사용자는 그대로 잠겨 있다.
	if done, _ := l.Begin("192.0.2.1", "admin", ""); done != nil {
		t.Error("locked user evicted before unlocked records")
	}
}
