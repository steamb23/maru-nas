// maru-helper는 온마루의 권한 도우미다. root로 돌며, maru가 로그인한 사용자의 권한이나
// (비밀번호를 다시 확인한 관리자의) root 권한으로 정해진 연산을 실행하게 해 준다.
//
// systemd 소켓 활성화(maru-helper.socket)로 실행하는 것이 기본이다. 설계와 보안 경계는
// .claude/docs/helper.md에 있다. maru 웹 코드는 이 실행 파일에 들어가지 않는다.
//
//	maru-helper [-client-user maru] [-elevate-group sudo] [-listen unix:/경로] [-idle-exit 10m]
//	maru-helper worker     (도우미가 연산마다 스스로 실행하는 자식 프로세스)
package main

import (
	"cmp"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"maru/backend/config"
	"maru/backend/internal/helper/daemon"
	"maru/backend/internal/helper/worker"
)

const chkpwdPath = "/usr/sbin/unix_chkpwd"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "worker" {
		os.Exit(worker.Main())
	}
	log.SetFlags(0)
	clientUser := flag.String("client-user", cmp.Or(os.Getenv(config.EnvHelperClientUser), config.DefaultHelperClientUser),
		"연결을 받을 사용자(maru 서비스 사용자, 환경 변수 MARU_HELPER_CLIENT_USER)")
	elevateGroup := flag.String("elevate-group", cmp.Or(os.Getenv(config.EnvHelperElevateGroup), config.DefaultElevateGroup),
		"root 권한 상승에 openmediavault-admin과 함께 필요한 그룹(환경 변수 MARU_HELPER_ELEVATE_GROUP)")
	listen := flag.String("listen", "", "소켓 활성화 없이 띄울 때의 수신 주소(unix:/절대/경로)")
	idleExit := flag.Duration("idle-exit", 10*time.Minute, "소켓 활성화일 때 티켓과 연결이 없이 이만큼 지나면 종료, 0이면 종료하지 않음")
	flag.Parse()

	if err := run(*clientUser, *elevateGroup, *listen, *idleExit); err != nil {
		log.Fatalf("maru-helper: %v", err)
	}
}

func run(clientUser, elevateGroup, listen string, idleExit time.Duration) error {
	if os.Geteuid() != 0 {
		return errors.New("must run as root")
	}
	clientUID, err := lookupClient(clientUser)
	if err != nil {
		return err
	}
	if !daemon.ValidGroupName(elevateGroup) {
		return fmt.Errorf("invalid elevate group %q", elevateGroup)
	}
	accounts := daemon.SystemAccounts{}
	if _, ok, _ := accounts.GroupID(elevateGroup); !ok {
		log.Printf("warning: group %q does not exist; nobody can elevate until it is created", elevateGroup)
	}
	if err := checkChkpwd(chkpwdPath); err != nil {
		return err
	}

	l, err := daemon.ActivationListener()
	if err != nil {
		return err
	}
	activated := l != nil
	if !activated {
		path, ok := strings.CutPrefix(listen, "unix:")
		if !ok || !strings.HasPrefix(path, "/") {
			return errors.New("not started by systemd socket activation; pass -listen unix:/absolute/path")
		}
		if l, err = daemon.ListenUnix(path); err != nil {
			return err
		}
		// 직접 띄운 소켓은 클라이언트 사용자만 연결할 수 있게 한다(상대 uid도 따로 확인한다).
		if err := os.Chown(path, clientUID, 0); err != nil {
			return err
		}
		defer os.Remove(path)
	}
	cfg := daemon.Config{
		ClientUID:    clientUID,
		ElevateGroup: elevateGroup,
		Accounts:     accounts,
		Verifier:     daemon.ChkPwd{Path: chkpwdPath},
		Spawner:      daemon.ExecSpawner{},
	}
	if activated {
		cfg.IdleExit = idleExit
	}
	srv, err := daemon.New(cfg)
	if err != nil {
		return err
	}
	log.Printf("maru-helper ready: client=%s(uid %d) elevate=%s+%s activated=%t",
		clientUser, clientUID, daemon.DefaultAdminGroup, elevateGroup, activated)
	return srv.Serve(l)
}

// lookupClient는 클라이언트 사용자의 uid다. root나 없는 사용자는 받지 않는다.
func lookupClient(name string) (int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, fmt.Errorf("client user %q: %w", name, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil || uid == 0 {
		return 0, fmt.Errorf("client user %q must be a non-root account", name)
	}
	return uid, nil
}

// checkChkpwd는 비밀번호 확인 도우미가 root 소유이고 다른 사용자가 고칠 수 없는지 확인한다.
func checkChkpwd(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("password checker: %w (install libpam-modules-bin)", err)
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok || sys.Uid != 0 || st.Mode().Perm()&0o022 != 0 || !st.Mode().IsRegular() {
		return fmt.Errorf("password checker %s must be a regular file owned by root and not writable by others", path)
	}
	return nil
}
