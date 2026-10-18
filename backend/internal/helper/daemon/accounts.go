package daemon

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"time"

	"onmaru/backend/config"
)

// Account는 로컬 계정 하나다.
type Account struct {
	Name string
	UID  int
	GID  int
	// Groups는 기본 그룹을 포함한 그룹 ID 목록이다.
	Groups []int
	Home   string
}

// ErrNoAccount는 계정이 없다는 오류다.
var ErrNoAccount = errors.New("no such account")

// Accounts는 계정·그룹 정보다. 도우미는 onmaru가 보낸 값이 아니라 이것만 믿는다.
type Accounts interface {
	// Lookup은 이름으로 계정을 찾는다. 없으면 ErrNoAccount다.
	Lookup(name string) (Account, error)
	// GroupID는 그룹 이름의 gid다. 없으면 ok가 false다.
	GroupID(name string) (gid int, ok bool, err error)
	// UIDRange는 일반 사용자 uid 범위다.
	UIDRange() (min, max int)
}

// SystemAccounts는 /etc/passwd·/etc/group(os/user)과 /etc/login.defs로 계정을 읽는다.
// 도우미는 CGO_ENABLED=0으로 빌드하므로 os/user는 NSS를 거치지 않고 로컬 파일만 읽는다(LDAP 계정은 없는 것으로 본다).
type SystemAccounts struct {
	// LoginDefs는 login.defs 경로다. 비우면 /etc/login.defs.
	LoginDefs string
}

func (SystemAccounts) Lookup(name string) (Account, error) {
	u, err := user.Lookup(name)
	if err != nil {
		var unknown user.UnknownUserError
		if errors.As(err, &unknown) {
			return Account{}, ErrNoAccount
		}
		return Account{}, err
	}
	if u.Username != name {
		return Account{}, ErrNoAccount
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 != nil || err2 != nil {
		return Account{}, errors.New("non-numeric uid or gid")
	}
	ids, err := u.GroupIds()
	if err != nil {
		return Account{}, err
	}
	acc := Account{Name: name, UID: uid, GID: gid, Home: u.HomeDir, Groups: []int{gid}}
	for _, s := range ids {
		g, err := strconv.Atoi(s)
		if err != nil || g == gid {
			continue
		}
		acc.Groups = append(acc.Groups, g)
	}
	return acc, nil
}

func (SystemAccounts) GroupID(name string) (int, bool, error) {
	g, err := user.LookupGroup(name)
	if err != nil {
		var unknown user.UnknownGroupError
		if errors.As(err, &unknown) {
			return 0, false, nil
		}
		return 0, false, err
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, false, err
	}
	return gid, true, nil
}

func (a SystemAccounts) UIDRange() (int, int) {
	path := a.LoginDefs
	if path == "" {
		path = "/etc/login.defs"
	}
	f, err := os.Open(path)
	if err != nil {
		return defaultUIDMin, defaultUIDMax
	}
	defer f.Close()
	return ParseUIDRange(f)
}

// login.defs가 없거나 값이 없을 때의 범위(Debian 기본값)
const (
	defaultUIDMin = 1000
	defaultUIDMax = 60000
)

// ParseUIDRange는 login.defs의 UID_MIN·UID_MAX를 읽는다. OMV도 같은 값으로 시스템 계정을 가른다.
func ParseUIDRange(r io.Reader) (int, int) {
	lo, hi := defaultUIDMin, defaultUIDMax
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		v, err := strconv.Atoi(fields[1])
		if err != nil || v < 0 {
			continue
		}
		switch fields[0] {
		case "UID_MIN":
			lo = v
		case "UID_MAX":
			hi = v
		}
	}
	return lo, hi
}

// userNameRE는 받는 사용자 이름이다. 실행 인자로 넘기므로 -로 시작하지 않게 한다.
var userNameRE = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`)

// ValidUserName은 사용자 이름 꼴이 맞는지다.
func ValidUserName(name string) bool { return userNameRE.MatchString(name) }

// ValidGroupName은 그룹 이름 꼴이 맞는지다(설정 검사용, 설치기와 같은 규칙인 config.ValidGroupName).
func ValidGroupName(name string) bool { return config.ValidGroupName(name) }

// ErrAuth는 비밀번호가 틀렸거나 계정이 만료·잠금 상태라는 오류다.
var ErrAuth = errors.New("authentication failed")

// Verifier는 비밀번호를 확인한다. 틀리면 ErrAuth, 확인 자체를 못 하면 다른 오류다.
type Verifier interface {
	Verify(ctx context.Context, user, password string) error
}

// maxPassword는 PAM이 받는 응답의 최대 길이(PAM_MAX_RESP_SIZE)다.
const maxPassword = 512

// ChkPwd는 pam_unix의 확인 도우미 unix_chkpwd로 비밀번호를 확인한다. root가 실행하면
// 아무 사용자나 확인할 수 있고, 비밀번호는 NUL로 끝나는 stdin 파이프로 받는다. 해시 방식
// (yescrypt 등)은 libcrypt가 처리하므로 Go로 shadow를 직접 검증하지 않는다.
// pam_faillock 등 PAM 스택은 거치지 않으므로 실패 제한은 도우미가 따로 한다.
type ChkPwd struct {
	// Path는 unix_chkpwd 경로다. 비우면 /usr/sbin/unix_chkpwd.
	Path string
}

func (v ChkPwd) path() string {
	if v.Path == "" {
		return "/usr/sbin/unix_chkpwd"
	}
	return v.Path
}

func (v ChkPwd) Verify(ctx context.Context, name, password string) error {
	if !ValidUserName(name) || password == "" || len(password) > maxPassword || strings.ContainsRune(password, 0) {
		return ErrAuth
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// nonull: 빈 비밀번호 계정은 받지 않는다.
	if err := v.run(ctx, name, "nonull", password+"\x00"); err != nil {
		return err
	}
	// chkexpiry: 계정 만료·비밀번호 변경 필요도 실패로 본다(OMV 로그인도 거절한다).
	return v.run(ctx, name, "chkexpiry", "")
}

func (v ChkPwd) run(ctx context.Context, name, mode, stdin string) error {
	cmd := exec.CommandContext(ctx, v.path(), name, mode)
	cmd.Env = []string{}
	cmd.Dir = "/"
	cmd.Stdin = strings.NewReader(stdin)
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && ctx.Err() == nil {
		return ErrAuth
	}
	return err
}
