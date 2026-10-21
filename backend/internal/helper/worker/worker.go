// Package worker는 권한 도우미가 연산 하나를 실행하는 자식 프로세스다.
//
// 도우미(root)는 연산마다 자기 실행 파일을 `worker` 하위 명령으로 실행하면서 사용자 범위면
// 그 사용자의 uid·gid·보조 그룹으로 강등하고, 작업(Job)은 stdin으로, 받은 연결은 fd 3으로 넘긴다.
// worker는 그 연결에 응답 머리와 본문을 직접 주고받는다. 그래서 root 부모는 사용자 데이터를
// 읽거나 쓰지 않고, 파일 권한(소유권·ACL)은 커널이 사용자 권한으로 검사한다.
//
// 테스트와 가짜 도우미는 프로세스 없이 Serve를 바로 부른다.
package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/user"
	"slices"
	"strconv"
	"syscall"
	"time"

	"onmaru/backend/config"
	"onmaru/backend/internal/helper/proto"
)

// MaxJob은 stdin으로 받는 작업의 최대 크기다.
const MaxJob = 256 << 10

// Job은 부모가 worker에 넘기는 작업이다. 부모가 티켓·범위·연산 허용 여부를 이미 확인했다.
type Job struct {
	Scope  string          `json:"scope"`
	Action string          `json:"action"`
	Params json.RawMessage `json:"params,omitempty"`
	// Size는 요청 머리 뒤에 따라오는 본문의 바이트 수다(fs.write).
	Size int64 `json:"size,omitempty"`
	// User와 Home은 티켓 사용자의 이름과 홈 디렉터리다(/etc/passwd에서 부모가 읽음).
	User string `json:"user"`
	Home string `json:"home"`
}

// Config는 worker 설정이다. 빈 값은 기본값을 쓴다.
type Config struct {
	// SrvDir는 데이터 파일시스템이 마운트되는 디렉터리다. 기본 /srv.
	SrvDir string
	// MountInfo는 마운트 목록(/proc/self/mountinfo 형식)을 연다.
	MountInfo func() (io.ReadCloser, error)
	// IdleTimeout은 연결에서 이만큼 진행이 없으면 끊는 시간이다. 기본 60초.
	IdleTimeout time.Duration
	// Now는 현재 시각이다(ssh.authorize의 키 줄 만료 시각, upgrade.apply의 요청 시각). 기본 time.Now.
	Now func() time.Time
	// UpgradeDir은 upgrade.apply가 적용 요청을 쓰는 디렉터리다. 기본 config.UpgradeRequestDir(도우미 유닛의
	// RuntimeDirectory=, 대시보드에서 업그레이드를 켰을 때만 있다).
	UpgradeDir string
}

func (c Config) withDefaults() Config {
	if c.SrvDir == "" {
		c.SrvDir = "/srv"
	}
	if c.MountInfo == nil {
		c.MountInfo = func() (io.ReadCloser, error) { return os.Open("/proc/self/mountinfo") }
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 60 * time.Second
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.UpgradeDir == "" {
		c.UpgradeDir = config.UpgradeRequestDir
	}
	return c
}

// Main은 `onmaru-helper worker`의 본체다. 종료 코드를 돌려준다.
// 0은 성공, 2는 연산 실패(stderr에 code=<코드>), 1은 실행 오류다.
func Main() int {
	// OMV Samba의 create mask 0664 / directory mask 0775와 같게 그룹 쓰기를 남긴다.
	syscall.Umask(0o002)
	job, err := readJob(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read job: %v\n", err)
		return 1
	}
	f := os.NewFile(3, "conn")
	if f == nil {
		fmt.Fprintln(os.Stderr, "no connection on fd 3")
		return 1
	}
	conn, err := net.FileConn(f)
	f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "connection: %v\n", err)
		return 1
	}
	defer conn.Close()
	// 부모가 범위에 맞게 강등했는지 다시 확인한다. 사용자 범위가 root로 돌면 안 된다.
	if err := checkPrivileges(job.Scope, os.Getuid(), os.Geteuid()); err != nil {
		cfg := Config{}.withDefaults()
		respond(conn, cfg, proto.Fail(proto.NewError(proto.CodeInternal)))
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if err := Serve(job, conn, Config{}); err != nil {
		fmt.Fprintf(os.Stderr, "code=%s\n", Code(err))
		return 2
	}
	return 0
}

