package files

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"maru/backend/internal/api"
	"maru/backend/internal/helper/proto"
)

// SortKey는 보기의 정렬 기준이다.
type SortKey string

const (
	SortName  SortKey = "name"
	SortMTime SortKey = "mtime"
	SortSize  SortKey = "size"
)

// ParseSortKey는 브라우저가 보낸 정렬 기준이다. 빈 값은 이름순이다.
func ParseSortKey(s string) (SortKey, bool) {
	switch SortKey(s) {
	case "", SortName:
		return SortName, true
	case SortMTime, SortSize:
		return SortKey(s), true
	}
	return "", false
}

// View는 목록을 보는 방식이다. Query가 있으면 이름에 그 글자가 든(대소문자 무시) 항목만 본다.
type View struct {
	Sort  SortKey
	Desc  bool
	Query string
}

// ErrSortPending은 날짜·크기순 보기에 필요한 항목 정보를 아직 다 읽지 않았다는 뜻이다.
var ErrSortPending = errors.New("files: entry info not loaded yet")

// maxViews는 목록 하나가 기억하는 보기 수다. 넘으면 모두 버리고 다시 만든다.
const maxViews = 6

// Stat은 항목 하나의 정보다(fs.statmany). Code가 있으면 읽지 못한 것이다(지워졌으면 not_found).
type Stat struct {
	Type      string
	Size      int64
	ModTimeNs int64
	Locked    bool
	LinkType  string
	Code      string
}

// CodeBadName은 이름이 UTF-8이 아니라 정보를 읽지 않은 항목의 Stat.Code다(JSON으로 바이트를 옮길 수 없다).
const CodeBadName = "bad_name"

type entry struct {
	name string
	typ  byte
	st   *Stat
}

func (e *entry) isDir() bool { return e.typ == proto.NameDir }

// Listing은 폴더 하나의 이름 목록과 지금까지 읽은 정보다. 여러 요청이 함께 써도 된다.
type Listing struct {
	ID           string
	Path         string
	DirModTimeNs int64
	Truncated    bool
	Writable     bool

	mu      sync.Mutex
	entries []entry
	filled  int
	views   map[View][]int32
	lower   []string
}

// NewListing은 fs.names 결과로 목록을 만든다.
func NewListing(id, path string, res proto.NamesResult, names []proto.NameEntry) *Listing {
	l := &Listing{ID: id, Path: path, DirModTimeNs: res.DirModTimeNs, Truncated: res.Truncated, Writable: res.Writable,
		entries: make([]entry, len(names)), views: map[View][]int32{}}
	for i, n := range names {
		l.entries[i] = entry{name: n.Name, typ: n.Type}
	}
	return l
}

// Len은 항목 수다.
func (l *Listing) Len() int { return len(l.entries) }

// Filled는 정보를 읽은 항목 수와 전체 항목 수다.
func (l *Listing) Filled() (int, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.filled, len(l.entries)
}

// Page는 보기 v에서 offset부터 limit개 항목의 색인과 보기의 항목 수다. 날짜·크기순은 모든 항목의 정보가
// 있어야 한다(ErrSortPending).
func (l *Listing) Page(v View, offset, limit int) ([]int, int, error) {
	if v.Sort == "" {
		v.Sort = SortName
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	order, err := l.view(v)
	if err != nil {
		return nil, 0, err
	}
	total := len(order)
	if offset >= total {
		return []int{}, total, nil
	}
	end := min(offset+limit, total)
	out := make([]int, 0, end-offset)
	for _, i := range order[offset:end] {
		out = append(out, int(i))
	}
	return out, total, nil
}

// IndexOf는 보기 v에서 이름이 name인 항목의 자리다. 없으면 -1이다. 날짜·크기순은 Page와 같이 ErrSortPending일 수 있다.
func (l *Listing) IndexOf(v View, name string) (int, error) {
	if v.Sort == "" {
		v.Sort = SortName
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	order, err := l.view(v)
	if err != nil {
		return 0, err
	}
	for k, i := range order {
		if l.entries[i].name == name {
			return k, nil
		}
	}
	return -1, nil
}

// Unfilled는 idx 가운데 정보를 아직 읽지 않은 항목이다.
func (l *Listing) Unfilled(idx []int) []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []int
	for _, i := range idx {
		if l.entries[i].st == nil {
			out = append(out, i)
		}
	}
	return out
}

// NextUnfilled는 정보를 아직 읽지 않은 항목을 앞에서부터 n개까지 고른다.
func (l *Listing) NextUnfilled(n int) []int {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []int
	for i := range l.entries {
		if len(out) >= n {
			break
		}
		if l.entries[i].st == nil {
			out = append(out, i)
		}
	}
	return out
}

// Name은 항목 i의 이름(바이트 그대로)이다.
func (l *Listing) Name(i int) string { return l.entries[i].name }

// SetStat은 항목 i의 정보를 둔다. 처음 채울 때만 센다.
func (l *Listing) SetStat(i int, st Stat) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.entries[i].st == nil {
		l.filled++
	}
	l.entries[i].st = &st
}

