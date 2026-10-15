package auth

import (
	"slices"
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
	// maxEntries는 실패 기록 수 상한이다. 넘으면 오래된 기록부터 지운다.
	maxEntries = 10000

	// DeviceTTL은 신뢰 기기 토큰의 유효 기간이다. 로그인에 성공할 때마다 새로 발급한다.
	DeviceTTL = 30 * 24 * time.Hour
	// MaxDeviceFailures번 틀리면 그 신뢰 기기 토큰을 폐기한다. 토큰을 빼앗겨도
	// 사용자 이름별 잠금을 건너뛰어 비밀번호를 계속 대입하지 못하게 한다.
	MaxDeviceFailures = FreeFailures + 1
	// MaxDevices는 보관하는 신뢰 기기 토큰 수 상한이다. 넘치면 가장 먼저 만료될 토큰부터 지운다.
	MaxDevices = 10000
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
//
// 사용자 이름별 잠금은 IP와 무관하므로 누구나 남의 계정을 잠글 수 있다. 이를 막기 위해
// 로그인에 성공한 브라우저에 신뢰 기기 토큰(Trust)을 주고, 같은 사용자 이름으로 그 토큰을
// 보낸 시도는 사용자 이름별 잠금 대신 토큰별 실패 횟수로 제한한다. IP별 제한은 그대로 받는다.
type Limiter struct {
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time

	mu       sync.Mutex
	fails    map[string]*failRecord
	inflight map[string]int
	devices  map[string]*device
}

type failRecord struct {
	count int
	last  time.Time
	until time.Time
}

// device는 신뢰 기기 토큰 하나다.
type device struct {
	user    string // normUser로 정규화한 사용자 이름
	expires time.Time
	fails   int
}

// NewLimiter는 빈 Limiter를 만든다.
func NewLimiter() *Limiter {
	return &Limiter{
		Now:      time.Now,
		fails:    make(map[string]*failRecord),
		inflight: make(map[string]int),
		devices:  make(map[string]*device),
	}
}

func normUser(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

// attempt는 진행 중인 로그인 시도 하나다.
type attempt struct {
	ipKey, userKey string
	// token과 dev는 신뢰 기기로 인정된 시도일 때만 있다.
	token string
	dev   *device
}

// lockKeys는 잠금을 확인하고 실패를 세는 기록 키다. 신뢰 기기면 사용자 이름별 기록은 쓰지 않는다.
func (a attempt) lockKeys() []string {
	if a.dev != nil {
		return []string{a.ipKey}
	}
	return []string{a.ipKey, a.userKey}
}

// Begin은 로그인 시도를 시작한다. device는 브라우저가 보낸 신뢰 기기 토큰이며 없으면 빈 문자열이다.
// 잠겨 있으면 남은 시간(>0)을 반환하고 done은 nil이다.
// 허용되면 done을 반환하며, 시도가 끝나면 결과와 함께 반드시 한 번 불러야 한다.
func (l *Limiter) Begin(ip, username, device string) (done func(Outcome), retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	user := normUser(username)
	a := attempt{ipKey: "ip:" + ip, userKey: "user:" + user}
	if d := l.deviceLocked(device, now); d != nil && d.user == user {
		a.token, a.dev = device, d
	}
	for _, k := range a.lockKeys() {
		if r := l.fails[k]; r != nil && now.Before(r.until) {
			retryAfter = max(retryAfter, r.until.Sub(now))
		}
	}
	if retryAfter > 0 {
		return nil, retryAfter
	}
	if l.inflight[a.ipKey] >= MaxInflight {
		return nil, inflightRetry
	}
	l.inflight[a.ipKey]++

	var once sync.Once
	return func(o Outcome) {
		once.Do(func() { l.finish(a, o) })
	}, 0
}

func (l *Limiter) finish(a attempt, o Outcome) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inflight[a.ipKey]--; l.inflight[a.ipKey] <= 0 {
		delete(l.inflight, a.ipKey)
	}
	// 시도 중에 토큰이 폐기·재발급되었으면 그 토큰의 기록은 바꾸지 않는다.
	dev := a.dev
	if dev != nil && l.devices[a.token] != dev {
		dev = nil
	}
	switch o {
	case Success:
		// 맞는 비밀번호는 신뢰 기기 여부와 상관없이 IP·사용자 이름 기록을 모두 지운다.
		delete(l.fails, a.ipKey)
		delete(l.fails, a.userKey)
		if dev != nil {
			dev.fails = 0
		}
		return
	case Neutral:
		return
	}
	now := l.Now()
	for _, k := range a.lockKeys() {
		r := l.fails[k]
		if r == nil || now.Sub(r.last) > FailWindow {
			if r == nil && len(l.fails) >= maxEntries {
				l.evictLocked(now)
			}
			r = &failRecord{}
			l.fails[k] = r
		}
		r.count++
		r.last = now
		if r.count > FreeFailures {
			r.until = now.Add(lockout(r.count - FreeFailures))
		}
	}
	if dev != nil {
		if dev.fails++; dev.fails >= MaxDeviceFailures {
			delete(l.devices, a.token)
		}
	}
}

// Trust는 username으로 인증을 마친 브라우저에 줄 신뢰 기기 토큰을 새로 만든다.
// old는 브라우저가 갖고 있던 토큰이며, 있으면 폐기한다(브라우저 하나에 토큰 하나).
func (l *Limiter) Trust(username, old string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	delete(l.devices, old)
	if len(l.devices) >= MaxDevices {
		for t, d := range l.devices {
			if !now.Before(d.expires) {
				delete(l.devices, t)
			}
		}
	}
	if len(l.devices) >= MaxDevices {
		oldest := ""
		for t, d := range l.devices {
			if oldest == "" || d.expires.Before(l.devices[oldest].expires) {
				oldest = t
			}
		}
		delete(l.devices, oldest)
	}
	token := newID()
	l.devices[token] = &device{user: normUser(username), expires: now.Add(DeviceTTL)}
	return token
}

// deviceLocked는 만료되지 않은 신뢰 기기 토큰을 찾는다. 만료된 토큰은 지운다.
func (l *Limiter) deviceLocked(token string, now time.Time) *device {
	if token == "" {
		return nil
	}
	d := l.devices[token]
	if d == nil {
		return nil
	}
	if !now.Before(d.expires) {
		delete(l.devices, token)
		return nil
	}
	return d
}

// lockout은 n번째 잠금의 시간이다(1부터).
func lockout(n int) time.Duration {
	d := BaseLockout
	for i := 1; i < n && d < MaxLockout; i++ {
		d *= 2
	}
	return min(d, MaxLockout)
}

// evictLocked는 실패 기록을 maxEntries의 90%까지 줄인다. 기간이 지난 기록을 먼저 지우고,
// 그래도 많으면 잠기지 않은 기록, 잠긴 기록 순으로 마지막 실패가 오래된 것부터 지운다.
// 잠긴 기록을 지우면 그 잠금이 풀리지만, 그러려면 여러 IP에서 서로 다른 사용자 이름으로
// 수천 번 실패해야 한다. 기록이 한없이 늘어 메모리를 다 쓰는 것보다 낫다.
func (l *Limiter) evictLocked(now time.Time) {
	for k, r := range l.fails {
		if now.Sub(r.last) > FailWindow && !now.Before(r.until) {
			delete(l.fails, k)
		}
	}
	target := maxEntries * 9 / 10
	if len(l.fails) <= target {
		return
	}
	keys := make([]string, 0, len(l.fails))
	for k := range l.fails {
		keys = append(keys, k)
	}
	locked := func(k string) bool { return now.Before(l.fails[k].until) }
	slices.SortFunc(keys, func(a, b string) int {
		if la, lb := locked(a), locked(b); la != lb {
			if la {
				return 1
			}
			return -1
		}
		return l.fails[a].last.Compare(l.fails[b].last)
	})
	for _, k := range keys[:len(keys)-target] {
		delete(l.fails, k)
	}
}
