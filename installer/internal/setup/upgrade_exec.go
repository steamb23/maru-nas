package setup

import (
	"bytes"
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"maru/backend/config"
)

// 업그레이드 서비스(--upgrade-service)가 명령을 실행하고 파일을 다루는 부분. 흐름은 upgrade_service.go에 있다.
//
// git과 빌드는 저장소 소유자로 강등해 실행한다. root로 실행하는 것은 마지막 설치 단계의 새 설치기뿐이다. 소유자가 만든
// 출력(git, 계획 JSON)은 크기를 제한해 읽고, 소유자가 쓸 수 있는 파일(요청, 빌드 결과)은 심볼릭 링크를 따라가지 않고 연다.

// 명령의 시간 제한. 서비스 전체는 유닛의 TimeoutStartSec(2시간)이 막는다.
const (
	gitTimeout     = 2 * time.Minute
	fetchTimeout   = 10 * time.Minute
	planTimeout    = 15 * time.Minute
	buildTimeout   = time.Hour
	installTimeout = 30 * time.Minute
)

// 읽는 크기의 상한.
const (
	maxOutput = 4 << 20
	// maxCheckRequest는 maru가 쓰는 확인 요청, maxRootFile은 root가 쓰는 설정 파일과 적용 요청(설정 32개 × 값 512바이트)이다.
	maxCheckRequest = 4 << 10
	maxRootFile     = 64 << 10
	maxStatusFile   = 4 << 20
	maxRecordFile   = 64 << 10
	maxTail         = 16 << 10
	maxTailLines    = 60
)

// rootPath는 root로 실행하는 마지막 설치 단계의 PATH다(systemd 서비스 기본값과 같다).
const rootPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// upgradeConfig는 /etc/default/maru-upgrade(config.UpgradeEnvFile)다. 저장소 경로는 이 파일에서만 정한다
// (서비스의 설치기는 저장소 밖의 root 소유 복사본이다).
type upgradeConfig struct {
	Repo, User, Path string
}

// parseUpgradeConfig는 설정 파일을 읽고 검사한다.
func parseUpgradeConfig(data []byte) (upgradeConfig, error) {
	env := ParseEnv(data)
	cfg := upgradeConfig{Repo: env.Value(config.EnvUpgradeRepo), User: env.Value(config.EnvUpgradeUser), Path: env.Value(config.EnvUpgradePath)}
	switch {
	case !filepath.IsAbs(cfg.Repo) || filepath.Clean(cfg.Repo) != cfg.Repo:
		return cfg, errors.New(config.EnvUpgradeRepo + " must be an absolute path")
	case !userNameRE.MatchString(cfg.User) || len(cfg.User) > 32:
		return cfg, errors.New(config.EnvUpgradeUser + " is not a valid user name")
	}
	if err := checkBuildPath(cfg.Path); err != nil {
		return cfg, fmt.Errorf("%s: %w", config.EnvUpgradePath, err)
	}
	return cfg, nil
}

// ownerAccount는 git과 빌드를 실행하는 저장소 소유자다.
type ownerAccount struct {
	Name     string
	UID, GID int
	Groups   []int
	Home     string
}

// lookupOwner는 소유자 계정을 로컬 계정 데이터베이스에서 찾는다. root는 받지 않는다.
func lookupOwner(name string) (ownerAccount, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return ownerAccount{}, err
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil {
		return ownerAccount{}, errors.New("unsupported uid/gid of " + name)
	}
	if uid == 0 {
		return ownerAccount{}, errors.New(name + " is root")
	}
	a := ownerAccount{Name: u.Username, UID: uid, GID: gid, Home: u.HomeDir}
	ids, err := u.GroupIds()
	if err != nil {
		return ownerAccount{}, err
	}
	for _, id := range ids {
		if g, err := strconv.Atoi(id); err == nil {
			a.Groups = append(a.Groups, g)
		}
	}
	return a, nil
}

// path는 가짜 루트(테스트) 아래의 경로다. 저장소와 임시 디렉터리는 실제 경로를 그대로 쓴다.
func (s *upgradeService) path(p string) string {
	if s.root == "" {
		return p
	}
	return filepath.Join(s.root, p)
}