// Items는 항목 idx를 API 형태로 바꾼다. names가 참이면 이름과 종류만 담는다.
func (l *Listing) Items(idx []int, names bool) []api.FileItem {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]api.FileItem, len(idx))
	for k, i := range idx {
		e := l.entries[i]
		it := api.FileItem{Name: e.name, Type: nameTypeName(e.typ), State: "pending"}
		if !utf8.ValidString(e.name) {
			it.Name, it.BadName = strings.ToValidUTF8(e.name, "�"), true
		}
		if e.st != nil && !names {
			switch {
			case e.st.Code == proto.CodeNotFound:
				it.State = "gone"
			case e.st.Code != "":
				it.State = "error"
			default:
				it.State = "ok"
				it.Type = e.st.Type
				it.Size = e.st.Size
				it.ModTime = e.st.ModTimeNs / int64(time.Millisecond)
				it.Locked = e.st.Locked
				it.LinkType = e.st.LinkType
			}
		}
		out[k] = it
	}
	return out
}

func nameTypeName(t byte) string {
	switch t {
	case proto.NameDir:
		return proto.TypeDir
	case proto.NameFile:
		return proto.TypeFile
	case proto.NameSymlink:
		return proto.TypeSymlink
	case proto.NameOther:
		return proto.TypeOther
	}
	return "unknown"
}

// view는 보기 v의 항목 순서다. mu를 잡은 채 부른다.
func (l *Listing) view(v View) ([]int32, error) {
	if order, ok := l.views[v]; ok {
		return order, nil
	}
	if v.Sort != SortName && l.filled < len(l.entries) {
		return nil, ErrSortPending
	}
	order := make([]int32, 0, len(l.entries))
	if v.Query == "" {
		for i := range l.entries {
			order = append(order, int32(i))
		}
	} else {
		if l.lower == nil {
			l.lower = make([]string, len(l.entries))
			for i, e := range l.entries {
				l.lower[i] = strings.ToLower(e.name)
			}
		}
		q := strings.ToLower(v.Query)
		for i, n := range l.lower {
			if strings.Contains(n, q) {
				order = append(order, int32(i))
			}
		}
	}
	slices.SortStableFunc(order, l.compare(v))
	if len(l.views) >= maxViews {
		clear(l.views)
	}
	l.views[v] = order
	return order, nil
}

// compare는 보기 v의 순서다. 폴더가 언제나 먼저이고, 방향은 기준 값에만 적용한다(같으면 이름 오름차순).
// 이름순 내림차순은 이름까지 뒤집는다.
func (l *Listing) compare(v View) func(a, b int32) int {
	return func(a, b int32) int {
		ea, eb := &l.entries[a], &l.entries[b]
		if da, db := ea.isDir(), eb.isDir(); da != db {
			if da {
				return -1
			}
			return 1
		}
		c := 0
		switch v.Sort {
		case SortMTime:
			c = cmp.Compare(ea.st.ModTimeNs, eb.st.ModTimeNs)
		case SortSize:
			c = cmp.Compare(ea.st.Size, eb.st.Size)
		default:
			c = CompareNames(ea.name, eb.name)
		}
		if v.Desc {
			c = -c
		}
		if c == 0 {
			c = CompareNames(ea.name, eb.name)
		}
		return c
	}
}
