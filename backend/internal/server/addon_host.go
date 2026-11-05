package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"maru/backend/internal/addonpkg"
	"maru/backend/internal/addonpkg/format"
	"maru/backend/internal/addons"
	"maru/backend/internal/auth"
	"maru/backend/internal/omv"
)

// 호스트 소켓(.claude/docs/addon-packages.md의 "호스트 소켓"). 패키지 백엔드는 OMV를 직접 부르지 못하고, maru가 백엔드
// 패키지마다 연 유닉스 소켓에 요청을 보낸다. 요청에는 maru가 바깥 요청(데이터·조작)마다 만든 호출 토큰을 붙인다. 토큰은
// 바깥 요청이 끝나면 지우므로 백엔드는 그 요청을 처리하는 동안에만 OMV를 부를 수 있다.

// 호스트 소켓의 상한.
const (
	hostReadHeaderTimeout = 5 * time.Second
	// hostMaxHeaderBytes는 http.Server.MaxHeaderBytes 값이다. net/http는 이 값에 bufio 여유 4096바이트를 더한 만큼
	// 읽게 하므로(initialReadLimitSize), 요청 줄과 헤더를 합친 실제 상한이 계약의 8KiB가 되도록 그만큼 뺀다.
	hostMaxHeaderBytes = 8<<10 - 4096
	hostIdleTimeout    = 30 * time.Second
	// hostBodyTimeout은 처리기가 시작한 뒤 본문을 읽는 시간 제한이다. 처리기가 오래 도는 요청(rpc-bg)이 있어 서버의
	// ReadTimeout은 쓰지 않는다(ReadTimeout이 지나면 처리 중인 요청의 문맥이 끝난다).
	hostBodyTimeout = 5 * time.Second
	// hostMaxConns는 소켓 하나의 동시 연결 수다. 넘으면 연결을 받지 않고 기다리게 한다(운영 소켓은 0666이라 토큰 없는
	// 연결도 들어온다). 연결한 uid 확인(BackendPaths.HostPeer)에서 거절한 연결은 칸을 차지하지 않는다. 칸이 모두 차
	// 있으면 거절할 연결도 받기 전이라 커널 대기열에서 칸이 날 때까지 기다린다.
	hostMaxConns = 16
	// hostMaxRequests는 애드온 하나의 동시 요청 수다. 넘으면 429 busy다.
	hostMaxRequests = 8
	// hostMaxBody는 요청 본문의 상한이다.
	hostMaxBody = 64 << 10
	// hostMaxApplyCalls는 /v1/apply 한 번의 호출 수 상한이다.
	hostMaxApplyCalls = 16
	// hostMaxBgOutput은 /v1/rpc-bg가 돌려주는 출력의 상한이다.
	hostMaxBgOutput = 1 << 20
	// hostMaxOMVMessage는 백엔드에 넘기는 OMV 오류 문구의 상한(바이트)이다.
	hostMaxOMVMessage = 1 << 10
	// maxHostCalls는 호출 토큰 표의 크기다. 가득 차면 바깥 요청이 503 addon_backend_unavailable이다.
	maxHostCalls = 256
	// hostCallGrace는 토큰 만료가 바깥 요청의 제한 시각보다 늦은 시간이다.
	hostCallGrace = 5 * time.Second
	// hostCallHeader는 호출 토큰 헤더다(백엔드 요청과 호스트 요청이 같은 이름을 쓴다).
	hostCallHeader = "X-Maru-Call"
)

// 호스트 소켓의 오류 코드({"error", "code"}). 대시보드 API 오류 코드와 따로 둔다(백엔드만 본다).
const (
	hostErrBadRequest     = "bad_request"
	hostErrNotFound       = "not_found"
	hostErrCallInvalid    = "call_invalid"
	hostErrUnauthorized   = "unauthorized"
	hostErrForbidden      = "rpc_forbidden"
	hostErrAdminRequired  = "admin_required"
	hostErrBusy           = "busy"
	hostErrOMV            = "omv_error"
	hostErrOMVUnavailable = "omv_unavailable"
	hostErrTimeout        = "timeout"
)