// lookPath는 빌드 PATH에서 실행할 수 있는 name의 경로다. 없으면 빈 값이다.
func (s *upgradeService) lookPath(name string) string {
	for _, d := range strings.Split(s.cfg.Path, ":") {
		p := filepath.Join(d, name)
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// ownerEnv는 소유자 명령의 환경 변수다. 서비스의 환경을 물려주지 않고 새로 만든다. 묻는 프롬프트는 모두 끈다
// (git 자격 증명, SSH 비밀번호·호스트 키 확인). HOME이 있어야 Go 모듈 캐시와 npm 캐시가 소유자 것을 쓴다.
func (s *upgradeService) ownerEnv() []string {
	env := []string{
		"HOME=" + s.owner.Home,
		"USER=" + s.owner.Name,
		"LOGNAME=" + s.owner.Name,
		"PATH=" + s.cfg.Path,
		"LANG=C.UTF-8",
		"TERM=dumb",
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
	}
	if s.sshCommand != "" {
		env = append(env, "GIT_SSH_COMMAND="+s.sshCommand)
	}
	return append(env, s.extraEnv...)
}

// rootEnv는 마지막 설치 단계(새 설치기를 root로)의 환경 변수다.
func (s *upgradeService) rootEnv() []string {
	return append([]string{"HOME=/root", "PATH=" + rootPath, "LANG=C.UTF-8", "TERM=dumb"}, s.extraEnv...)
}

// cmdOpts는 명령 하나의 실행 방식이다.
type cmdOpts struct {
	// owner면 저장소 소유자로 강등하고(테스트는 dropPrivileges가 거짓이라 지금 계정 그대로) 이름을 빌드 PATH에서 찾는다.
	// 아니면 root로 실행하며 이름은 절대 경로다(마지막 설치 단계의 새 설치기).
	owner   bool
	dir     string
	timeout time.Duration
	// stdout은 표준 출력을 받는 곳이다. nil이면 로그다.
	stdout io.Writer
	// stderr는 오류 출력을 로그와 함께 받는 곳이다. nil이면 로그만이다.
	stderr io.Writer
}

// run은 명령 하나를 실행한다. 표준 입력은 /dev/null이고 새 세션에서 돈다(터미널 없음).
func (s *upgradeService) run(o cmdOpts, name string, args ...string) error {
	bin, env, who := name, s.rootEnv(), "root"
	if o.owner {
		if !strings.Contains(name, "/") {
			if bin = s.lookPath(name); bin == "" {
				return fmt.Errorf("%s not found in the build PATH", name)
			}
		}
		env, who = s.ownerEnv(), s.owner.Name
	}
	fmt.Fprintf(s.log, "[%s] %s\n", who, commandLine(name, args))
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = o.dir
	cmd.Env = env
	cmd.Stdout = s.log
	if o.stdout != nil {
		cmd.Stdout = o.stdout
	}
	cmd.Stderr = s.log
	if o.stderr != nil {
		cmd.Stderr = io.MultiWriter(s.log, o.stderr)
	}
	cmd.WaitDelay = 10 * time.Second
	attr := &syscall.SysProcAttr{Setsid: true}
	if o.owner && s.dropPrivileges {
		groups := make([]uint32, len(s.owner.Groups))
		for i, g := range s.owner.Groups {
			groups[i] = uint32(g)
		}
		attr.Credential = &syscall.Credential{Uid: uint32(s.owner.UID), Gid: uint32(s.owner.GID), Groups: groups}
	}
	cmd.SysProcAttr = attr
	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("%s: timed out after %v", commandLine(name, args), o.timeout)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", commandLine(name, args), err)
	}
	return nil
}

// git은 저장소에서 소유자로 git을 실행하고 표준 출력을 돌려준다(끝의 줄 바꿈은 벗긴다).
func (s *upgradeService) git(args ...string) (string, error) {
	var out limitedWriter
	out.max = maxOutput
	err := s.run(cmdOpts{owner: true, dir: s.cfg.Repo, timeout: gitTimeout, stdout: &out}, "git", append([]string{"-C", s.cfg.Repo}, args...)...)
	return strings.TrimRight(out.String(), "\n"), err
}

// gitTest는 참·거짓을 종료 코드로 알리는 git 명령(merge-base --is-ancestor, rev-parse --verify --quiet)이다. 0이면 참, 1이면 거짓,
// 그 밖은 오류다.
func (s *upgradeService) gitTest(args ...string) (bool, error) {
	_, err := s.git(args...)
	var ee *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return false, nil
	}
	return false, err
}

