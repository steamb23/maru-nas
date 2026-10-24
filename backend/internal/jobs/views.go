package jobs

import (
	"log"
	"slices"
	"strings"
	"time"

	"maru/backend/internal/api"
	"maru/backend/internal/notify"
)

// 한 번에 돌려주는 출력 크기와 결과 알림 수.
const (
	maxReadBytes = 64 << 10
	maxNotices   = 5
)

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (a *activeJob) view(user string) api.Job {
	return api.Job{
		ID:               a.rec.ID,
		Kind:             string(a.rec.Kind),
		State:            api.JobRunning,
		Owner:            a.rec.Owner,
		Mine:             a.rec.Owner == user,
		StartedAt:        formatTime(a.rec.StartedAt),
		Waiting:          a.waiting,
		LastLine:         a.lastLine,
		OutputEnd:        a.rec.Output.End,
		OutputIncomplete: a.rec.Output.Truncated || a.rec.Output.Incomplete,
	}
}

func (r Result) view(user string) api.Job {
	return api.Job{
		ID:               r.ID,
		Kind:             string(r.Kind),
		State:            r.State,
		Code:             r.Code,
		Owner:            r.Owner,
		Mine:             r.Owner == user,
		StartedAt:        formatTime(r.StartedAt),
		EndedAt:          formatTime(r.EndedAt),
		OutputEnd:        r.Output.End,
		OutputIncomplete: r.Output.Truncated || r.Output.Incomplete,
		Count:            r.Count,
		Items:            slices.Clone(r.Items),
		Dismissed:        r.Dismissed,
	}
}

// Jobs는 진행 중인 작업(누가 시작했든)과 user의 결과다.
func (m *Manager) Jobs(user string) api.JobsResponse {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := api.JobsResponse{Recent: []api.Job{}}
	if m.active != nil {
		j := m.active.view(user)
		res.Current = &j
	}
	for _, r := range m.resultsLocked(user) {
		res.Recent = append(res.Recent, r.view(user))
	}
	return res
}

// Tasks는 진행 중인 작업이다(알림 센터의 백그라운드 작업 구역). 없으면 빈 목록이다.
func (m *Manager) Tasks(user string) []api.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active == nil {
		return []api.Job{}
	}
	return []api.Job{m.active.view(user)}
}

// Get은 작업 하나다. 진행 중인 작업, 마지막으로 끝난 작업, user의 결과에서 찾는다.
func (m *Manager) Get(id, user string) (api.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.active != nil && m.active.rec.ID == id:
		return m.active.view(user), nil
	case m.last != nil && m.last.ID == id:
		return m.last.view(user), nil
	}
	for _, r := range m.resultsLocked(user) {
		if r.ID == id {
			return r.view(user), nil
		}
	}
	return api.Job{}, ErrNotFound
}

func (m *Manager) resultsLocked(user string) []Result {
	rs, err := m.store.LoadResults(user)
	if err != nil {
		log.Printf("jobs: read results: %v", err)
	}
	return rs
}

// Output은 작업 id의 출력을 from(논리 위치)부터 돌려준다. 진행 중인 작업과 마지막으로 끝난 작업은 어느 관리자나,
// 그 밖의 결과는 시작한 사용자만 읽는다.
func (m *Manager) Output(id, user string, from int64) (api.JobOutput, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var rng OutputRange
	var out api.JobOutput
	switch {
	case m.active != nil && m.active.rec.ID == id:
		rng, out.State, out.Waiting = m.active.rec.Output, api.JobRunning, m.active.waiting
	case m.last != nil && m.last.ID == id:
		rng, out.State = m.last.Output, m.last.State
	default:
		rs := m.resultsLocked(user)
		i := slices.IndexFunc(rs, func(r Result) bool { return r.ID == id })
		if i < 0 {
			return api.JobOutput{}, ErrNotFound
		}
		rng, out.State = rs[i].Output, rs[i].State
	}
	from = min(max(from, rng.Start), rng.End)
	n := min(int64(maxReadBytes), rng.End-from)
	var b []byte
	if n > 0 {
		var err error
		b, err = m.store.ReadOutput(id, from-rng.Start, int(n))
		if err != nil {
			log.Printf("jobs: %s: read output: %v", id, err)
		}
		if from+int64(len(b)) < rng.End {
			b = cutPartialRune(b)
		}
	}
	out.From, out.Next, out.Text = from, from+int64(len(b)), string(b)
	if len(b) == 0 && n > 0 {
		// 파일이 없거나 짧다(지워짐). 더 기다려도 오지 않으므로 끝으로 본다.
		out.Next = rng.End
	}
	out.More = out.Next < rng.End
	return out, nil
}

// cutPartialRune은 b 끝에서 잘린 UTF-8 글자를 뺀다(다음 요청이 그 글자부터 받는다).
func cutPartialRune(b []byte) []byte {
	for i := 1; i <= 3 && i <= len(b); i++ {
		c := b[len(b)-i]
		if c < 0x80 {
			return b
		}
		if c >= 0xC0 {
			need := 2
			if c >= 0xF0 {
				need = 4
			} else if c >= 0xE0 {
				need = 3
			}
			if i < need {
				return b[:len(b)-i]
			}
			return b
		}
	}
	return b
}

// NoticesFor는 user가 시작한 작업의 결과 알림이다(지우지 않은 것, 최신 maxNotices개).
func (m *Manager) NoticesFor(user string) []api.Notice {
	m.mu.Lock()
	rs := m.resultsLocked(user)
	m.mu.Unlock()
	out := []api.Notice{}
	for _, r := range rs {
		if r.Dismissed {
			continue
		}
		n := r.notice()
		if err := notify.Check(n); err != "" {
			log.Printf("jobs: dropped result notice %q: %s", n.ID, err)
			continue
		}
		out = append(out, n)
		if len(out) == maxNotices {
			break
		}
	}
	return out
}

func (r Result) notice() api.Notice {
	n := api.Notice{
		ID:          NoticeID(r.ID),
		Revision:    notify.Revision(r.ID, r.State),
		Since:       formatTime(r.EndedAt),
		Count:       r.Count,
		Items:       slices.Clone(r.Items),
		Dismissible: true,
		Job:         &api.NoticeJob{ID: r.ID, Kind: string(r.Kind), State: r.State, Code: r.Code},
	}
	switch r.State {
	case api.JobSucceeded:
		n.Code, n.Level = api.NoticeJobSucceeded, api.NoticeSuccess
	case api.JobFailed:
		n.Code, n.Level = api.NoticeJobFailed, api.NoticeError
	default:
		n.Code, n.Level = api.NoticeJobUnknown, api.NoticeWarning
	}
	if spec, ok := Kinds[r.Kind]; ok {
		n.Action = &api.NoticeAction{Kind: api.NoticeActionApp, App: spec.Addon}
	}
	return n
}

// Dismiss는 user의 결과 알림 하나를 지운다. user의 지울 수 있는 알림이 아니면 ErrNotFound다.
// 이미 지운 알림이면 그대로 성공한다.
func (m *Manager) Dismiss(user, noticeID string) error {
	id, ok := strings.CutPrefix(noticeID, "job-")
	if !ok || !ValidID(id) {
		return ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rs := m.resultsLocked(user)
	i := slices.IndexFunc(rs, func(r Result) bool { return r.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if rs[i].Dismissed {
		return nil
	}
	rs[i].Dismissed = true
	if m.last != nil && m.last.ID == id {
		m.last.Dismissed = true
	}
	return m.store.SaveResults(user, rs)
}
