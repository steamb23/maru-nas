package server

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"onmaru/backend/internal/api"
	"onmaru/backend/internal/auth"
)

const (
	// rejectDrainIdle은 거절한 올리기의 남은 본문을 비우다가 이만큼 받지 못하면 멈추는 시간이다(도우미의 무진행
	// 제한과 같다).
	rejectDrainIdle = 60 * time.Second
	// rejectDrainAnon은 세션 없이(401) 거절했거나 길이를 모르는 본문을 비우는 상한이다. 인증 없이 끝없이 받아 주는
	// 곳을 만들지 않는다. 넘으면 Go 서버의 기본 동작대로 연결을 닫는다.
	rejectDrainAnon = 1 << 20
)

// filesUpload는 요청 본문을 파일 하나로 올린다. 본문은 메모리에 모으지 않고 도우미로 바로 흘려 보내며, 도우미가
// 같은 폴더 임시 파일에 다 받은 뒤 이름을 바꾼다. 원시 본문(application/octet-stream)만 받는다(CSRF 보조 장치,
// HTML 폼은 이 형식을 보낼 수 없고 다른 출처의 fetch는 preflight에서 막힌다). 라우트는 drainRejected로 감싼다.
func (h *handler) filesUpload(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/octet-stream" {
		writeError(w, http.StatusUnsupportedMediaType, api.ErrBadRequest, "content type must be application/octet-stream")
		return
	}
	p := r.URL.Query().Get("path")
	if !h.uploadTarget(w, r, s, ticket, p) {
		return
	}
	if r.ContentLength < 0 {
		writeError(w, http.StatusLengthRequired, api.ErrBadRequest, "content length required")
		return
	}
	overwrite := r.URL.Query().Get("overwrite") == "1"
	e, err := h.Helper.Write(r.Context(), ticket, p, overwrite, r.Body, r.ContentLength)
	if err != nil {
		if r.Context().Err() != nil {
			// 브라우저가 올리기를 취소했다. 도우미가 임시 파일을 지운다.
			return
		}
		h.writeFilesError(w, s, err)
		return
	}
	writeJSON(w, http.StatusOK, fileItemOf(e))
}

