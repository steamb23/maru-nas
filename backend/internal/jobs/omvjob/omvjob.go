// Package omvjob은 OMV 백그라운드 작업을 관리자 OMV 세션으로 시작하고 Exec.getOutput으로 끝까지 읽는 jobs.Runner다.
//
// OMV 상태 파일은 세션·사용자에 묶이지 않아 어느 관리자 세션으로도 읽힌다. 그래서 시작한 세션이 끊기면(로그아웃,
// 만료, maru 재시작) 살아 있는 다른 관리자 세션으로 옮겨 이어 읽고, 아무도 없으면 기다린다. 호출은 모두
// updatelastaccess=false라 누구의 OMV 세션도 연장하지 않는다.
//
// Exec.isRunning은 쓰지 않는다. isRunning과 getOutput 모두 작업이 끝나면 상태 파일을 지우므로 둘을 함께 쓰면 출력을
// 잃는다. Exec.stop도 쓰지 않는다(설치 도중에 멈추는 것이 끝까지 기다리는 것보다 위험하다).
package omvjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
	"maru/backend/internal/jobs"
	"maru/backend/internal/omv"
)

// Name은 이 Runner의 이름이다(작업 기록에 남는다).
const Name = "omv-session"

const (
	// readLength는 Exec.getOutput 한 번에 읽는 상한이다. 생략하면 끝까지 읽어 오래 멈췄다 읽을 때 응답이 너무 커진다.
	readLength = 2 << 20
	// maxChunksPerTick은 pendingOutput일 때 쉬지 않고 이어 읽는 횟수다.
	maxChunksPerTick = 4
	// waitRetry는 관리자 세션이 없을 때 다시 보는 간격이다(관리자 요청이 오면 바로 본다).
	waitRetry = 30 * time.Second
	// goneWindow는 상태 파일이 사라졌을 때 성공으로 볼 수 있는, 마지막 성공 폴링 이후의 시간이다.
	goneWindow = 10 * time.Second
	// failConfirm은 모르는 500 오류를 실패로 보기 전에 같은 문구가 이어져야 하는 횟수다(Runner.FailConfirmFor와 함께).
	failConfirm = 3
)

// filenamePattern은 OMV execBgProc의 상태 파일 이름(tempnam)이다. 브라우저가 아니라 OMV가 준 값이지만 기록에서
// 다시 읽어 OMV에 넘기므로 검사한다.
var filenamePattern = regexp.MustCompile(`^/tmp/bgstatus[A-Za-z0-9]+$`)

// Session은 감시에 쓸 수 있는 관리자 대시보드 세션 하나다.
type Session struct {
	// Key는 대시보드 세션을 가리킨다(메모리에만, 로그에 넣지 않는다).
	Key  string
	User string
	// RPC는 그 세션의 OMV 호출기다(updatelastaccess=false, 작업을 시작한 애드온의 허용 목록).
	RPC addons.RPCCaller
}

// Sessions는 감시에 쓸 관리자 세션을 고른다(서버가 구현한다).
type Sessions interface {
	// Candidates는 쓸 수 있는 관리자 세션이다. prefer가 있으면 그 세션을 먼저 둔다.
	Candidates(prefer, addon string) []Session
	// Dead는 OMV가 그 세션을 파기했다는 뜻이다(401). 서버가 대시보드 세션도 지운다.
	Dead(key string)
}

// Runner는 OMV 세션 감시자다.
type Runner struct {
	sessions Sessions
	// indexModTime은 업데이트 목록 파일의 수정 시각이다. 상태 파일이 사라졌을 때 작업이 끝까지 돌았는지 가늠한다.
	indexModTime func() (time.Time, error)
	// Now, Interval은 테스트에서 바꾼다.
	Now      func() time.Time
	Interval time.Duration
	// Backoff는 일시 오류 뒤 기다리는 시간의 순서다(마지막 값을 계속 쓴다).
	Backoff []time.Duration
	// WaitRetry는 세션이 없을 때 다시 보는 간격이다.
	WaitRetry time.Duration
	// FailConfirmFor는 모르는 500 오류를 실패로 보기 전에 같은 문구가 이어져야 하는 시간이다.
	FailConfirmFor time.Duration
	// OutageLimit은 일시 오류가 이어질 때 결과를 모름으로 보는 시간이다.
	OutageLimit time.Duration
}

