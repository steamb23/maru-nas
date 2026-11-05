// SPDX-License-Identifier: MIT

package maruaddon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
)

// HandlerFunc answers one data request or action. ctx ends when maru gives up on the request and closes the
// connection (its time limit is 15 seconds for data and the manifest's timeoutSeconds for actions).
//
// Shutting down does not end ctx: after SIGTERM or SIGINT, running handlers get up to [ShutdownTimeout] to finish,
// and ctx ends only if a handler is still running when that time runs out and its connection is closed.
type HandlerFunc func(ctx context.Context, r *Request) (any, error)

// Handler is a backend's request handlers.
type Handler struct {
	// Data answers GET /v1/data. A (v, nil) result answers 200 with v as JSON; a nil v answers JSON null. If Data
	// is nil, data requests answer 404 with code "no_data".
	Data HandlerFunc

	// Actions maps action names, as declared in the manifest's "actions", to their handlers. A (nil, nil) result
	// answers 204 (no result); a (v, nil) result answers 200 with v as JSON. Return a literal nil for 204: a typed
	// nil pointer is a value and answers JSON null. Unknown names answer 404 with code "unknown_action".
	Actions map[string]HandlerFunc

	// ErrorLog receives internal errors, handler panics and server errors. If nil, they are written to standard
	// error, which systemd writes to the unit's journal. Request headers, and so the call token, are never logged.
	ErrorLog *log.Logger
}

// ShutdownTimeout is how long [Serve] and [ServeListener] wait for in-flight requests after SIGTERM or SIGINT
// (and [ServeContext] after its context ends) before closing their connections. It is shorter than systemd's default
// stop timeout of 90 seconds.
const ShutdownTimeout = 60 * time.Second

// maxResponseBytes는 maru가 받는 백엔드 응답의 상한(4MiB)이다. 넘으면 maru가 502로 바꾸므로 SDK가 500으로 기록한다.
const maxResponseBytes = 4 << 20

// Serve serves h until the process receives SIGTERM or SIGINT, then shuts down gracefully: it stops accepting
// connections and waits up to [ShutdownTimeout] for running requests. It returns nil after a graceful shutdown, and
// an error if requests were still running when the time ran out (their connections are then closed). A second
// SIGTERM or SIGINT during the wait ends the process at once, as the signal does by default.
//
// The listener comes from the environment:
//
//   - systemd socket activation: if LISTEN_PID is this process and LISTEN_FDS is 1, Serve uses file descriptor 3
//     and removes LISTEN_PID, LISTEN_FDS and LISTEN_FDNAMES from the environment. This is how installed backends
//     run. Any other LISTEN_FDS for this process is an error.
//   - MARU_ADDON_SOCKET: otherwise Serve listens on this Unix socket path (development). A leftover socket file
//     that no process is listening on is removed first; any other file at the path is an error. The socket is
//     made mode 0600 and removed when Serve returns.
//
// If neither is set, Serve returns an error. OMV calls go to the host socket at MARU_ADDON_HOST, or
// [DefaultHostSocket] if it is not set.
func Serve(h Handler) error {
	if err := h.check(); err != nil {
		return err
	}
	l, err := listen(osEnviron())
	if err != nil {
		return err
	}
	return ServeListener(l, h)
}

// ServeListener is like [Serve] with a listener the caller made. It closes l when it returns.
func ServeListener(l net.Listener, h Handler) error {
	// 처리기가 돌기 전에 신호를 받도록 먼저 등록한다
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	// 첫 신호를 받으면 등록을 풀어 기본 동작으로 돌린다. 끝내기를 기다리는 동안 두 번째 신호(개발 중의 Ctrl+C)는
	// 프로세스를 바로 끝낸다. 신호 없이 돌아올 때(수신 소켓 오류)를 위해 defer도 둔다
	context.AfterFunc(ctx, stop)
	defer stop()
	return ServeContext(ctx, l, h)
}

// ServeContext serves h on l until ctx ends, then shuts down gracefully: it stops accepting connections and waits
// up to [ShutdownTimeout] for in-flight requests. It returns nil after a graceful shutdown and an error if l fails
// or requests were still running when the time ran out. It closes l when it returns. It does not handle signals.
func ServeContext(ctx context.Context, l net.Listener, h Handler) error {
	return serve(ctx, l, h, hostSocketPath(os.Getenv), ShutdownTimeout)
}

func (h Handler) check() error {
	if h.Data == nil && len(h.Actions) == 0 {
		return errors.New("maruaddon: handler has no Data function and no Actions")
	}
	for name, fn := range h.Actions {
		if name == "" || fn == nil {
			return fmt.Errorf("maruaddon: action %q has no handler or no name", name)
		}
	}
	return nil
}

// server는 처리기 하나와 그 요청들이 함께 쓰는 호스트 클라이언트다.
type server struct {
	h    Handler
	log  *log.Logger
	host *hostClient
}

