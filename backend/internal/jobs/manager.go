package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
)

const (
	// outputCap을 넘으면 출력 앞부분을 버리고 뒤 outputKeep 정도만 남긴다(오류는 끝에 있다).
	outputCap  = 4 << 20
	outputKeep = 2 << 20
	// 사용자마다 남기는 결과 수와 기간.
	maxResults = 10
	resultTTL  = 30 * 24 * time.Hour
	// 진행 기록(current.json)을 다시 쓰는 간격. 출력 조각마다 쓰지 않는다(플래시 마모).
	checkpointEvery = 10 * time.Second
	// maxSummaryItems는 결과 요약에 담는 패키지 이름 수다(알림 Items 상한과 같다).
	maxSummaryItems = 20
	// maxBefore는 시작할 때 기록하는 업데이트 목록의 상한이다.
	maxBefore = 1000
)

// IndexReader는 업데이트 목록을 읽는다(aptindex.Reader). 결과 요약에 쓴다.
type IndexReader interface {
	Index() (aptindex.Index, error)
}

// Config는 Manager를 만드는 입력이다.
type Config struct {
	Store Store
	// Index가 nil이면 결과에 요약(Count, Items)을 넣지 않는다.
	Index IndexReader
	// BootID는 지금 부트 id다. nil이면 /proc/sys/kernel/random/boot_id를 읽는다.
	BootID func() string
	Now    func() time.Time
	// OnFinish는 작업이 끝난 뒤(결과를 저장한 뒤) 잠금 밖에서 부른다. 알림 Center를 다시 읽게 한다.
	OnFinish func()
}

// Manager는 작업 하나를 시작하고 끝까지 지켜본다. 여러 고루틴에서 함께 써도 된다.
type Manager struct {
	store    Store
	index    IndexReader
	bootID   func() string
	now      func() time.Time
	onFinish func()

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	offers chan struct{}

	mu       sync.Mutex
	runners  []Runner
	reserved bool
	active   *activeJob
	// last는 마지막으로 끝난 작업이다. 시작하지 않은 관리자도 끝난 직후의 출력을 볼 수 있게 메모리에 둔다.
	last    *Result
	resumed bool
	closed  bool
}

// New는 Manager를 만든다. Runner를 Register한 뒤 Resume으로 남은 작업을 이어 지켜본다.
func New(cfg Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		store:    cfg.Store,
		index:    cfg.Index,
		bootID:   cfg.BootID,
		now:      cfg.Now,
		onFinish: cfg.OnFinish,
		ctx:      ctx,
		cancel:   cancel,
		offers:   make(chan struct{}, 1),
	}
	if m.store == nil {
		m.store = NewMemoryStore()
	}
	if m.bootID == nil {
		m.bootID = ReadBootID
	}
	if m.now == nil {
		m.now = time.Now
	}
	return m
}

// ReadBootID는 지금 부트 id다. 읽지 못하면 빈 문자열이다(그때는 다시 시작을 감지하지 않는다).
func ReadBootID() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Register는 Runner를 더한다. 처음 더한 Runner로 새 작업을 시작한다. Resume 전에 부른다.
func (m *Manager) Register(r Runner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runners = append(m.runners, r)
}

func (m *Manager) runnerLocked(name string) Runner {
	for _, r := range m.runners {
		if r.Name() == name {
			return r
		}
	}
	return nil
}

// Resume은 상태 디렉터리에 남은 진행 중 작업을 이어 지켜본다. 한 번만 동작한다.
func (m *Manager) Resume() {
	m.mu.Lock()
	if m.resumed {
		m.mu.Unlock()
		return
	}
	m.resumed = true
	m.mu.Unlock()

	rec, err := m.store.LoadCurrent()
	if err != nil {
		log.Printf("jobs: read current job: %v", err)
		return
	}
	keep := ""
	if rec != nil {
		keep = rec.ID
	}
	if err := m.store.PruneOutputs(m.now().Add(-resultTTL-24*time.Hour), keep); err != nil {
		log.Printf("jobs: prune outputs: %v", err)
	}
	if rec == nil {
		return
	}
	if _, ok := Kinds[rec.Kind]; !ok || !ValidID(rec.ID) {
		log.Printf("jobs: dropping unreadable current job %q", rec.ID)
		m.store.SaveCurrent(nil)
		return
	}
	// 결과를 저장한 뒤 진행 기록을 지우기 전에 멈췄으면 결과를 믿는다.
	if rs, _ := m.store.LoadResults(rec.Owner); slices.ContainsFunc(rs, func(r Result) bool { return r.ID == rec.ID }) {
		m.store.SaveCurrent(nil)
		return
	}
	// 기록보다 뒤에 붙은 출력은 다시 받으므로 자른다(중복 없이 이어 붙인다).
	if err := m.store.TruncateOutput(rec.ID, rec.Output.End-rec.Output.Start); err != nil {
		log.Printf("jobs: truncate output %s: %v", rec.ID, err)
	}

	m.mu.Lock()
	a := &activeJob{m: m, rec: *rec, saved: m.now()}
	m.active = a
	r := m.runnerLocked(rec.Runner)
	switch {
	case rec.BootID != "" && rec.BootID != m.bootID():
		m.mu.Unlock()
		log.Printf("jobs: %s: system restarted while the job ran", rec.ID)
		m.finish(a, Outcome{State: api.JobUnknown, Code: api.JobCodeReboot})
	case r == nil:
		m.mu.Unlock()
		log.Printf("jobs: %s: runner %q is not available", rec.ID, rec.Runner)
		m.finish(a, Outcome{State: api.JobUnknown, Code: api.JobCodeLost})
	default:
		log.Printf("jobs: resuming %s (%s, started by %q)", rec.ID, rec.Kind, rec.Owner)
		m.launchLocked(a, r)
		m.mu.Unlock()
	}
}