// bgStatusPattern은 OMV 백그라운드 작업의 상태 파일 이름이다(execBgProc).
var bgStatusPattern = regexp.MustCompile(`^/tmp/bgstatus[A-Za-z0-9]+$`)

// hostScope는 호출 토큰의 범위다. 데이터 요청은 읽기, 조작은 쓰기다.
type hostScope int

const (
	hostRead hostScope = iota + 1
	hostWrite
)

// hostCall은 호출 토큰 하나에 묶인 바깥 요청이다.
type hostCall struct {
	addon   string
	session auth.Session
	scope   hostScope
	// caller는 바깥 요청의 호출기다. 백엔드가 부른 OMV가 401을 주면 바깥 요청이 대시보드 세션을 지운다.
	caller *rpcCaller
	// ctx는 바깥 요청의 문맥이다. 끝나면(제한 시각) 호스트 요청도 끝낸다.
	ctx     context.Context
	expires time.Time
}

// hostCalls는 호출 토큰 표다. 토큰은 32바이트 무작위 값(base64url)이고 로그·응답에 남기지 않는다.
type hostCalls struct {
	// now는 만료를 보는 시각이다. 문맥의 제한 시각(실제 시각)과 비교하므로 서버의 Config.Now(테스트 시계)를 쓰지 않는다.
	now func() time.Time
	max int

	mu    sync.Mutex
	calls map[string]*hostCall
}

func newHostCalls() *hostCalls {
	return &hostCalls{now: time.Now, max: maxHostCalls, calls: map[string]*hostCall{}}
}

// issue는 c의 토큰을 만든다. 만료는 c.ctx의 제한 시각 + hostCallGrace다. 표가 가득 차면 false다.
func (t *hostCalls) issue(c *hostCall) (string, bool) {
	deadline, ok := c.ctx.Deadline()
	if !ok {
		deadline = t.now().Add(addons.DefaultActionTimeout)
	}
	c.expires = deadline.Add(hostCallGrace)
	b := make([]byte, 32)
	rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for k, e := range t.calls {
		if now.After(e.expires) {
			delete(t.calls, k)
		}
	}
	if len(t.calls) >= t.max {
		return "", false
	}
	t.calls[tok] = c
	return tok, true
}

// lookup은 토큰의 바깥 요청이다. 없거나 만료됐으면 false다.
func (t *hostCalls) lookup(tok string) (*hostCall, bool) {
	if tok == "" {
		return nil, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.calls[tok]
	if !ok || t.now().After(c.expires) {
		return nil, false
	}
	return c, true
}

// revoke는 바깥 요청이 끝날 때 토큰을 지운다.
func (t *hostCalls) revoke(tok string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.calls, tok)
}

// removeSession은 대시보드 세션이 끝날 때 그 세션의 토큰을 지운다.
func (t *hostCalls) removeSession(session string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, c := range t.calls {
		if c.session.ID == session {
			delete(t.calls, k)
		}
	}
}

// addonHosts는 백엔드 패키지마다 연 호스트 소켓이다. 레지스트리가 바뀔 때마다 맞춘다(reconcile).
type addonHosts struct {
	h     *handler
	paths addonpkg.BackendPaths
	// bgInterval은 rpc-bg가 백그라운드 작업을 묻는 간격이다(나노초, 테스트에서 줄인다). 소켓 처리기가 읽으므로 원자 값이다.
	bgInterval atomic.Int64

	mu     sync.Mutex
	open   map[string]*hostSocket
	closed bool
	wg     sync.WaitGroup
	// failed는 열지 못한 소켓의 마지막 이유다(같은 이유를 다시 로그에 남기지 않는다).
	failed map[string]string
}

// hostSocket은 애드온 하나의 호스트 소켓이다.
type hostSocket struct {
	path string
	srv  *http.Server
	ln   net.Listener
}

// close는 소켓을 닫는다. http.Server.Close는 Serve가 아직 리스너를 받기 전이면 그 리스너를 닫지 않으므로 직접도
// 닫는다(닫으면 소켓 파일이 바로 지워진다. 운영 경로의 <id>/ 디렉터리는 남긴다, listen 참고).
func (s *hostSocket) close() {
	s.srv.Close()
	s.ln.Close()
}

