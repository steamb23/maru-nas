package omvfake

import (
	"encoding/json"
	"net/http"
	"strings"
)

// 백그라운드 작업. OMV의 execBgProc·callMethodBg를 쓰는 메서드는 상태 파일 이름(/tmp/bgstatusXXXX)만 바로 돌려주고,
// 클라이언트는 Exec.isRunning이나 Exec.getOutput(admin)으로 끝났는지 묻는다(engined/rpc/exec.inc).
//
//   - 둘 다 작업의 오류를 먼저 확인한다. 실패한 작업이면 그 오류를 예외(TraceException)로 던지고 상태 파일은 지우지
//     않는다(실제 OMV도 지우지 않아 tmpfiles 정리까지 남는다). 그래서 같은 오류가 매번 나온다
//   - isRunning은 {filename, running}을 돌려주고, 끝났으면 상태 파일을 지운다(출력을 읽지 않았어도)
//   - getOutput {filename, pos, length?}은 {filename, pos, output, pendingOutput, running}을 돌려준다. length가 있으면
//     그만큼만 읽고 더 남았으면 pendingOutput이다. 끝났고 남은 출력이 없으면 상태 파일을 지운다
//   - 모르는(지워진) 상태 파일이면 "Failed to open file (filename=…, mode=r)." 오류다
//
// 가짜 작업은 폴링(isRunning·getOutput 호출)마다 한 걸음 나아간다. 출력 조각(chunks)이 남았으면 하나를 출력에 붙이고,
// 없으면 남은 polls를 하나 줄인다. 둘 다 없으면 끝난 것이다.

// DefaultBgPolls는 백그라운드 작업이 끝나기까지 running: true를 돌려주는 횟수다(출력 조각 수와 별개).
const DefaultBgPolls = 1

type bgJob struct {
	// chunks는 아직 출력에 붙지 않은 조각이다. 폴링마다 하나씩 붙는다.
	chunks []string
	// out은 지금까지의 출력이다.
	out []byte
	// polls는 조각을 다 붙인 뒤 끝나기 전까지 남은 running: true 응답 수다.
	polls int
	// fail이 비어 있지 않으면 끝날 때 이 메시지로 실패한다.
	fail string
	// onDone은 작업이 성공해 끝날 때 한 번 부른다(s.mu를 쥔 채로).
	onDone func()
	done   bool
}

// step은 작업을 한 걸음 진행하고 아직 실행 중인지 돌려준다.
func (j *bgJob) step() bool {
	switch {
	case len(j.chunks) > 0:
		j.out = append(j.out, j.chunks[0]...)
		j.chunks = j.chunks[1:]
		return true
	case j.polls > 0:
		j.polls--
		return true
	}
	return false
}

// StartBg는 백그라운드 작업을 등록하고 상태 파일 이름을 돌려준다. fail이 비어 있지 않으면 그 메시지로
// 실패한다. 테스트가 쓰기 메서드 없이 Exec.isRunning 흐름을 확인할 때 쓴다.
func (s *Server) StartBg(fail string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startBg(fail, nil)
}

// SetBgPolls는 백그라운드 작업이 끝나기까지 running: true를 돌려주는 횟수를 바꾼다.
func (s *Server) SetBgPolls(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bgPolls = n
}

// DropBg는 등록된 백그라운드 작업을 모두 지운다. 다른 클라이언트(워크벤치의 작업 패널)가 먼저 끝까지 읽어
// 상태 파일을 지운 상황을 흉내 낸다.
func (s *Server) DropBg() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.bg)
}

// BgJobs는 등록된(아직 지워지지 않은) 백그라운드 작업 수다.
func (s *Server) BgJobs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.bg)
}

func (s *Server) startBg(fail string, onDone func()) string {
	return s.startJob(&bgJob{polls: s.bgPolls, fail: fail, onDone: onDone})
}

func (s *Server) startJob(j *bgJob) string {
	name := "/tmp/bgstatus" + newID()
	s.bg[name] = j
	return name
}