func newServer(h Handler, hostPath string) *server {
	s := &server{h: h, log: h.ErrorLog, host: newHostClient(hostPath)}
	if s.log == nil {
		s.log = log.New(os.Stderr, "", 0)
	}
	return s
}

func serve(ctx context.Context, l net.Listener, h Handler, hostPath string, shutdownTimeout time.Duration) error {
	if err := h.check(); err != nil {
		l.Close()
		return err
	}
	s := newServer(h, hostPath)
	defer s.host.close()
	srv := &http.Server{
		Handler: s.routes(),
		// 조작은 manifest의 시간 제한(600초까지) 동안 돌 수 있으므로 쓰기 시간 제한은 두지 않는다
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          s.log,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(l) }()
	select {
	case err := <-errc:
		return fmt.Errorf("maruaddon: serve: %w", err)
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close()
		<-errc
		return fmt.Errorf("maruaddon: shutdown: requests still running after %s: %w", shutdownTimeout, err)
	}
	<-errc
	return nil
}

// routes는 계약의 두 경로와, JSON으로 답하는 405·404다(기본 ServeMux의 404·405는 평문이다).
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/data", s.data)
	mux.HandleFunc("/v1/data", s.methodNotAllowed(http.MethodGet))
	mux.HandleFunc("POST /v1/actions/{name}", s.action)
	mux.HandleFunc("/v1/actions/{name}", s.methodNotAllowed(http.MethodPost))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, &Error{Status: http.StatusNotFound, Code: "not_found", Message: "not found"})
	})
	return s.recoverPanics(mux)
}

func (s *server) methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Allow", allow)
		writeError(w, &Error{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed",
			Message: "method not allowed"})
	}
}

// recoverPanics는 처리기의 panic을 500으로 답한다. net/http에 맡기면 응답 없이 연결을 끊어 maru가 백엔드를 쓸 수
// 없다고(503) 본다. 응답은 처리기가 끝난 뒤에 한 번에 쓰므로 panic 때는 아직 쓰지 않았다.
func (s *server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v)
			}
			s.log.Printf("maruaddon: %s %q: panic: %v\n%s", r.Method, r.URL.Path, v, debug.Stack())
			writeError(w, errInternal)
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *server) data(w http.ResponseWriter, r *http.Request) {
	if s.h.Data == nil {
		writeError(w, &Error{Status: http.StatusNotFound, Code: "no_data", Message: "this backend has no data"})
		return
	}
	req, e := newRequest(r, s.host)
	if e != nil {
		writeError(w, e)
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, badRequest("invalid query string"))
		return
	}
	req.Query = q
	v, err := s.h.Data(r.Context(), req)
	s.respond(w, "data", req, v, err, false)
}

func (s *server) action(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fn := s.h.Actions[name]
	if fn == nil {
		writeError(w, &Error{Status: http.StatusNotFound, Code: "unknown_action",
			Message: fmt.Sprintf("unknown action %q", name)})
		return
	}
	req, e := newRequest(r, s.host)
	if e != nil {
		writeError(w, e)
		return
	}
	target, input, e := readActionBody(w, r)
	if e != nil {
		writeError(w, e)
		return
	}
	req.Action, req.Target, req.Input = name, target, input
	v, err := fn(r.Context(), req)
	s.respond(w, fmt.Sprintf("action %q", name), req, v, err, true)
}

// errInternal은 처리기 오류를 숨기는 응답이다. 실제 오류는 ErrorLog에만 남긴다.
var errInternal = &Error{Status: http.StatusInternalServerError, Code: "internal", Message: "internal error"}

// respond는 처리기의 결과를 응답으로 바꾼다. 로그에는 무엇(데이터·조작 이름)과 사용자, 오류만 남긴다.
func (s *server) respond(w http.ResponseWriter, what string, req *Request, v any, err error, action bool) {
	if err != nil {
		var e *Error
		if errors.As(err, &e) && e.valid() {
			writeError(w, e)
			return
		}
		if e != nil {
			s.log.Printf("maruaddon: %s (user %q): error status %d and code %q cannot be passed to maru, "+
				"answering 500: %v", what, req.User, e.Status, e.Code, err)
		} else {
			s.log.Printf("maruaddon: %s (user %q): %v", what, req.User, err)
		}
		writeError(w, errInternal)
		return
	}
	if action && v == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		s.log.Printf("maruaddon: %s (user %q): encode result: %v", what, req.User, err)
		writeError(w, errInternal)
		return
	}
	if len(body) > maxResponseBytes {
		s.log.Printf("maruaddon: %s (user %q): result is %d bytes, larger than the 4 MiB maru accepts",
			what, req.User, len(body))
		writeError(w, errInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func writeError(w http.ResponseWriter, e *Error) {
	body, _ := json.Marshal(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{e.Code, e.Message})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	w.Write(body)
}