func newAddonHosts(h *handler, paths addonpkg.BackendPaths) *addonHosts {
	a := &addonHosts{h: h, paths: paths, open: map[string]*hostSocket{}, failed: map[string]string{}}
	a.bgInterval.Store(int64(addons.BgPollInterval))
	return a
}

// reconcile은 지금 레지스트리의 백엔드 패키지(켜져 있고 받은 설치본, 개발본)마다 호스트 소켓을 열고, 빠진 패키지의
// 소켓을 닫는다. Manager.OnChange가 Manager의 잠금을 쥔 채로 부르므로 잠금 없이 읽는 Active만 쓴다.
func (a *addonHosts) reconcile() {
	want := map[string]bool{}
	for _, p := range a.h.Packages.Active() {
		if p.Manifest.HasBackend() {
			want[p.ID] = true
		}
	}
	a.h.backends.Retain(want)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}
	for id, s := range a.open {
		if !want[id] {
			s.close()
			delete(a.open, id)
			log.Printf("addon %s: host socket closed", id)
		}
	}
	for id := range a.failed {
		if !want[id] {
			delete(a.failed, id)
		}
	}
	for id := range want {
		// 열지 못하면 다음 레지스트리 변경이나 그 백엔드로 보내는 요청(ensure) 때 다시 연다.
		a.openLocked(id)
	}
}

// ensure는 id의 호스트 소켓이 열려 있게 한다(백엔드로 요청을 보내기 전). 운영 경로는 백엔드의 소켓 유닛이 디렉터리를
// 만든 뒤에야 열 수 있으므로 레지스트리가 바뀔 때 열지 못했을 수 있다. 지금 레지스트리에 없는 패키지는 열지 않는다
// (제거와 겹치면 reconcile이 닫는다).
func (a *addonHosts) ensure(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return false
	}
	if _, ok := a.open[id]; ok {
		return true
	}
	if p, ok := a.h.Packages.Get(id); !ok || !p.Manifest.HasBackend() {
		return false
	}
	return a.openLocked(id)
}

// openLocked는 id의 소켓이 없으면 연다. 열지 못한 이유는 바뀔 때만 로그에 남긴다. a.mu를 쥐고 부른다.
func (a *addonHosts) openLocked(id string) bool {
	if _, ok := a.open[id]; ok {
		return true
	}
	s, err := a.listen(id)
	if err != nil {
		if msg := err.Error(); a.failed[id] != msg {
			a.failed[id] = msg
			log.Printf("addon %s: host socket: %v", id, err)
		}
		return false
	}
	delete(a.failed, id)
	a.open[id] = s
	log.Printf("addon %s: host socket %s", id, s.path)
	return true
}

// close는 모든 호스트 소켓을 닫는다. 그 뒤의 reconcile은 아무것도 하지 않는다.
func (a *addonHosts) close() {
	a.mu.Lock()
	a.closed = true
	for id, s := range a.open {
		s.close()
		delete(a.open, id)
	}
	a.mu.Unlock()
	a.wg.Wait()
}

// socketPath는 id의 호스트 소켓이 열려 있으면 그 경로다(테스트).
func (a *addonHosts) socketPath(id string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.open[id]
	if !ok {
		return "", false
	}
	return s.path, true
}

// listen은 id의 호스트 소켓을 연다. 운영 경로(HostDirFromUnit)의 <AddonHostDir>/<id>/는 백엔드의 소켓 유닛이 만들고
// maru는 만들지도 지우지도 않는다(상위 디렉터리가 root 소유다). 백엔드 유닛이 그 디렉터리를 /run/maru-host로 바인드하므로
// maru가 다시 시작해도 디렉터리는 그대로이고, 남은 소켓 파일은 아무도 받지 않으므로 listenUnixSocket이 지우고 같은
// 디렉터리에 새로 연다.
func (a *addonHosts) listen(id string) (*hostSocket, error) {
	path := a.paths.Host(id)
	if a.paths.HostDirFromUnit {
		dir := filepath.Dir(path)
		fi, err := os.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s does not exist yet (the backend socket unit creates it when the sync enables it)", dir)
		}
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("%s is not a directory", dir)
		}
	}
	ln, err := listenUnixSocket(path, a.paths.HostMode)
	if err != nil {
		return nil, err
	}
	srv := &http.Server{
		Handler:           a.handler(id, make(chan struct{}, hostMaxRequests)),
		ReadHeaderTimeout: hostReadHeaderTimeout,
		MaxHeaderBytes:    hostMaxHeaderBytes,
		IdleTimeout:       hostIdleTimeout,
	}
	// 연결한 uid 확인은 받은 연결에 바로 한다(limitListener가 잡은 칸 하나로 받기를 되풀이하며, 받지 않은 연결은 칸을
	// 더 쓰지 않고 닫힌다).
	if allow := a.paths.HostPeer; allow != nil {
		ln = &peerListener{Listener: ln, id: id, allow: allow}
	}
	limited := newLimitListener(ln, hostMaxConns)
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := srv.Serve(limited); !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Printf("addon %s: host socket: %v", id, err)
		}
	}()
	return &hostSocket{path: path, srv: srv, ln: limited}, nil
}

