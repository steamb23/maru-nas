package daemon

import (
	"context"
	"math"
	"sync"
	"time"

	"onmaru/backend/internal/helper/proto"
)

// 비밀번호 확인 실패 제한. onmaru의 로그인 제한과 별개로, onmaru가 뚫려 비밀번호를 대입하는 경우를 막는다.
// unix_chkpwd는 pam_faillock을 거치지 않으므로 계정이 영구히 잠기지는 않는다.
const (
	failWindow = 5 * time.Minute
	failLimit  = 3
	lockBase   = 30 * time.Second
	lockMax    = 15 * time.Minute
	// levelReset이 지나도록 실패가 없으면 잠금 단계를 처음으로 되돌린다.
	levelReset = time.Hour
	// maxChecks는 동시에 실행하는 비밀번호 확인 수다(사용자 하나는 한 번에 하나).
	maxChecks = 4
	// maxTracked는 기록을 두는 사용자 수 상한이다.
	maxTracked = 10000
	// slotWait는 동시 확인 자리를 기다리는 최대 시간이다.
	slotWait = 5 * time.Second
)

// outcome은 비밀번호 확인 결과다.
type outcome int

const (
	// passed는 비밀번호가 맞았다는 뜻이다. 그 사용자의 실패 기록을 지운다.
	passed outcome = iota
	// failed는 비밀번호가 틀렸거나 계정이 만료됐다는 뜻이다. 실패로 센다.
	failed
	// unchecked는 확인 자체를 하지 못했다는 뜻이다(unix_chkpwd 실행 실패 등). 실패로 세지 않는다.
	// 세면 확인 도구가 고장 났을 때 정상 사용자까지 잠긴다.
	unchecked
)

type attempts struct {
	fails       []time.Time
	level       int
	lockedUntil time.Time
	lastFail    time.Time
	busy        bool
}

type limiter struct {
	mu    sync.Mutex
	now   func() time.Time
	users map[string]*attempts
	slots chan struct{}
}

func newLimiter(now func() time.Time) *limiter {
	return &limiter{now: now, users: make(map[string]*attempts), slots: make(chan struct{}, maxChecks)}
}

// begin은 user의 비밀번호 확인을 시작한다. 잠겨 있으면 rate_limited, 같은 사용자의 확인이
// 진행 중이거나 자리가 없으면 busy다. 확인이 끝나면 done(결과)을 반드시 부른다.
func (l *limiter) begin(ctx context.Context, user string) (done func(outcome), err error) {
	l.mu.Lock()
	now := l.now()
	a := l.users[user]
	if a == nil {
		l.pruneLocked(now)
		a = &attempts{}
		l.users[user] = a
	}
	if now.Before(a.lockedUntil) {
		wait := int(math.Ceil(a.lockedUntil.Sub(now).Seconds()))
		l.mu.Unlock()
		return nil, &proto.Error{Code: proto.CodeRateLimited, RetryAfter: wait}
	}
	if a.busy {
		l.mu.Unlock()
		return nil, proto.NewError(proto.CodeBusy)
	}
	a.busy = true
	l.mu.Unlock()

	release := func() {
		l.mu.Lock()
		a.busy = false
		l.mu.Unlock()
	}
	t := time.NewTimer(slotWait)
	defer t.Stop()
	select {
	case l.slots <- struct{}{}:
	case <-t.C:
		release()
		return nil, proto.NewError(proto.CodeBusy)
	case <-ctx.Done():
		release()
		return nil, proto.NewError(proto.CodeBusy)
	}
	return func(res outcome) {
		<-l.slots
		l.mu.Lock()
		defer l.mu.Unlock()
		a.busy = false
		switch res {
		case passed:
			delete(l.users, user)
			return
		case unchecked:
			return
		}
		now := l.now()
		if now.Sub(a.lastFail) > levelReset {
			a.level = 0
		}
		a.lastFail = now
		kept := a.fails[:0]
		for _, f := range a.fails {
			if now.Sub(f) < failWindow {
				kept = append(kept, f)
			}
		}
		a.fails = append(kept, now)
		if len(a.fails) >= failLimit {
			d := lockBase << a.level
			if d > lockMax || d <= 0 {
				d = lockMax
			}
			a.lockedUntil = now.Add(d)
			a.level++
			a.fails = nil
		}
	}, nil
}

// pruneLocked는 기록이 넘치면 잠기지 않았고 진행 중이 아닌 오래된 기록을 지운다.
func (l *limiter) pruneLocked(now time.Time) {
	if len(l.users) < maxTracked {
		return
	}
	for u, a := range l.users {
		if !a.busy && !now.Before(a.lockedUntil) && now.Sub(a.lastFail) > failWindow {
			delete(l.users, u)
		}
	}
	// 그래도 넘치면 잠기지 않은 기록부터 지운다.
	for u, a := range l.users {
		if len(l.users) < maxTracked {
			return
		}
		if !a.busy && !now.Before(a.lockedUntil) {
			delete(l.users, u)
		}
	}
}