// New는 Runner를 만든다. indexModTime이 nil이면 사라진 작업을 늘 결과 모름으로 본다.
func New(sessions Sessions, indexModTime func() (time.Time, error)) *Runner {
	return &Runner{
		sessions:     sessions,
		indexModTime: indexModTime,
		Now:          time.Now,
		Interval:     time.Second,
		Backoff:      []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second},
		WaitRetry:    waitRetry,
		// 엔진 재시작 문구가 바뀌어도 잘못 실패로 보지 않게 잠깐 지켜본다.
		FailConfirmFor: 10 * time.Second,
		OutageLimit:    time.Hour,
	}
}

func (r *Runner) Name() string { return Name }

// handle은 이어 읽을 자리다(작업 기록에 저장된다).
type handle struct {
	Filename string `json:"filename"`
	Pos      int64  `json:"pos"`
}

// Start는 작업을 시작하는 OMV 메서드를 부른다. 다시 시도하지 않는다.
func (r *Runner) Start(ctx context.Context, kind jobs.Kind, req jobs.StartRequest) (json.RawMessage, error) {
	spec, ok := jobs.Kinds[kind]
	if !ok {
		return nil, jobs.ErrUnknownKind
	}
	var filename string
	if err := addons.CallInto(ctx, req.RPC, spec.OMV.Service, spec.OMV.Method, nil, &filename); err != nil {
		var oe *omv.Error
		if errors.As(err, &oe) && oe.FromRPC() {
			switch {
			case strings.Contains(oe.Message, "is held by another process"):
				return nil, fmt.Errorf("%w: %w", jobs.ErrLocked, err)
			case strings.Contains(oe.Message, "Please apply the configuration changes first"):
				return nil, fmt.Errorf("%w: %w", jobs.ErrConfigDirty, err)
			}
		}
		return nil, err
	}
	if !filenamePattern.MatchString(filename) {
		return nil, fmt.Errorf("omvjob: unexpected background status file name from %s.%s", spec.OMV.Service, spec.OMV.Method)
	}
	return json.Marshal(handle{Filename: filename})
}

// output은 Exec.getOutput 응답이다.
type output struct {
	Pos           int64  `json:"pos"`
	Output        string `json:"output"`
	PendingOutput bool   `json:"pendingOutput"`
	Running       bool   `json:"running"`
}

// class는 Exec.getOutput 오류의 종류다.
type class int

const (
	classOther        class = iota // 모르는 500: 같은 문구가 이어지면 실패
	classUnauthorized              // 세션 파기(401): 다른 세션으로
	classForbidden                 // 관리자가 아님(403): 그 세션을 건너뜀
	classTransient                 // 네트워크, nginx 오류 페이지, 엔진 재시작: 물러났다 다시
	classGone                      // 상태 파일이 사라짐: 누가 먼저 끝까지 읽음
	classCoreRemoval               // openmediavault를 지우게 되어 멈춤
	classExecFailed                // apt가 오류로 끝남
	classNotAllowed                // 허용 목록 밖(프로그래밍 오류)
)

// transientPrefixes는 rpc.php가 엔진(engined)에 닿지 못할 때의 문구다(rpc/rpc.inc). 작업은 KillMode=process라
// 엔진이 다시 시작해도 계속 돈다.
var transientPrefixes = []string{
	"Failed to connect to socket",
	"Failed to write to socket",
	"Failed to read from socket",
	"Invalid RPC response",
	"Failed to decode JSON string",
}

// classify는 Exec.getOutput 오류를 나눈다. OMV 예외 코드는 늘 0이라 문구 앞부분으로 가린다(테스트로 고정한다).
func classify(err error, filename string) class {
	if errors.Is(err, addons.ErrRPCNotAllowed) {
		return classNotAllowed
	}
	var oe *omv.Error
	if !errors.As(err, &oe) {
		return classTransient
	}
	switch oe.HTTPStatus {
	case http.StatusUnauthorized:
		return classUnauthorized
	case http.StatusForbidden:
		return classForbidden
	}
	if !oe.FromRPC() {
		return classTransient
	}
	msg := oe.Message
	switch {
	case strings.HasPrefix(msg, "Failed to open file (filename="+filename+","),
		strings.HasPrefix(msg, "Failed to open the command output file"):
		return classGone
	case strings.Contains(msg, "would remove the 'openmediavault' package itself"):
		return classCoreRemoval
	case strings.HasPrefix(msg, "Failed to execute command"):
		return classExecFailed
	}
	for _, p := range transientPrefixes {
		if strings.HasPrefix(msg, p) {
			return classTransient
		}
	}
	return classOther
}

