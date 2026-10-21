package files

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/helper/proto"
)

func TestCompareNames(t *testing.T) {
	got := []string{"b.txt", "a10.txt", "a2.txt", "A1.txt", "가나", "a02.txt", "Zeta", "alpha", "10", "9", "나다", "a2.txt.bak"}
	slices.SortFunc(got, CompareNames)
	want := []string{"9", "10", "A1.txt", "a02.txt", "a2.txt", "a2.txt.bak", "a10.txt", "alpha", "b.txt", "Zeta", "가나", "나다"}
	if !slices.Equal(got, want) {
		t.Fatalf("sorted = %q", got)
	}
	// 같은 글자이면 바이트 순서로 정한다(대문자가 앞).
	if CompareNames("ABC", "abc") >= 0 || CompareNames("abc", "abc") != 0 {
		t.Fatal("tie break")
	}
}

func TestPaths(t *testing.T) {
	for _, p := range []string{"/", "/srv", "/srv/disk1/사진"} {
		if !ValidPath(p) {
			t.Errorf("ValidPath(%q) = false", p)
		}
	}
	for _, p := range []string{"", "srv", "/srv/", "/srv/../etc", "/srv//x", "/a\x00b"} {
		if ValidPath(p) {
			t.Errorf("ValidPath(%q) = true", p)
		}
	}
	if !Within("/srv/disk1/a", "/srv/disk1") || !Within("/srv/disk1", "/srv/disk1") || Within("/srv/disk10", "/srv/disk1") {
		t.Fatal("Within")
	}
	if !Reachable("/srv/dev-disk-by-uuid-x/media", "/srv") || Reachable("/kvm", "/srv") || Reachable("/srv", "/srv") {
		t.Fatal("Reachable")
	}
}

func TestParseShares(t *testing.T) {
	// OMV 8.1 이상: absdirpath·status가 있다.
	raw := `[
		{"uuid":"u1","name":"미디어","reldirpath":"media/","absdirpath":"/srv/d1/media/","status":"OK","mntent":{"dir":"/srv/d1"}},
		{"uuid":"u2","name":"Backup","reldirpath":"backup/","absdirpath":"/srv/d1/backup/","status":"ENOENT","mntent":{"dir":"/srv/d1"}},
		{"uuid":"u3","name":"Lost","reldirpath":"lost/","absdirpath":"","status":"ENOENT_DB_MNTENT","mntent":{"dir":""}},
		{"uuid":"u4","name":"docs","reldirpath":"docs/","absdirpath":"/srv/d1/docs/","status":"OK","mntent":{"dir":"/srv/d1"}},
		{"uuid":"u5","name":"kvm","reldirpath":"kvm/","absdirpath":"/kvm/","status":"OK","mntent":{"dir":"/"}}
	]`
	got, err := ParseShares(json.RawMessage(raw))
	want := []api.SharedFolder{{UUID: "u4", Name: "docs", Path: "/srv/d1/docs"}, {UUID: "u5", Name: "kvm", Path: "/kvm"},
		{UUID: "u1", Name: "미디어", Path: "/srv/d1/media"}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("8.1+ = %+v %v", got, err)
	}
	// OMV 8.0: absdirpath·status가 없으므로 마운트 지점과 상대 경로로 만든다.
	raw = `[
		{"uuid":"u1","name":"media","reldirpath":"media/","mntent":{"dir":"/srv/d1"}},
		{"uuid":"u2","name":"nomount","reldirpath":"x/","mntent":{"dir":""}}
	]`
	got, err = ParseShares(json.RawMessage(raw))
	if err != nil || !slices.Equal(got, []api.SharedFolder{{UUID: "u1", Name: "media", Path: "/srv/d1/media"}}) {
		t.Fatalf("8.0 = %+v %v", got, err)
	}
	if _, err := ParseShares(json.RawMessage(`{"total":0}`)); err == nil {
		t.Fatal("object accepted")
	}
}

func names(list ...string) []proto.NameEntry {
	out := make([]proto.NameEntry, len(list))
	for i, s := range list {
		out[i] = proto.NameEntry{Type: s[0], Name: s[1:]}
	}
	return out
}

func pageNames(t *testing.T, l *Listing, v View, offset, limit int) ([]string, int) {
	t.Helper()
	idx, total, err := l.Page(v, offset, limit)
	if err != nil {
		t.Fatalf("page %+v: %v", v, err)
	}
	out := make([]string, len(idx))
	for k, i := range idx {
		out[k] = l.Name(i)
	}
	return out, total
}

