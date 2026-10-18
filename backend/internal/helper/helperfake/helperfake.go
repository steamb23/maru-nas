// Package helperfake는 개발·테스트용 가짜 권한 도우미다.
//
// 진짜 도우미 데몬(daemon.Server)에 가짜 계정·비밀번호(omvfake.DefaultUsers와 같은 이름·비밀번호)와
// 프로세스 없는 worker 실행을 넣는다. 권한을 바꾸지 않으므로 파일 연산은 이 프로세스의 권한으로,
// Root 아래의 가짜 홈(home/<사용자>)과 가짜 데이터 디스크(srv/disk1)에서만 한다. id 연산은 실제
// 권한 대신 가짜 계정의 uid·그룹(root 범위면 root)을 돌려준다.
//
// 운영 바이너리(cmd/onmaru, cmd/onmaru-helper)에서 import하지 않는다.
package helperfake

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"onmaru/backend/internal/helper/daemon"
	"onmaru/backend/internal/helper/proto"
	"onmaru/backend/internal/helper/worker"
)

// 가짜 그룹 ID(합성 값)
const (
	gidUsers = 100
	gidAdmin = 990
	gidSudo  = 27
)

// User는 가짜 계정이다.
type User struct {
	Name     string
	Password string
	UID      int
	Groups   []string
}

// DefaultUsers는 omvfake.DefaultUsers와 같은 계정이다. admin은 OMV 내장 관리자처럼 시스템 uid이고
// sudo 그룹이 아니라 상승할 수 없다. totp는 관리자이면서 sudo 그룹이라 상승할 수 있다.
func DefaultUsers() []User {
	return []User{
		{Name: "admin", Password: "admin", UID: 998, Groups: []string{"users", daemon.DefaultAdminGroup}},
		{Name: "user", Password: "user", UID: 1001, Groups: []string{"users"}},
		{Name: "totp", Password: "totp", UID: 1002, Groups: []string{"users", daemon.DefaultAdminGroup, daemon.DefaultElevateGroup}},
	}
}

// Options는 가짜 도우미 설정이다. 빈 값은 daemon.Config의 기본값을 쓴다.
type Options struct {
	Now        func() time.Time
	FailDelay  time.Duration
	ElevateTTL time.Duration
	Logf       func(format string, args ...any)
}

// Server는 실행 중인 가짜 도우미다.
type Server struct {
	// Socket은 수신 소켓 경로, Root는 가짜 홈·디스크의 루트 디렉터리다.
	Socket string
	Root   string

	accounts *accounts
	l        *net.UnixListener
	done     chan struct{}
}

// Start는 socket에서 가짜 도우미를 띄운다. root 아래에 가짜 홈과 디스크를 만든다.
func Start(socket, root string, users []User, opts Options) (*Server, error) {
	acc := &accounts{users: map[string]daemon.Account{}, passwords: map[string]string{},
		groups: map[string]int{"users": gidUsers, daemon.DefaultAdminGroup: gidAdmin, daemon.DefaultElevateGroup: gidSudo}}
	srvDir := filepath.Join(root, "srv")
	disk := filepath.Join(srvDir, "disk1")
	if err := os.MkdirAll(disk, 0o755); err != nil {
		return nil, err
	}
	for _, u := range users {
		home := filepath.Join(root, "home", u.Name)
		if err := os.MkdirAll(home, 0o755); err != nil {
			return nil, err
		}
		a := daemon.Account{Name: u.Name, UID: u.UID, GID: gidUsers, Home: home}
		for _, g := range u.Groups {
			gid, ok := acc.groups[g]
			if !ok {
				return nil, fmt.Errorf("unknown fake group %q", g)
			}
			a.Groups = append(a.Groups, gid)
		}
		acc.users[u.Name] = a
		acc.passwords[u.Name] = u.Password
	}
	// upgrade.apply는 /run 대신 Root 아래의 요청 디렉터리에 쓴다(UpgradeDir).
	upgradeDir := filepath.Join(root, "run", "onmaru-upgrade")
	if err := os.MkdirAll(upgradeDir, 0o700); err != nil {
		return nil, err
	}
	mountinfo := fmt.Sprintf("1 0 8:1 / / rw - ext4 /dev/fake rw\n2 1 8:17 / %s rw - ext4 /dev/fake1 rw\n", escape(disk))
	sp := &spawner{acc: acc, cfg: worker.Config{
		SrvDir:     srvDir,
		MountInfo:  func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(mountinfo)), nil },
		UpgradeDir: upgradeDir,
	}}
	srv, err := daemon.New(daemon.Config{
		ClientUID:  os.Getuid(),
		Accounts:   acc,
		Verifier:   acc,
		Spawner:    sp,
		Now:        opts.Now,
		FailDelay:  opts.FailDelay,
		ElevateTTL: opts.ElevateTTL,
		Logf:       opts.Logf,
	})
	if err != nil {
		return nil, err
	}
	l, err := daemon.ListenUnix(socket)
	if err != nil {
		return nil, err
	}
	s := &Server{Socket: socket, Root: root, accounts: acc, l: l, done: make(chan struct{})}
	go func() {
		srv.Serve(l)
		close(s.done)
	}()
	return s, nil
}

