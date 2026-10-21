package files

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"sync"
	"time"
)

// Cache의 기본 상한. 이름·정보·보기 색인을 합쳐 이름 하나에 최대 약 100바이트이므로 전체 60만 개면 최악 약 60MB다.
const (
	DefaultMaxNames   = 600000
	DefaultPerSession = 4
	DefaultIdle       = 10 * time.Minute
)

// Cache는 대시보드 세션별 폴더 목록이다. 사용자 사이에 나누지 않는다(같은 폴더라도 볼 수 있는 것이 다를 수 있다).
// 세션별 신원(권한 도우미 id의 홈 폴더, 사용자·그룹 이름)도 함께 둔다.
type Cache struct {
	// MaxNames는 모든 목록의 이름 수 합의 상한이다. 넘으면 오래 쓰지 않은 목록부터 버린다.
	MaxNames int
	// PerSession은 세션 하나가 두는 목록 수다.
	PerSession int
	// Idle 동안 쓰지 않은 목록은 버린다.
	Idle time.Duration
	Now  func() time.Time

	mu       sync.Mutex
	listings map[string]*cached
	homes    map[string]Identity
}

type cached struct {
	session string
	l       *Listing
	used    time.Time
}

// NewCache는 기본 상한의 캐시다.
func NewCache() *Cache {
	return &Cache{MaxNames: DefaultMaxNames, PerSession: DefaultPerSession, Idle: DefaultIdle, Now: time.Now,
		listings: map[string]*cached{}, homes: map[string]Identity{}}
}

// NewID는 목록 ID(무작위 128비트)다.
func NewID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Add는 session의 목록을 둔다. 같은 경로의 이전 목록은 버린다.
func (c *Cache) Add(session string, l *Listing) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.Now()
	for id, e := range c.listings {
		if e.session == session && e.l.Path == l.Path {
			delete(c.listings, id)
		}
	}
	c.listings[l.ID] = &cached{session: session, l: l, used: now}
	c.evict(now, l.ID)
}

// evict는 유휴 목록과 상한을 넘는 목록을 버린다. keep은 버리지 않는다(방금 넣은 목록).
func (c *Cache) evict(now time.Time, keep string) {
	for id, e := range c.listings {
		if id != keep && now.Sub(e.used) > c.Idle {
			delete(c.listings, id)
		}
	}
	session := c.listings[keep].session
	for c.count(session) > c.PerSession {
		c.dropOldest(func(e *cached) bool { return e.session == session }, keep)
	}
	for c.names() > c.MaxNames && len(c.listings) > 1 {
		c.dropOldest(func(*cached) bool { return true }, keep)
	}
}

func (c *Cache) count(session string) int {
	n := 0
	for _, e := range c.listings {
		if e.session == session {
			n++
		}
	}
	return n
}

func (c *Cache) names() int {
	n := 0
	for _, e := range c.listings {
		n += e.l.Len()
	}
	return n
}

func (c *Cache) dropOldest(match func(*cached) bool, keep string) {
	oldest := ""
	for id, e := range c.listings {
		if id == keep || !match(e) {
			continue
		}
		if oldest == "" || e.used.Before(c.listings[oldest].used) {
			oldest = id
		}
	}
	if oldest == "" {
		// 버릴 수 있는 목록이 없다(keep 하나뿐).
		delete(c.listings, keep)
		return
	}
	delete(c.listings, oldest)
}

// Get은 session의 목록 id다. 다른 세션의 목록이거나 유휴 기간이 지났으면 없다.
func (c *Cache) Get(session, id string) (*Listing, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.listings[id]
	if !ok || e.session != session {
		return nil, false
	}
	now := c.Now()
	if now.Sub(e.used) > c.Idle {
		delete(c.listings, id)
		return nil, false
	}
	e.used = now
	return e.l, true
}

// Find는 session이 연 path의 목록이다.
func (c *Cache) Find(session, path string) (*Listing, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.Now()
	for id, e := range c.listings {
		if e.session != session || e.l.Path != path {
			continue
		}
		if now.Sub(e.used) > c.Idle {
			delete(c.listings, id)
			return nil, false
		}
		e.used = now
		return e.l, true
	}
	return nil, false
}

// Remove는 session의 목록 id를 버린다.
func (c *Cache) Remove(session, id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.listings[id]; ok && e.session == session {
		delete(c.listings, id)
	}
}

// Identity는 세션 사용자의 신원 가운데 파일 탐색기가 쓰는 것이다(권한 도우미 id).
type Identity struct {
	// Home은 홈 폴더 경로다(없으면 빈 값).
	Home string
	// User와 Groups는 Linux 사용자 이름과 속한 그룹 이름이다(공유 폴더 권한 따르기).
	User   string
	Groups []string
}

// SetIdentity는 session 사용자의 신원을 둔다. 로그인한 동안 바뀌지 않는 것으로 본다(그룹을 바꾸면 다시 로그인한다).
func (c *Cache) SetIdentity(session string, id Identity) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id.Groups = slices.Clone(id.Groups)
	c.homes[session] = id
}

// Identity는 SetIdentity로 둔 신원이다. 아직 모르면 false다.
func (c *Cache) Identity(session string) (Identity, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	id, ok := c.homes[session]
	id.Groups = slices.Clone(id.Groups)
	return id, ok
}

// RemoveSession은 session의 목록과 홈 폴더를 모두 버린다(세션이 끝날 때).
func (c *Cache) RemoveSession(session string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, e := range c.listings {
		if e.session == session {
			delete(c.listings, id)
		}
	}
	delete(c.homes, session)
}
