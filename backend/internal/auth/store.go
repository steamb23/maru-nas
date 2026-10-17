// Package auth는 대시보드 세션 저장소와 로그인 시도 제한을 둔다.
//
// 인증 자체는 OMV(Session.login)에 위임한다. 이 패키지는 계정·비밀번호를 다루지
// 않고, 로그인에 성공한 뒤의 대시보드 세션과 그에 묶인 OMV 세션 ID만 메모리에
// 보관한다. 백엔드를 재시작하면 모든 세션이 사라진다.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const (
	// PendingTTL은 2차 인증 대기 시간이다. OMV의 대기 중 로그인 유효 시간과 같다.
	PendingTTL = 5 * time.Minute
	// IdleTTL은 대시보드 세션이 요청 없이 남아 있을 수 있는 시간이다. 세션 유효성은
	// OMV 세션이 정하며, 이 값은 버려진 세션이 메모리에 쌓이지 않게 하는 상한이다
	// (OMV timeout을 0으로 꺼 둔 경우 포함).
	IdleTTL = 24 * time.Hour
	// MaxSessions는 동시에 보관하는 대시보드 세션 수 상한이다. 넘치면 가장 오래
	// 쓰지 않은 세션부터 지운다.
	MaxSessions = 1000
	// MaxPending은 동시에 보관하는 2차 인증 대기 수 상한이다.
	MaxPending = 100
)

// Assurance는 세션이 거친 확인의 수준이다(보안 강화 모드). 높을수록 강하며, 높은 수준은 낮은 수준의 조건도 만족한다.
// 세션의 속성이라 따로 만료가 없다(세션이 끝나면 함께 끝난다).
type Assurance int

const (
	// AssuranceLogin은 로그인만 한 세션이다. 로그인할 때 넣은 비밀번호는 재확인으로 치지 않는다(세션을 빼앗은 쪽도 이 수준이다).
	AssuranceLogin Assurance = iota
	// AssurancePassword는 세션 중에 비밀번호를 다시 확인한 세션이다.
	AssurancePassword
	// AssuranceTwoFactor는 2차 인증(패스키, TOTP, 복구 코드, OMV 2차 인증)을 거친 세션이다.
	AssuranceTwoFactor
)

// Session은 로그인한 대시보드 세션이다.
type Session struct {
	ID       string
	Username string
	Admin    bool
	// OMVSessionID는 이 세션이 OMV를 호출할 때 쓰는 세션 ID다. 로그·응답에 넣지 않는다.
	OMVSessionID string
	Created      time.Time
	LastSeen     time.Time
	// LastOMVCheck는 OMV 세션이 살아 있음을 마지막으로 확인한 시각이다.
	LastOMVCheck time.Time
	// LastTouch는 OMV 세션의 마지막 접근 시각을 갱신한 호출(로그인, 활동 신호)의 시각이다.
	// OMV 세션을 쓰는 것은 이 백엔드뿐이므로 LastTouch + Timeout이 OMV 세션 만료 시각이다.
	LastTouch time.Time
	// Timeout은 OMV 자동 로그아웃 시간이다. TimeoutKnown이 false면 모른다. 0이면 꺼져 있다.
	Timeout      time.Duration
	TimeoutKnown bool
	// HelperTicket은 권한 도우미가 이 세션에 발급한 티켓이다. 로그·응답에 넣지 않는다.
	// 비어 있으면 도우미를 쓸 수 없는 세션이다(도우미 미설정, 발급 실패).
	HelperTicket string
	// HelperLost는 티켓이 도우미에서 사라졌다는 표시다(도우미 재시작 등). 다시 로그인해야 한다.
	HelperLost bool
	// Assurance는 이 세션이 거친 확인의 수준이다. 올라가기만 한다(Raise).
	Assurance Assurance
	// ManageUntil은 인증 수단을 등록·삭제할 수 있는 관리 창이 끝나는 시각이다. 비밀번호와 등록한 수단을
	// 다시 확인하면 잠깐 열린다.
	ManageUntil time.Time
}