func TestListingViews(t *testing.T) {
	l := NewListing("id", "/srv/d1/media", proto.NamesResult{Writable: true},
		names("fb.mkv", "dZeta", "fa10.txt", "da", "fa2.txt", "lalink", "?unknown"))
	got, total := pageNames(t, l, View{Sort: SortName}, 0, 100)
	if want := []string{"a", "Zeta", "a2.txt", "a10.txt", "alink", "b.mkv", "unknown"}; !slices.Equal(got, want) || total != 7 {
		t.Fatalf("name asc = %q (%d)", got, total)
	}
	got, _ = pageNames(t, l, View{Sort: SortName, Desc: true}, 0, 100)
	if want := []string{"Zeta", "a", "unknown", "b.mkv", "alink", "a10.txt", "a2.txt"}; !slices.Equal(got, want) {
		t.Fatalf("name desc = %q", got)
	}
	got, total = pageNames(t, l, View{Sort: SortName}, 2, 3)
	if want := []string{"a2.txt", "a10.txt", "alink"}; !slices.Equal(got, want) || total != 7 {
		t.Fatalf("range = %q", got)
	}
	if got, total := pageNames(t, l, View{Sort: SortName}, 10, 3); len(got) != 0 || total != 7 {
		t.Fatalf("past end = %q %d", got, total)
	}
	// 찾기는 대소문자를 무시한다.
	got, total = pageNames(t, l, View{Sort: SortName, Query: "A"}, 0, 100)
	if want := []string{"a", "Zeta", "a2.txt", "a10.txt", "alink"}; !slices.Equal(got, want) || total != 5 {
		t.Fatalf("query = %q (%d)", got, total)
	}

	// 이름의 자리는 보기 순서를 따른다(만든 폴더를 목록에서 고를 때).
	for _, c := range []struct {
		v    View
		name string
		want int
	}{
		{View{Sort: SortName}, "a2.txt", 2},
		{View{Sort: SortName, Desc: true}, "a2.txt", 6},
		{View{Sort: SortName, Query: "zet"}, "Zeta", 0},
		{View{Sort: SortName, Query: "zet"}, "a2.txt", -1},
		{View{Sort: SortName}, "none", -1},
	} {
		if got, err := l.IndexOf(c.v, c.name); err != nil || got != c.want {
			t.Fatalf("IndexOf(%+v, %q) = %d, %v; want %d", c.v, c.name, got, err, c.want)
		}
	}

	// 날짜·크기순은 모든 정보가 있어야 한다.
	if _, _, err := l.Page(View{Sort: SortMTime}, 0, 10); !errors.Is(err, ErrSortPending) {
		t.Fatalf("mtime before fill: %v", err)
	}
	if _, err := l.IndexOf(View{Sort: SortMTime}, "a"); !errors.Is(err, ErrSortPending) {
		t.Fatalf("IndexOf mtime before fill: %v", err)
	}
	next := l.NextUnfilled(100)
	if len(next) != 7 {
		t.Fatalf("unfilled = %v", next)
	}
	mtimes := map[string]int64{"b.mkv": 3, "Zeta": 1, "a10.txt": 2, "a": 5, "a2.txt": 2, "alink": 9, "unknown": 1}
	sizes := map[string]int64{"b.mkv": 300, "a10.txt": 10, "a2.txt": 20, "alink": 0, "unknown": 5}
	for _, i := range next {
		n := l.Name(i)
		st := Stat{Type: proto.TypeFile, ModTimeNs: mtimes[n] * int64(time.Second), Size: sizes[n]}
		if n == "unknown" {
			st = Stat{Code: proto.CodeNotFound}
		}
		l.SetStat(i, st)
	}
	if f, tot := l.Filled(); f != 7 || tot != 7 {
		t.Fatalf("filled = %d/%d", f, tot)
	}
	got, _ = pageNames(t, l, View{Sort: SortMTime}, 0, 100)
	if want := []string{"Zeta", "a", "unknown", "a2.txt", "a10.txt", "b.mkv", "alink"}; !slices.Equal(got, want) {
		t.Fatalf("mtime asc = %q", got)
	}
	got, _ = pageNames(t, l, View{Sort: SortSize, Desc: true}, 0, 100)
	if want := []string{"a", "Zeta", "b.mkv", "a2.txt", "a10.txt", "alink", "unknown"}; !slices.Equal(got, want) {
		t.Fatalf("size desc = %q", got)
	}
}

