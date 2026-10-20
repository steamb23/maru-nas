// Package proto는 onmaru와 권한 도우미(onmaru-helper) 사이의 Unix 소켓 프로토콜이다.
//
// 연결 하나에 요청 하나다. 요청과 응답은 모두 프레임(4바이트 big-endian 길이 + JSON 머리)으로
// 시작하고, 머리의 Size만큼 원시 본문이 뒤따를 수 있다(올리기는 요청 뒤, 읽기는 응답 뒤).
// 길이 접두사라 도우미 부모 프로세스가 머리만 정확히 읽고 남은 본문은 worker에 넘길 수 있다.
//
// 요청에는 비밀번호와 티켓이 들어 있으므로 Request를 로그에 남기지 않는다.
package proto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Version은 프로토콜 버전이다. 요청의 V가 다르면 도우미가 거절한다.
const Version = 1

// 프레임 머리(JSON)의 최대 크기. 요청은 경로·비밀번호 정도라 작고, 응답은 목록(fs.list)을 담는다.
const (
	MaxRequest  = 64 << 10
	MaxResponse = 8 << 20
)

// 요청 종류(Request.Op)
const (
	// OpOpen은 비밀번호를 확인하고 티켓을 발급한다. Pending이면 Activate 전까지 쓸 수 없다.
	OpOpen = "session.open"
	// OpActivate는 2차 인증을 마친 대기 티켓을 쓸 수 있게 한다.
	OpActivate = "session.activate"
	// OpClose는 티켓을 폐기한다.
	OpClose = "session.close"
	// OpReset은 모든 티켓을 폐기한다. onmaru가 시작할 때 이전 실행의 티켓을 정리한다.
	OpReset = "session.reset"
	// OpStatus는 티켓 상태(상승 여부 등)를 알려 준다.
	OpStatus = "session.status"
	// OpElevate는 비밀번호를 다시 확인하고 티켓에 root 권한 상승을 짧게 붙인다.
	OpElevate = "session.elevate"
	// OpDrop은 상승을 바로 끝낸다.
	OpDrop = "session.drop"
	// OpRun은 연산(Action)을 Scope 권한으로 실행한다.
	OpRun = "run"
)

// 실행 범위(Request.Scope)
const (
	// ScopeUser는 티켓 사용자의 uid·gid·보조 그룹으로 실행한다.
	ScopeUser = "user"
	// ScopeRoot는 root로 실행한다. 상승한 티켓만 쓸 수 있다.
	ScopeRoot = "root"
)

// 연산(Request.Action)
const (
	ActionID     = "id"
	ActionList   = "fs.list"
	ActionStat   = "fs.stat"
	ActionRead   = "fs.read"
	ActionWrite  = "fs.write"
	ActionMkdir  = "fs.mkdir"
	ActionRename = "fs.rename"
	ActionRemove = "fs.remove"
	// ActionNames는 폴더의 이름과 종류만 읽는다(항목마다 lstat 하지 않음). 이름은 응답 본문으로 보낸다.
	ActionNames = "fs.names"
	// ActionStatMany는 한 폴더 안의 이름 여러 개를 lstat한다.
	ActionStatMany = "fs.statmany"
	// ActionInfo는 파일 하나의 소유자·그룹·ACL 유무·내 권한을 함께 읽는다(파일 탐색기의 정보 패널).
	ActionInfo = "fs.info"
	// ActionSSHAuthorize는 ~/.ssh/authorized_keys에 onmaru가 접속할 때만 쓰는 짧은 수명의 키 줄을 넣는다.
	ActionSSHAuthorize = "ssh.authorize"
	// ActionSSHRevoke는 그 키 줄(과 만료된 onmaru 키 줄)을 지운다.
	ActionSSHRevoke = "ssh.revoke"
	// ActionUpgradeApply는 대시보드에서 업그레이드의 적용 요청(대상 커밋, 설정 값)을 업그레이드 서비스의 요청
	// 디렉터리에 쓴다(root 범위). 명령은 받지 않는다.
	ActionUpgradeApply = "upgrade.apply"
)

// Actions는 범위마다 실행할 수 있는 연산이다. root 연산은 여기에 타입 있는 함수로만 더한다.
var Actions = map[string][]string{
	ScopeUser: {ActionID, ActionList, ActionStat, ActionRead, ActionWrite, ActionMkdir, ActionRename, ActionRemove,
		ActionNames, ActionStatMany, ActionInfo, ActionSSHAuthorize, ActionSSHRevoke},
	ScopeRoot: {ActionID, ActionUpgradeApply},
}

// Allowed는 scope에서 action을 실행할 수 있는지다.
func Allowed(scope, action string) bool {
	for _, a := range Actions[scope] {
		if a == action {
			return true
		}
	}
	return false
}

// Request는 요청 머리다.
type Request struct {
	V        int    `json:"v"`
	Op       string `json:"op"`
	Ticket   string `json:"ticket,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
	// Pending은 OpOpen에서 2차 인증 대기 티켓을 요청한다.
	Pending bool            `json:"pending,omitempty"`
	Scope   string          `json:"scope,omitempty"`
	Action  string          `json:"action,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	// Size는 머리 뒤에 따라오는 본문의 바이트 수다(fs.write).
	Size int64 `json:"size,omitempty"`
}