// Raise는 보증 수준을 a로 올린다. 이미 더 높으면 그대로 둔다.
func (s *Session) Raise(a Assurance) {
	s.Assurance = max(s.Assurance, a)
}

// ExpiresAt은 OMV 세션의 예상 만료 시각이다. 모르거나 자동 로그아웃이 꺼져 있으면 false다.
func (s Session) ExpiresAt() (time.Time, bool) {
	if !s.TimeoutKnown || s.Timeout <= 0 {
		return time.Time{}, false
	}
	return s.LastTouch.Add(s.Timeout), true
}

// Pending은 2차 인증을 기다리는 로그인이다.
type Pending struct {
	ID           string
	Username     string
	OMVSessionID string
	Expires      time.Time
	// HelperTicket은 2차 인증을 기다리는 도우미 티켓이다. 인증을 마치면 세션으로 옮긴다.
	HelperTicket string
}

// Store는 메모리 세션 저장소다. 여러 고루틴에서 함께 써도 된다.
// 꺼내는 값은 복사본이며, 바꾸려면 Store의 메서드를 쓴다.
type Store struct {
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time
	// OnRemove는 세션이 지워질 때(로그아웃, 만료, 밀려남, 유휴 정리) 잠금 밖에서 불린다.
	// OnRemovePending은 2차 인증 대기가 인증을 마치지 못하고 지워질 때 불린다. 둘 다 서비스 시작 전에 정한다.
	OnRemove        func(Session)
	OnRemovePending func(Pending)

	mu       sync.Mutex
	sessions map[string]*Session
	pending  map[string]*Pending
}

// NewStore는 빈 저장소를 만든다.
func NewStore() *Store {
	return &Store{
		Now:      time.Now,
		sessions: make(map[string]*Session),
		pending:  make(map[string]*Pending),
	}
}