// Start는 kind 작업을 시작하고 id를 돌려준다. 이미 진행 중이면 ErrBusy다. Runner가 거절하면 그 오류다.
func (m *Manager) Start(ctx context.Context, kind Kind, req StartRequest) (string, error) {
	if _, ok := Kinds[kind]; !ok {
		return "", ErrUnknownKind
	}
	m.mu.Lock()
	switch {
	case m.closed || len(m.runners) == 0:
		m.mu.Unlock()
		return "", ErrNoRunner
	case m.reserved || m.active != nil:
		m.mu.Unlock()
		return "", ErrBusy
	}
	// OMV를 부르는 동안 다른 시작을 막는다(실패하면 푼다).
	m.reserved = true
	r := m.runners[0]
	m.mu.Unlock()

	before := m.snapshot()
	handle, err := r.Start(ctx, kind, req)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.reserved = false
	if err != nil {
		return "", err
	}
	now := m.now()
	a := &activeJob{m: m, prefer: req.SessionKey, rec: Record{
		ID:        newID(now),
		Kind:      kind,
		Owner:     req.Owner,
		Runner:    r.Name(),
		Handle:    handle,
		BootID:    m.bootID(),
		StartedAt: now.UTC(),
		Before:    before,
	}}
	m.checkpointLocked(a)
	m.active = a
	m.launchLocked(a, r)
	log.Printf("jobs: started %s (%s) by %q", a.rec.ID, kind, req.Owner)
	return a.rec.ID, nil
}

// snapshot은 지금 업데이트 목록의 "이름@버전"이다. 읽지 못하면 nil이다.
func (m *Manager) snapshot() []string {
	if m.index == nil {
		return nil
	}
	idx, err := m.index.Index()
	if err != nil {
		return nil
	}
	keys := aptindex.Keys(idx.Packages)
	if len(keys) > maxBefore {
		keys = keys[:maxBefore]
	}
	return keys
}

// IndexModTime은 업데이트 목록 파일의 수정 시각이다. 감시자가 사라진 작업이 끝까지 돌았는지 가늠할 때 쓴다.
// 목록을 읽을 수 없으면(Config.Index가 수정 시각을 주지 않으면) 오류다.
func (m *Manager) IndexModTime() (time.Time, error) {
	if r, ok := m.index.(interface{ IndexModTime() (time.Time, error) }); ok {
		return r.IndexModTime()
	}
	return time.Time{}, errors.New("jobs: index modification time is not available")
}

// Offer는 감시자에게 관리자 세션이 새로 생겼을 수 있다고 알린다(기다리는 중이면 바로 다시 본다).
func (m *Manager) Offer() {
	select {
	case m.offers <- struct{}{}:
	default:
	}
}

// Close는 감시를 멈추고 진행 기록을 마지막으로 저장한다. OMV 쪽 작업은 계속 돌고, 다시 시작하면 이어 지켜본다.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	m.wg.Wait()
}

func (m *Manager) launchLocked(a *activeJob, r Runner) {
	m.wg.Add(1)
	job := WatchJob{ID: a.rec.ID, Kind: a.rec.Kind, Handle: slices.Clone(a.rec.Handle), StartedAt: a.rec.StartedAt, Prefer: a.prefer}
	go m.watch(a, r, job)
}

func (m *Manager) watch(a *activeJob, r Runner, job WatchJob) {
	defer m.wg.Done()
	out := r.Watch(m.ctx, job, a)
	if out.State == "" {
		// 끝나지 않고 멈췄다(서비스 종료). 다시 시작하면 기록한 자리부터 이어 읽는다.
		m.mu.Lock()
		m.checkpointLocked(a)
		m.mu.Unlock()
		return
	}
	m.finish(a, out)
}