// limitedWriter는 max 바이트까지만 받는 버퍼다. 넘치면 쓰기 오류로 명령을 끊는다.
type limitedWriter struct {
	buf bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, errors.New("output too large")
	}
	return w.buf.Write(p)
}

func (w *limitedWriter) String() string { return w.buf.String() }

// tailBuffer는 로그의 끝부분(상태 파일의 Error.Log)이다. 단계를 시작할 때 비운다.
type tailBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 2*maxTail {
		t.buf = append([]byte(nil), t.buf[len(t.buf)-maxTail:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) reset() {
	t.mu.Lock()
	t.buf = nil
	t.mu.Unlock()
}

// String은 마지막 maxTailLines줄(maxTail 바이트 안)이다.
func (t *tailBuffer) String() string {
	t.mu.Lock()
	b := t.buf
	if len(b) > maxTail {
		b = b[len(b)-maxTail:]
	}
	s := strings.ToValidUTF8(string(b), "?")
	t.mu.Unlock()
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > maxTailLines {
		lines = lines[len(lines)-maxTailLines:]
	}
	return strings.Join(lines, "\n")
}

// readNoFollow는 path의 마지막 구성 요소가 심볼릭 링크면 따라가지 않고, 일반 파일만 max 바이트까지 읽는다(FIFO에서
// 멈추지 않게 O_NONBLOCK).
func readNoFollow(path string, max int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if fi.Size() > max {
		return nil, errors.New("file too large")
	}
	return io.ReadAll(io.LimitReader(f, max+1))
}

// takeFile은 요청 파일을 max 바이트까지 읽고 지운다(경로 유닛의 PathExists=가 다시 실행하지 않게, 내용과 상관없이
// 지운다). 없으면 fs.ErrNotExist다. 일반 파일이 아니면(디렉터리 등) 따라가지 않고 통째로 지운 뒤 오류다.
func takeFile(path string, max int64) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	data, rerr := readNoFollow(path, max)
	var derr error
	if fi.IsDir() {
		derr = os.RemoveAll(path)
	} else {
		derr = os.Remove(path)
	}
	if derr != nil && !errors.Is(derr, fs.ErrNotExist) {
		return nil, derr
	}
	return data, rerr
}

// writeStatus는 상태 파일을 원자적으로 쓴다(root 소유 0644, maru와 다른 사용자가 읽는다).
func (s *upgradeService) writeStatus() error {
	s.status.Format = config.UpgradeStatusFormat
	s.status.UpdatedAt = s.stamp()
	data, err := json.MarshalIndent(s.status, "", "  ")
	if err != nil {
		return err
	}
	path := s.path(config.UpgradeStatusFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".status-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// readStatus는 지난 상태 파일이다. 없거나 읽을 수 없으면 빈 상태다.
func (s *upgradeService) readStatus() config.UpgradeStatus {
	var st config.UpgradeStatus
	data, err := readNoFollow(s.path(config.UpgradeStatusFile), maxStatusFile)
	if err != nil || json.Unmarshal(data, &st) != nil || st.Format != config.UpgradeStatusFormat {
		return config.UpgradeStatus{}
	}
	return st
}

// readRecord는 설치 기록이다. 없거나 읽을 수 없거나 버전·커밋이 비면 nil이다.
func (s *upgradeService) readRecord() *config.InstallRecord {
	data, err := readNoFollow(s.path(config.InstallRecordFile), maxRecordFile)
	if err != nil {
		return nil
	}
	var rec config.InstallRecord
	if json.Unmarshal(data, &rec) != nil || rec.Format != config.InstallRecordFormat ||
		!config.ValidUpgradeTarget(rec.Revision) || rec.Version == "" {
		return nil
	}
	return &rec
}

// readBuildRevision은 소유자가 빌드한 실행 파일의 커밋과 변경 여부다. 심볼릭 링크를 따라가지 않는다.
func readBuildRevision(path string) (string, bool, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		return "", false, errors.New("not a regular file")
	}
	bi, err := buildinfo.Read(f)
	if err != nil {
		return "", false, err
	}
	rev, _, modified := vcsInfo(bi)
	return rev, modified, nil
}

// stamp는 지금 시각(RFC 3339, UTC)이다.
func (s *upgradeService) stamp() string { return s.now().UTC().Format(time.RFC3339) }