// Close는 수신을 멈추고 소켓 파일을 지운다.
func (s *Server) Close() {
	s.l.Close()
	<-s.done
	os.Remove(s.Socket)
}

// Home은 가짜 사용자의 홈 디렉터리다.
func (s *Server) Home(user string) string { return filepath.Join(s.Root, "home", user) }

// UpgradeDir은 upgrade.apply가 적용 요청을 쓰는 가짜 요청 디렉터리다.
func (s *Server) UpgradeDir() string { return filepath.Join(s.Root, "run", "onmaru-upgrade") }

// Disk는 가짜 데이터 디스크(/srv 아래 마운트) 경로다.
func (s *Server) Disk() string { return filepath.Join(s.Root, "srv", "disk1") }

// SetGroups는 가짜 사용자의 그룹을 바꾼다(그룹 변경 시험용).
func (s *Server) SetGroups(user string, groups ...string) {
	s.accounts.mu.Lock()
	defer s.accounts.mu.Unlock()
	a := s.accounts.users[user]
	a.Groups = nil
	for _, g := range groups {
		a.Groups = append(a.Groups, s.accounts.groups[g])
	}
	s.accounts.users[user] = a
}

func escape(p string) string { return strings.ReplaceAll(p, " ", `\040`) }

// accounts는 가짜 계정 DB이자 비밀번호 확인이다.
type accounts struct {
	mu        sync.Mutex
	users     map[string]daemon.Account
	passwords map[string]string
	groups    map[string]int
}

func (a *accounts) Lookup(name string) (daemon.Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	u, ok := a.users[name]
	if !ok {
		return daemon.Account{}, daemon.ErrNoAccount
	}
	u.Groups = slices.Clone(u.Groups)
	return u, nil
}

func (a *accounts) GroupID(name string) (int, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	g, ok := a.groups[name]
	return g, ok, nil
}

func (a *accounts) UIDRange() (int, int) { return 1000, 60000 }

func (a *accounts) Verify(_ context.Context, user, password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if pw, ok := a.passwords[user]; ok && pw != "" && pw == password {
		return nil
	}
	return daemon.ErrAuth
}

func (a *accounts) groupName(gid int) string {
	for name, g := range a.groups {
		if g == gid {
			return name
		}
	}
	return fmt.Sprint(gid)
}

// spawner는 프로세스 없이 worker를 실행한다. id는 가짜 계정의 권한으로 답한다.
type spawner struct {
	acc *accounts
	cfg worker.Config
}

func (s *spawner) Spawn(_ context.Context, cred *daemon.Credential, job worker.Job, conn *net.UnixConn) (string, bool, error) {
	if job.Action == proto.ActionID {
		id := proto.Identity{User: "root", Groups: []proto.Group{{ID: 0, Name: "root"}}}
		if cred != nil {
			id = proto.Identity{UID: cred.UID, GID: cred.GID, User: job.User}
			for _, g := range cred.Groups {
				id.Groups = append(id.Groups, proto.Group{ID: g, Name: s.acc.groupName(g)})
			}
		}
		resp, err := proto.Success(id)
		if err != nil {
			return proto.CodeInternal, false, err
		}
		return "", true, proto.WriteResponse(conn, resp)
	}
	if err := worker.Serve(job, conn, s.cfg); err != nil {
		return worker.Code(err), true, nil
	}
	return "", true, nil
}