// finish는 결과를 저장하고 진행 기록을 지운다.
func (m *Manager) finish(a *activeJob, out Outcome) {
	m.mu.Lock()
	res := Result{
		ID:        a.rec.ID,
		Kind:      a.rec.Kind,
		Owner:     a.rec.Owner,
		Runner:    a.rec.Runner,
		State:     out.State,
		Code:      out.Code,
		StartedAt: a.rec.StartedAt,
		EndedAt:   m.now().UTC(),
		Output:    a.rec.Output,
	}
	if out.Incomplete {
		res.Output.Incomplete = true
	}
	if out.State == api.JobSucceeded {
		res.Count, res.Items = m.summary(a.rec)
	}
	m.addResultLocked(res)
	if err := m.store.SaveCurrent(nil); err != nil {
		log.Printf("jobs: clear current job: %v", err)
	}
	if m.active == a {
		m.active = nil
	}
	m.last = &res
	m.mu.Unlock()
	log.Printf("jobs: %s finished: %s %s", res.ID, res.State, res.Code)
	if m.onFinish != nil {
		m.onFinish()
	}
}

// summary는 성공한 작업의 요약이다. 확인은 찾은 업데이트, 설치는 시작할 때 목록에서 사라진 패키지다.
func (m *Manager) summary(rec Record) (*int, []string) {
	if m.index == nil {
		return nil, nil
	}
	idx, err := m.index.Index()
	if err != nil {
		return nil, nil
	}
	var names []string
	switch rec.Kind {
	case KindAptUpdate:
		for _, p := range idx.Packages {
			names = append(names, p.Name)
		}
	case KindAptUpgrade:
		after := aptindex.Keys(idx.Packages)
		for _, key := range rec.Before {
			if !slices.Contains(after, key) {
				name, _, _ := strings.Cut(key, "@")
				names = append(names, name)
			}
		}
	default:
		return nil, nil
	}
	count := len(names)
	if len(names) > maxSummaryItems {
		names = names[:maxSummaryItems]
	}
	return &count, names
}

// addResultLocked는 결과를 시작한 사용자의 목록 앞에 넣고 오래된 결과를 정리한다.
func (m *Manager) addResultLocked(res Result) {
	rs, err := m.store.LoadResults(res.Owner)
	if err != nil {
		log.Printf("jobs: read results: %v", err)
	}
	rs = append([]Result{res}, rs...)
	kept, dropped := pruneResults(rs, m.now())
	if err := m.store.SaveResults(res.Owner, kept); err != nil {
		log.Printf("jobs: save results: %v", err)
	}
	for _, d := range dropped {
		if err := m.store.DeleteOutput(d.ID); err != nil {
			log.Printf("jobs: delete output %s: %v", d.ID, err)
		}
	}
}

// pruneResults는 최신 maxResults개 중 resultTTL 안의 결과만 남긴다.
func pruneResults(rs []Result, now time.Time) (kept, dropped []Result) {
	for i, r := range rs {
		if i >= maxResults || now.Sub(r.EndedAt) > resultTTL {
			dropped = append(dropped, r)
		} else {
			kept = append(kept, r)
		}
	}
	return kept, dropped
}

func (m *Manager) checkpointLocked(a *activeJob) {
	rec := a.rec
	if err := m.store.SaveCurrent(&rec); err != nil {
		log.Printf("jobs: save current job: %v", err)
	}
	a.saved, a.dirty = m.now(), false
}

// activeJob은 진행 중인 작업이다. Runner의 Sink다.
type activeJob struct {
	m      *Manager
	rec    Record
	prefer string
	// waiting은 출력을 읽을 관리자 세션이 없어 기다리는 중인지다(메모리에만).
	waiting bool
	dirty   bool
	saved   time.Time
}

func (a *activeJob) Output(chunk []byte, handle json.RawMessage) {
	m := a.m
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(chunk) > 0 {
		if err := m.store.AppendOutput(a.rec.ID, chunk); err != nil {
			log.Printf("jobs: %s: append output: %v", a.rec.ID, err)
			a.rec.Output.Incomplete = true
		} else {
			a.rec.Output.End += int64(len(chunk))
		}
		if a.rec.Output.End-a.rec.Output.Start > outputCap {
			if dropped, err := m.store.DropOutputHead(a.rec.ID, outputKeep); err != nil {
				log.Printf("jobs: %s: trim output: %v", a.rec.ID, err)
			} else if dropped > 0 {
				a.rec.Output.Start += dropped
				a.rec.Output.Truncated = true
			}
		}
	}
	a.rec.Handle = slices.Clone(handle)
	a.dirty = true
	if m.now().Sub(a.saved) >= checkpointEvery {
		m.checkpointLocked(a)
	}
}

func (a *activeJob) Waiting(w bool) {
	a.m.mu.Lock()
	defer a.m.mu.Unlock()
	if a.waiting != w {
		if w {
			log.Printf("jobs: %s: no administrator session to watch with; waiting", a.rec.ID)
		}
		a.waiting = w
	}
}

func (a *activeJob) Offers() <-chan struct{} { return a.m.offers }