// listenUnixSocket은 path에 유닉스 소켓을 열고 권한을 mode로 둔다. 이전 실행이 남긴 소켓 파일은 지우지만, 소켓이 아닌
// 파일과 다른 프로세스가 쓰고 있는 소켓은 건드리지 않는다(cmd/maru의 listenOn과 같은 규칙). 닫으면 리스너가 파일을 지운다.
func listenUnixSocket(path string, mode fs.FileMode) (net.Listener, error) {
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket", path)
		}
		if c, err := net.Dial("unix", path); err == nil {
			c.Close()
			return nil, fmt.Errorf("%s is in use by another process", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, mode); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// peerListener는 연결한 프로세스의 uid(SO_PEERCRED)가 allow를 통과한 연결만 돌려준다. 나머지는 받자마자 닫고 다음
// 연결을 기다린다. 거절은 로그를 남기되 소켓마다 peerLogEvery에 한 번만 남긴다(연결을 계속 다시 맺는 클라이언트가
// 로그를 채우지 않게).
type peerListener struct {
	net.Listener
	id    string
	allow func(uid int) bool

	mu      sync.Mutex
	lastLog time.Time
	dropped int
}

// peerLogEvery는 거절 로그의 간격이다.
const peerLogEvery = time.Minute

func (l *peerListener) Accept() (net.Conn, error) {
	for {
		c, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		uid, err := peerUID(c)
		if err == nil && l.allow(uid) {
			return c, nil
		}
		l.logRejected(uid, err)
		c.Close()
	}
}

func (l *peerListener) logRejected(uid int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dropped++
	if now := time.Now(); now.Sub(l.lastLog) >= peerLogEvery {
		if err != nil {
			log.Printf("addon %s: host socket: closed a connection (%d since the last report): peer credentials: %v", l.id, l.dropped, err)
		} else {
			log.Printf("addon %s: host socket: closed a connection from uid %d (%d since the last report)", l.id, uid, l.dropped)
		}
		l.lastLog, l.dropped = now, 0
	}
}

// peerUID는 유닉스 소켓 연결 상대 프로세스의 uid(SO_PEERCRED)다. 커널이 connect 때 기록한 값이라 위조할 수 없다
// (internal/helper/daemon의 peerUID와 같다).
func peerUID(c net.Conn) (int, error) {
	sc, ok := c.(syscall.Conn)
	if !ok {
		return -1, errors.New("not a socket connection")
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return -1, err
	}
	var cred *syscall.Ucred
	var serr error
	if err := raw.Control(func(fd uintptr) {
		cred, serr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return -1, err
	}
	if serr != nil {
		return -1, serr
	}
	return int(cred.Uid), nil
}

// limitListener는 동시 연결을 n개로 묶는다. 가득 차면 Accept가 연결이 닫힐 때까지 기다린다.
type limitListener struct {
	net.Listener
	sem  chan struct{}
	done chan struct{}
	once sync.Once
}

func newLimitListener(ln net.Listener, n int) *limitListener {
	return &limitListener{Listener: ln, sem: make(chan struct{}, n), done: make(chan struct{})}
}

func (l *limitListener) Accept() (net.Conn, error) {
	select {
	case l.sem <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	c, err := l.Listener.Accept()
	if err != nil {
		<-l.sem
		return nil, err
	}
	return &limitConn{Conn: c, release: func() { <-l.sem }}, nil
}

func (l *limitListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type limitConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(c.release)
	return err
}

// hostOp는 호스트 API 하나의 처리기다. 본문은 읽어 둔 값이다.
type hostOp func(a *addonHosts, ctx context.Context, w http.ResponseWriter, call *hostCall, body []byte)

var hostOps = map[string]hostOp{
	"/v1/rpc":    (*addonHosts).rpc,
	"/v1/rpc-bg": (*addonHosts).rpcBg,
	"/v1/apply":  (*addonHosts).apply,
}

// handler는 id 애드온 소켓의 처리기다. 순서: 경로·메서드 → 토큰(그 애드온의 것) → 동시 요청 → 본문.
func (a *addonHosts) handler(id string, sem chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 본문 읽기 제한 시각은 맨 먼저 건다. ReadTimeout이 없어 헤더를 읽은 뒤에는 연결에 읽기 제한이 없는데, net/http는
		// 처리기가 읽지 않은 본문을 응답 전후에 읽어 버린다. 제한 시각이 없으면 본문을 보내지 않는 연결이 동시 연결 칸을
		// 끝없이 쥔다.
		rc := http.NewResponseController(w)
		rc.SetReadDeadline(time.Now().Add(hostBodyTimeout))
		// reject는 본문을 읽기 전의 거절이다. Connection: close를 붙이면 net/http가 응답 전에 남은 본문을 기다리지 않아
		// 바로 응답하고, 응답 뒤에는 남은 본문을 위 제한 시각까지만 읽고 연결을 닫는다(유휴 연결로 칸을 쥐지 않게).
		reject := func(status int, code, msg string) {
			w.Header().Set("Connection", "close")
			writeHostError(w, status, code, msg)
		}
		op, ok := hostOps[r.URL.Path]
		if !ok {
			reject(http.StatusNotFound, hostErrNotFound, "not found")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			reject(http.StatusMethodNotAllowed, hostErrBadRequest, "method must be POST")
			return
		}
		call, ok := a.h.hostCalls.lookup(r.Header.Get(hostCallHeader))
		if !ok || call.addon != id {
			reject(http.StatusUnauthorized, hostErrCallInvalid, "invalid or expired call token")
			return
		}
		if call.caller.unauthorized.Load() {
			reject(http.StatusUnauthorized, hostErrUnauthorized, "omv session expired")
			return
		}
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		default:
			reject(http.StatusTooManyRequests, hostErrBusy, "too many concurrent host requests")
			return
		}
		if ct := r.Header.Get("Content-Type"); !isJSONType(ct) {
			reject(http.StatusBadRequest, hostErrBadRequest, "content type must be application/json")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, hostMaxBody))
		rc.SetReadDeadline(time.Time{})
		if err != nil {
			reject(http.StatusBadRequest, hostErrBadRequest, "request body is too large or unreadable")
			return
		}
		// 바깥 요청이 끝나면(제한 시각, 조작 완료) 이 요청도 끝낸다.
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		stop := context.AfterFunc(call.ctx, cancel)
		defer stop()
		op(a, ctx, w, call, body)
	})
}

