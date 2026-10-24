// maru는 마루 NAS 대시보드의 백엔드다.
package main

import (
	"context"
	"errors"
	"log"
	"maps"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"maru/backend/config"
	"maru/backend/internal/addons"
	"maru/backend/internal/addons/builtin"
	"maru/backend/internal/aptindex"
	"maru/backend/internal/helper/client"
	"maru/backend/internal/jobs"
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
	o := parseFlags()

	network, address, err := parseListen(o.listen, o.allowNonLoopback)
	if err != nil {
		log.Fatal(err)
	}
	if config.CheckListen(o.listen, false) != nil {
		log.Printf("WARNING: listening on non-loopback address %s. Requests can bypass the reverse proxy: "+
			"traffic is plain HTTP and X-Forwarded-For/X-Forwarded-Proto can be forged by any client "+
			"(login rate limit, OMV auth log, cookie Secure flag)", address)
	}
	if err := config.CheckOMVURL(o.omvURL); err != nil {
		log.Fatal(err)
	}
	if err := config.CheckWorkbenchURL(o.workbenchURL); err != nil {
		log.Fatal(err)
	}
	if err := config.CheckHelperSocket(o.helperSocket); err != nil {
		log.Fatal(err)
	}
	if err := terminal.CheckAddr(o.sshAddr); err != nil {
		log.Fatal(err)
	}
	rp := relyingParty(o.publicOrigin)
	// 인증 앱에 보이는 계정 이름(<사용자>@<호스트>)에 쓴다. 읽지 못하면 사용자 이름만 쓴다.
	hostname, _ := os.Hostname()

	store, jobStore := openStores(o.stateDir)

	deps := builtin.HostDeps()
	hour, minute, err := config.ParseRefreshAt(o.smartAt)
	if err != nil {
		log.Fatal(err)
	}
	deps.Smart.RefreshHour, deps.Smart.RefreshMinute = hour, minute
	plugins := detectPlugins()

	runtime := o.runtime()

	upgrades := upgrade.New(upgradePaths(os.Getenv(envDevUpgradeRoot), o.stateDir), nil)

	var mounts server.MountSource = deps.Storage
	filesSrv := os.Getenv(envDevFilesSrv)
	if filesSrv != "" {
		log.Printf("files: dev disks under %s (%s)", filesSrv, envDevFilesSrv)
		mounts = devDisks{}
	}

	apt, notices := newNotices()
	deps.Apt = apt
	// 작업이 끝나면 APT·재부팅 알림이 새 목록을 바로 다시 읽게 한다.
	jobManager := jobs.New(jobs.Config{Store: jobStore, Index: apt, OnFinish: notices.Invalidate})

	helper := connectHelper(o.helperSocket)

	srv := &http.Server{
		Handler: server.New(server.Config{
			System:       deps.System,
			Static:       web.Handler(),
			OMV:          omv.NewClient(o.omvURL, userAgent),
			Addons:       builtin.New(deps),
			Plugins:      plugins,
			Prefs:        store,
			Site:         store,
			Images:       store,
			CookiePath:   o.cookiePath,
			WorkbenchURL: o.workbenchURL,
			// 설정의 서버 페이지에서 덮어쓴 SMART 기준 시각을 실행 중인 캐시에 반영한다.
			SmartRefreshAt:    o.smartAt,
			SetSmartRefreshAt: deps.Smart.SetRefreshAt,
			Runtime:           runtime,
			Helper:            helper,
			Terminal: terminal.NewManager(&terminal.Dialer{
				Addr:     o.sshAddr,
				HostKeys: o.sshHostKeys,
				Logf:     log.Printf,
			}),
			TerminalEnabled: o.terminalEnabled,
			Mounts:          mounts,
			FilesSrvDir:     filesSrv,
			Upgrade:         upgrades,
			Notify:          notices,
			Jobs:            jobManager,
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
		// 작업 감시를 멈추고 진행 기록을 남긴다. OMV 쪽 작업은 계속 돌고, 다시 시작하면 이어 지켜본다.
		jobManager.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	ln, err := listenOn(network, address)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s:%s (omv: %s)", network, address, o.omvURL)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// relyingParty는 -public-origin으로 정한 패스키의 신뢰 당사자다. 비우면 nil(패스키를 끈다)이고, 해석할 수 없으면 종료한다.
func relyingParty(publicOrigin string) *security.RP {
	if publicOrigin == "" {
		return nil
	}
	r, err := security.ParseOrigin(publicOrigin)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("passkeys: rp id %s (origin %s)", r.ID, r.Origin)
	return &r
}

// stateStore는 사용자 설정, 전역 설정, 배경 이미지, 보안 강화 모드의 수단 기록을 함께 저장한다.
type stateStore interface {
	prefs.Store
	prefs.SiteStore
	prefs.ImageStore
	security.Store
}

// openStores는 상태 디렉터리의 저장소를 연다. 비우면 메모리에만 둔다. 둘째 값은 서버 백그라운드 작업(시스템 업데이트)의
// 기록이며, 상태 디렉터리가 있으면 그 안에 둔다.
func openStores(stateDir string) (stateStore, jobs.Store) {
	if stateDir == "" {
		log.Printf("no state directory: preferences will not persist across restarts")
		return prefs.NewMemoryStore(), jobs.NewMemoryStore()
	}
	fileStore, err := prefs.NewFileStore(stateDir)
	if err != nil {
		log.Fatalf("state directory: %v", err)
	}
	log.Printf("state directory: %s", stateDir)
	return fileStore, fileStore
}

// detectPlugins는 설치된 OMV 플러그인과 OMV 버전을 읽어 기록한다.
func detectPlugins() *addons.Detector {
	plugins := addons.NewDetector()
	plugins.Refresh()
	log.Printf("detected omv plugins: %v", slices.Sorted(maps.Keys(plugins.Plugins())))
	if omv := plugins.OMV(); omv.Version != "" && omv.VersionName != "" {
		log.Printf("detected omv: %s (%s)", omv.Version, omv.VersionName)
	} else if omv.Version != "" {
		log.Printf("detected omv: %s", omv.Version)
	}
	return plugins
}

// newNotices는 APT 업데이트 목록과 재부팅 권장 파일을 읽는 Reader와, 그것을 출처로 쓰는 서버 알림이다.
// 개발 서버는 가짜 OMV가 쓰는 가짜 루트를 읽고, 개발용 알림 파일이 있으면 그 출처도 더한다.
func newNotices() (*aptindex.Reader, *notify.Center) {
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
	return apt, notices
}

// connectHelper는 권한 도우미 클라이언트다. 소켓을 비우면 nil(도우미 기능을 끈다)이다.
func connectHelper(socket string) server.Helper {
	if socket == "" {
		return nil
	}
	hc := client.New(socket)
	// 이전 실행의 세션은 메모리와 함께 사라졌으므로 그 티켓도 폐기한다.
	resetCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := hc.Reset(resetCtx); err != nil {
		log.Printf("helper: %s: not reachable yet (%v); logins will retry", socket, err)
	} else {
		log.Printf("helper: %s", socket)
	}
	cancel()
	return hc
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
