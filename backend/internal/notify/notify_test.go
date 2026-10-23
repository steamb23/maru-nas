package notify

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"time"

	"maru/backend/internal/api"
)

// fakeSource는 정해 둔 결과를 돌려주고 불린 횟수를 센다.
type fakeSource struct {
	name    string
	admin   bool
	notices []api.Notice
	err     error
	calls   int
	// ctxErr는 마지막 Collect가 받은 컨텍스트의 Err다.
	ctxErr error
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) Admin() bool  { return f.admin }
func (f *fakeSource) Collect(ctx context.Context) ([]api.Notice, error) {
	f.calls++
	f.ctxErr = ctx.Err()
	return f.notices, f.err
}

// clock은 테스트가 움직이는 시계다.
type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

func newCenter(sources ...Source) (*Center, *clock) {
	clk := &clock{t: time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)}
	c := New(sources...)
	c.Now = clk.Now
	return c, clk
}

func notice(id, level string) api.Notice {
	return api.Notice{ID: id, Code: api.NoticeSample, Level: level, Revision: "r1"}
}

func ids(ns []api.Notice) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, n.ID)
	}
	return out
}

func TestNoSources(t *testing.T) {
	c, _ := newCenter()
	got := c.Notices(context.Background(), true)
	if got == nil || len(got) != 0 {
		t.Fatalf("notices = %#v, want empty non-nil", got)
	}
}

func TestCacheTTLAndInvalidate(t *testing.T) {
	src := &fakeSource{name: "a", notices: []api.Notice{notice("a", api.NoticeInfo)}}
	c, clk := newCenter(src)
	ctx := context.Background()

	c.Notices(ctx, false)
	c.Notices(ctx, false)
	if src.calls != 1 {
		t.Fatalf("calls within TTL = %d, want 1", src.calls)
	}
	clk.t = clk.t.Add(c.TTL)
	c.Notices(ctx, false)
	if src.calls != 2 {
		t.Fatalf("calls after TTL = %d, want 2", src.calls)
	}
	c.Invalidate()
	c.Notices(ctx, false)
	if src.calls != 3 {
		t.Fatalf("calls after Invalidate = %d, want 3", src.calls)
	}
}

func TestErrorKeepsLastResult(t *testing.T) {
	src := &fakeSource{name: "a", err: errors.New("read failed")}
	c, clk := newCenter(src)
	ctx := context.Background()

	// 처음부터 읽지 못하면 알릴 것이 없다.
	if got := c.Notices(ctx, true); len(got) != 0 {
		t.Fatalf("first error: %v", ids(got))
	}
	src.err, src.notices = nil, []api.Notice{notice("a", api.NoticeWarning)}
	clk.t = clk.t.Add(c.TTL)
	if got := ids(c.Notices(ctx, true)); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("after success: %v", got)
	}
	// 다시 읽지 못하면 직전 결과를 유지한다. 실패한 출처를 요청마다 다시 읽지는 않는다.
	src.err, src.notices = errors.New("read failed"), nil
	clk.t = clk.t.Add(c.TTL)
	if got := ids(c.Notices(ctx, true)); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("after error: %v", got)
	}
	calls := src.calls
	c.Notices(ctx, true)
	if src.calls != calls {
		t.Errorf("failing source retried within TTL")
	}
}

func TestAdminFilter(t *testing.T) {
	admin := &fakeSource{name: "admin", admin: true, notices: []api.Notice{notice("admin-only", api.NoticeWarning)}}
	all := &fakeSource{name: "all", notices: []api.Notice{notice("everyone", api.NoticeInfo)}}
	c, _ := newCenter(admin, all)
	ctx := context.Background()
	if got := ids(c.Notices(ctx, true)); !slices.Equal(got, []string{"admin-only", "everyone"}) {
		t.Errorf("admin sees %v", got)
	}
	if got := ids(c.Notices(ctx, false)); !slices.Equal(got, []string{"everyone"}) {
		t.Errorf("user sees %v", got)
	}
}