// filesUploadCheck는 올리기의 사전 확인이다. 브라우저는 PUT으로 본문을 보내기 전에 이것을 불러, 본문 없이 알 수
// 있는 거절(세션, 보안 확인, 이름, 범위)을 먼저 받는다. 본문을 다 보낸 뒤에야 거절이 오는 프록시에서도 보안 확인
// 대화상자를 바로 띄우기 위해서다. 같은 가드(requireFilesWrite)와 같은 검사(uploadTarget)를 거치지만 사용성을
// 위한 것일 뿐이고, PUT은 모든 검사를 그대로 다시 한다. 도우미는 부르지 않는다(같은 이름, 쓰기 권한은 PUT이 안다).
func (h *handler) filesUploadCheck(w http.ResponseWriter, r *http.Request, s auth.Session, ticket string) {
	var req api.FilesUploadCheckRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if !h.uploadTarget(w, r, s, ticket, req.Path) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// uploadTarget은 p가 올릴 수 있는 경로인지 확인한다(새 이름 검사, 보이는 폴더 안). 아니면 오류 응답을 쓰고 false다.
func (h *handler) uploadTarget(w http.ResponseWriter, r *http.Request, s auth.Session, ticket, p string) bool {
	dir, _, ok := splitNew(p)
	if !ok {
		writeError(w, http.StatusBadRequest, api.ErrBadRequest, "invalid file name")
		return false
	}
	return h.allowedPath(w, r, s, ticket, dir)
}

// drainRejected는 본문이 큰 라우트(올리기)를 감싸, 본문을 다 받기 전에 오류로 답할 때 응답을 먼저 끝까지 보내고
// 남은 본문을 읽어 버린다.
//
// Go 서버는 핸들러가 본문을 읽기 전에 응답하고 남은 본문이 256KiB 이상이면 본문을 버리지 않고 연결을 닫는다
// (Connection: close, 500ms 뒤 닫기). 브라우저(HTTP/1.1)와 일부 리버스 프록시는 본문을 끝까지 보낸 뒤에야 응답을
// 읽으므로, 닫힌 연결에 쓰다가 RST를 받는다. 그러면 Vite 프록시는 거절을 502로 바꾸고 Caddy는 응답을 늦게 주거나
// 멈춘다. 그래서 오류 응답은 길이를 붙여 바로 보내고(응답을 일찍 읽는 프록시는 바로 넘긴다) 남은 본문을 받은 뒤
// 연결을 닫는다.
//
// 세션이 있는 요청은 남은 본문을 Content-Length까지 받는다. 성공한 올리기보다 더 받지 않으며, rejectDrainIdle 동안
// 받지 못하거나 연결이 끊기면 멈춘다. 세션이 없는 401과 길이를 모르는 본문은 rejectDrainAnon까지만 받는다.
func drainRejected(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		// HTTP/1은 응답을 쓰기 시작할 때 남은 본문을 버릴지 연결을 닫을지 정하므로 쓰기 전에 켠다. HTTP/2와
		// httptest.ResponseRecorder는 지원하지 않는다는 오류를 주고, 그때는 그대로 둔다.
		_ = rc.EnableFullDuplex()
		body := &countingBody{ReadCloser: r.Body}
		r.Body = body
		rw := &heldError{ResponseWriter: w}
		next(rw, r)
		if !rw.held {
			return
		}
		// left는 남은 본문이다. 길이를 모르면 -1이다.
		var left int64
		switch {
		case body.eof:
		case r.ContentLength < 0:
			left = -1
		default:
			left = max(r.ContentLength-body.n, 0)
		}
		limit := left
		if left < 0 || rw.status == http.StatusUnauthorized {
			limit = rejectDrainAnon
			if left >= 0 {
				limit = min(limit, left)
			}
		}
		if left != 0 {
			// 이 연결은 응답 뒤에 닫는다. 남은 본문은 그 전에 받으므로 RST가 가지 않고, Go 트랜스포트 같은 프록시는
			// 연결을 다시 쓸지 기다리지 않고 응답을 넘긴다.
			w.Header().Set("Connection", "close")
		}
		w.Header().Set("Content-Length", strconv.Itoa(rw.buf.Len()))
		w.WriteHeader(rw.status)
		w.Write(rw.buf.Bytes())
		if limit <= 0 {
			return
		}
		rc.Flush()
		drainBody(rc, body, limit)
	}
}

// drainBody는 body를 limit 바이트까지 읽어 버린다. rejectDrainIdle 동안 받지 못하거나 연결이 끊기면 멈춘다.
func drainBody(rc *http.ResponseController, body io.Reader, limit int64) {
	defer rc.SetReadDeadline(time.Time{})
	buf := make([]byte, 32<<10)
	for limit > 0 {
		// 지원하지 않으면(httptest) 기한 없이 읽는다.
		_ = rc.SetReadDeadline(time.Now().Add(rejectDrainIdle))
		n, err := body.Read(buf[:min(int64(len(buf)), limit)])
		limit -= int64(n)
		if err != nil {
			return
		}
	}
}

// countingBody는 핸들러가 읽은 본문의 양과 끝을 기록한다.
type countingBody struct {
	io.ReadCloser
	n   int64
	eof bool
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	if err == io.EOF {
		b.eof = true
	}
	return n, err
}

// heldError는 오류 응답(400 이상)을 바로 쓰지 않고 모아 둔다. drainRejected가 길이를 붙여 쓴다. 성공 응답은 그대로
// 지나간다.
type heldError struct {
	http.ResponseWriter
	held   bool
	status int
	buf    bytes.Buffer
	wrote  bool
}

func (w *heldError) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	if status >= 400 {
		w.held, w.status = true, status
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *heldError) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.held {
		return w.buf.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap은 http.ResponseController가 원래 응답에 닿게 한다.
func (w *heldError) Unwrap() http.ResponseWriter { return w.ResponseWriter }
