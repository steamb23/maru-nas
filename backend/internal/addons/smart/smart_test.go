package smart

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"omv-desktop-dashboard/backend/internal/api"
)

const sampleList = `{"total": 3, "data": [
	{"devicename": "sda", "model": "Fake SSD", "size": "2000398934016", "temperature": 31,
	 "overallstatus": "GOOD", "monitor": true, "serialnumber": "FAKE-SERIAL-1", "wwn": "0x0"},
	{"devicename": "sdb", "model": "Fake HDD", "size": 8001563222016, "temperature": "",
	 "overallstatus": "BAD_SECTOR_MANY", "monitor": false, "serialnumber": "FAKE-SERIAL-2"},
	{"devicename": "sdc", "model": "Fake USB", "size": "x", "temperature": "40",
	 "overallstatus": "SOMETHING_NEW", "monitor": false}
]}`

func TestParseList(t *testing.T) {
	disks, err := ParseList(json.RawMessage(sampleList))
	if err != nil {
		t.Fatal(err)
	}
	if len(disks) != 3 {
		t.Fatalf("got %d disks", len(disks))
	}
	a, b, c := disks[0], disks[1], disks[2]
	if a.Device != "sda" || a.SizeBytes != 2000398934016 || a.Celsius == nil || *a.Celsius != 31 ||
		a.Status != api.SmartGood || !a.Monitored {
		t.Errorf("sda %+v", a)
	}
	if b.SizeBytes != 8001563222016 || b.Celsius != nil || b.Status != api.SmartFailing {
		t.Errorf("sdb %+v", b)
	}
	if c.SizeBytes != 0 || *c.Celsius != 40 || c.Status != api.SmartUnknown || c.OverallStatus != "SOMETHING_NEW" {
		t.Errorf("sdc %+v", c)
	}
	// 시리얼 번호는 응답에 남지 않는다.
	out, _ := json.Marshal(disks)
	if strings.Contains(string(out), "FAKE-SERIAL") {
		t.Error("serial number leaked")
	}
	if _, err := ParseList(json.RawMessage(`[]`)); err == nil {
		t.Error("want error")
	}
}