// finishJob은 끝난 작업을 지우고 성공했으면 onDone을 부른다.
func (s *Server) finishJob(name string, j *bgJob) {
	delete(s.bg, name)
	if !j.done && j.onDone != nil {
		j.done = true
		j.onDone()
	}
}

func errOpenStatus(name string) *rpcError {
	return errStatus(http.StatusInternalServerError, "Failed to open file (filename="+name+", mode=r).")
}

func (s *Server) execIsRunning(sess *session, params json.RawMessage) (any, *rpcError) {
	if !sess.admin {
		return nil, errStatus(http.StatusForbidden, "Invalid context role.")
	}
	var p struct {
		Filename *string `json:"filename"`
	}
	if json.Unmarshal(params, &p) != nil || p.Filename == nil {
		return nil, errStatus(http.StatusInternalServerError, "Invalid RPC parameters.")
	}
	job, ok := s.bg[*p.Filename]
	if !ok {
		return nil, errOpenStatus(*p.Filename)
	}
	if job.step() {
		return map[string]any{"filename": *p.Filename, "running": true}, nil
	}
	if job.fail != "" {
		return nil, errStatus(http.StatusInternalServerError, job.fail)
	}
	s.finishJob(*p.Filename, job)
	return map[string]any{"filename": *p.Filename, "running": false}, nil
}

func (s *Server) execGetOutput(sess *session, params json.RawMessage) (any, *rpcError) {
	if !sess.admin {
		return nil, errStatus(http.StatusForbidden, "Invalid context role.")
	}
	var p struct {
		Filename *string `json:"filename"`
		Pos      *int64  `json:"pos"`
		Length   *int64  `json:"length"`
	}
	// rpc.exec.getoutput: filename·pos는 필수, pos는 0 이상, length는 1024 이상.
	if json.Unmarshal(params, &p) != nil || p.Filename == nil || p.Pos == nil || *p.Pos < 0 ||
		(p.Length != nil && *p.Length < 1024) {
		return nil, errStatus(http.StatusBadRequest, "Invalid RPC parameters.")
	}
	job, ok := s.bg[*p.Filename]
	if !ok {
		return nil, errOpenStatus(*p.Filename)
	}
	running := job.step()
	if !running && job.fail != "" {
		return nil, errStatus(http.StatusInternalServerError, job.fail)
	}
	pos := min(*p.Pos, int64(len(job.out)))
	chunk := job.out[pos:]
	pending := false
	if p.Length != nil && int64(len(chunk)) > *p.Length {
		chunk, pending = chunk[:*p.Length], true
	}
	if !running && !pending {
		s.finishJob(*p.Filename, job)
	}
	return map[string]any{
		"filename":      *p.Filename,
		"pos":           pos + int64(len(chunk)),
		"output":        strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(string(chunk)),
		"pendingOutput": pending,
		"running":       running,
	}, nil
}

// Glitch는 다음 호출 몇 번을 실패시키는 방식이다.
type Glitch int

const (
	// GlitchProxy는 nginx 오류 페이지(HTML, 502)다. php-fpm이 다시 시작하는 동안을 흉내 낸다.
	GlitchProxy Glitch = iota + 1
	// GlitchEngine은 rpc.php의 JSON 500 "Failed to connect to socket"이다. engined가 다시 시작하는 동안을 흉내 낸다.
	GlitchEngine
)

// SetGlitch는 service.method 호출의 다음 n번을 g 방식으로 실패시킨다.
func (s *Server) SetGlitch(service, method string, n int, g Glitch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.glitches == nil {
		s.glitches = make(map[string]glitch)
	}
	s.glitches[service+"."+method] = glitch{n: n, kind: g}
}

type glitch struct {
	n    int
	kind Glitch
}

// takeGlitch는 이 호출을 실패시킬지 정한다. s.mu를 쥔 채로 부른다.
func (s *Server) takeGlitch(service, method string) Glitch {
	g, ok := s.glitches[service+"."+method]
	if !ok || g.n <= 0 {
		return 0
	}
	g.n--
	s.glitches[service+"."+method] = g
	return g.kind
}