// Create는 새 대시보드 세션을 만든다.
func (s *Store) Create(username string, admin bool, omvSessionID string) Session {
	var gone removed
	defer gone.notify(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	s.pruneLocked(now, &gone)
	if len(s.sessions) >= MaxSessions {
		var oldest *Session
		for _, ss := range s.sessions {
			if oldest == nil || ss.LastSeen.Before(oldest.LastSeen) {
				oldest = ss
			}
		}
		delete(s.sessions, oldest.ID)
		gone.sessions = append(gone.sessions, *oldest)
	}
	ss := &Session{
		ID:           newID(),
		Username:     username,
		Admin:        admin,
		OMVSessionID: omvSessionID,
		Created:      now,
		LastSeen:     now,
		LastOMVCheck: now,
		LastTouch:    now,
	}
	s.sessions[ss.ID] = ss
	return *ss
}

// Get은 세션을 찾고 마지막 사용 시각을 갱신한다.
func (s *Store) Get(id string) (Session, bool) {
	var gone removed
	defer gone.notify(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	now := s.Now()
	if now.Sub(ss.LastSeen) > IdleTTL {
		delete(s.sessions, id)
		gone.sessions = append(gone.sessions, *ss)
		return Session{}, false
	}
	ss.LastSeen = now
	return *ss, true
}

// Update는 세션을 fn으로 고치고 고친 결과를 반환한다. 세션이 없으면 false다.
// ID는 바꾸지 않는다.
func (s *Store) Update(id string, fn func(*Session)) (Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	fn(ss)
	ss.ID = id
	return *ss, true
}

// Delete는 세션을 지우고, 있었다면 지운 세션을 반환한다.
func (s *Store) Delete(id string) (Session, bool) {
	var gone removed
	defer gone.notify(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	delete(s.sessions, id)
	gone.sessions = append(gone.sessions, *ss)
	return *ss, true
}

// DeleteUser는 username의 세션과 2차 인증 대기를 모두 지우고 지운 세션 수를 돌려준다. 관리자가 그 사용자의
// 인증 수단을 초기화할 때 쓴다(이미 강화된 세션이 남지 않게).
func (s *Store) DeleteUser(username string) int {
	var gone removed
	defer gone.notify(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ss := range s.sessions {
		if ss.Username == username {
			delete(s.sessions, id)
			gone.sessions = append(gone.sessions, *ss)
		}
	}
	for id, p := range s.pending {
		if p.Username == username {
			delete(s.pending, id)
			gone.pending = append(gone.pending, *p)
		}
	}
	return len(gone.sessions)
}

// List는 보관 중인 세션의 복사본이다(순서 없음). 정책이 바뀌었을 때 세션마다 확인하는 데 쓴다.
func (s *Store) List() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0, len(s.sessions))
	for _, ss := range s.sessions {
		out = append(out, *ss)
	}
	return out
}

// Len은 보관 중인 세션 수다.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// CreatePending은 2차 인증 대기를 만든다. helperTicket은 함께 보관할 도우미 티켓이다(없으면 빈 값).
func (s *Store) CreatePending(username, omvSessionID, helperTicket string) Pending {
	var gone removed
	defer gone.notify(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	s.pruneLocked(now, &gone)
	if len(s.pending) >= MaxPending {
		var oldest *Pending
		for _, p := range s.pending {
			if oldest == nil || p.Expires.Before(oldest.Expires) {
				oldest = p
			}
		}
		delete(s.pending, oldest.ID)
		gone.pending = append(gone.pending, *oldest)
	}
	p := &Pending{ID: newID(), Username: username, OMVSessionID: omvSessionID, Expires: now.Add(PendingTTL), HelperTicket: helperTicket}
	s.pending[p.ID] = p
	return *p
}

// GetPending은 만료되지 않은 2차 인증 대기를 찾는다.
func (s *Store) GetPending(id string) (Pending, bool) {
	var gone removed
	defer gone.notify(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[id]
	if !ok {
		return Pending{}, false
	}
	if s.Now().After(p.Expires) {
		delete(s.pending, id)
		gone.pending = append(gone.pending, *p)
		return Pending{}, false
	}
	return *p, true
}

// DeletePending은 인증을 마치지 못한 2차 인증 대기를 지운다.
func (s *Store) DeletePending(id string) {
	var gone removed
	defer gone.notify(s)
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.pending[id]; ok {
		delete(s.pending, id)
		gone.pending = append(gone.pending, *p)
	}
}

// ClaimPending은 인증을 마친 2차 인증 대기를 꺼내 지운다. 도우미 티켓은 세션으로 옮겨 가므로
// OnRemovePending을 부르지 않는다.
func (s *Store) ClaimPending(id string) (Pending, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[id]
	if !ok {
		return Pending{}, false
	}
	delete(s.pending, id)
	return *p, true
}

func (s *Store) pruneLocked(now time.Time, gone *removed) {
	for id, ss := range s.sessions {
		if now.Sub(ss.LastSeen) > IdleTTL {
			delete(s.sessions, id)
			gone.sessions = append(gone.sessions, *ss)
		}
	}
	for id, p := range s.pending {
		if now.After(p.Expires) {
			delete(s.pending, id)
			gone.pending = append(gone.pending, *p)
		}
	}
}

// removed는 잠금 안에서 지운 세션·대기다. 잠금을 푼 뒤 notify로 훅을 부른다.
type removed struct {
	sessions []Session
	pending  []Pending
}

func (r *removed) notify(s *Store) {
	if f := s.OnRemove; f != nil {
		for _, ss := range r.sessions {
			f(ss)
		}
	}
	if f := s.OnRemovePending; f != nil {
		for _, p := range r.pending {
			f(p)
		}
	}
}

// newID는 추측할 수 없는 256비트 식별자를 만든다.
func newID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