func isJSONType(ct string) bool {
	mt, _, _ := strings.Cut(ct, ";")
	return strings.EqualFold(strings.TrimSpace(mt), "application/json")
}

// hostErrorBody는 호스트 소켓의 오류 응답이다.
type hostErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeHostError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, hostErrorBody{Error: msg, Code: code})
}

// decodeHost는 본문을 엄격하게 읽는다(모르는 필드, 뒤에 남은 값을 거부한다).
func decodeHost(body []byte, v any) bool {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if dec.Decode(v) != nil {
		return false
	}
	_, err := dec.Token()
	return err == io.EOF
}

// hostRPCRequest는 OMV 호출 하나다.
type hostRPCRequest struct {
	Service string          `json:"service"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func (r hostRPCRequest) params() any {
	if p := bytes.TrimSpace(r.Params); len(p) > 0 && !bytes.Equal(p, []byte("null")) {
		return json.RawMessage(p)
	}
	return nil
}

// allowed는 패키지가 부를 수 있는 꼴의 메서드인지 먼저 본다(금지 표는 등록할 때 막았지만 한 번 더 본다). 실제 허용
// (선언, 범위)은 바깥 요청의 호출기가 본다.
func allowedHostMethod(req hostRPCRequest) bool {
	name := req.Service + "." + req.Method
	_, denied := addons.DeniedForPackage(name)
	return addons.ValidMethod(name) && !denied
}

// writeHostResult는 {"<key>": <OMV 결과>}를 쓴다. 큰 정수의 정밀도를 지키려고 결과를 다시 직렬화하지 않는다.
func writeHostResult(w http.ResponseWriter, key string, result json.RawMessage) {
	if len(result) == 0 {
		result = json.RawMessage("null")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{%q:`, key)
	w.Write(result)
	w.Write([]byte("}\n"))
}

