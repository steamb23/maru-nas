// Package client는 onmaru가 권한 도우미(onmaru-helper)를 부르는 클라이언트다.
//
// 요청마다 Unix 소켓 연결을 새로 연다. 도우미가 돌려준 오류는 *proto.Error(코드)이고,
// 연결 자체를 못 하면 ErrUnavailable로 감싼 오류다. 티켓과 비밀번호는 로그에 남기지 않는다.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"onmaru/backend/internal/helper/proto"
)

// ErrUnavailable은 도우미에 연결하지 못했거나 응답을 받지 못했다는 오류다.
var ErrUnavailable = errors.New("helper unavailable")

const (
	// controlTimeout은 티켓 관리 요청의 시간 제한이다.
	controlTimeout = 5 * time.Second
	// verifyTimeout은 비밀번호를 확인하는 요청(open, elevate)의 시간 제한이다.
	// 도우미의 확인 자리 대기(5초)와 실패 지연(1초), unix_chkpwd 실행을 덮는다.
	verifyTimeout = 20 * time.Second
	// opTimeout은 스트림이 아닌 연산의 시간 제한이다(도우미의 연산 제한보다 조금 길다).
	opTimeout = 35 * time.Second
)

// Client는 도우미 클라이언트다.
type Client struct {
	// Socket은 도우미 소켓 경로다.
	Socket string
}

// New는 socket에 연결하는 클라이언트다.
func New(socket string) *Client { return &Client{Socket: socket} }

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.Socket)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return conn, nil
}