// checkPrivileges는 범위와 실제 uid가 맞는지 확인한다.
func checkPrivileges(scope string, uid, euid int) error {
	switch scope {
	case proto.ScopeUser:
		if uid == 0 || euid == 0 {
			return errors.New("user scope worker is running as root")
		}
	case proto.ScopeRoot:
		if uid != 0 || euid != 0 {
			return errors.New("root scope worker is not running as root")
		}
	default:
		return fmt.Errorf("unknown scope %q", scope)
	}
	return nil
}

func readJob(r io.Reader) (Job, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxJob+1))
	if err != nil {
		return Job{}, err
	}
	if len(b) > MaxJob {
		return Job{}, errors.New("job too large")
	}
	var job Job
	if err := json.Unmarshal(b, &job); err != nil {
		return Job{}, err
	}
	return job, nil
}

// Serve는 작업 하나를 실행하고 응답을 conn에 쓴다. 실패하면 오류 응답을 쓴 뒤 그 오류를 돌려준다.
// 본문을 보내는 도중 실패하면 응답 머리를 다시 쓸 수 없으므로 연결만 끊긴다.
func Serve(job Job, conn net.Conn, cfg Config) error {
	cfg = cfg.withDefaults()
	if !proto.Allowed(job.Scope, job.Action) {
		err := proto.NewError(proto.CodeBadRequest)
		respond(conn, cfg, proto.Fail(err))
		return err
	}
	c := idleConn{conn: conn, timeout: cfg.IdleTimeout}
	var err error
	switch job.Action {
	case proto.ActionID:
		err = reply(c, func() (any, error) { return identity(job) })
	case proto.ActionList:
		err = withPath(job, c, func(p proto.PathParams) (any, error) { return list(job, cfg, p) })
	case proto.ActionStat:
		err = withPath(job, c, func(p proto.PathParams) (any, error) { return stat(job, cfg, p) })
	case proto.ActionMkdir:
		err = withPath(job, c, func(p proto.PathParams) (any, error) { return mkdir(job, cfg, p) })
	case proto.ActionNames:
		err = names(job, cfg, c)
	case proto.ActionStatMany:
		err = reply(c, func() (any, error) { return statMany(job, cfg) })
	case proto.ActionInfo:
		err = withPath(job, c, func(p proto.PathParams) (any, error) { return info(job, cfg, p) })
	case proto.ActionRead:
		err = read(job, cfg, c)
	case proto.ActionWrite:
		err = write(job, cfg, c)
	case proto.ActionRename:
		err = rename(job, cfg, c)
	case proto.ActionRemove:
		err = remove(job, cfg, c)
	case proto.ActionCopy:
		err = copyFiles(job, cfg, c)
	case proto.ActionSSHAuthorize, proto.ActionSSHRevoke:
		err = sshKey(job, cfg, c)
	case proto.ActionUpgradeApply:
		err = reply(c, func() (any, error) { return nil, upgradeApply(job, cfg) })
	case proto.ActionTrash:
		err = withPath(job, c, func(p proto.PathParams) (any, error) { return trash(job, cfg, p) })
	case proto.ActionTrashDirs:
		err = reply(c, func() (any, error) { return trashVolumes(job, cfg) })
	case proto.ActionTrashList:
		err = reply(c, func() (any, error) { return trashList(job, cfg) })
	case proto.ActionTrashRestore:
		err = reply(c, func() (any, error) {
			var p proto.TrashRestoreParams
			if err := proto.DecodeParams(job.Params, &p); err != nil {
				return nil, err
			}
			return trashRestore(job, cfg, p)
		})
	case proto.ActionTrashPurge:
		err = reply(c, func() (any, error) {
			var p proto.TrashItemParams
			if err := proto.DecodeParams(job.Params, &p); err != nil {
				return nil, err
			}
			return nil, trashPurge(job, cfg, p)
		})
	case proto.ActionTrashEmpty:
		err = reply(c, func() (any, error) { return nil, trashEmpty(job, cfg) })
	case proto.ActionTrashPrepare:
		err = reply(c, func() (any, error) { return nil, trashPrepare(job, cfg) })
	}
	return err
}

// reply는 fn의 결과를 응답한다.
func reply(c idleConn, fn func() (any, error)) error {
	v, err := fn()
	if err != nil {
		e := toError(err)
		c.respond(proto.Fail(e))
		return e
	}
	resp, err := proto.Success(v)
	if err != nil {
		c.respond(proto.Fail(err))
		return err
	}
	return c.respond(resp)
}