func TestOrder(t *testing.T) {
	src := &fakeSource{name: "a", notices: []api.Notice{
		notice("b-success", api.NoticeSuccess),
		notice("b-info", api.NoticeInfo),
		notice("a-info", api.NoticeInfo),
		notice("c-error", api.NoticeError),
		notice("d-warning", api.NoticeWarning),
	}}
	c, _ := newCenter(src)
	want := []string{"c-error", "d-warning", "a-info", "b-info", "b-success"}
	if got := ids(c.Notices(context.Background(), false)); !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestValidation(t *testing.T) {
	two := 2
	neg := -1
	good := notice("good", api.NoticeInfo)
	good.Count, good.Since = &two, "2026-10-23T12:00:00Z"
	good.Action = &api.NoticeAction{Kind: api.NoticeActionWorkbench, Route: "system/updatemgmt/updates"}
	app := notice("app", api.NoticeInfo)
	app.Action = &api.NoticeAction{Kind: api.NoticeActionApp, App: "settings", Page: "update"}
	power := notice("power", api.NoticeWarning)
	power.Action = &api.NoticeAction{Kind: api.NoticeActionPower}

	bad := func(id string, edit func(*api.Notice)) api.Notice {
		n := notice(id, api.NoticeInfo)
		edit(&n)
		return n
	}
	src := &fakeSource{name: "a", notices: []api.Notice{
		good, app, power,
		bad("Bad ID", func(*api.Notice) {}),
		bad("code", func(n *api.Notice) { n.Code = "nope" }),
		bad("level", func(n *api.Notice) { n.Level = "fatal" }),
		bad("revision", func(n *api.Notice) { n.Revision = "" }),
		bad("revision-chars", func(n *api.Notice) { n.Revision = "a b" }),
		bad("since", func(n *api.Notice) { n.Since = "yesterday" }),
		bad("count", func(n *api.Notice) { n.Count = &neg }),
		bad("action-kind", func(n *api.Notice) { n.Action = &api.NoticeAction{Kind: "shell"} }),
		bad("action-app", func(n *api.Notice) { n.Action = &api.NoticeAction{Kind: api.NoticeActionApp, App: "../x"} }),
		bad("action-route", func(n *api.Notice) {
			n.Action = &api.NoticeAction{Kind: api.NoticeActionWorkbench, Route: "//evil.example"}
		}),
		bad("action-route-abs", func(n *api.Notice) {
			n.Action = &api.NoticeAction{Kind: api.NoticeActionWorkbench, Route: "/system"}
		}),
		bad("action-power-extra", func(n *api.Notice) {
			n.Action = &api.NoticeAction{Kind: api.NoticeActionPower, Route: "x"}
		}),
		notice("good", api.NoticeError), // 같은 출처 안에서 겹치는 id
	}}
	c, _ := newCenter(src)
	got := ids(c.Notices(context.Background(), true))
	if want := []string{"power", "app", "good"}; !slices.Equal(got, want) {
		t.Errorf("accepted = %v, want %v", got, want)
	}
}

func TestDuplicateAcrossSources(t *testing.T) {
	first := &fakeSource{name: "first", admin: true, notices: []api.Notice{notice("same", api.NoticeWarning)}}
	second := &fakeSource{name: "second", notices: []api.Notice{notice("same", api.NoticeInfo), notice("other", api.NoticeInfo)}}
	c, _ := newCenter(first, second)
	got := c.Notices(context.Background(), true)
	if len(got) != 2 || got[0].ID != "same" || got[0].Level != api.NoticeWarning || got[1].ID != "other" {
		t.Errorf("notices = %+v", got)
	}
}

func TestLimits(t *testing.T) {
	var many []api.Notice
	for i := range maxPerSource + 5 {
		n := notice(fmt.Sprintf("n%02d", i), api.NoticeInfo)
		for j := range maxItems + 3 {
			n.Items = append(n.Items, fmt.Sprintf("item%d", j))
		}
		many = append(many, n)
	}
	c, _ := newCenter(&fakeSource{name: "a", notices: many})
	got := c.Notices(context.Background(), false)
	if len(got) != maxPerSource {
		t.Fatalf("notices = %d, want %d", len(got), maxPerSource)
	}
	if len(got[0].Items) != maxItems {
		t.Errorf("items = %d, want %d", len(got[0].Items), maxItems)
	}
}

func TestReturnsCopies(t *testing.T) {
	n := notice("a", api.NoticeInfo)
	count := 3
	n.Count, n.Items = &count, []string{"x"}
	n.Action = &api.NoticeAction{Kind: api.NoticeActionPower}
	src := &fakeSource{name: "a", notices: []api.Notice{n}}
	c, _ := newCenter(src)

	// 출처가 준 값을 나중에 바꿔도 보관한 값은 그대로다.
	got := c.Notices(context.Background(), false)
	*src.notices[0].Count, src.notices[0].Items[0] = 9, "changed"
	// 돌려준 값을 바꿔도 보관한 값은 그대로다.
	*got[0].Count, got[0].Items[0], got[0].Action.Kind = 7, "changed", "app"

	again := c.Notices(context.Background(), false)[0]
	if *again.Count != 3 || again.Items[0] != "x" || again.Action.Kind != api.NoticeActionPower {
		t.Errorf("stored notice changed: %+v", again)
	}
}

func TestCollectSurvivesCanceledRequest(t *testing.T) {
	src := &fakeSource{name: "a", notices: []api.Notice{notice("a", api.NoticeInfo)}}
	c, _ := newCenter(src)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := ids(c.Notices(ctx, false)); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("notices = %v", got)
	}
	if src.ctxErr != nil {
		t.Errorf("source got a canceled context: %v", src.ctxErr)
	}
}