// roundTrip은 본문 없는 요청 하나를 보내고 응답 머리를 받는다.
func (c *Client) roundTrip(ctx context.Context, req proto.Request, timeout time.Duration) (proto.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := c.dial(ctx)
	if err != nil {
		return proto.Response{}, err
	}
	defer conn.Close()
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	req.V = proto.Version
	if err := proto.WriteRequest(conn, req); err != nil {
		return proto.Response{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	resp, err := proto.ReadResponse(conn)
	if err != nil {
		return proto.Response{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.Size != 0 {
		return proto.Response{}, fmt.Errorf("%w: unexpected body", ErrUnavailable)
	}
	return resp, nil
}

func (c *Client) call(ctx context.Context, req proto.Request, timeout time.Duration, out any) error {
	resp, err := c.roundTrip(ctx, req, timeout)
	if err != nil {
		return err
	}
	if out == nil {
		return resp.Err()
	}
	return resp.Decode(out)
}

// Open은 비밀번호를 도우미가 직접 확인하게 하고 티켓을 받는다. pending이면 Activate 전까지 쓸 수 없다.
func (c *Client) Open(ctx context.Context, user, password string, pending bool) (proto.OpenResult, error) {
	var res proto.OpenResult
	err := c.call(ctx, proto.Request{Op: proto.OpOpen, User: user, Password: password, Pending: pending}, verifyTimeout, &res)
	return res, err
}

// Activate는 2차 인증을 마친 대기 티켓을 쓸 수 있게 한다.
func (c *Client) Activate(ctx context.Context, ticket string) error {
	return c.call(ctx, proto.Request{Op: proto.OpActivate, Ticket: ticket}, controlTimeout, nil)
}

// Close는 티켓을 폐기한다.
func (c *Client) Close(ctx context.Context, ticket string) error {
	return c.call(ctx, proto.Request{Op: proto.OpClose, Ticket: ticket}, controlTimeout, nil)
}

// Reset은 도우미의 티켓을 모두 폐기한다. onmaru가 시작할 때 이전 실행의 티켓을 정리한다.
func (c *Client) Reset(ctx context.Context) error {
	return c.call(ctx, proto.Request{Op: proto.OpReset}, controlTimeout, nil)
}

// Status는 티켓 상태다.
func (c *Client) Status(ctx context.Context, ticket string) (proto.StatusResult, error) {
	var res proto.StatusResult
	err := c.call(ctx, proto.Request{Op: proto.OpStatus, Ticket: ticket}, controlTimeout, &res)
	return res, err
}

// Elevate는 비밀번호를 다시 확인하고 티켓에 root 권한 상승을 붙인다.
func (c *Client) Elevate(ctx context.Context, ticket, password string) (proto.ElevateResult, error) {
	var res proto.ElevateResult
	err := c.call(ctx, proto.Request{Op: proto.OpElevate, Ticket: ticket, Password: password}, verifyTimeout, &res)
	return res, err
}

// Drop은 상승을 끝낸다.
func (c *Client) Drop(ctx context.Context, ticket string) error {
	return c.call(ctx, proto.Request{Op: proto.OpDrop, Ticket: ticket}, controlTimeout, nil)
}

func (c *Client) run(ctx context.Context, ticket, scope, action string, params any, out any) error {
	req := proto.Request{Op: proto.OpRun, Ticket: ticket, Scope: scope, Action: action}
	if params != nil {
		b, err := marshal(params)
		if err != nil {
			return err
		}
		req.Params = b
	}
	return c.call(ctx, req, opTimeout, out)
}

// Identity는 scope 범위 worker가 실제로 가진 권한이다.
func (c *Client) Identity(ctx context.Context, ticket, scope string) (proto.Identity, error) {
	var id proto.Identity
	err := c.run(ctx, ticket, scope, proto.ActionID, nil, &id)
	return id, err
}

// List는 폴더의 항목이다(사용자 권한).
func (c *Client) List(ctx context.Context, ticket, path string) (proto.ListResult, error) {
	var res proto.ListResult
	err := c.run(ctx, ticket, proto.ScopeUser, proto.ActionList, proto.PathParams{Path: path}, &res)
	return res, err
}

// Stat은 파일 하나의 정보다(사용자 권한).
func (c *Client) Stat(ctx context.Context, ticket, path string) (proto.Entry, error) {
	var e proto.Entry
	err := c.run(ctx, ticket, proto.ScopeUser, proto.ActionStat, proto.PathParams{Path: path}, &e)
	return e, err
}

// StatMany는 폴더 dir 바로 안의 이름 여러 개(proto.MaxStatMany개까지)의 정보다(사용자 권한). 읽지 못한
// 항목은 Code만 있다.
func (c *Client) StatMany(ctx context.Context, ticket, dir string, names []string) (proto.StatManyResult, error) {
	var res proto.StatManyResult
	err := c.run(ctx, ticket, proto.ScopeUser, proto.ActionStatMany, proto.StatManyParams{Dir: dir, Names: names}, &res)
	return res, err
}

// Info는 파일 하나의 정보와 소유자·그룹 이름, ACL 유무, 내 권한이다(사용자 권한).
func (c *Client) Info(ctx context.Context, ticket, path string) (proto.InfoResult, error) {
	var res proto.InfoResult
	err := c.run(ctx, ticket, proto.ScopeUser, proto.ActionInfo, proto.PathParams{Path: path}, &res)
	return res, err
}

// Names는 폴더의 이름과 종류를 디렉터리 순서대로 받는다(사용자 권한). 항목마다 lstat 하지 않으므로 크기·시각은
// 없다. 이름 목록은 응답 머리가 아니라 그 뒤 본문으로 온다.
func (c *Client) Names(ctx context.Context, ticket, path string) (proto.NamesResult, []proto.NameEntry, error) {
	params, err := marshal(proto.PathParams{Path: path})
	if err != nil {
		return proto.NamesResult{}, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	conn, err := c.dial(ctx)
	if err != nil {
		return proto.NamesResult{}, nil, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	req := proto.Request{V: proto.Version, Op: proto.OpRun, Ticket: ticket, Scope: proto.ScopeUser, Action: proto.ActionNames, Params: params}
	if err := proto.WriteRequest(conn, req); err != nil {
		return proto.NamesResult{}, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	resp, err := proto.ReadResponse(conn)
	if err != nil {
		return proto.NamesResult{}, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var res proto.NamesResult
	if err := resp.Decode(&res); err != nil {
		return proto.NamesResult{}, nil, err
	}
	if resp.Size < 0 || resp.Size > proto.MaxNamesBytes {
		return proto.NamesResult{}, nil, fmt.Errorf("%w: names body of %d bytes", ErrUnavailable, resp.Size)
	}
	body := make([]byte, resp.Size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return proto.NamesResult{}, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	list, err := proto.ParseNames(body, res.Count)
	if err != nil {
		return proto.NamesResult{}, nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return res, list, nil
}

// Mkdir는 폴더를 만든다(사용자 권한).
func (c *Client) Mkdir(ctx context.Context, ticket, path string) (proto.Entry, error) {
	var e proto.Entry
	err := c.run(ctx, ticket, proto.ScopeUser, proto.ActionMkdir, proto.PathParams{Path: path}, &e)
	return e, err
}

// Rename은 이름을 바꾸거나 같은 기준 디렉터리 안에서 옮긴다(사용자 권한).
func (c *Client) Rename(ctx context.Context, ticket, from, to string) (proto.Entry, error) {
	var e proto.Entry
	err := c.run(ctx, ticket, proto.ScopeUser, proto.ActionRename, proto.RenameParams{From: from, To: to}, &e)
	return e, err
}

// Remove는 파일이나 폴더를 지운다(사용자 권한). recursive가 false면 빈 폴더만 지운다.
func (c *Client) Remove(ctx context.Context, ticket, path string, recursive bool) error {
	return c.run(ctx, ticket, proto.ScopeUser, proto.ActionRemove, proto.RemoveParams{Path: path, Recursive: recursive}, nil)
}

// SSHAuthorize는 사용자의 ~/.ssh/authorized_keys에 publicKey("ssh-ed25519 <base64>")를 짧은 시간 동안
// 루프백 접속에만 받는 키 줄로 넣는다(사용자 권한). 키 줄의 옵션과 만료 시각은 도우미가 정한다.
func (c *Client) SSHAuthorize(ctx context.Context, ticket, publicKey string) error {
	return c.run(ctx, ticket, proto.ScopeUser, proto.ActionSSHAuthorize, proto.SSHKeyParams{PublicKey: publicKey}, nil)
}

// SSHRevoke는 SSHAuthorize로 넣은 키 줄(과 만료된 onmaru 키 줄)을 지운다(사용자 권한).
func (c *Client) SSHRevoke(ctx context.Context, ticket, publicKey string) error {
	return c.run(ctx, ticket, proto.ScopeUser, proto.ActionSSHRevoke, proto.SSHKeyParams{PublicKey: publicKey}, nil)
}

// UpgradeApply는 대시보드에서 업그레이드의 적용 요청(대상 커밋, 설정 이름·값)을 업그레이드 서비스에 넘긴다(root 권한,
// 상승한 티켓). 진행할 수 있는지는 업그레이드 서비스가 확인하고 결과는 상태 파일에 쓴다.
func (c *Client) UpgradeApply(ctx context.Context, ticket, target string, settings map[string]string) error {
	return c.run(ctx, ticket, proto.ScopeRoot, proto.ActionUpgradeApply, proto.UpgradeApplyParams{Target: target, Settings: settings}, nil)
}

// Body는 도우미가 보내는 파일 내용이다. 다 읽었거나 그만 읽을 때 Close한다.
// 도우미가 Size보다 적게 보내고 끊으면 io.ErrUnexpectedEOF다.
type Body struct {
	// Size는 보낼 바이트 수다.
	Size int64
	conn net.Conn
	left int64
	stop func() bool
}

func (b *Body) Read(p []byte) (int, error) {
	if b.left <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > b.left {
		p = p[:b.left]
	}
	n, err := b.conn.Read(p)
	b.left -= int64(n)
	if errors.Is(err, io.EOF) && b.left > 0 {
		err = io.ErrUnexpectedEOF
	}
	if err == nil && b.left == 0 {
		err = io.EOF
	}
	return n, err
}

// Close는 연결을 닫는다.
func (b *Body) Close() error {
	b.stop()
	return b.conn.Close()
}

// Read는 파일의 offset부터 length바이트(0이면 끝까지)를 받는다(사용자 권한). 내용은 Body로 읽는다.
// ctx가 끝나면 연결을 닫는다.
func (c *Client) Read(ctx context.Context, ticket, path string, offset, length int64) (proto.ReadResult, *Body, error) {
	params, err := marshal(proto.ReadParams{Path: path, Offset: offset, Length: length})
	if err != nil {
		return proto.ReadResult{}, nil, err
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return proto.ReadResult{}, nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	fail := func(err error) (proto.ReadResult, *Body, error) {
		stop()
		conn.Close()
		return proto.ReadResult{}, nil, err
	}
	req := proto.Request{V: proto.Version, Op: proto.OpRun, Ticket: ticket, Scope: proto.ScopeUser, Action: proto.ActionRead, Params: params}
	if err := proto.WriteRequest(conn, req); err != nil {
		return fail(fmt.Errorf("%w: %v", ErrUnavailable, err))
	}
	resp, err := proto.ReadResponse(conn)
	if err != nil {
		return fail(fmt.Errorf("%w: %v", ErrUnavailable, err))
	}
	var res proto.ReadResult
	if err := resp.Decode(&res); err != nil {
		return fail(err)
	}
	return res, &Body{Size: resp.Size, conn: conn, left: resp.Size, stop: stop}, nil
}

// Write는 r에서 size바이트를 읽어 path에 올린다(사용자 권한). overwrite가 false면 같은 이름이 있을 때 exists다.
// 도우미는 다 받은 뒤에 이름을 바꾸므로 실패해도 기존 파일은 그대로다.
func (c *Client) Write(ctx context.Context, ticket, path string, overwrite bool, r io.Reader, size int64) (proto.Entry, error) {
	params, err := marshal(proto.WriteParams{Path: path, Overwrite: overwrite})
	if err != nil {
		return proto.Entry{}, err
	}
	conn, err := c.dial(ctx)
	if err != nil {
		return proto.Entry{}, err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	req := proto.Request{V: proto.Version, Op: proto.OpRun, Ticket: ticket, Scope: proto.ScopeUser, Action: proto.ActionWrite, Params: params, Size: size}
	if err := proto.WriteRequest(conn, req); err != nil {
		return proto.Entry{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	_, copyErr := io.CopyN(conn, r, size)
	if copyErr != nil {
		// 내용이 모자라면 쓰기 쪽을 닫아 도우미가 마감 시간까지 기다리지 않고 바로 거절하게 한다.
		if uc, ok := conn.(*net.UnixConn); ok {
			uc.CloseWrite()
		}
	}
	// 보내는 도중 도우미가 거절하고 끊었어도 그 전에 보낸 응답은 읽을 수 있다.
	resp, err := proto.ReadResponse(conn)
	if err != nil {
		if copyErr != nil {
			return proto.Entry{}, copyErr
		}
		return proto.Entry{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	var e proto.Entry
	if err := resp.Decode(&e); err != nil {
		return proto.Entry{}, err
	}
	return e, nil
}

func marshal(v any) (json.RawMessage, error) { return json.Marshal(v) }