// Response는 응답 머리다.
type Response struct {
	OK   bool   `json:"ok"`
	Code string `json:"code,omitempty"`
	// RetryAfterSeconds는 Code가 rate_limited일 때 다시 시도할 수 있을 때까지의 초다.
	RetryAfterSeconds int             `json:"retryAfterSeconds,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
	// Size는 머리 뒤에 따라오는 본문의 바이트 수다(fs.read).
	Size int64 `json:"size,omitempty"`
}

// 오류 코드(Response.Code)
const (
	CodeBadRequest          = "bad_request"
	CodeForbiddenPeer       = "forbidden_peer"
	CodeTicketInvalid       = "ticket_invalid"
	CodeAuthFailed          = "auth_failed"
	CodeRateLimited         = "rate_limited"
	CodeAccountNotAllowed   = "account_not_allowed"
	CodeElevationNotAllowed = "elevation_not_allowed"
	CodeElevationRequired   = "elevation_required"
	CodeNotFound            = "not_found"
	CodePermissionDenied    = "permission_denied"
	CodeExists              = "exists"
	CodeOutsideRoots        = "outside_roots"
	CodeNotRegular          = "not_regular"
	CodeNotDir              = "not_dir"
	CodeIsDir               = "is_dir"
	CodeNotEmpty            = "not_empty"
	CodeReadOnly            = "read_only"
	CodeNoSpace             = "no_space"
	CodeBusy                = "busy"
	CodeTimeout             = "timeout"
	// CodeNoHome은 사용자 홈 디렉터리가 없거나 쓸 수 없는 경로(/)라는 뜻이다.
	CodeNoHome = "no_home"
	// CodeSSHStrictModes는 홈이나 ~/.ssh가 다른 사용자에게 쓰기를 허용해 sshd(StrictModes)가
	// authorized_keys를 무시한다는 뜻이다.
	CodeSSHStrictModes = "ssh_strict_modes"
	// CodeUpgradeDisabled는 대시보드에서 업그레이드가 꺼져 있다는 뜻이다(요청 디렉터리가 없다).
	CodeUpgradeDisabled = "upgrade_disabled"
	CodeInternal        = "internal"
)

// Error는 도우미가 돌려준 오류 코드다.
type Error struct {
	Code string
	// RetryAfter는 rate_limited일 때 기다릴 초다.
	RetryAfter int
}

func (e *Error) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("helper: %s (retry after %ds)", e.Code, e.RetryAfter)
	}
	return "helper: " + e.Code
}

// NewError는 코드만 담은 오류다. 경로·사용자 이름 같은 세부는 응답에 넣지 않고 도우미 로그에만 남긴다.
func NewError(code string) *Error { return &Error{Code: code} }

// CodeOf는 err가 *Error면 그 코드, 아니면 빈 문자열이다.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// Fail은 오류 응답 머리다.
func Fail(err error) Response {
	var e *Error
	if errors.As(err, &e) {
		return Response{Code: e.Code, RetryAfterSeconds: e.RetryAfter}
	}
	return Response{Code: CodeInternal}
}

// Success는 result를 담은 성공 응답 머리다.
func Success(result any) (Response, error) {
	if result == nil {
		return Response{OK: true}, nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return Response{}, err
	}
	return Response{OK: true, Result: b}, nil
}

// Err는 오류 응답을 *Error로 바꾼다. 성공이면 nil이다.
func (r Response) Err() error {
	if r.OK {
		return nil
	}
	code := r.Code
	if code == "" {
		code = CodeInternal
	}
	return &Error{Code: code, RetryAfter: r.RetryAfterSeconds}
}

// Decode는 성공 응답의 Result를 v로 읽는다.
func (r Response) Decode(v any) error {
	if err := r.Err(); err != nil {
		return err
	}
	if len(r.Result) == 0 {
		return errors.New("helper: empty result")
	}
	return json.Unmarshal(r.Result, v)
}

// WriteRequest는 요청 머리를 쓴다.
func WriteRequest(w io.Writer, req Request) error { return writeFrame(w, req, MaxRequest) }

// WriteResponse는 응답 머리를 쓴다.
func WriteResponse(w io.Writer, resp Response) error { return writeFrame(w, resp, MaxResponse) }

// writeFrame은 v를 JSON으로 바꿔 프레임 하나로 쓴다.
func writeFrame(w io.Writer, v any, limit int) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > limit {
		return errors.New("helper: frame too large")
	}
	buf := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(buf, uint32(len(b)))
	copy(buf[4:], b)
	_, err = w.Write(buf)
	return err
}

// readFrame은 프레임 하나를 읽는다. 머리 뒤의 바이트는 읽지 않는다(버퍼링하지 않음).
func readFrame(r io.Reader, limit uint32) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(n[:])
	if size == 0 || size > limit {
		return nil, errors.New("helper: invalid frame size")
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

// ReadRequest는 요청 머리를 읽는다. 모르는 필드가 있으면 거부한다.
func ReadRequest(r io.Reader) (Request, error) {
	b, err := readFrame(r, MaxRequest)
	if err != nil {
		return Request{}, err
	}
	var req Request
	if err := decodeStrict(b, &req); err != nil {
		return Request{}, err
	}
	return req, nil
}

// ReadResponse는 응답 머리를 읽는다.
func ReadResponse(r io.Reader) (Response, error) {
	b, err := readFrame(r, MaxResponse)
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(b, &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// DecodeParams는 Params를 v로 읽는다. 모르는 필드가 있으면 거부한다.
func DecodeParams(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	if err := decodeStrict(raw, v); err != nil {
		return NewError(CodeBadRequest)
	}
	return nil
}

func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("helper: trailing data")
	}
	return nil
}
