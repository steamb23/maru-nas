// Package notify는 서버 알림(업데이트가 있음 같은 상태)을 모은다.
//
// 알림은 출처(Source)마다 한 번에 읽고, 결과를 모든 세션이 함께 쓰도록 잠시 보관한다(Center).
// 보관 기간이 지난 뒤 처음 묻는 요청에서 다시 읽으며 백그라운드 고루틴은 없다(addons.Detector와 같은 방식).
// 문구는 다루지 않고 코드와 값만 다룬다. 읽음 상태는 사용자 설정(prefs)에 있다.
package notify

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"maru/backend/internal/api"
)

// Source는 알림 출처 하나다.
type Source interface {
	// Name은 로그에 남길 출처 이름이다.
	Name() string
	// Admin이 true면 이 출처의 알림은 관리자 세션에만 보인다.
	Admin() bool
	// Collect는 지금 알릴 것을 돌려준다. 알릴 것이 없으면 빈 목록이다.
	Collect(ctx context.Context) ([]api.Notice, error)
}

// 한 출처의 알림 수와 알림 하나의 Items 수 상한. 넘으면 앞쪽만 남긴다(전체 수는 Count가 알린다).
const (
	maxPerSource = 20
	maxItems     = 20
)

// Center는 출처의 알림을 모아 보관한다. 여러 고루틴에서 함께 써도 된다.
type Center struct {
	// TTL은 결과를 보관하는 시간이다. 지난 뒤 처음 묻는 요청에서 다시 읽는다.
	TTL time.Duration
	// Timeout은 출처 하나를 읽는 시간 제한이다.
	Timeout time.Duration
	Now     func() time.Time

	sources []Source
	mu      sync.Mutex
	// last는 출처마다 마지막으로 읽은 결과다(검사를 거친 값). 읽지 못하면 그대로 둔다.
	last    [][]api.Notice
	checked time.Time
	stale   bool
}

// New는 sources를 읽는 Center를 반환한다. 출처가 없으면 늘 빈 목록이다.
func New(sources ...Source) *Center {
	return &Center{
		TTL:     30 * time.Second,
		Timeout: 5 * time.Second,
		Now:     time.Now,
		sources: slices.Clone(sources),
		last:    make([][]api.Notice, len(sources)),
		stale:   true,
	}
}

// Notices는 이 세션이 볼 알림이다. admin이 아니면 관리자 전용 출처를 뺀다. 결과가 오래됐으면 다시 읽는다.
// 돌려준 값은 복사본이고 nil이 아니다.
func (c *Center) Notices(ctx context.Context, admin bool) []api.Notice {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stale || c.Now().Sub(c.checked) >= c.TTL {
		c.refreshLocked(ctx)
	}
	out := []api.Notice{}
	for i, s := range c.sources {
		if s.Admin() && !admin {
			continue
		}
		for _, n := range c.last[i] {
			out = append(out, cloneNotice(n))
		}
	}
	slices.SortFunc(out, func(a, b api.Notice) int {
		return cmp.Or(cmp.Compare(levelRank[a.Level], levelRank[b.Level]), strings.Compare(a.ID, b.ID))
	})
	return out
}

// Invalidate는 다음 Notices 호출 때 다시 읽게 한다.
func (c *Center) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stale = true
}

// refreshLocked는 모든 출처를 다시 읽는다. c.mu를 잡고 부른다.
// 요청이 끊겨도 모든 세션이 함께 쓰는 결과이므로 끝까지 읽는다(시간 제한은 둔다).
func (c *Center) refreshLocked(ctx context.Context) {
	for i, s := range c.sources {
		got, err := c.collect(ctx, s)
		if err != nil {
			// 읽지 못하면 직전 결과를 유지한다. 처음이면 알릴 것이 없는 것으로 본다.
			log.Printf("notifications: %s: %v", s.Name(), err)
		} else {
			c.last[i] = c.accept(i, got)
		}
	}
	c.checked, c.stale = c.Now(), false
}

func (c *Center) collect(ctx context.Context, s Source) ([]api.Notice, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.Timeout)
	defer cancel()
	return s.Collect(ctx)
}

// accept는 i번째 출처가 준 알림 중 형식이 맞는 것만 상한까지 남긴다. 다른 출처와 겹치는 id는 앞 출처의 것을 남긴다.
func (c *Center) accept(i int, got []api.Notice) []api.Notice {
	s := c.sources[i]
	var out []api.Notice
	for _, n := range got {
		err := Check(n)
		if err == "" && n.Dismissible {
			// 지울 수 있는 알림(작업 결과)은 사용자별이다. 모든 세션이 함께 쓰는 출처는 주지 않는다.
			err = "dismissible notice from a shared source"
		}
		if err != "" {
			log.Printf("notifications: %s: dropped notice %q: %s", s.Name(), n.ID, err)
			continue
		}
		if c.taken(i, n.ID) || slices.ContainsFunc(out, func(o api.Notice) bool { return o.ID == n.ID }) {
			log.Printf("notifications: %s: dropped duplicate notice %q", s.Name(), n.ID)
			continue
		}
		if len(out) == maxPerSource {
			log.Printf("notifications: %s: more than %d notices, rest dropped", s.Name(), maxPerSource)
			break
		}
		n = cloneNotice(n)
		if len(n.Items) > maxItems {
			n.Items = n.Items[:maxItems]
		}
		out = append(out, n)
	}
	return out
}

