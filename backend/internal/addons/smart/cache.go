package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
)

const (
	// ManualInterval은 새로고침으로 다시 가져올 수 있는 최소 간격이다.
	ManualInterval = time.Minute
	// RetryInterval은 실패한 뒤 다시 OMV를 부르기까지 기다리는 시간이다.
	RetryInterval = time.Minute
	// fetchTimeout은 Smart.getList 한 번의 제한 시간이다. 디스크마다 smartctl을 실행하므로 길게 둔다.
	fetchTimeout = 2 * time.Minute
)

// Snapshot은 Smart.getList 결과와 가져온 시각이다.
type Snapshot struct {
	Disks     []api.SmartDisk
	CheckedAt time.Time
}

// Cache는 Smart.getList 결과를 모든 세션이 함께 쓰도록 보관한다.
//
// Smart.getList는 smartctl을 --nocheck 없이 실행하므로 대기(스핀다운) 중인 디스크를 깨운다.
// 그래서 주기적으로 부르지 않고 다음 경우에만 OMV에 묻는다.
//   - 보관한 결과가 없다
//   - 보관한 결과가 가장 최근 기준 시각(하루 한 번, 기본 자정) 이전이다. 매일 첫 접속 때 건강 상태를 확인한다
//   - 새로고침을 요청했고 마지막으로 가져온 지 ManualInterval이 지났다
//
// 동시에 들어온 요청은 OMV 호출 하나를 함께 기다린다. 실패하면 RetryInterval 동안 다시 부르지 않는다.
// 결과는 사용자별이 아니며, 접근은 관리자 capability가 먼저 막는다.
type Cache struct {
	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time
	// RefreshHour, RefreshMinute는 하루 한 번 다시 가져오는 기준 시각(서버 로컬 시간)이다.
	RefreshHour, RefreshMinute int

	mu       sync.Mutex
	snap     *Snapshot
	failedAt time.Time
	lastErr  error
	inflight chan struct{}
}

// NewCache는 기준 시각이 hour:minute(서버 로컬 시간)인 캐시를 만든다.
func NewCache(hour, minute int) *Cache {
	return &Cache{Now: time.Now, RefreshHour: hour, RefreshMinute: minute}
}

// ParseRefreshAt은 "HH:MM" 형식의 기준 시각을 읽는다.
func ParseRefreshAt(s string) (hour, minute int, err error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, 0, fmt.Errorf("smart refresh time must be HH:MM, got %q", s)
	}
	return t.Hour(), t.Minute(), nil
}

// lastBoundary는 now 이전(같은 시각 포함)의 가장 최근 기준 시각이다.
func (c *Cache) lastBoundary(now time.Time) time.Time {
	b := time.Date(now.Year(), now.Month(), now.Day(), c.RefreshHour, c.RefreshMinute, 0, 0, now.Location())
	if now.Before(b) {
		b = b.AddDate(0, 0, -1)
	}
	return b
}

// Peek은 보관한 결과를 OMV에 묻지 않고 돌려준다. 온도 애드온이 디스크 온도를 더할 때 쓴다.
func (c *Cache) Peek() (Snapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.snap == nil {
		return Snapshot{}, false
	}
	return *c.snap, true
}

// RefreshIn은 snap을 가져온 뒤 새로고침으로 다시 가져올 수 있기까지 남은 시간이다.
func (c *Cache) RefreshIn(snap Snapshot) time.Duration {
	return max(0, ManualInterval-c.Now().Sub(snap.CheckedAt))
}

// Get은 규칙(Cache 설명)에 따라 보관한 결과를 주거나 rpc로 새로 가져온다. force는 새로고침 요청이다.
func (c *Cache) Get(ctx context.Context, rpc addons.RPCCaller, force bool) (Snapshot, error) {
	for {
		c.mu.Lock()
		now := c.Now()
		if wait := c.inflight; wait != nil {
			c.mu.Unlock()
			select {
			case <-wait:
				// 끝난 결과를 다시 확인한다. 같은 요청이 또 부르지 않도록 force는 끈다.
				force = false
				continue
			case <-ctx.Done():
				return Snapshot{}, ctx.Err()
			}
		}
		stale := c.snap == nil || c.snap.CheckedAt.Before(c.lastBoundary(now)) ||
			(force && now.Sub(c.snap.CheckedAt) >= ManualInterval)
		if !stale {
			snap := *c.snap
			c.mu.Unlock()
			return snap, nil
		}
		if !c.failedAt.IsZero() && now.Sub(c.failedAt) < RetryInterval {
			err := c.lastErr
			snap, ok := c.snap, c.snap != nil
			c.mu.Unlock()
			if ok {
				return *snap, nil
			}
			return Snapshot{}, err
		}
		done := make(chan struct{})
		c.inflight = done
		c.mu.Unlock()
		return c.fetch(ctx, rpc, done)
	}
}

func (c *Cache) fetch(ctx context.Context, rpc addons.RPCCaller, done chan struct{}) (Snapshot, error) {
	// 브라우저가 요청을 취소해도 기다리는 다른 요청을 위해 끝까지 가져온다.
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
	defer cancel()
	// OMV workbench의 SMART 대시보드 위젯과 같은 파라미터(limit -1은 전체)다.
	raw, err := rpc.Call(fctx, "Smart", "getList", map[string]any{"start": 0, "limit": -1})
	var disks []api.SmartDisk
	if err == nil {
		disks, err = ParseList(json.RawMessage(raw))
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.inflight = nil
	close(done)
	if err != nil {
		c.failedAt, c.lastErr = c.Now(), err
		if c.snap != nil {
			// 새로 가져오지 못해도 보관한 결과는 보여 준다.
			return *c.snap, nil
		}
		return Snapshot{}, err
	}
	c.failedAt, c.lastErr = time.Time{}, nil
	c.snap = &Snapshot{Disks: disks, CheckedAt: c.Now()}
	return *c.snap, nil
}