func TestSummarize(t *testing.T) {
	for in, want := range map[string]string{
		"GOOD": api.SmartGood, "BAD_ATTRIBUTE_IN_THE_PAST": api.SmartWarning, "BAD_SECTOR": api.SmartWarning,
		"BAD_ATTRIBUTE_NOW": api.SmartFailing, "BAD_SECTOR_MANY": api.SmartFailing, "BAD_STATUS": api.SmartFailing,
		"": api.SmartUnknown,
	} {
		if got := Summarize(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}

// fakeRPC는 Smart.getList 호출 수를 센다. block이 있으면 닫힐 때까지 응답하지 않는다.
type fakeRPC struct {
	calls atomic.Int32
	err   error
	block chan struct{}
}

func (f *fakeRPC) Call(_ context.Context, service, method string, params any) (json.RawMessage, error) {
	f.calls.Add(1)
	if service != "Smart" || method != "getList" {
		return nil, errors.New("unexpected method")
	}
	if p, _ := json.Marshal(params); string(p) != `{"limit":null,"start":0}` {
		return nil, errors.New("bad params " + string(p))
	}
	if f.block != nil {
		<-f.block
	}
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(sampleList), nil
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

func newTestCache(at time.Time) (*Cache, *testClock) {
	clk := &testClock{t: at}
	c := NewCache(0, 0)
	c.Now = clk.Now
	return c, clk
}

func at(day, hour, min int) time.Time { return time.Date(2026, 3, day, hour, min, 0, 0, time.Local) }

func TestCacheDailyRefresh(t *testing.T) {
	c, clk := newTestCache(at(10, 9, 0))
	rpc := &fakeRPC{}
	ctx := context.Background()

	if _, ok := c.Peek(); ok {
		t.Fatal("peek before first fetch")
	}
	snap, err := c.Get(ctx, rpc, false)
	if err != nil || len(snap.Disks) != 3 || !snap.CheckedAt.Equal(at(10, 9, 0)) {
		t.Fatalf("snap %+v err %v", snap, err)
	}
	// 같은 날에는 다시 부르지 않는다.
	clk.Set(at(10, 23, 59))
	c.Get(ctx, rpc, false)
	if n := rpc.calls.Load(); n != 1 {
		t.Fatalf("calls = %d, want 1", n)
	}
	// 기준 시각(자정)을 넘긴 뒤 첫 요청에서 다시 가져온다.
	clk.Set(at(11, 0, 0))
	c.Get(ctx, rpc, false)
	c.Get(ctx, rpc, false)
	if n := rpc.calls.Load(); n != 2 {
		t.Fatalf("calls = %d, want 2", n)
	}
	if snap, ok := c.Peek(); !ok || !snap.CheckedAt.Equal(at(11, 0, 0)) {
		t.Errorf("peek %+v", snap)
	}
}

func TestCacheCustomRefreshTime(t *testing.T) {
	c, clk := newTestCache(at(10, 5, 0))
	c.RefreshHour, c.RefreshMinute = 6, 30
	rpc := &fakeRPC{}
	c.Get(context.Background(), rpc, false)
	clk.Set(at(10, 6, 29))
	c.Get(context.Background(), rpc, false)
	clk.Set(at(10, 6, 30))
	c.Get(context.Background(), rpc, false)
	if n := rpc.calls.Load(); n != 2 {
		t.Errorf("calls = %d, want 2", n)
	}
}

func TestCacheManualRefreshLimit(t *testing.T) {
	c, clk := newTestCache(at(10, 9, 0))
	rpc := &fakeRPC{}
	ctx := context.Background()
	c.Get(ctx, rpc, false)
	clk.Set(at(10, 9, 0).Add(30 * time.Second))
	c.Get(ctx, rpc, true)
	if n := rpc.calls.Load(); n != 1 {
		t.Fatalf("refresh within a minute called omv (%d calls)", n)
	}
	clk.Set(at(10, 9, 1))
	c.Get(ctx, rpc, true)
	if n := rpc.calls.Load(); n != 2 {
		t.Fatalf("calls = %d, want 2", n)
	}
}

func TestCacheFailureBackoff(t *testing.T) {
	c, clk := newTestCache(at(10, 9, 0))
	rpc := &fakeRPC{err: errors.New("omv down")}
	ctx := context.Background()
	if _, err := c.Get(ctx, rpc, false); err == nil {
		t.Fatal("want error")
	}
	if _, err := c.Get(ctx, rpc, true); err == nil || rpc.calls.Load() != 1 {
		t.Fatalf("retried too soon (%d calls, err %v)", rpc.calls.Load(), err)
	}
	clk.Set(at(10, 9, 1))
	rpc.err = nil
	if _, err := c.Get(ctx, rpc, false); err != nil || rpc.calls.Load() != 2 {
		t.Fatalf("calls %d err %v", rpc.calls.Load(), err)
	}
	// 보관한 결과가 있으면 새로고침이 실패해도 그 결과를 준다.
	clk.Set(at(10, 9, 3))
	rpc.err = errors.New("omv down")
	if snap, err := c.Get(ctx, rpc, true); err != nil || !snap.CheckedAt.Equal(at(10, 9, 1)) {
		t.Errorf("snap %+v err %v", snap, err)
	}
}

func TestCacheCoalescesConcurrentRequests(t *testing.T) {
	c, _ := newTestCache(at(10, 9, 0))
	rpc := &fakeRPC{block: make(chan struct{})}
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Go(func() {
			_, err := c.Get(context.Background(), rpc, true)
			errs <- err
		})
	}
	// 첫 호출이 시작될 때까지 기다린 뒤 응답을 풀어 준다.
	for rpc.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(10 * time.Millisecond)
	close(rpc.block)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if n := rpc.calls.Load(); n != 1 {
		t.Errorf("calls = %d, want 1", n)
	}
}

func TestCacheWaiterHonoursContext(t *testing.T) {
	c, _ := newTestCache(at(10, 9, 0))
	rpc := &fakeRPC{block: make(chan struct{})}
	go c.Get(context.Background(), rpc, false)
	for rpc.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Get(ctx, rpc, false); !errors.Is(err, context.Canceled) {
		t.Errorf("err %v", err)
	}
	close(rpc.block)
}

func TestParseRefreshAt(t *testing.T) {
	if h, m, err := ParseRefreshAt("06:30"); err != nil || h != 6 || m != 30 {
		t.Errorf("%d:%d %v", h, m, err)
	}
	for _, bad := range []string{"24:00", "12:60", "noon", ""} {
		if _, _, err := ParseRefreshAt(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