// taken은 i번째보다 앞에 있는 출처가 id를 이미 쓰고 있는지다.
func (c *Center) taken(i int, id string) bool {
	for _, prev := range c.last[:i] {
		if slices.ContainsFunc(prev, func(n api.Notice) bool { return n.ID == id }) {
			return true
		}
	}
	return false
}

// levelRank는 정렬 순서다. 급한 것이 앞이다.
var levelRank = map[string]int{api.NoticeError: 0, api.NoticeWarning: 1, api.NoticeInfo: 2, api.NoticeSuccess: 3}

// knownCodes는 프론트가 문구를 가진 코드다. api.Notice.Code의 tstype과 같아야 한다(테스트가 확인).
var knownCodes = map[string]bool{
	api.NoticeSample: true, api.NoticeAptUpdates: true, api.NoticeRebootRequired: true,
	api.NoticeJobSucceeded: true, api.NoticeJobFailed: true, api.NoticeJobUnknown: true,
}

var (
	idPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]{0,63}$`)
	revisionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	// appPattern은 앱 id와 설정 페이지 id의 형식이다.
	appPattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	// routePattern은 OMV 워크벤치 라우트의 형식이다(앞 슬래시 없음).
	routePattern = regexp.MustCompile(`^[A-Za-z0-9_.-][A-Za-z0-9/_.-]{0,127}$`)
)

// ValidID는 알림 id 형식이 맞는지다. 읽음 상태의 키를 검사할 때도 쓴다.
func ValidID(s string) bool { return idPattern.MatchString(s) }

// ValidRevision은 알림 리비전 형식이 맞는지다. 읽음 상태의 값을 검사할 때도 쓴다.
func ValidRevision(s string) bool { return revisionPattern.MatchString(s) }

// Check는 알림 하나의 형식을 검사한다. 맞으면 빈 문자열, 틀리면 이유다. 출처 밖에서 만든 알림(작업 결과)도 이것으로 검사한다.
func Check(n api.Notice) string {
	switch {
	case !ValidID(n.ID):
		return "bad id"
	case !knownCodes[n.Code]:
		return "unknown code"
	case !ValidRevision(n.Revision):
		return "bad revision"
	}
	if _, ok := levelRank[n.Level]; !ok {
		return "unknown level"
	}
	if n.Since != "" {
		if _, err := time.Parse(time.RFC3339, n.Since); err != nil {
			return "bad since"
		}
	}
	if n.Count != nil && *n.Count < 0 {
		return "negative count"
	}
	if j := n.Job; j != nil && !ValidID(j.ID) {
		return "bad job"
	}
	if a := n.Action; a != nil {
		switch a.Kind {
		case api.NoticeActionApp:
			if !appPattern.MatchString(a.App) || (a.Page != "" && !appPattern.MatchString(a.Page)) || a.Route != "" {
				return "bad app action"
			}
		case api.NoticeActionWorkbench:
			if !routePattern.MatchString(a.Route) || strings.Contains(a.Route, "//") || a.App != "" || a.Page != "" {
				return "bad workbench action"
			}
		case api.NoticeActionPower:
			if a.App != "" || a.Page != "" || a.Route != "" {
				return "bad power action"
			}
		default:
			return "unknown action"
		}
	}
	return ""
}

// Revision은 parts로 리비전을 만든다. 순서와 무관하다(정렬한 뒤 해시). 내용이 같으면 같은 값이다.
func Revision(parts ...string) string {
	sorted := slices.Clone(parts)
	slices.Sort(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\x00")))
	return hex.EncodeToString(sum[:8])
}

func cloneNotice(n api.Notice) api.Notice {
	if n.Count != nil {
		v := *n.Count
		n.Count = &v
	}
	n.Items = slices.Clone(n.Items)
	if n.Action != nil {
		a := *n.Action
		n.Action = &a
	}
	return n
}

// SeenFor는 읽음 상태 중 notices에 있는 id만 남긴 것이다. 늘 nil이 아니다.
func SeenFor(seen map[string]string, notices []api.Notice) map[string]string {
	out := map[string]string{}
	for _, n := range notices {
		if r, ok := seen[n.ID]; ok {
			out[n.ID] = r
		}
	}
	return out
}
