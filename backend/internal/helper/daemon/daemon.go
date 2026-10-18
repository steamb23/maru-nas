// Package daemon은 권한 도우미(onmaru-helper)의 root 부모 프로세스다.
//
// 도우미는 onmaru를 믿지 않는다. 믿는 것은 세 가지뿐이다.
//   - 소켓 상대의 uid(SO_PEERCRED)가 onmaru 서비스 사용자인지
//   - 자기가 비밀번호를 직접 확인하고 발급한 티켓인지
//   - 자기가 /etc/passwd·/etc/group에서 직접 읽은 계정·그룹 정보
//
// 그래서 onmaru가 뚫려도 지금 로그인해 있는 사용자의 티켓만 쓸 수 있고, root 범위 연산은
// 비밀번호를 다시 확인해 상승한 티켓으로만, 정해진 목록(proto.Actions)의 연산만 할 수 있다.
// 연산은 worker 자식 프로세스가 사용자 범위면 그 사용자 권한으로 실행하며, 부모는 사용자
// 데이터를 읽거나 쓰지 않는다.
package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"onmaru/backend/config"
	"onmaru/backend/internal/helper/proto"
	"onmaru/backend/internal/helper/worker"
)

const (
	// DefaultAdminGroup은 OMV 웹 관리자 그룹(OMV_WEBGUI_ADMINGROUP_NAME)이다.
	DefaultAdminGroup = "openmediavault-admin"
	// DefaultElevateGroup은 상승에 함께 필요한 그룹의 기본값이다.
	DefaultElevateGroup = config.DefaultElevateGroup

	// maxConns는 동시에 처리하는 연결 수다.
	maxConns = 64
	// maxWorkers는 동시에 실행하는 worker 수, maxUserWorkers는 사용자 하나의 worker 수다.
	maxWorkers     = 16
	maxUserWorkers = 4
	// maxTickets는 보관하는 티켓 수 상한이다.
	maxTickets = 1000
	// headerTimeout은 요청 머리를 받는 시간, replyTimeout은 부모가 응답을 쓰는 시간이다.
	headerTimeout = 5 * time.Second
	replyTimeout  = 5 * time.Second
	// opTimeout은 목록·정보 같은 연산의 최대 시간, streamTimeout은 읽기·쓰기 스트림의 최대 시간이다.
	// 스트림은 worker가 진행이 멈추면(60초) 따로 끊는다.
	opTimeout     = 30 * time.Second
	streamTimeout = 6 * time.Hour
)

// Config는 도우미 설정이다.
type Config struct {
	// ClientUID는 연결을 받을 상대 uid(onmaru 서비스 사용자)다.
	ClientUID int
	// AdminGroup은 OMV 관리자 그룹이다. 비우면 DefaultAdminGroup.
	AdminGroup string
	// ElevateGroup은 상승에 함께 필요한 그룹이다. 비우면 DefaultElevateGroup.
	ElevateGroup string

	Accounts Accounts
	Verifier Verifier
	Spawner  Spawner

	// Now는 현재 시각이다. 테스트에서 바꿔 끼운다.
	Now func() time.Time
	// FailDelay는 비밀번호가 틀렸을 때 응답 전에 기다리는 시간이다. 기본 1초, 음수면 기다리지 않는다.
	FailDelay time.Duration
	// ElevateTTL은 상승이 유지되는 시간이다(연산마다 늘리지 않는다). 기본 5분.
	ElevateTTL time.Duration
	// PendingTTL은 2차 인증 대기 티켓이 activate를 기다리는 시간이다. 기본 5분(OMV 대기 시간과 같다).
	PendingTTL time.Duration
	// TicketIdle은 쓰지 않은 티켓이 남는 시간이다. 기본 24시간(onmaru 세션 상한과 같다).
	TicketIdle time.Duration
	// IdleExit이 0보다 크면 티켓과 연결이 없이 이만큼 지났을 때 Serve를 끝낸다(소켓 활성화).
	IdleExit time.Duration
	// Logf는 감사 로그다. 기본 log.Printf.
	Logf func(format string, args ...any)
}

