package setup

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"maru/backend/config"
)

// Account는 계정 데이터베이스(getent passwd) 항목 일부다.
type Account struct {
	Name string
	UID  int
	Home string
}

// System은 설치기가 시스템에 하는 모든 일이다. 실제 시스템(realSystem), 바꾸는 작업을 보여 주기만 하는
// dry-run(dryRunSystem), 테스트용 가짜 루트가 같은 인터페이스를 따른다. 경로는 설치 대상 시스템 기준 절대 경로다.
type System interface {
	ReadFile(path string) ([]byte, error)
	Stat(path string) (fs.FileInfo, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	// ReadNoFollow는 다른 계정이 쓸 수 있는 파일을 읽는다. 마지막 구성 요소가 심볼릭 링크면 따라가지 않고, 일반 파일만
	// max 바이트까지 읽는다(FIFO에서 멈추지 않게 O_NONBLOCK). dry-run에서도 실제로 읽는다.
	ReadNoFollow(path string, max int64) ([]byte, error)
	// TakeFile은 요청 파일을 ReadNoFollow처럼 읽고 내용과 상관없이 지운다(경로 유닛의 PathExists=가 다시 실행하지
	// 않게). 없으면 fs.ErrNotExist다. 일반 파일이 아니면 따라가지 않고 통째로 지운 뒤 오류다. 지우지 못하면
	// errRequestKept로 감싼 오류다.
	TakeFile(path string, max int64) ([]byte, error)
	// ReadBuildInfo는 Go 실행 파일에 들어 있는 빌드 정보(커밋 등)를 읽는다. dry-run에서도 실제로 읽는다.
	ReadBuildInfo(path string) (*debug.BuildInfo, error)
	// WriteFile은 path를 root:root 소유로 원자적으로(같은 디렉터리의 임시 파일 → 이름 바꾸기) 쓴다.
	WriteFile(path string, data []byte, perm fs.FileMode) error
	// CopyFile은 cp -p처럼 권한·소유자·시각을 유지해 복사한다(설정 파일 백업).
	CopyFile(src, dst string) error
	// InstallFile은 install -o root -g root -m perm처럼 실행 파일을 설치한다. 새 inode로 바꾸므로 실행 중인
	// 파일 위에도 쓸 수 있다.
	InstallFile(src, dst string, perm fs.FileMode) error
	// Remove는 rm -f처럼 파일을 지운다. 없으면 아무것도 하지 않는다.
	Remove(path string) error
	// RemoveAll은 rm -rf다.
	RemoveAll(path string) error
	// Lock은 path(없으면 0600으로 만든다. 디렉터리는 있어야 한다)에 배타 잠금(flock)을 잡는다. 다른 쪽이 잡고 있으면 wait
	// 동안 기다린다. 잡은 뒤 path가 그 파일을 그대로 가리키는지 다시 본다(그 사이에 지우고 다시 만들었으면 새 파일을
	// 다시 잡는다). 돌려준 함수가 잠금을 푼다. dry-run은 잠그지 않는다.
	Lock(path string, wait time.Duration) (unlock func(), err error)

	// Run은 시스템을 바꾸는 명령을 실행한다. 출력을 그대로 보여 준다.
	Run(name string, args ...string) error
	// RunQuiet는 Run과 같지만 표준 출력을 버린다(>/dev/null).
	RunQuiet(name string, args ...string) error
	// RunSilent는 Run과 같지만 표준 출력과 오류 출력을 모두 버린다(>/dev/null 2>&1).
	RunSilent(name string, args ...string) error
	// Check는 읽기 전용 확인 명령(systemctl is-active 등)이다. 출력을 버리고 종료 코드만 본다. dry-run에서도 실행한다.
	Check(name string, args ...string) error
	// Output은 읽기 전용 명령(node --version 등)의 표준 출력이다. 오류 출력은 버린다. dry-run에서도 실행한다.
	Output(name string, args ...string) ([]byte, error)
	// RunUser는 실행한 계정의 빌드 명령이다. 저장소 안에만 쓰므로 dry-run에서도 실행한다.
	RunUser(dir, name string, args ...string) error
	// LookPath는 PATH에 명령이 있는지다.
	LookPath(name string) bool
	// Which는 PATH에서 찾은 명령의 절대 경로다. 없으면 false다(빌드 PATH를 정할 때 쓴다).
	Which(name string) (string, bool)

	// CurrentUser는 실행한 계정 이름이다.
	CurrentUser() string
	// LookupUser는 계정을 찾는다. 없으면 false다.
	LookupUser(name string) (Account, bool, error)
	// PrimaryGroup은 계정의 기본 그룹 이름(id -gn)이다.
	PrimaryGroup(name string) (string, error)
	// GroupExists는 그룹이 있는지다.
	GroupExists(name string) (bool, error)
	// GroupID는 그룹의 gid를 NSS까지 보고 찾는다(getent group). 없으면 false다.
	GroupID(name string) (int, bool, error)
	// FileOwner는 파일 소유자다(UID, 계정 이름과 홈). 이름을 찾을 수 없으면 이름은 uid 숫자다.
	FileOwner(path string) (Account, error)

	// PortInUse는 TCP 수신 주소를 다른 프로세스가 쓰고 있는지다. 알 수 없으면 false다.
	PortInUse(addr string) bool
	// HealthCheck는 수신 주소의 /api/health를 한 번 확인한다.
	HealthCheck(listen string) error
	Sleep(d time.Duration)
	Now() time.Time

	// DryRun은 바꾸는 작업을 실제로 하지 않고 보여 주기만 하는지다.
	DryRun() bool
}

// fsOps는 root 아래의 파일 작업이다. 실제 시스템은 root가 비어 있고, 테스트는 임시 디렉터리를 root로 쓴다.
type fsOps struct {
	root string
	// chown은 쓴 파일을 root:root(또는 원본 소유자)로 바꾸는지다. root로 실행할 때만 켠다.
	chown bool
}

func (f fsOps) path(p string) string {
	if f.root == "" {
		return p
	}
	return filepath.Join(f.root, p)
}

func (f fsOps) ReadFile(p string) ([]byte, error)       { return os.ReadFile(f.path(p)) }
func (f fsOps) Stat(p string) (fs.FileInfo, error)      { return os.Stat(f.path(p)) }
func (f fsOps) ReadDir(p string) ([]fs.DirEntry, error) { return os.ReadDir(f.path(p)) }
func (f fsOps) RemoveAll(p string) error                { return os.RemoveAll(f.path(p)) }
func (f fsOps) ReadNoFollow(p string, max int64) ([]byte, error) {
	return readNoFollow(f.path(p), max)
}
func (f fsOps) TakeFile(p string, max int64) ([]byte, error) { return takeFile(f.path(p), max) }
func (f fsOps) ReadBuildInfo(p string) (*debug.BuildInfo, error) {
	return buildinfo.ReadFile(f.path(p))
}
func (f fsOps) WriteFile(p string, data []byte, perm fs.FileMode) error {
	return f.writeAtomic(f.path(p), perm, 0, 0, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

func (f fsOps) FileOwner(p string) (Account, error) {
	fi, err := os.Stat(f.path(p))
	if err != nil {
		return Account{}, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return Account{}, errors.New("cannot read the owner of " + p)
	}
	a := Account{UID: int(st.Uid), Name: strconv.Itoa(int(st.Uid))}
	if u, err := user.LookupId(a.Name); err == nil {
		a.Name, a.Home = u.Username, u.HomeDir
	}
	return a, nil
}

func (f fsOps) Remove(p string) error {
	if err := os.Remove(f.path(p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func (f fsOps) Lock(p string, wait time.Duration) (func(), error) {
	path := f.path(p)
	deadline := time.Now().Add(wait)
	for {
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
		if err != nil {
			return nil, fmt.Errorf("lock %s: %w", p, err)
		}
		for {
			err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
				break
			}
			if time.Now().After(deadline) {
				file.Close()
				return nil, fmt.Errorf("lock %s: still held by another process after %s", p, wait)
			}
			time.Sleep(100 * time.Millisecond)
		}
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("lock %s: %w", p, err)
		}
		held, herr := file.Stat()
		now, nerr := os.Lstat(path)
		if herr == nil && nerr == nil && os.SameFile(held, now) {
			return func() { file.Close() }, nil
		}
		// 기다리는 동안 다른 쪽이 잠금 파일(또는 디렉터리)을 지웠다. 다시 연다(디렉터리가 없으면 위에서 오류로 끝난다).
		file.Close()
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock %s: the lock file kept changing", p)
		}
	}
}

func (f fsOps) CopyFile(src, dst string) error {
	in, err := os.Open(f.path(src))
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	uid, gid := 0, 0
	st, _ := fi.Sys().(*syscall.Stat_t)
	if st != nil {
		uid, gid = int(st.Uid), int(st.Gid)
	}
	out := f.path(dst)
	if err := f.writeAtomic(out, fi.Mode().Perm(), uid, gid, func(w io.Writer) error {
		_, err := io.Copy(w, in)
		return err
	}); err != nil {
		return err
	}
	atime := fi.ModTime()
	if st != nil {
		atime = time.Unix(st.Atim.Sec, st.Atim.Nsec)
	}
	return os.Chtimes(out, atime, fi.ModTime())
}

func (f fsOps) InstallFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(f.path(src))
	if err != nil {
		return err
	}
	defer in.Close()
	return f.writeAtomic(f.path(dst), perm, 0, 0, func(w io.Writer) error {
		_, err := io.Copy(w, in)
		return err
	})
}

// writeAtomic은 같은 디렉터리의 임시 파일에 쓰고 권한·소유자를 맞춘 뒤 이름을 바꾼다. 디렉터리가 없으면 만들고,
// umask와 상관없이 0755로 둔다(설치 기록은 일반 사용자도 읽는다).
func (f fsOps) writeAtomic(path string, perm fs.FileMode, uid, gid int, write func(io.Writer) error) error {
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if err := write(tmp); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if f.chown {
		if err := tmp.Chown(uid, gid); err != nil {
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	ok = true
	return nil
}

// realSystem은 실제 시스템이다.
type realSystem struct {
	fsOps
	out, errOut io.Writer
}

// NewRealSystem은 실제 시스템을 만든다. 명령 출력은 out·errOut에 쓴다.
func NewRealSystem(out, errOut io.Writer) System {
	return &realSystem{fsOps: fsOps{chown: os.Geteuid() == 0}, out: out, errOut: errOut}
}

func (s *realSystem) exec(stdout, stderr io.Writer, dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return nil
}

func (s *realSystem) Output(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return out, nil
}

func (s *realSystem) Run(name string, args ...string) error {
	return s.exec(s.out, s.errOut, "", name, args...)
}

func (s *realSystem) RunQuiet(name string, args ...string) error {
	return s.exec(io.Discard, s.errOut, "", name, args...)
}

func (s *realSystem) RunSilent(name string, args ...string) error {
	return s.exec(io.Discard, io.Discard, "", name, args...)
}

func (s *realSystem) Check(name string, args ...string) error {
	return s.exec(io.Discard, io.Discard, "", name, args...)
}

func (s *realSystem) RunUser(dir, name string, args ...string) error {
	return s.exec(s.out, s.errOut, dir, name, args...)
}

func (s *realSystem) LookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (s *realSystem) Which(name string) (string, bool) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	if p, err = filepath.Abs(p); err != nil {
		return "", false
	}
	return p, true
}

func (s *realSystem) CurrentUser() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return strconv.Itoa(os.Getuid())
}

// LookupUser는 NSS까지 보는 getent passwd로 찾는다(설치 스크립트와 같게).
func (s *realSystem) LookupUser(name string) (Account, bool, error) {
	out, err := exec.Command("getent", "passwd", name).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 2 {
			return Account{}, false, nil
		}
		return Account{}, false, fmt.Errorf("getent passwd %s: %w", name, err)
	}
	return passwdAccount(name, out)
}

// passwdAccount는 찾은 계정의 getent passwd 출력을 읽는다. 읽지 못한 줄을 "계정 없음"으로 보면 계정이 있는지 확인하는
// 쪽(애드온 백엔드의 고정 계정 확인 등)이 통과하므로 오류다.
func passwdAccount(name string, out []byte) (Account, bool, error) {
	a, ok := parsePasswd(strings.TrimSpace(string(out)))
	if !ok {
		return Account{}, false, fmt.Errorf("getent passwd %s: unexpected output", name)
	}
	return a, true, nil
}

// parsePasswd는 /etc/passwd 형식 한 줄을 읽는다.
func parsePasswd(line string) (Account, bool) {
	f := strings.Split(line, ":")
	if len(f) < 7 {
		return Account{}, false
	}
	uid, err := parseID(f[2])
	if err != nil {
		return Account{}, false
	}
	return Account{Name: f[0], UID: uid, Home: f[5]}, true
}

func (s *realSystem) PrimaryGroup(name string) (string, error) {
	out, err := exec.Command("id", "-gn", name).Output()
	if err != nil {
		return "", fmt.Errorf("id -gn %s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *realSystem) GroupExists(name string) (bool, error) {
	err := exec.Command("getent", "group", name).Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 2 {
		return false, nil
	}
	return false, fmt.Errorf("getent group %s: %w", name, err)
}

func (s *realSystem) GroupID(name string) (int, bool, error) {
	out, err := exec.Command("getent", "group", name).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 2 {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("getent group %s: %w", name, err)
	}
	gid, ok := parseGroupID(strings.TrimSpace(string(out)))
	if !ok {
		return 0, false, fmt.Errorf("getent group %s: unexpected output", name)
	}
	return gid, true, nil
}

// parseGroupID는 /etc/group 형식 한 줄(이름:비밀번호:gid:구성원)의 gid다.
func parseGroupID(line string) (int, bool) {
	f := strings.Split(line, ":")
	if len(f) < 3 {
		return 0, false
	}
	gid, err := parseID(f[2])
	return gid, err == nil
}

// PortInUse는 그 주소에 잠깐 수신해 본다. 주소가 이미 쓰이고 있으면(EADDRINUSE) true다.
// 같은 포트를 다른 주소(0.0.0.0 등)에서 쓰는 경우도 겹치면 잡힌다.
func (s *realSystem) PortInUse(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.Is(err, syscall.EADDRINUSE)
	}
	ln.Close()
	return false
}

func (s *realSystem) HealthCheck(listen string) error {
	client, url := healthRequest(listen)
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	return nil
}

// healthRequest는 수신 주소로 /api/health를 확인할 클라이언트와 URL을 만든다. 모든 주소에서 수신하면
// 루프백으로 확인한다. Unix 소켓은 그 소켓으로 접속한다. 프록시 환경 변수는 쓰지 않는다.
func healthRequest(listen string) (*http.Client, string) {
	tr := &http.Transport{}
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	if path, ok := strings.CutPrefix(listen, config.UnixPrefix); ok {
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}
		return client, "http://localhost/api/health"
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return client, "http://" + listen + "/api/health"
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return client, "http://" + net.JoinHostPort(host, port) + "/api/health"
}

func (s *realSystem) Sleep(d time.Duration) { time.Sleep(d) }
func (s *realSystem) Now() time.Time        { return time.Now() }
func (s *realSystem) DryRun() bool          { return false }

// commandLine은 명령을 셸에서 그대로 쓸 수 있게 보여 준다.
func commandLine(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(name))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote는 셸이 다르게 해석할 문자가 있으면 작은따옴표로 감싼다.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=@,+%", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
