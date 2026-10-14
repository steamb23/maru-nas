package omvfake

import (
	"encoding/json"
	"net/http"
)

// 백그라운드 작업. OMV의 execBgProc·callMethodBg를 쓰는 메서드는 상태 파일 이름만 바로 돌려주고,
// 클라이언트는 Exec.isRunning(admin)으로 끝났는지 묻는다(engined/rpc/exec.inc).
//
//   - 끝나지 않았으면 {filename, running: true}
//   - 끝났으면 {filename, running: false}를 돌려주고 상태 파일을 지운다
//   - 작업이 실패했으면 그 오류를 예외(TraceException)로 던지고 상태 파일을 지운다
//   - 모르는(지워진) 상태 파일이면 오류다

// DefaultBgPolls는 백그라운드 작업이 끝나기까지 Exec.isRunning이 running: true를 돌려주는 횟수다.
const DefaultBgPolls = 1

type bgJob struct {
	// polls는 끝나기 전까지 남은 running: true 응답 수다.
	polls int
	// fail이 비어 있지 않으면 끝날 때 이 메시지로 실패한다.
	fail string
	// onDone은 작업이 성공해 끝날 때 부른다(s.mu를 쥔 채로).
	onDone func()
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

func (s *Server) startBg(fail string, onDone func()) string {
	name := "bgstatus" + newID()
	s.bg[name] = &bgJob{polls: s.bgPolls, fail: fail, onDone: onDone}
	return name
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
		return nil, errStatus(http.StatusInternalServerError, "Failed to read the background process status file.")
	}
	if job.polls > 0 {
		job.polls--
		return map[string]any{"filename": *p.Filename, "running": true}, nil
	}
	delete(s.bg, *p.Filename)
	if job.fail != "" {
		return nil, errStatus(http.StatusInternalServerError, job.fail)
	}
	if job.onDone != nil {
		job.onDone()
	}
	return map[string]any{"filename": *p.Filename, "running": false}, nil
}