func TestRevision(t *testing.T) {
	a := Revision("pkg-a@1", "pkg-b@2")
	if b := Revision("pkg-b@2", "pkg-a@1"); a != b {
		t.Errorf("order changed revision: %s vs %s", a, b)
	}
	if b := Revision("pkg-a@1", "pkg-b@3"); a == b {
		t.Errorf("different content, same revision %s", a)
	}
	// 구분자가 없으면 이어 붙인 결과가 같아질 수 있다.
	if Revision("ab", "c") == Revision("a", "bc") {
		t.Errorf("parts are not separated")
	}
	if !ValidRevision(a) || !ValidRevision(Revision()) {
		t.Errorf("revision %q is not valid", a)
	}
}

func TestSeenFor(t *testing.T) {
	seen := map[string]string{"a": "r1", "gone": "r9"}
	got := SeenFor(seen, []api.Notice{notice("a", api.NoticeInfo), notice("b", api.NoticeInfo)})
	if !maps.Equal(got, map[string]string{"a": "r1"}) {
		t.Errorf("seen = %v", got)
	}
	if got := SeenFor(nil, nil); got == nil {
		t.Errorf("SeenFor(nil) = nil, want empty map")
	}
}

// TestCodesMatchTSType은 검사가 받는 코드와 프론트 타입(tstype)의 코드가 같은지 확인한다.
// 코드를 더할 때 한쪽만 고치면 프론트가 모르는 코드를 받거나 쓰지 않는 문구가 남는다.
func TestCodesMatchTSType(t *testing.T) {
	f, _ := reflect.TypeOf(api.Notice{}).FieldByName("Code")
	var fromTag []string
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(f.Tag.Get("tstype"), -1) {
		fromTag = append(fromTag, m[1])
	}
	slices.Sort(fromTag)
	known := slices.Sorted(maps.Keys(knownCodes))
	if !slices.Equal(fromTag, known) {
		t.Errorf("tstype codes %v, knownCodes %v", fromTag, known)
	}
}
