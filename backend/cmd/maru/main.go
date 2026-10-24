// maru는 마루 NAS 대시보드의 백엔드다.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"maru/backend/config"
	"maru/backend/internal/addons"
	"maru/backend/internal/addons/builtin"
	"maru/backend/internal/addons/storage"
	"maru/backend/internal/api"
	"maru/backend/internal/aptindex"
	"maru/backend/internal/helper/client"
	"maru/backend/internal/notify"
	"maru/backend/internal/omv"
	"maru/backend/internal/prefs"
	"maru/backend/internal/security"
	"maru/backend/internal/server"
	"maru/backend/internal/terminal"
	"maru/backend/internal/upgrade"
	"maru/backend/internal/web"
)

// userAgent는 OMV를 호출할 때 쓰는 고정 User-Agent다. OMV는 세션을 User-Agent에 묶는다.
const userAgent = "maru/" + config.Version

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv(config.EnvListen), config.DefaultListen),
		"수신 주소. host:port(기본은 루프백만 허용) 또는 unix:/절대/경로.sock (환경 변수 MARU_LISTEN)")
	allowNonLoopback := flag.Bool("allow-non-loopback", envBool(config.EnvAllowNonLoopback),
		"루프백이 아닌 TCP 주소(0.0.0.0 등)에서도 수신한다. 위험: 리버스 프록시를 거치지 않은 접속을 받는다 (환경 변수 MARU_ALLOW_NON_LOOPBACK)")
	omvURL := flag.String("omv-url", cmp.Or(os.Getenv(config.EnvOMVURL), config.DefaultOMVURL),
		"OMV 웹 UI 주소 (루프백만 허용, 환경 변수 MARU_OMV_URL)")
	cookiePath := flag.String("cookie-path", cmp.Or(os.Getenv(config.EnvCookiePath), config.DefaultCookiePath),
		"브라우저가 보는 대시보드 경로 (세션 쿠키 Path, 환경 변수 MARU_COOKIE_PATH)")
	// systemd의 StateDirectory=를 쓰면 STATE_DIRECTORY가 설정된다.
	stateDir := flag.String("state-dir", cmp.Or(os.Getenv(config.EnvStateDir), os.Getenv(config.EnvSystemdStateDir)),
		"사용자 설정(위젯 배치 등)을 저장할 디렉터리. 비우면 메모리에만 둔다 (환경 변수 MARU_STATE_DIR, STATE_DIRECTORY)")
	workbenchURL := flag.String("workbench-url", os.Getenv(config.EnvWorkbenchURL),
		"브라우저에서 OMV 웹 UI(workbench)를 열 주소. http(s) URL 또는 대시보드 기준 경로(/, ../). 비우면 OMV 워크벤치 앱을 숨긴다 (환경 변수 MARU_WORKBENCH_URL)")
	smartAt := flag.String("smart-refresh-at", cmp.Or(os.Getenv(config.EnvSmartRefreshAt), config.DefaultSmartRefreshAt),
		"SMART 정보를 하루 한 번 다시 가져오는 기준 시각 HH:MM, 서버 로컬 시간 (환경 변수 MARU_SMART_REFRESH_AT)")
	helperSocket := flag.String("helper-socket", os.Getenv(config.EnvHelperSocket),
		"권한 도우미(maru-helper) 소켓의 절대 경로. 비우면 도우미 기능을 끈다 (환경 변수 MARU_HELPER_SOCKET)")
	terminalEnabled := flag.Bool("terminal", envBool(config.EnvTerminal),
		"웹 터미널(같은 호스트의 sshd에 SSH로 접속)을 허용한다. 설정 앱에서 덮어쓸 수 있다 (환경 변수 MARU_TERMINAL)")
	sshAddr := flag.String("ssh-addr", cmp.Or(os.Getenv(config.EnvSSHAddr), config.DefaultSSHAddr),
		"웹 터미널이 접속할 sshd 주소. auto는 /etc/ssh/sshd_config의 Port로 127.0.0.1에 접속한다. 루프백 IP:포트만 허용 (환경 변수 MARU_SSH_ADDR)")
	sshHostKeys := flag.String("ssh-host-keys", cmp.Or(os.Getenv(config.EnvSSHHostKeys), config.DefaultSSHHostKeys),
		"웹 터미널이 받을 sshd 호스트 공개 키 파일의 glob 패턴 (환경 변수 MARU_SSH_HOST_KEYS)")
	publicOrigin := flag.String("public-origin", os.Getenv(config.EnvPublicOrigin),
		"브라우저가 대시보드에 접속하는 공개 HTTPS 주소(https://호스트[:포트], 도메인 이름). 보안 강화 모드의 패스키에 쓴다. 비우면 패스키를 끈다 (환경 변수 MARU_PUBLIC_ORIGIN)")
	flag.Parse()

	network, address, err := parseListen(*listen, *allowNonLoopback)
	if err != nil {
		log.Fatal(err)
	}
	if config.CheckListen(*listen, false) != nil {
		log.Printf("WARNING: listening on non-loopback address %s. Requests can bypass the reverse proxy: "+
			"traffic is plain HTTP and X-Forwarded-For/X-Forwarded-Proto can be forged by any client "+
			"(login rate limit, OMV auth log, cookie Secure flag)", address)
	}
	if err := config.CheckOMVURL(*omvURL); err != nil {
		log.Fatal(err)
	}
	if err := config.CheckWorkbenchURL(*workbenchURL); err != nil {
		log.Fatal(err)
	}
	if err := config.CheckHelperSocket(*helperSocket); err != nil {
		log.Fatal(err)
	}
	if err := terminal.CheckAddr(*sshAddr); err != nil {
		log.Fatal(err)
	}
	var rp *security.RP
	if *publicOrigin != "" {
		r, err := security.ParseOrigin(*publicOrigin)
		if err != nil {
			log.Fatal(err)
		}
		rp = &r
		log.Printf("passkeys: rp id %s (origin %s)", r.ID, r.Origin)
	}
	// 인증 앱에 보이는 계정 이름(<사용자>@<호스트>)에 쓴다. 읽지 못하면 사용자 이름만 쓴다.
	hostname, _ := os.Hostname()

	// store는 사용자 설정, 전역 설정, 배경 이미지, 보안 강화 모드의 수단 기록을 함께 저장한다.
	var store interface {
		prefs.Store
		prefs.SiteStore
		prefs.ImageStore
		security.Store
	}
	if *stateDir == "" {
		log.Printf("no state directory: preferences will not persist across restarts")
		store = prefs.NewMemoryStore()
	} else {
		fileStore, err := prefs.NewFileStore(*stateDir)
		if err != nil {
			log.Fatalf("state directory: %v", err)
		}
		log.Printf("state directory: %s", *stateDir)
		store = fileStore
	}

	deps := builtin.HostDeps()
	hour, minute, err := config.ParseRefreshAt(*smartAt)
	if err != nil {
		log.Fatal(err)
	}
	deps.Smart.RefreshHour, deps.Smart.RefreshMinute = hour, minute
	plugins := addons.NewDetector()
	plugins.Refresh()
	log.Printf("detected omv plugins: %v", slices.Sorted(maps.Keys(plugins.Plugins())))
	if omv := plugins.OMV(); omv.Version != "" && omv.VersionName != "" {
		log.Printf("detected omv: %s (%s)", omv.Version, omv.VersionName)
	} else if omv.Version != "" {
		log.Printf("detected omv: %s", omv.Version)
	}

	runtime := runtimeSettings(explicitFlags(), os.LookupEnv, []runtimeEntry{
		{flag: "listen", env: config.EnvListen, value: *listen},
		{flag: "allow-non-loopback", env: config.EnvAllowNonLoopback, value: strconv.FormatBool(*allowNonLoopback)},
		{flag: "omv-url", env: config.EnvOMVURL, value: *omvURL},
		{flag: "cookie-path", env: config.EnvCookiePath, value: *cookiePath},
		{flag: "state-dir", env: config.EnvStateDir, value: *stateDir, systemdEnv: config.EnvSystemdStateDir},
		// 빈 값은 "끔"이라는 뜻으로 일부러 넣는 값이다(설치기, installer/).
		{flag: "workbench-url", env: config.EnvWorkbenchURL, value: *workbenchURL, emptyIsSet: true},
		{flag: "smart-refresh-at", env: config.EnvSmartRefreshAt, value: *smartAt},
		{flag: "helper-socket", env: config.EnvHelperSocket, value: *helperSocket},
		{flag: "terminal", env: config.EnvTerminal, value: strconv.FormatBool(*terminalEnabled)},
		{flag: "ssh-addr", env: config.EnvSSHAddr, value: *sshAddr},
		{flag: "ssh-host-keys", env: config.EnvSSHHostKeys, value: *sshHostKeys},
		// 빈 값은 "패스키 끔"이라는 뜻으로 일부러 넣는 값이다(설치기, installer/).
		{flag: "public-origin", env: config.EnvPublicOrigin, value: *publicOrigin, emptyIsSet: true},
	})

	upgrades := upgrade.New(upgradePaths(os.Getenv(envDevUpgradeRoot), *stateDir), nil)

	var mounts server.MountSource = deps.Storage
	filesSrv := os.Getenv(envDevFilesSrv)
	if filesSrv != "" {
		log.Printf("files: dev disks under %s (%s)", filesSrv, envDevFilesSrv)
		mounts = devDisks{}
	}

	// APT 업데이트 목록과 재부팅 권장 파일. 개발 서버는 가짜 OMV가 쓰는 가짜 루트를 읽는다.
	apt := aptindex.NewReader()
	if root := os.Getenv(envDevNotifyRoot); root != "" {
		log.Printf("apt index: dev root %s (%s)", root, envDevNotifyRoot)
		apt = aptindex.NewReaderFS(os.DirFS(root))
	}
	sources := []notify.Source{aptindex.UpdatesSource{Reader: apt}, aptindex.RebootSource{Reader: apt}}
	notices := notify.New(sources...)
	if path := os.Getenv(envDevNotices); path != "" {
		log.Printf("notifications: dev fixture %s (%s)", path, envDevNotices)
		notices = notify.New(append(sources, devNotices{path: path, admin: true}, devNotices{path: path})...)
		notices.TTL = 2 * time.Second
	}

	var helper server.Helper
	if *helperSocket != "" {
		hc := client.New(*helperSocket)
		// 이전 실행의 세션은 메모리와 함께 사라졌으므로 그 티켓도 폐기한다.
		resetCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := hc.Reset(resetCtx); err != nil {
			log.Printf("helper: %s: not reachable yet (%v); logins will retry", *helperSocket, err)
		} else {
			log.Printf("helper: %s", *helperSocket)
		}
		cancel()
		helper = hc
	}

	srv := &http.Server{
		Handler: server.New(server.Config{
			System:       deps.System,
			Static:       web.Handler(),
			OMV:          omv.NewClient(*omvURL, userAgent),
			Addons:       builtin.New(deps),
			Plugins:      plugins,
			Prefs:        store,
			Site:         store,
			Images:       store,
			CookiePath:   *cookiePath,
			WorkbenchURL: *workbenchURL,
			// 설정의 서버 페이지에서 덮어쓴 SMART 기준 시각을 실행 중인 캐시에 반영한다.
			SmartRefreshAt:    *smartAt,
			SetSmartRefreshAt: deps.Smart.SetRefreshAt,
			Runtime:           runtime,
			Helper:            helper,
			Terminal: terminal.NewManager(&terminal.Dialer{
				Addr:     *sshAddr,
				HostKeys: *sshHostKeys,
				Logf:     log.Printf,
			}),
			TerminalEnabled: *terminalEnabled,
			Mounts:          mounts,
			FilesSrvDir:     filesSrv,
			Upgrade:         upgrades,
			Notify:          notices,
			Security:        store,
			RP:              rp,
			Hostname:        hostname,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	ln, err := listenOn(network, address)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s:%s (omv: %s)", network, address, *omvURL)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// parseListen은 -listen 값을 확인하고 net.Listen 인자로 바꾼다. 검사 규칙은 config.CheckListen이다.
func parseListen(addr string, allowNonLoopback bool) (network, address string, err error) {
	if err := config.CheckListen(addr, allowNonLoopback); err != nil {
		return "", "", err
	}
	if path, ok := strings.CutPrefix(addr, config.UnixPrefix); ok {
		return "unix", path, nil
	}
	return "tcp", addr, nil
}

// listenOn은 수신을 시작한다. Unix 소켓이면 이전 실행이 남긴 소켓 파일을 지우고
// 권한을 0660(소유자·그룹만 접속)으로 둔다. 종료할 때 소켓 파일은 리스너가 지운다.
func listenOn(network, address string) (net.Listener, error) {
	if network != "unix" {
		return net.Listen(network, address)
	}
	if fi, err := os.Lstat(address); err == nil {
		// 소켓이 아닌 파일은 경로 실수일 수 있으므로 지우지 않는다.
		if fi.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("%s exists and is not a socket", address)
		}
		// 다른 인스턴스가 쓰고 있는 소켓은 빼앗지 않는다.
		if c, err := net.Dial("unix", address); err == nil {
			c.Close()
			return nil, fmt.Errorf("%s is in use by another process", address)
		}
		if err := os.Remove(address); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	ln, err := net.Listen("unix", address)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(address, 0o660); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

// envBool은 불리언 환경 변수를 읽는다. 비어 있으면 false이고, 해석할 수 없으면 종료한다.
func envBool(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Fatalf("%s: invalid boolean %q", key, v)
	}
	return b
}

// runtimeEntry는 설정의 서버 페이지에 보여 줄 가동 설정 하나다.
type runtimeEntry struct {
	flag, env, value string
	// systemdEnv는 env가 없을 때 값을 주는 systemd 환경 변수다(StateDirectory=의 STATE_DIRECTORY).
	systemdEnv string
	// emptyIsSet이면 빈 환경 변수도 설정한 값으로 본다(빈 값이 기능을 끈다는 뜻인 설정).
	emptyIsSet bool
}

// explicitFlags는 명령줄에서 준 플래그 이름이다.
func explicitFlags() map[string]bool {
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

// runtimeSettings는 가동 설정 값마다 출처를 정한다. 명령줄 플래그가 환경 변수보다 우선한다(main의 플래그 기본값이
// 환경 변수이므로). 빈 환경 변수는 기본값을 쓰므로 출처를 기본값으로 본다.
func runtimeSettings(explicit map[string]bool, lookup func(string) (string, bool), entries []runtimeEntry) []api.RuntimeSetting {
	out := make([]api.RuntimeSetting, 0, len(entries))
	for _, e := range entries {
		source := api.RuntimeSourceDefault
		if v, ok := lookup(e.env); ok && (v != "" || e.emptyIsSet) {
			source = api.RuntimeSourceEnv
		} else if v, _ := lookup(e.systemdEnv); e.systemdEnv != "" && v != "" {
			source = api.RuntimeSourceSystemd
		}
		if explicit[e.flag] {
			source = api.RuntimeSourceFlag
		}
		out = append(out, api.RuntimeSetting{Flag: e.flag, Env: e.env, Value: e.value, Source: source})
	}
	return out
}

// envDevUpgradeRoot는 개발 서버(make dev-fake) 전용 환경 변수다. 대시보드에서 업그레이드의 root 쪽 설정 파일과 상태
// 파일 경로 앞에 붙는 가짜 루트이며, 가짜 업그레이드 서비스(cmd/fakeomv)가 그 아래에 쓴다. 운영에서 바꿀 이유가 없는
// 경로이므로 플래그로 두지 않는다(가동 설정 목록에도 넣지 않는다).
const envDevUpgradeRoot = "MARU_DEV_UPGRADE_ROOT"

// envDevNotifyRoot는 개발 서버(make dev-fake) 전용 환경 변수다. APT 업데이트 목록(upgradeindex.json)과 재부팅 권장
// 파일을 읽는 가짜 루트이며, 가짜 OMV 프로세스(cmd/fakeomv)가 FAKEAPT 시나리오대로 그 아래에 쓴다. 운영에서는 언제나
// /이므로 플래그로 두지 않는다(가동 설정 목록·설치기 레지스트리에도 넣지 않는다).
const envDevNotifyRoot = "MARU_DEV_NOTIFY_ROOT"

// envDevFilesSrv는 개발 서버(make dev-fake) 전용 환경 변수다. 파일 탐색기가 데이터 디스크로 보는 디렉터리를
// 가짜 권한 도우미의 루트(<상태 디렉터리>/helper-root/srv)로 바꾸고, 그 바로 아래 폴더를 디스크로 본다(가짜 디스크는
// 마운트가 아니라 폴더다). 운영에서는 언제나 /srv이므로 플래그로 두지 않는다(가동 설정 목록에도 넣지 않는다).
const envDevFilesSrv = "MARU_DEV_FILES_SRV"

// devDisks는 개발 서버의 가짜 디스크(dir 바로 아래 폴더)다.
type devDisks struct{}

func (devDisks) MountsUnder(dir string) ([]storage.MountLabel, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []storage.MountLabel
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, storage.MountLabel{MountPoint: filepath.Join(dir, e.Name())})
		}
	}
	return out, nil
}

// upgradePaths는 대시보드에서 업그레이드의 파일 경로다. 업그레이드 서비스의 경로 유닛은 설치한 상태 디렉터리
// (config.InstalledStateDir) 안의 확인 요청만 지켜보므로, 상태 디렉터리가 그곳이 아니면 확인 요청을 쓰지 않는다
// (업데이트 페이지는 state-dir로 꺼진다). 개발용 가짜 루트가 있으면 지금 상태 디렉터리에 쓴다.
func upgradePaths(devRoot, stateDir string) upgrade.Paths {
	if devRoot != "" {
		log.Printf("upgrade: dev root %s (%s)", devRoot, envDevUpgradeRoot)
		return upgrade.NewPaths(devRoot, stateDir)
	}
	if stateDir != "" && filepath.Clean(stateDir) != config.InstalledStateDir {
		stateDir = ""
	}
	return upgrade.NewPaths("", stateDir)
}
