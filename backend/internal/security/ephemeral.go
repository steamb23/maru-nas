package security

import (
	"sync"
	"time"
)

// maxEphemeral은 Ephemeral 하나가 보관하는 값의 상한이다. 넘치면 가장 먼저 만료될 값부터 지운다.
const maxEphemeral = 1000

// Ephemeral은 세션에 묶인 짧은 수명의 값(패스키 challenge, 설정 중인 TOTP 비밀)이다. 메모리에만 둔다.
// 키는 부르는 쪽이 정한다(대시보드 세션 ID와 목적). 여러 고루틴에서 함께 써도 된다.
type Ephemeral[T any] struct {
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time
	ttl time.Duration

	mu sync.Mutex
	m  map[string]ephemeralEntry[T]
}

type ephemeralEntry[T any] struct {
	v       T
	expires time.Time
}

// NewEphemeral은 값마다 ttl 동안 두는 빈 저장소를 만든다.
func NewEphemeral[T any](ttl time.Duration) *Ephemeral[T] {
	return &Ephemeral[T]{Now: time.Now, ttl: ttl, m: make(map[string]ephemeralEntry[T])}
}

// Put은 key의 값을 v로 바꾼다(이전 값은 버린다).
func (e *Ephemeral[T]) Put(key string, v T) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.Now()
	for k, en := range e.m {
		if !now.Before(en.expires) {
			delete(e.m, k)
		}
	}
	if _, ok := e.m[key]; !ok && len(e.m) >= maxEphemeral {
		oldest := ""
		for k, en := range e.m {
			if oldest == "" || en.expires.Before(e.m[oldest].expires) {
				oldest = k
			}
		}
		delete(e.m, oldest)
	}
	e.m[key] = ephemeralEntry[T]{v: v, expires: now.Add(e.ttl)}
}

// Take는 key의 값을 꺼내 지운다(한 번만 쓸 수 있다). 없거나 만료됐으면 false다.
func (e *Ephemeral[T]) Take(key string) (T, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	en, ok := e.m[key]
	delete(e.m, key)
	if !ok || !e.Now().Before(en.expires) {
		var zero T
		return zero, false
	}
	return en.v, true
}

// TakeIf는 key의 값이 ok를 만족할 때만 꺼내 지운다. 확인과 지우기를 한 잠금 안에서 하므로 같은 값을 동시에
// 두 번 쓸 수 없다. 없거나 만료됐거나 ok가 false면 값을 그대로 두고 false다.
func (e *Ephemeral[T]) TakeIf(key string, ok func(T) bool) (T, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var zero T
	en, found := e.m[key]
	if !found || !e.Now().Before(en.expires) || !ok(en.v) {
		return zero, false
	}
	delete(e.m, key)
	return en.v, true
}

// Peek은 key의 값을 지우지 않고 본다.
func (e *Ephemeral[T]) Peek(key string) (T, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	en, ok := e.m[key]
	if !ok || !e.Now().Before(en.expires) {
		var zero T
		return zero, false
	}
	return en.v, true
}

// Delete는 key의 값을 지운다.
func (e *Ephemeral[T]) Delete(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.m, key)
}