// rpc는 POST /v1/rpc다. 그 애드온이 선언한 메서드만 부른다(읽기 토큰이면 읽기만). Config·Exec는 선언할 수 없으므로
// 부를 수 없다.
func (a *addonHosts) rpc(ctx context.Context, w http.ResponseWriter, call *hostCall, body []byte) {
	var req hostRPCRequest
	if !decodeHost(body, &req) {
		writeHostError(w, http.StatusBadRequest, hostErrBadRequest, "invalid request body")
		return
	}
	if !allowedHostMethod(req) {
		a.logForbidden(call, req)
		writeHostError(w, http.StatusForbidden, hostErrForbidden, "rpc method is not allowed")
		return
	}
	res, err := call.caller.Call(ctx, req.Service, req.Method, req.params())
	if err != nil {
		a.writeCallError(w, call, err)
		return
	}
	writeHostResult(w, "result", res)
}

// hostBgRequest는 POST /v1/rpc-bg의 본문이다.
type hostBgRequest struct {
	Service string          `json:"service"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	// Output이면 작업 출력을 끝까지 읽어 돌려준다(hostMaxBgOutput까지).
	Output bool `json:"output"`
}

// rpcBg는 POST /v1/rpc-bg다(관리자 세션만). 선언한 메서드를 부르고, 돌려준 상태 파일 이름에만 Exec.isRunning·
// getOutput을 쓰는 보조 호출기(bgCaller)로 작업이 끝날 때까지 기다린다.
func (a *addonHosts) rpcBg(ctx context.Context, w http.ResponseWriter, call *hostCall, body []byte) {
	if !call.session.Admin {
		writeHostError(w, http.StatusForbidden, hostErrAdminRequired, "background calls need an administrator session")
		return
	}
	var req hostBgRequest
	if !decodeHost(body, &req) {
		writeHostError(w, http.StatusBadRequest, hostErrBadRequest, "invalid request body")
		return
	}
	rr := hostRPCRequest{Service: req.Service, Method: req.Method, Params: req.Params}
	if !allowedHostMethod(rr) {
		a.logForbidden(call, rr)
		writeHostError(w, http.StatusForbidden, hostErrForbidden, "rpc method is not allowed")
		return
	}
	res, err := call.caller.Call(ctx, req.Service, req.Method, rr.params())
	if err != nil {
		a.writeCallError(w, call, err)
		return
	}
	var filename string
	if json.Unmarshal(res, &filename) != nil || !bgStatusPattern.MatchString(filename) {
		writeHostError(w, http.StatusBadGateway, hostErrOMV, "rpc did not return a background status file")
		return
	}
	bg := &bgCaller{outer: call.caller, filename: filename}
	var (
		out string
		cut bool
	)
	interval := time.Duration(a.bgInterval.Load())
	if req.Output {
		out, cut, err = addons.ReadBg(ctx, bg, filename, interval, hostMaxBgOutput)
	} else {
		err = addons.WaitBg(ctx, bg, filename, interval)
	}
	if err != nil {
		a.writeCallError(w, call, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Output    string `json:"output"`
		Truncated bool   `json:"truncated"`
	}{out, cut})
}

// hostApplyRequest는 POST /v1/apply의 본문이다.
type hostApplyRequest struct {
	Calls []hostRPCRequest `json:"calls"`
}

// apply는 POST /v1/apply다(쓰기 토큰만). 호출은 모두 그 애드온이 선언한 쓰기여야 하고, addons.ApplyChange(잠금,
// 적용 대기가 있으면 409 config_dirty, Config.applyChangesBg → WaitBg)로 실행한다. Config·Exec는 이 처리기 안에서만
// 만드는 보조 호출기(applyCaller)가 부른다.
func (a *addonHosts) apply(ctx context.Context, w http.ResponseWriter, call *hostCall, body []byte) {
	if call.scope != hostWrite {
		writeHostError(w, http.StatusForbidden, hostErrForbidden, "apply needs an action call")
		return
	}
	var req hostApplyRequest
	if !decodeHost(body, &req) || len(req.Calls) == 0 || len(req.Calls) > hostMaxApplyCalls {
		writeHostError(w, http.StatusBadRequest, hostErrBadRequest, "invalid request body")
		return
	}
	reg := a.h.addons()
	for _, c := range req.Calls {
		kind, ids := reg.Classify(c.Service, c.Method)
		if !allowedHostMethod(c) || kind != addons.RPCWrite || !slices.Contains(ids, call.addon) {
			a.logForbidden(call, c)
			writeHostError(w, http.StatusForbidden, hostErrForbidden, "apply calls must be declared write methods")
			return
		}
	}
	results := make([]json.RawMessage, len(req.Calls))
	err := addons.ApplyChange(ctx, &applyCaller{outer: call.caller}, func(ctx context.Context) (bool, error) {
		for i, c := range req.Calls {
			res, err := call.caller.Call(ctx, c.Service, c.Method, c.params())
			if err != nil {
				return false, err
			}
			if len(res) == 0 {
				res = json.RawMessage("null")
			}
			results[i] = res
		}
		return true, nil
	})
	if err != nil {
		a.writeCallError(w, call, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Results []json.RawMessage `json:"results"`
	}{results})
}

func (a *addonHosts) logForbidden(call *hostCall, req hostRPCRequest) {
	log.Printf("addon %s: host rpc forbidden: user=%q method=%q", call.addon, call.session.Username, req.Service+"."+req.Method)
}

// writeCallError는 OMV 호출 실패를 호스트 오류로 바꾼다. OMV 오류 문구는 백엔드에 넘긴다(trace 없음, 1KiB까지).
func (a *addonHosts) writeCallError(w http.ResponseWriter, call *hostCall, err error) {
	var (
		ae *addons.ActionError
		oe *omv.Error
	)
	switch {
	case call.caller.unauthorized.Load():
		writeHostError(w, http.StatusUnauthorized, hostErrUnauthorized, "omv session expired")
	case call.ctx.Err() != nil || errors.Is(err, addons.ErrActionTimeout):
		writeHostError(w, http.StatusGatewayTimeout, hostErrTimeout, "the call ran past the request deadline")
	case errors.As(err, &ae):
		writeHostError(w, ae.Status, ae.Code, ae.Message)
	case errors.Is(err, addons.ErrActionFailed):
		msg := "background job failed"
		if errors.As(err, &oe) {
			msg = omvMessage(oe)
		}
		writeHostError(w, http.StatusBadGateway, hostErrOMV, msg)
	case errors.Is(err, addons.ErrRPCNotAllowed):
		writeHostError(w, http.StatusForbidden, hostErrForbidden, "rpc method is not allowed")
	case errors.As(err, &oe):
		writeHostError(w, http.StatusBadGateway, hostErrOMV, omvMessage(oe))
	default:
		log.Printf("addon %s: host rpc: %v", call.addon, err)
		writeHostError(w, http.StatusServiceUnavailable, hostErrOMVUnavailable, "omv is unavailable")
	}
}

// omvMessage는 OMV 오류 문구를 hostMaxOMVMessage 바이트까지(글자 경계) 준다.
func omvMessage(e *omv.Error) string {
	msg := strings.ToValidUTF8(e.Message, "")
	if len(msg) > hostMaxOMVMessage {
		msg = msg[:hostMaxOMVMessage]
		for !utf8.ValidString(msg) {
			msg = msg[:len(msg)-1]
		}
	}
	if msg == "" {
		return "omv rpc failed"
	}
	return msg
}

// paramFilename은 Exec.isRunning·getOutput 파라미터의 filename이다.
func paramFilename(params any) string {
	b, err := json.Marshal(params)
	if err != nil {
		return ""
	}
	var p struct {
		Filename string `json:"filename"`
	}
	if json.Unmarshal(b, &p) != nil {
		return ""
	}
	return p.Filename
}

// bgCaller는 /v1/rpc-bg 처리기 안에서만 만드는 보조 호출기다. 선언한 메서드가 돌려준 상태 파일 이름에만
// Exec.isRunning·getOutput을 부른다. 실패 표시(401 등)는 바깥 요청의 호출기에 남는다.
type bgCaller struct {
	outer    *rpcCaller
	filename string
}

func (c *bgCaller) Call(ctx context.Context, service, method string, params any) (json.RawMessage, error) {
	name := service + "." + method
	if (name != "Exec.isRunning" && name != "Exec.getOutput") || paramFilename(params) != c.filename {
		return nil, fmt.Errorf("rpc %s: %w", name, addons.ErrRPCNotAllowed)
	}
	return c.outer.omvCall(ctx, service, method, params)
}

var (
	// errApplyStatus는 Config.applyChangesBg가 상태 파일 이름을 주지 않았다는 뜻이다.
	errApplyStatus = errors.New("applyChangesBg did not return a background status file")
	// errHelperCall은 보조 호출기가 정해진 호출 밖을 거절했다는 뜻이다(addons.ApplyChange가 부르는 것 밖이라 생기지 않는다).
	errHelperCall = errors.New("call is not allowed for the apply helper")
)

// applyCaller는 /v1/apply 처리기 안에서만 만드는 보조 호출기다. addons.ApplyChange가 부르는 Config.isDirty,
// Config.applyChangesBg, 그리고 applyChangesBg가 돌려준 상태 파일 이름의 Exec.isRunning만 부른다.
type applyCaller struct {
	outer *rpcCaller

	mu       sync.Mutex
	filename string
}

func (c *applyCaller) Call(ctx context.Context, service, method string, params any) (json.RawMessage, error) {
	switch name := service + "." + method; name {
	case "Config.isDirty":
		return c.outer.omvCall(ctx, service, method, params)
	case "Config.applyChangesBg":
		res, err := c.outer.omvCall(ctx, service, method, params)
		if err != nil {
			return nil, err
		}
		var f string
		if json.Unmarshal(res, &f) != nil || !bgStatusPattern.MatchString(f) {
			return nil, errApplyStatus
		}
		c.mu.Lock()
		c.filename = f
		c.mu.Unlock()
		return res, nil
	case "Exec.isRunning":
		c.mu.Lock()
		f := c.filename
		c.mu.Unlock()
		if f != "" && paramFilename(params) == f {
			return c.outer.omvCall(ctx, service, method, params)
		}
	}
	return nil, fmt.Errorf("rpc %s.%s: %w", service, method, errHelperCall)
}

// packageCall은 바깥 요청 하나의 토큰을 만들고 백엔드 요청 헤더를 채운다. 표가 가득 차면 false다(503).
func (h *handler) packageCall(ctx context.Context, r *http.Request, s auth.Session, id string, scope hostScope,
	caller *rpcCaller) (addonpkg.BackendCall, func(), bool) {
	tok, ok := h.hostCalls.issue(&hostCall{addon: id, session: s, scope: scope, caller: caller, ctx: ctx})
	if !ok {
		return addonpkg.BackendCall{}, nil, false
	}
	return addonpkg.BackendCall{User: s.Username, Admin: s.Admin, Lang: requestLanguage(r), Token: tok},
		func() { h.hostCalls.revoke(tok) }, true
}

// requestLanguage는 백엔드에 넘길 대시보드 화면 언어다. 브라우저가 보낸 X-Maru-Lang이 지원 언어면 그 값, 아니면 en이다.
func requestLanguage(r *http.Request) string {
	if lang := r.Header.Get("X-Maru-Lang"); slices.Contains(format.Languages, lang) {
		return lang
	}
	return format.FallbackLanguage
}
