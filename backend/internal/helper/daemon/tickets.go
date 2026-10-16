package daemon

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"sync"
	"time"
)

// ticket은 도우미가 비밀번호를 직접 확인하고 발급한 티켓 하나다.
type ticket struct {
	user string
	// uid는 발급할 때의 uid다. 쓸 때마다 계정이 그대로인지 비교한다.
	uid      int
	created  time.Time
	lastUsed time.Time
	// pendingUntil이 0이 아니면 2차 인증을 기다리는 티켓이고, 그때까지 activate해야 한다.
	pendingUntil time.Time
	// elevatedUntil까지 root 범위 연산을 쓸 수 있다.
	elevatedUntil time.Time
}

func (t ticket) pending() bool { return !t.pendingUntil.IsZero() }

// tickets는 메모리 티켓 저장소다. 도우미가 다시 시작하면 모두 사라진다.
// 티켓 값은 SHA-256으로만 보관하므로 저장소를 들여다봐도 쓸 수 있는 값이 나오지 않는다.
type tickets struct {
	mu   sync.Mutex
	m    map[[32]byte]*ticket
	now  func() time.Time
	idle time.Duration
	// pendingTTL은 2차 인증 대기 티켓이 activate를 기다리는 시간이다.
	pendingTTL time.Duration
	max        int
}

func newTickets(now func() time.Time, idle, pendingTTL time.Duration, max int) *tickets {
	return &tickets{m: make(map[[32]byte]*ticket), now: now, idle: idle, pendingTTL: pendingTTL, max: max}
}

func ticketKey(tok string) [32]byte { return sha256.Sum256([]byte(tok)) }

// issue는 새 티켓을 만들고 그 값을 돌려준다. 가득 차면 가장 오래 쓰지 않은 티켓을 지운다.
func (ts *tickets) issue(user string, uid int, pending bool) string {
	b := make([]byte, 32)
	rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	ts.mu.Lock()
	defer ts.mu.Unlock()
	now := ts.now()
	ts.pruneLocked(now)
	if len(ts.m) >= ts.max {
		var oldest [32]byte
		var found *ticket
		for k, t := range ts.m {
			if found == nil || t.lastUsed.Before(found.lastUsed) {
				oldest, found = k, t
			}
		}
		delete(ts.m, oldest)
	}
	t := &ticket{user: user, uid: uid, created: now, lastUsed: now}
	if pending {
		t.pendingUntil = now.Add(ts.pendingTTL)
	}
	ts.m[ticketKey(tok)] = t
	return tok
}

// lookup은 만료되지 않은 티켓을 찾고 마지막 사용 시각을 갱신한다. allowPending이 false면
// 2차 인증 대기 티켓은 없는 것으로 본다.
func (ts *tickets) lookup(tok string, allowPending bool) (ticket, bool) {
	if len(tok) != 43 {
		return ticket{}, false
	}
	ts.mu.Lock()
	defer ts.mu.Unlock()
	k := ticketKey(tok)
	t, ok := ts.m[k]
	if !ok {
		return ticket{}, false
	}
	now := ts.now()
	if ts.expired(t, now) {
		delete(ts.m, k)
		return ticket{}, false
	}
	if t.pending() && !allowPending {
		return ticket{}, false
	}
	t.lastUsed = now
	return *t, true
}

// activate는 2차 인증 대기 티켓을 쓸 수 있게 한다. 이미 쓸 수 있는 티켓이어도 성공이다.
func (ts *tickets) activate(tok string) bool {
	return ts.update(tok, func(t *ticket) { t.pendingUntil = time.Time{} })
}

// elevate는 티켓에 until까지 상승을 붙인다.
func (ts *tickets) elevate(tok string, until time.Time) bool {
	return ts.update(tok, func(t *ticket) { t.elevatedUntil = until })
}

// drop은 상승을 끝낸다.
func (ts *tickets) drop(tok string) bool {
	return ts.update(tok, func(t *ticket) { t.elevatedUntil = time.Time{} })
}

func (ts *tickets) update(tok string, fn func(*ticket)) bool {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	k := ticketKey(tok)
	t, ok := ts.m[k]
	if !ok {
		return false
	}
	now := ts.now()
	if ts.expired(t, now) {
		delete(ts.m, k)
		return false
	}
	fn(t)
	t.lastUsed = now
	return true
}

// close는 티켓을 지운다. 없으면 아무것도 하지 않는다.
func (ts *tickets) close(tok string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.m, ticketKey(tok))
}

// reset은 티켓을 모두 지우고 지운 수를 돌려준다.
func (ts *tickets) reset() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	n := len(ts.m)
	clear(ts.m)
	return n
}

// count는 만료되지 않은 티켓 수다.
func (ts *tickets) count() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.pruneLocked(ts.now())
	return len(ts.m)
}

func (ts *tickets) expired(t *ticket, now time.Time) bool {
	if t.pending() && now.After(t.pendingUntil) {
		return true
	}
	return now.Sub(t.lastUsed) > ts.idle
}

func (ts *tickets) pruneLocked(now time.Time) {
	for k, t := range ts.m {
		if ts.expired(t, now) {
			delete(ts.m, k)
		}
	}
}
