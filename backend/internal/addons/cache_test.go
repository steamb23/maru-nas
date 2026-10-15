package addons

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestCache() (*SharedCache[int], *testClock) {
	clk := &testClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	c := NewSharedCache[int](5*time.Second, time.Minute)
	c.Now = clk.Now
	return c, clk
}

// counter는 부를 때마다 1, 2, 3...을 돌려주는 fetch다.
func counter() (func(context.Context) (int, error), *atomic.Int32) {
	var n atomic.Int32
	return func(context.Context) (int, error) { return int(n.Add(1)), nil }, &n
}

func mustGet(t *testing.T, c *SharedCache[int], force bool, fetch func(context.Context) (int, error)) int {
	t.Helper()
	v, err := c.Get(context.Background(), force, fetch)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSharedCacheTTL(t *testing.T) {
	c, clk := newTestCache()
	fetch, n := counter()
	if v := mustGet(t, c, false, fetch); v != 1 {
		t.Fatalf("first = %d", v)
	}
	clk.Add(4999 * time.Millisecond)
	if v := mustGet(t, c, false, fetch); v != 1 {
		t.Errorf("within ttl = %d, want cached 1", v)
	}
	clk.Add(time.Millisecond)
	if v := mustGet(t, c, false, fetch); v != 2 {
		t.Errorf("after ttl = %d, want 2", v)
	}
	if n.Load() != 2 {
		t.Errorf("fetch calls = %d", n.Load())
	}
}

func TestSharedCacheForceAndInvalidate(t *testing.T) {
	c, _ := newTestCache()
	fetch, _ := counter()
	mustGet(t, c, false, fetch)
	if v := mustGet(t, c, true, fetch); v != 2 {
		t.Errorf("force = %d, want 2", v)
	}
	c.Invalidate()
	if v := mustGet(t, c, false, fetch); v != 3 {
		t.Errorf("after invalidate = %d, want 3", v)
	}
}

func TestSharedCacheErrorNotCached(t *testing.T) {
	c, _ := newTestCache()
	boom := errors.New("boom")
	if _, err := c.Get(context.Background(), false, func(context.Context) (int, error) { return 0, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	fetch, _ := counter()
	if v := mustGet(t, c, false, fetch); v != 1 {
		t.Errorf("after error = %d, want fresh 1", v)
	}
}

// 동시에 들어온 요청은 호출 하나를 함께 기다린다. 처음 요청이 취소되어도 호출은 끝까지 간다.
func TestSharedCacheSingleFlight(t *testing.T) {
	c, _ := newTestCache()
	release := make(chan struct{})
	started := make(chan struct{})
	var calls atomic.Int32
	fetch := func(ctx context.Context) (int, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return 42, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderErr := make(chan error, 1)
	go func() {
		_, err := c.Get(leaderCtx, false, fetch)
		leaderErr <- err
	}()
	<-started

	const waiters = 5
	results := make(chan int, waiters)
	for range waiters {
		go func() {
			v, err := c.Get(context.Background(), false, fetch)
			if err != nil {
				t.Error(err)
			}
			results <- v
		}()
	}
	cancelLeader()
	if err := <-leaderErr; !errors.Is(err, context.Canceled) {
		t.Errorf("leader err = %v", err)
	}
	// 기다리는 요청이 모두 붙을 시간을 준다. 늦게 붙어도 진행 중인 호출이나 보관한 결과를 받는다.
	time.Sleep(20 * time.Millisecond)
	close(release)
	for range waiters {
		if v := <-results; v != 42 {
			t.Errorf("waiter got %d", v)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("fetch calls = %d, want 1", calls.Load())
	}
}

// Invalidate 전에 시작한 호출의 결과는 보관하지 않고, 그 뒤의 요청은 새로 부른다.
func TestSharedCacheInvalidateDuringFetch(t *testing.T) {
	c, _ := newTestCache()
	release := make(chan struct{})
	started := make(chan struct{})
	old := make(chan int, 1)
	go func() {
		v, _ := c.Get(context.Background(), false, func(context.Context) (int, error) {
			close(started)
			<-release
			return 1, nil
		})
		old <- v
	}()
	<-started
	c.Invalidate()

	fetch, _ := counter()
	if v := mustGet(t, c, false, fetch); v != 1 {
		t.Fatalf("after invalidate = %d, want new fetch result 1", v)
	}
	close(release)
	if v := <-old; v != 1 {
		t.Errorf("old caller got %d", v)
	}
	// 오래된 호출이 끝나도 새 결과를 덮어쓰지 않는다.
	fetch2 := func(context.Context) (int, error) { return 99, nil }
	if v := mustGet(t, c, false, fetch2); v != 1 {
		t.Errorf("cached = %d, want 1 from post-invalidate fetch", v)
	}
}
