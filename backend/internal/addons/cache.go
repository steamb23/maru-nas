package addons

import (
	"context"
	"sync"
	"time"
)

// SharedCache는 Fetch 결과를 모든 세션이 짧게(TTL) 함께 쓰는 캐시다. OMV가 매번 프로세스를 실행해서 답하는
// 느린 RPC(Services.getStatus, Compose.getContainers)를 탭·사용자 수만큼 부르지 않게 한다.
//
//   - 결과가 사용자와 무관한 RPC에만 쓴다. 접근은 서버가 capability로 먼저 막는다
//   - 성공한 결과만 TTL 동안 보관한다. 실패는 보관하지 않는다
//   - 동시에 들어온 요청은 진행 중인 호출 하나를 함께 기다리고 결과(실패 포함)를 같이 받는다.
//     호출은 처음 요청한 사용자의 세션으로 나가며, 그 요청이 취소되어도 기다리는 요청을 위해 끝까지 간다
//   - force(새로고침)는 보관한 결과를 건너뛴다. 진행 중인 호출이 있으면 그것을 기다린다
//   - Invalidate(조작 뒤)는 보관한 결과를 버리고, 그 전에 시작한 호출의 결과도 보관하지 않게 한다
type SharedCache[T any] struct {
	// TTL은 결과를 보관하는 시간이다.
	TTL time.Duration
	// Timeout은 OMV 호출 하나의 제한 시간이다.
	Timeout time.Duration
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time

	mu       sync.Mutex
	val      T
	at       time.Time
	ok       bool
	gen      uint64
	inflight *sharedCall[T]
}

type sharedCall[T any] struct {
	done chan struct{}
	gen  uint64
	val  T
	err  error
}

// NewSharedCache는 ttl 동안 결과를 보관하는 캐시를 만든다.
func NewSharedCache[T any](ttl, timeout time.Duration) *SharedCache[T] {
	return &SharedCache[T]{TTL: ttl, Timeout: timeout, Now: time.Now}
}

// Get은 보관한 결과를 주거나 fetch로 새로 가져온다. force는 새로고침 요청이다.
func (c *SharedCache[T]) Get(ctx context.Context, force bool, fetch func(context.Context) (T, error)) (T, error) {
	c.mu.Lock()
	if !force && c.ok && c.Now().Sub(c.at) < c.TTL {
		v := c.val
		c.mu.Unlock()
		return v, nil
	}
	call := c.inflight
	if call == nil || call.gen != c.gen {
		call = &sharedCall[T]{done: make(chan struct{}), gen: c.gen}
		c.inflight = call
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.Timeout)
		go func() {
			defer cancel()
			v, err := fetch(fctx)
			c.finish(call, v, err)
		}()
	}
	c.mu.Unlock()

	select {
	case <-call.done:
		return call.val, call.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

func (c *SharedCache[T]) finish(call *sharedCall[T], val T, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	call.val, call.err = val, err
	if c.inflight == call {
		c.inflight = nil
	}
	if err == nil && call.gen == c.gen {
		c.val, c.at, c.ok = val, c.Now(), true
	}
	close(call.done)
}

// Invalidate는 보관한 결과를 버린다. 상태를 바꾸는 조작이 끝난 뒤(성공·실패 모두) 부른다.
func (c *SharedCache[T]) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	var zero T
	c.val, c.ok = zero, false
	c.gen++
}
