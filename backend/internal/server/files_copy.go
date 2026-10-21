package server

import (
	"encoding/json"
	"log"
	"net/http"
	"path"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
	"onmaru/backend/internal/files"
	"onmaru/backend/internal/helper/proto"
)

// filesCopy는 한 폴더 안의 항목을 다른 폴더로 복사한다(다른 디스크로 끌어 놓기). 복사는 도우미가 사용자 권한으로
// 하고, 진행은 요청이 열려 있는 동안 text/event-stream으로 보낸다(서버에 백그라운드 작업은 없다). 브라우저가 요청을
// 끊으면 도우미 연결을 닫고, 도우미는 하던 항목의 임시 이름을 지우고 멈춘다.
//
// 시작하기 전의 실패(보안 정책, 범위, 대상이 폴더가 아님 등)는 보통의 오류 응답이고, 첫 진행을 받은 뒤의 실패는
// 마지막 사건(Error)이다. 이름마다의 실패는 결과(Failed)에 담긴다.
func (h *handler) filesCopy(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesCopyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if len(req.Names) == 0 || len(req.Names) > proto.MaxCopy {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "names must have 1 to 512 entries")
		return
	}
	for _, n := range req.Names {
		// 같은 이름으로 만들므로 새 이름과 같은 검사를 한다.
		if !newNameOK(n) {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid name")
			return
		}
		if files.Within(req.To, path.Join(req.Dir, n)) {
			writeError(w, http.StatusBadRequest, api.ErrBadRequest, "cannot copy a folder into itself")
			return
		}
	}
	params := proto.CopyParams{Dir: req.Dir, Names: req.Names, To: req.To}
	if b, err := json.Marshal(params); err != nil || len(b) > proto.MaxCopyParams {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "names are too long for one request")
		return
	}
	roots, ok := h.allowedPaths(w, r, s, ticket, req.Dir, req.To)
	if !ok {
		return
	}
	// 원본은 읽기만 하므로 대상만 쓸 수 있는 곳이어야 한다.
	if readOnlyAt(roots, req.To) {
		writeError(w, http.StatusForbidden, api.ErrShareReadOnly, "the shared folder privileges allow reading only")
		return
	}

	ev := newEventStream(w)
	send := func(e api.FilesCopyEvent) { ev.send(e) }
	res, err := h.Helper.Copy(r.Context(), ticket, params, func(p proto.CopyProgress) {
		send(api.FilesCopyEvent{Progress: &api.FilesCopyProgress{
			Scanning: p.Scanning, Files: p.Files, Bytes: p.Bytes, TotalFiles: p.TotalFiles, TotalBytes: p.TotalBytes, Current: p.Current,
		}})
	})
	if err != nil {
		if r.Context().Err() != nil {
			// 브라우저가 복사를 취소했다.
			return
		}
		if !ev.started {
			h.writeFilesError(w, s, err)
			return
		}
		e, status := h.filesErrorEvent(s, err)
		send(api.FilesCopyEvent{Error: &e, Status: status})
		return
	}
	out := api.FilesCopyResult{Copied: res.Copied, Failed: []api.FilesCopyFailure{}, Skipped: res.Skipped}
	if out.Copied == nil {
		out.Copied = []string{}
	}
	for _, f := range res.Failed {
		code := api.ErrHelperUnavailable
		if _, c, ok := filesErrorCode(f.Code); ok {
			code = c
		} else if f.Code == proto.CodeBadRequest {
			code = api.ErrBadRequest
		} else {
			log.Printf("files: copy user=%q name=%q: %s", s.Username, f.Name, f.Code)
		}
		out.Failed = append(out.Failed, api.FilesCopyFailure{Name: f.Name, Code: code})
	}
	send(api.FilesCopyEvent{Result: &out})
}

// eventStream은 text/event-stream 응답이다(복사·검색의 진행). 첫 사건을 보낼 때 머리를 쓴다.
type eventStream struct {
	w       http.ResponseWriter
	rc      *http.ResponseController
	started bool
}

func newEventStream(w http.ResponseWriter) *eventStream {
	return &eventStream{w: w, rc: http.NewResponseController(w)}
}

// send는 사건 하나(data 줄의 JSON)를 보내고 바로 내보낸다.
func (e *eventStream) send(v any) {
	if !e.started {
		e.started = true
		e.w.Header().Set("Content-Type", "text/event-stream")
		e.w.Header().Set("Cache-Control", "no-store")
		// nginx 계열 프록시가 응답을 모아 두지 않게 한다.
		e.w.Header().Set("X-Accel-Buffering", "no")
		e.w.WriteHeader(http.StatusOK)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	e.w.Write([]byte("data: "))
	e.w.Write(b)
	e.w.Write([]byte("\n\n"))
	e.rc.Flush()
}

// filesErrorEvent는 진행을 보낸 뒤 멈춘 오류를 사건에 담을 오류와 HTTP 상태로 바꾼다. 같은 오류를 보통의 응답으로
// 만들어 읽는다(코드 변환과 도우미 상태 처리를 한곳에 둔다).
func (h *handler) filesErrorEvent(s auth.Session, err error) (api.ErrorResponse, int) {
	rec := &errorRecorder{header: http.Header{}}
	h.writeFilesError(rec, s, err)
	var e api.ErrorResponse
	if json.Unmarshal(rec.body, &e) != nil {
		e = api.ErrorResponse{Error: "privileged helper failed", Code: api.ErrHelperUnavailable}
	}
	return e, rec.status
}

// errorRecorder는 오류 응답 하나를 메모리에 받는다(복사를 시작한 뒤의 오류를 사건으로 보낼 때).
type errorRecorder struct {
	header http.Header
	status int
	body   []byte
}

func (e *errorRecorder) Header() http.Header { return e.header }

func (e *errorRecorder) Write(b []byte) (int, error) {
	e.body = append(e.body, b...)
	return len(b), nil
}

func (e *errorRecorder) WriteHeader(status int) { e.status = status }