// Watch는 작업이 끝날 때까지 출력을 읽는다.
func (r *Runner) Watch(ctx context.Context, job jobs.WatchJob, sink jobs.Sink) jobs.Outcome {
	var h handle
	if json.Unmarshal(job.Handle, &h) != nil || !filenamePattern.MatchString(h.Filename) || h.Pos < 0 {
		log.Printf("jobs: %s: unreadable handle", job.ID)
		return jobs.Outcome{State: api.JobUnknown, Code: api.JobCodeLost}
	}
	spec := jobs.Kinds[job.Kind]
	w := watch{r: r, ctx: ctx, job: job, sink: sink, h: h, addon: spec.Addon, skip: map[string]bool{}}
	return w.run(spec.MaxRun)
}

// watch는 Watch 한 번의 상태다.
type watch struct {
	r     *Runner
	ctx   context.Context
	job   jobs.WatchJob
	sink  jobs.Sink
	h     handle
	addon string

	cur  *Session
	skip map[string]bool
	// lastOK는 마지막으로 성공한 폴링 시각이다.
	lastOK time.Time
	// failingSince는 일시 오류가 이어지기 시작한 시각이다(성공하면 비운다).
	failingSince time.Time
	backoff      int
	// errMsg·errCount·errSince는 이어지는 같은 500 문구다.
	errMsg   string
	errCount int
	errSince time.Time
	gone     bool
}

func (w *watch) run(maxRun time.Duration) jobs.Outcome {
	r := w.r
	for {
		if w.ctx.Err() != nil {
			return jobs.Outcome{}
		}
		if w.cur == nil && !w.attach() {
			if !w.sleep(r.WaitRetry, true) {
				return jobs.Outcome{}
			}
			continue
		}
		res, err := w.poll()
		if err == nil {
			w.lastOK, w.failingSince, w.backoff, w.errCount, w.gone = r.Now(), time.Time{}, 0, 0, false
			if res.Pos != w.h.Pos || res.Output != "" {
				w.h.Pos = res.Pos
				w.sink.Output([]byte(res.Output), w.handleJSON())
			}
			if !res.Running && !res.PendingOutput {
				return jobs.Outcome{State: api.JobSucceeded}
			}
			if res.Running && maxRun > 0 && r.Now().Sub(w.job.StartedAt) > maxRun {
				log.Printf("jobs: %s: still running after %v", w.job.ID, maxRun)
				return jobs.Outcome{State: api.JobUnknown, Code: api.JobCodeTimeout}
			}
			if !w.sleep(r.Interval, false) {
				return jobs.Outcome{}
			}
			continue
		}
		if out, done := w.onError(err); done {
			return out
		}
	}
}

// attach는 쓸 수 있는 관리자 세션을 고른다. 없으면 false이고 기다림을 알린다.
func (w *watch) attach() bool {
	for _, s := range w.r.sessions.Candidates(w.job.Prefer, w.addon) {
		if w.skip[s.Key] {
			continue
		}
		w.cur = &s
		w.sink.Waiting(false)
		log.Printf("jobs: %s: watching via user=%q", w.job.ID, s.User)
		// 기다리는 동안은 일시 오류 시간에 넣지 않는다.
		w.failingSince = time.Time{}
		return true
	}
	w.sink.Waiting(true)
	return false
}

// poll은 지금 위치부터 출력을 읽는다. pendingOutput이면 몇 번 이어 읽는다.
func (w *watch) poll() (output, error) {
	var res output
	for i := 0; i < maxChunksPerTick; i++ {
		err := addons.CallInto(w.ctx, w.cur.RPC, "Exec", "getOutput",
			map[string]any{"filename": w.h.Filename, "pos": w.h.Pos, "length": readLength}, &res)
		if err != nil {
			return output{}, err
		}
		if !res.PendingOutput {
			return res, nil
		}
		w.h.Pos = res.Pos
		w.sink.Output([]byte(res.Output), w.handleJSON())
		res.Output = ""
	}
	return res, nil
}