// withPath는 PathParams를 받는 연산을 실행한다.
func withPath(job Job, c idleConn, fn func(proto.PathParams) (any, error)) error {
	return reply(c, func() (any, error) {
		var p proto.PathParams
		if err := proto.DecodeParams(job.Params, &p); err != nil {
			return nil, err
		}
		return fn(p)
	})
}

func respond(conn net.Conn, cfg Config, resp proto.Response) {
	idleConn{conn: conn, timeout: cfg.IdleTimeout}.respond(resp)
}

// identity는 worker가 실제로 가진 uid·gid·보조 그룹과, 티켓 사용자의 홈 디렉터리(파일 연산의 기준으로 쓸 수
// 있을 때)다.
func identity(job Job) (any, error) {
	uid, gid := os.Getuid(), os.Getgid()
	id := proto.Identity{UID: uid, GID: gid, User: strconv.Itoa(uid)}
	if job.Scope == proto.ScopeUser && usableBase(job.Home) {
		id.Home = job.Home
	}
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		id.User = u.Username
	}
	gids, err := os.Getgroups()
	if err != nil {
		return nil, err
	}
	all := append([]int{gid}, gids...)
	slices.Sort(all[1:])
	seen := map[int]bool{}
	for _, g := range all {
		if seen[g] {
			continue
		}
		seen[g] = true
		name := strconv.Itoa(g)
		if gr, err := user.LookupGroupId(name); err == nil {
			name = gr.Name
		}
		id.Groups = append(id.Groups, proto.Group{ID: g, Name: name})
	}
	return id, nil
}

// Code는 err의 오류 코드다.
func Code(err error) string {
	return toError(err).Code
}

// toError는 파일 시스템 오류를 오류 코드로 바꾼다.
func toError(err error) *proto.Error {
	var pe *proto.Error
	if errors.As(err, &pe) {
		return pe
	}
	code := proto.CodeInternal
	switch {
	case isEscape(err):
		code = proto.CodeOutsideRoots
	case errors.Is(err, os.ErrDeadlineExceeded):
		code = proto.CodeTimeout
	case errors.Is(err, syscall.ENOTEMPTY):
		// ENOTEMPTY는 fs.ErrExist로도 맞으므로 먼저 본다.
		code = proto.CodeNotEmpty
	case errors.Is(err, fs.ErrNotExist):
		code = proto.CodeNotFound
	case errors.Is(err, fs.ErrExist):
		code = proto.CodeExists
	case errors.Is(err, fs.ErrPermission):
		code = proto.CodePermissionDenied
	case errors.Is(err, syscall.ENOTDIR):
		code = proto.CodeNotDir
	case errors.Is(err, syscall.EISDIR):
		code = proto.CodeIsDir
	case errors.Is(err, syscall.EROFS):
		code = proto.CodeReadOnly
	case errors.Is(err, syscall.ENOSPC), errors.Is(err, syscall.EDQUOT):
		code = proto.CodeNoSpace
	case errors.Is(err, syscall.EXDEV):
		// 같은 기준 디렉터리 안이라도 그 아래에 다른 파일시스템이 마운트되어 있으면 이름 바꾸기가 EXDEV다.
		code = proto.CodeCrossDevice
	case errors.Is(err, syscall.EINVAL), errors.Is(err, syscall.ELOOP), errors.Is(err, syscall.ENAMETOOLONG):
		code = proto.CodeBadRequest
	}
	return proto.NewError(code)
}

// isEscape는 os.Root가 범위 밖으로 나가는 경로(.. 또는 심볼릭 링크)를 거부한 오류인지다.
// os 패키지가 이 오류 값을 내보내지 않으므로 문구로 판별한다.
func isEscape(err error) bool {
	for e := err; e != nil; e = errors.Unwrap(e) {
		if e.Error() == "path escapes from parent" {
			return true
		}
	}
	return false
}

// idleConn은 읽기·쓰기마다 마감 시각을 늦춰, 진행이 멈춘 연결만 끊는다.
type idleConn struct {
	conn    net.Conn
	timeout time.Duration
}

func (c idleConn) Read(p []byte) (int, error) {
	c.conn.SetReadDeadline(time.Now().Add(c.timeout))
	return c.conn.Read(p)
}

func (c idleConn) Write(p []byte) (int, error) {
	c.conn.SetWriteDeadline(time.Now().Add(c.timeout))
	return c.conn.Write(p)
}

func (c idleConn) respond(resp proto.Response) error {
	return proto.WriteResponse(c, resp)
}