// Server는 도우미 부모 프로세스다.
type Server struct {
	cfg     Config
	tickets *tickets
	limiter *limiter

	conns   chan struct{}
	workers chan struct{}

	mu          sync.Mutex
	userWorkers map[string]int

	active     atomic.Int64
	lastActive atomic.Int64
}

// New는 도우미를 만든다. Accounts, Verifier, Spawner는 필수다.
func New(cfg Config) (*Server, error) {
	if cfg.Accounts == nil || cfg.Verifier == nil || cfg.Spawner == nil {
		return nil, errors.New("helper: accounts, verifier and spawner are required")
	}
	if cfg.ClientUID <= 0 {
		return nil, errors.New("helper: client uid must be a non-root user")
	}
	if cfg.AdminGroup == "" {
		cfg.AdminGroup = DefaultAdminGroup
	}
	if cfg.ElevateGroup == "" {
		cfg.ElevateGroup = DefaultElevateGroup
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.FailDelay == 0 {
		cfg.FailDelay = time.Second
	}
	if cfg.ElevateTTL == 0 {
		cfg.ElevateTTL = 5 * time.Minute
	}
	if cfg.PendingTTL == 0 {
		cfg.PendingTTL = 5 * time.Minute
	}
	if cfg.TicketIdle == 0 {
		cfg.TicketIdle = 24 * time.Hour
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	s := &Server{
		cfg:         cfg,
		tickets:     newTickets(cfg.Now, cfg.TicketIdle, cfg.PendingTTL, maxTickets),
		limiter:     newLimiter(cfg.Now),
		conns:       make(chan struct{}, maxConns),
		workers:     make(chan struct{}, maxWorkers),
		userWorkers: make(map[string]int),
	}
	s.touch()
	return s, nil
}

// Serve는 l이 닫히거나 유휴 종료할 때까지 연결을 받는다. 유휴 종료면 nil이다.
func (s *Server) Serve(l *net.UnixListener) error {
	var idle atomic.Bool
	stop := make(chan struct{})
	defer close(stop)
	if s.cfg.IdleExit > 0 {
		go s.watchIdle(l, &idle, stop)
	}
	for {
		c, err := l.AcceptUnix()
		if err != nil {
			if idle.Load() {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		select {
		case s.conns <- struct{}{}:
		default:
			// 자리가 없으면 상대를 확인하지 않고 busy만 알린다.
			s.reply(c, proto.Fail(proto.NewError(proto.CodeBusy)))
			c.Close()
			continue
		}
		s.active.Add(1)
		go func() {
			defer func() {
				s.touch()
				s.active.Add(-1)
				<-s.conns
			}()
			s.handle(c)
		}()
	}
}

// watchIdle은 티켓과 연결이 없이 IdleExit이 지나면 수신을 닫는다.
func (s *Server) watchIdle(l *net.UnixListener, idle *atomic.Bool, stop <-chan struct{}) {
	tick := time.NewTicker(min(s.cfg.IdleExit/2, 30*time.Second))
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
		}
		if s.active.Load() > 0 || s.tickets.count() > 0 {
			s.touch()
			continue
		}
		if s.cfg.Now().Sub(time.Unix(0, s.lastActive.Load())) >= s.cfg.IdleExit {
			s.cfg.Logf("idle for %v without tickets, exiting", s.cfg.IdleExit)
			idle.Store(true)
			l.Close()
			return
		}
	}
}

func (s *Server) touch() { s.lastActive.Store(s.cfg.Now().UnixNano()) }

func (s *Server) handle(c *net.UnixConn) {
	defer c.Close()
	uid, err := peerUID(c)
	if err != nil || uid != s.cfg.ClientUID {
		s.cfg.Logf("rejected connection: peer uid=%d err=%v", uid, err)
		s.reply(c, proto.Fail(proto.NewError(proto.CodeForbiddenPeer)))
		return
	}
	c.SetReadDeadline(time.Now().Add(headerTimeout))
	req, err := proto.ReadRequest(c)
	if err != nil || req.V != proto.Version {
		s.reply(c, proto.Fail(proto.NewError(proto.CodeBadRequest)))
		return
	}
	c.SetReadDeadline(time.Time{})
	ctx := context.Background()
	if req.Op == proto.OpRun {
		s.run(ctx, c, req)
		return
	}
	result, err := s.control(ctx, req)
	if err != nil {
		s.reply(c, proto.Fail(err))
		return
	}
	resp, err := proto.Success(result)
	if err != nil {
		resp = proto.Fail(err)
	}
	s.reply(c, resp)
}

func (s *Server) reply(c *net.UnixConn, resp proto.Response) {
	c.SetWriteDeadline(time.Now().Add(replyTimeout))
	proto.WriteResponse(c, resp)
}

// control은 run이 아닌 요청(티켓 관리)을 처리한다.
func (s *Server) control(ctx context.Context, req proto.Request) (any, error) {
	switch req.Op {
	case proto.OpOpen:
		return s.open(ctx, req)
	case proto.OpActivate:
		if !s.tickets.activate(req.Ticket) {
			return nil, proto.NewError(proto.CodeTicketInvalid)
		}
		return nil, nil
	case proto.OpClose:
		s.tickets.close(req.Ticket)
		return nil, nil
	case proto.OpReset:
		if n := s.tickets.reset(); n > 0 {
			s.cfg.Logf("reset: dropped %d tickets", n)
		}
		return nil, nil
	case proto.OpStatus:
		return s.status(req)
	case proto.OpElevate:
		return s.elevate(ctx, req)
	case proto.OpDrop:
		if !s.tickets.drop(req.Ticket) {
			return nil, proto.NewError(proto.CodeTicketInvalid)
		}
		return nil, nil
	}
	return nil, proto.NewError(proto.CodeBadRequest)
}

func (s *Server) open(ctx context.Context, req proto.Request) (any, error) {
	if !ValidUserName(req.User) {
		return nil, proto.NewError(proto.CodeBadRequest)
	}
	if err := s.verify(ctx, req.User, req.Password, "open"); err != nil {
		return nil, err
	}
	acc, err := s.cfg.Accounts.Lookup(req.User)
	if err != nil {
		s.cfg.Logf("open failed: user=%q: account lookup: %v", req.User, err)
		return nil, proto.NewError(proto.CodeAuthFailed)
	}
	if !s.allowed(acc) {
		s.cfg.Logf("open denied: user=%q uid=%d: system account", req.User, acc.UID)
		return nil, proto.NewError(proto.CodeAccountNotAllowed)
	}
	tok := s.tickets.issue(acc.Name, acc.UID, req.Pending)
	can := s.canElevate(acc)
	s.cfg.Logf("open ok: user=%q uid=%d pending=%t can_elevate=%t", acc.Name, acc.UID, req.Pending, can)
	return proto.OpenResult{Ticket: tok, CanElevate: can}, nil
}

// verify는 실패 제한을 거쳐 비밀번호를 확인한다. 틀리면 FailDelay만큼 기다린 뒤 auth_failed다.
func (s *Server) verify(ctx context.Context, user, password, step string) error {
	done, err := s.limiter.begin(ctx, user)
	if err != nil {
		s.cfg.Logf("%s refused: user=%q: %s", step, user, proto.CodeOf(err))
		return err
	}
	err = s.cfg.Verifier.Verify(ctx, user, password)
	switch {
	case err == nil:
		done(passed)
		return nil
	case errors.Is(err, ErrAuth):
		done(failed)
		s.cfg.Logf("%s failed: user=%q: wrong password or expired account", step, user)
		if s.cfg.FailDelay > 0 {
			time.Sleep(s.cfg.FailDelay)
		}
		return proto.NewError(proto.CodeAuthFailed)
	default:
		// 확인 도구가 고장 난 것이지 비밀번호가 틀린 것이 아니므로 실패로 세지 않는다.
		done(unchecked)
		s.cfg.Logf("%s failed: user=%q: cannot verify password: %v", step, user, err)
		return proto.NewError(proto.CodeInternal)
	}
}

// session은 티켓과, 그 티켓의 계정이 그대로인지 확인한 계정 정보다. 계정이 없어졌거나
// uid가 바뀌었으면 티켓을 지운다.
func (s *Server) session(tok string, allowPending bool) (ticket, Account, error) {
	t, ok := s.tickets.lookup(tok, allowPending)
	if !ok {
		return ticket{}, Account{}, proto.NewError(proto.CodeTicketInvalid)
	}
	acc, err := s.cfg.Accounts.Lookup(t.user)
	if err != nil || acc.UID != t.uid || !s.allowed(acc) {
		s.tickets.close(tok)
		s.cfg.Logf("ticket dropped: user=%q: account changed or removed", t.user)
		return ticket{}, Account{}, proto.NewError(proto.CodeTicketInvalid)
	}
	return t, acc, nil
}

func (s *Server) status(req proto.Request) (any, error) {
	t, acc, err := s.session(req.Ticket, true)
	if err != nil {
		return nil, err
	}
	return proto.StatusResult{
		User:            t.user,
		Pending:         t.pending(),
		CanElevate:      s.canElevate(acc),
		ElevatedSeconds: s.elevatedSeconds(t),
	}, nil
}

func (s *Server) elevate(ctx context.Context, req proto.Request) (any, error) {
	t, acc, err := s.session(req.Ticket, false)
	if err != nil {
		return nil, err
	}
	if !s.canElevate(acc) {
		s.cfg.Logf("elevate denied: user=%q: not in %s and %s", t.user, s.cfg.AdminGroup, s.cfg.ElevateGroup)
		return nil, proto.NewError(proto.CodeElevationNotAllowed)
	}
	if err := s.verify(ctx, t.user, req.Password, "elevate"); err != nil {
		return nil, err
	}
	until := s.cfg.Now().Add(s.cfg.ElevateTTL)
	if !s.tickets.elevate(req.Ticket, until) {
		return nil, proto.NewError(proto.CodeTicketInvalid)
	}
	s.cfg.Logf("elevate ok: user=%q for %v", t.user, s.cfg.ElevateTTL)
	return proto.ElevateResult{ElevatedSeconds: int(s.cfg.ElevateTTL / time.Second)}, nil
}

func (s *Server) elevatedSeconds(t ticket) int {
	left := t.elevatedUntil.Sub(s.cfg.Now())
	if left <= 0 {
		return 0
	}
	return int((left + time.Second - 1) / time.Second)
}

// allowed는 티켓을 줄 수 있는 계정인지다. OMV UserMgmt.authUser와 같은 기준이다:
// root는 받지 않고, OMV 관리자 그룹이거나 일반 사용자 uid 범위 안이어야 한다.
func (s *Server) allowed(acc Account) bool {
	if acc.UID == 0 {
		return false
	}
	if s.inGroup(acc, s.cfg.AdminGroup) {
		return true
	}
	lo, hi := s.cfg.Accounts.UIDRange()
	return acc.UID >= lo && acc.UID <= hi
}

// canElevate는 root 범위로 상승할 수 있는 계정인지다. OMV 관리자 그룹과 상승 그룹 모두에 속해야 한다.
func (s *Server) canElevate(acc Account) bool {
	return acc.UID != 0 && s.inGroup(acc, s.cfg.AdminGroup) && s.inGroup(acc, s.cfg.ElevateGroup)
}

func (s *Server) inGroup(acc Account, name string) bool {
	gid, ok, err := s.cfg.Accounts.GroupID(name)
	return err == nil && ok && slices.Contains(acc.Groups, gid)
}

// run은 연산 하나를 worker로 실행한다. worker가 연결에 직접 응답한다.
func (s *Server) run(ctx context.Context, c *net.UnixConn, req proto.Request) {
	t, acc, err := s.session(req.Ticket, false)
	if err == nil && !proto.Allowed(req.Scope, req.Action) {
		err = proto.NewError(proto.CodeBadRequest)
	}
	if err == nil && req.Size < 0 {
		err = proto.NewError(proto.CodeBadRequest)
	}
	var cred *Credential
	if err == nil {
		switch req.Scope {
		case proto.ScopeUser:
			cred = &Credential{UID: acc.UID, GID: acc.GID, Groups: acc.Groups}
		case proto.ScopeRoot:
			switch {
			case s.elevatedSeconds(t) == 0:
				err = proto.NewError(proto.CodeElevationRequired)
			case !s.canElevate(acc):
				// 상승한 뒤 그룹에서 빠졌으면 상승을 거둔다.
				s.tickets.drop(req.Ticket)
				err = proto.NewError(proto.CodeElevationNotAllowed)
			}
		}
	}
	if err != nil {
		s.audit(t.user, req, proto.CodeOf(err))
		s.reply(c, proto.Fail(err))
		return
	}
	release, err := s.acquireWorker(acc.Name)
	if err != nil {
		s.reply(c, proto.Fail(err))
		return
	}
	defer release()

	timeout := opTimeout
	if req.Action == proto.ActionRead || req.Action == proto.ActionWrite {
		timeout = streamTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	job := worker.Job{Scope: req.Scope, Action: req.Action, Params: req.Params, Size: req.Size, User: acc.Name, Home: acc.Home}
	code, started, err := s.cfg.Spawner.Spawn(ctx, cred, job, c)
	if err != nil {
		s.cfg.Logf("worker error: user=%q action=%s: %v", acc.Name, req.Action, err)
	}
	if !started {
		code = proto.CodeInternal
		s.reply(c, proto.Fail(proto.NewError(code)))
	}
	s.audit(acc.Name, req, code)
}

func (s *Server) acquireWorker(user string) (func(), error) {
	s.mu.Lock()
	if s.userWorkers[user] >= maxUserWorkers {
		s.mu.Unlock()
		return nil, proto.NewError(proto.CodeBusy)
	}
	select {
	case s.workers <- struct{}{}:
	default:
		s.mu.Unlock()
		return nil, proto.NewError(proto.CodeBusy)
	}
	s.userWorkers[user]++
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.userWorkers[user]--
		if s.userWorkers[user] <= 0 {
			delete(s.userWorkers, user)
		}
		s.mu.Unlock()
		<-s.workers
	}, nil
}

// writeActions는 감사 로그에 경로를 남기는 연산이다. 읽기 연산은 사용자 데이터라 경로를 남기지 않는다.
// ssh.authorize·ssh.revoke는 경로가 정해져 있으므로(~/.ssh/authorized_keys) 키 지문만 남긴다.
var writeActions = map[string]bool{
	proto.ActionWrite: true, proto.ActionMkdir: true, proto.ActionRename: true, proto.ActionRemove: true,
}

// audit은 연산 결과를 남긴다. root 범위와 쓰기 연산은 언제나, 읽기 연산은 연산과 결과만 남긴다.
func (s *Server) audit(user string, req proto.Request, code string) {
	result := "ok"
	if code != "" {
		result = code
	}
	detail := ""
	if writeActions[req.Action] || req.Scope == proto.ScopeRoot {
		var p struct {
			Path      string `json:"path"`
			From      string `json:"from"`
			To        string `json:"to"`
			Recursive bool   `json:"recursive"`
			Overwrite bool   `json:"overwrite"`
		}
		if json.Unmarshal(req.Params, &p) == nil {
			switch {
			case p.From != "" || p.To != "":
				detail = fmt.Sprintf(" from=%q to=%q", p.From, p.To)
			case p.Path != "":
				detail = fmt.Sprintf(" path=%q", p.Path)
			}
			if p.Recursive {
				detail += " recursive"
			}
			if p.Overwrite {
				detail += " overwrite"
			}
		}
	}
	if req.Action == proto.ActionSSHAuthorize || req.Action == proto.ActionSSHRevoke {
		// 공개 키 대신 지문을 남긴다(sshd 로그의 "Accepted publickey ... SHA256:..."와 맞춰 볼 수 있다).
		var p proto.SSHKeyParams
		if json.Unmarshal(req.Params, &p) == nil {
			if blob, err := proto.ParseSSHPublicKey(p.PublicKey); err == nil {
				detail = " key=" + proto.SSHFingerprint(blob)
			}
		}
	}
	s.cfg.Logf("run: user=%q scope=%s action=%s%s result=%s", user, req.Scope, req.Action, detail, result)
}