// onError는 폴링 오류를 처리한다. 작업이 끝났다고 볼 수 있으면 done이다.
func (w *watch) onError(err error) (jobs.Outcome, bool) {
	r := w.r
	switch classify(err, w.h.Filename) {
	case classUnauthorized:
		log.Printf("jobs: %s: session of user=%q expired; switching", w.job.ID, w.cur.User)
		r.sessions.Dead(w.cur.Key)
		w.cur = nil
		return jobs.Outcome{}, false
	case classForbidden:
		w.skip[w.cur.Key] = true
		w.cur = nil
		return jobs.Outcome{}, false
	case classNotAllowed:
		log.Printf("jobs: %s: %v", w.job.ID, err)
		return jobs.Outcome{State: api.JobUnknown, Code: api.JobCodeLost}, true
	case classCoreRemoval:
		log.Printf("jobs: %s: %v", w.job.ID, err)
		return jobs.Outcome{State: api.JobFailed, Code: api.JobCodeAptCoreRemoval}, true
	case classExecFailed:
		log.Printf("jobs: %s: %v", w.job.ID, err)
		return jobs.Outcome{State: api.JobFailed, Code: api.JobCodeAptFailed}, true
	case classGone:
		if !w.gone {
			// 한 번 더 확인한다.
			w.gone = true
			return jobs.Outcome{}, !w.sleep(r.Interval, false)
		}
		return w.goneOutcome(), true
	case classTransient:
		now := r.Now()
		if w.failingSince.IsZero() {
			w.failingSince = now
			log.Printf("jobs: %s: omv not reachable, retrying: %v", w.job.ID, err)
		} else if now.Sub(w.failingSince) > r.OutageLimit {
			log.Printf("jobs: %s: omv not reachable for %v", w.job.ID, r.OutageLimit)
			return jobs.Outcome{State: api.JobUnknown, Code: api.JobCodeTimeout}, true
		}
	default:
		now := r.Now()
		msg := err.Error()
		if msg != w.errMsg {
			w.errMsg, w.errCount, w.errSince = msg, 0, now
		}
		w.errCount++
		if w.errCount >= failConfirm && now.Sub(w.errSince) >= r.FailConfirmFor {
			log.Printf("jobs: %s: %v", w.job.ID, err)
			return jobs.Outcome{State: api.JobFailed, Code: api.JobCodeFailed}, true
		}
	}
	d := r.Backoff[min(w.backoff, len(r.Backoff)-1)]
	w.backoff++
	return jobs.Outcome{}, !w.sleep(d, false)
}

// goneOutcome은 상태 파일이 사라졌을 때의 결과다. 실패한 작업은 OMV가 상태 파일을 지우지 않으므로, 사라졌다면
// 오류 없이 끝나 누가 먼저 끝까지 읽었거나(워크벤치의 작업 패널) 누가 멈춘 것이다. 바로 전 폴링이 성공했고 업데이트
// 목록이 작업 시작 뒤에 다시 만들어졌으면(apt 후크) 끝까지 돌았다고 보고 출력이 불완전한 성공으로 둔다.
func (w *watch) goneOutcome() jobs.Outcome {
	r := w.r
	recent := !w.lastOK.IsZero() && r.Now().Sub(w.lastOK) <= goneWindow
	refreshed := false
	if w.r.indexModTime != nil {
		if t, err := w.r.indexModTime(); err == nil && !t.Before(w.job.StartedAt) {
			refreshed = true
		}
	}
	if recent && refreshed {
		log.Printf("jobs: %s: status file gone after the job finished; output may be incomplete", w.job.ID)
		return jobs.Outcome{State: api.JobSucceeded, Incomplete: true}
	}
	log.Printf("jobs: %s: status file gone; result unknown", w.job.ID)
	return jobs.Outcome{State: api.JobUnknown, Code: api.JobCodeLost, Incomplete: true}
}

func (w *watch) handleJSON() json.RawMessage {
	b, _ := json.Marshal(w.h)
	return b
}

// sleep은 d만큼 기다린다. offers면 관리자 요청 신호에도 깬다. ctx가 끝나면 false다.
func (w *watch) sleep(d time.Duration, offers bool) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	var wake <-chan struct{}
	if offers {
		wake = w.sink.Offers()
	}
	select {
	case <-w.ctx.Done():
		return false
	case <-t.C:
	case <-wake:
	}
	return true
}