func TestListingItems(t *testing.T) {
	l := NewListing("id", "/x", proto.NamesResult{}, names("fa.txt", "dsub", "fgone", "fbad\xff", "?q"))
	idx, _, _ := l.Page(View{}, 0, 10)
	if un := l.Unfilled(idx); len(un) != 5 {
		t.Fatalf("unfilled = %v", un)
	}
	for _, i := range idx {
		switch l.Name(i) {
		case "a.txt":
			l.SetStat(i, Stat{Type: proto.TypeFile, Size: 5, ModTimeNs: 1_700_000_000_123_456_789})
		case "sub":
			l.SetStat(i, Stat{Type: proto.TypeDir, Locked: true})
		case "gone":
			l.SetStat(i, Stat{Code: proto.CodeNotFound})
		case "bad\xff":
			l.SetStat(i, Stat{Code: CodeBadName})
		}
	}
	items := map[string]api.FileItem{}
	for _, it := range l.Items(idx, false) {
		items[it.Name] = it
	}
	if it := items["a.txt"]; it.State != "ok" || it.Size != 5 || it.ModTime != 1_700_000_000_123 || it.Type != "file" {
		t.Fatalf("a.txt = %+v", it)
	}
	if it := items["sub"]; it.State != "ok" || !it.Locked || it.Type != "dir" {
		t.Fatalf("sub = %+v", it)
	}
	if it := items["gone"]; it.State != "gone" || it.Type != "file" {
		t.Fatalf("gone = %+v", it)
	}
	if it := items["bad�"]; !it.BadName || it.State != "error" {
		t.Fatalf("bad = %+v", items)
	}
	if it := items["q"]; it.State != "pending" || it.Type != "unknown" {
		t.Fatalf("unknown = %+v", it)
	}
	// 이름만 달라고 하면 정보를 넣지 않는다.
	for _, it := range l.Items(idx, true) {
		if it.State != "pending" || it.Size != 0 {
			t.Fatalf("names only = %+v", it)
		}
	}
}

func TestCache(t *testing.T) {
	now := time.Unix(1000, 0)
	c := NewCache()
	c.Now = func() time.Time { return now }
	c.PerSession = 2
	c.MaxNames = 10
	mk := func(id, path string, n int) *Listing {
		return NewListing(id, path, proto.NamesResult{}, make([]proto.NameEntry, n))
	}
	c.Add("s1", mk("a", "/a", 3))
	now = now.Add(time.Second)
	c.Add("s1", mk("b", "/b", 3))
	now = now.Add(time.Second)
	c.Add("s1", mk("c", "/c", 3))
	if _, ok := c.Get("s1", "a"); ok {
		t.Fatal("oldest listing kept over the per-session limit")
	}
	if _, ok := c.Get("s2", "b"); ok {
		t.Fatal("another session's listing returned")
	}
	if l, ok := c.Find("s1", "/c"); !ok || l.ID != "c" {
		t.Fatal("find")
	}
	// 같은 경로를 다시 열면 이전 목록을 버린다.
	c.Add("s1", mk("c2", "/c", 1))
	if _, ok := c.Get("s1", "c"); ok {
		t.Fatal("old listing of the same path kept")
	}
	// 전체 이름 수 상한
	now = now.Add(time.Second)
	c.Add("s2", mk("d", "/d", 8))
	if _, ok := c.Get("s1", "b"); ok {
		t.Fatal("oldest listing kept over the name limit")
	}
	if _, ok := c.Get("s2", "d"); !ok {
		t.Fatal("new listing dropped")
	}
	// 유휴
	now = now.Add(DefaultIdle + time.Second)
	if _, ok := c.Get("s2", "d"); ok {
		t.Fatal("idle listing returned")
	}
	c.SetHome("s1", "/home/u")
	if h, ok := c.Home("s1"); !ok || h != "/home/u" {
		t.Fatal("home")
	}
	c.RemoveSession("s1")
	if _, ok := c.Home("s1"); ok {
		t.Fatal("home kept after session removal")
	}
	if _, ok := c.Find("s1", "/c"); ok {
		t.Fatal("listing kept after session removal")
	}
}
