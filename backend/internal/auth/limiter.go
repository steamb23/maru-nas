package auth

import (
	"strings"
	"sync"
	"time"
)

const (
	// FailWindow 안의 실패만 센다. 마지막 실패에서 이만큼 지나면 기록을 지운다.
	FailWindow = 15 * time.Minute
	// FreeFailures번까지는 잠그지 않는다. 그 뒤로는 실패할 때마다 잠근다.
	FreeFailures = 4
	// BaseLockout은 첫 잠금 시간이다. 이후 실패할 때마다 두 배가 된다.
	BaseLockout = 30 * time.Second
	// MaxLockout은 잠금 시간 상한이다.
	MaxLockout = 15 * time.Minute
	// MaxInflight는 한 IP에서 동시에 진행할 수 있는 로그인 시도 수다.
	// 병렬 요청으로 잠금을 우회하지 못하게 한다.
	MaxInflight = 2
	// inflightRetry는 동시 시도 제한에 걸렸을 때 안내하는 재시도 시간이다.
	inflightRetry = time.Second
	// maxEntries를 넘으면 오래된 기록을 정리한다.
	maxEntries = 10000
)

// Outcome은 로그인 시도 하나의 결과다.
type Outcome int

const (
	// Success는 비밀번호가 맞았다는 뜻이다. 실패 기록을 지운다.
	Success Outcome = iota
	// Failure는 비밀번호·2차 인증 값이 틀렸다는 뜻이다. 실패로 센다.
	Failure
	// Neutral은 OMV 연결 실패처럼 판단할 수 없는 결과다. 기록을 바꾸지 않는다.
	Neutral
)

// Limiter는 클라이언트 IP별·사용자 이름별 로그인 실패를 세어 잠근다.
// 여러 고루틴에서 함께 써도 된다.
type Limiter struct {
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time

	mu       sync.Mutex
	fails    map[string]*failRecord
	inflight map[string]int
}

type failRecord struct {
	count int
	last  time.Time
	until time.Time
}

// NewLimiter는 빈 Limiter를 만든다.
func NewLimiter() *Limiter {
	return &Limiter{Now: time.Now, fails: make(map[string]*failRecord), inflight: make(map[string]int)}
}

func keys(ip, username string) []string {
	return []string{"ip:" + ip, "user:" + strings.ToLower(strings.TrimSpace(username))}
}

// Begin은 로그인 시도를 시작한다. 잠겨 있으면 남은 시간(>0)을 반환하고 done은 nil이다.
// 허용되면 done을 반환하며, 시도가 끝나면 결과와 함께 반드시 한 번 불러야 한다.
func (l *Limiter) Begin(ip, username string) (done func(Outcome), retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	ks := keys(ip, username)
	for _, k := range ks {
		if r := l.fails[k]; r != nil && now.Before(r.until) {
			retryAfter = max(retryAfter, r.until.Sub(now))
		}
	}
	if retryAfter > 0 {
		return nil, retryAfter
	}
	ipKey := ks[0]
	if l.inflight[ipKey] >= MaxInflight {
		return nil, inflightRetry
	}
	l.inflight[ipKey]++

	var once sync.Once
	return func(o Outcome) {
		once.Do(func() { l.finish(ks, o) })
	}, 0
}

func (l *Limiter) finish(ks []string, o Outcome) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[ks[0]]--; l.inflight[ks[0]] <= 0 {
		delete(l.inflight, ks[0])
	}
	switch o {
	case Success:
		for _, k := range ks {
			delete(l.fails, k)
		}
		return
	case Neutral:
		return
	}
	now := l.Now()
	if len(l.fails) >= maxEntries {
		l.pruneLocked(now)
	}
	for _, k := range ks {
		r := l.fails[k]
		if r == nil || now.Sub(r.last) > FailWindow {
			r = &failRecord{}
			l.fails[k] = r
		}
		r.count++
		r.last = now
		if r.count > FreeFailures {
			r.until = now.Add(lockout(r.count - FreeFailures))
		}
	}
}

// lockout은 n번째 잠금의 시간이다(1부터).
func lockout(n int) time.Duration {
	d := BaseLockout
	for i := 1; i < n && d < MaxLockout; i++ {
		d *= 2
	}
	return min(d, MaxLockout)
}

func (l *Limiter) pruneLocked(now time.Time) {
	for k, r := range l.fails {
		if now.Sub(r.last) > FailWindow && !now.Before(r.until) {
			delete(l.fails, k)
		}
	}
}
