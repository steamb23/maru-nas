// omvdesk는 OMV 데스크톱 대시보드의 백엔드다.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"log"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/addons/builtin"
	"omv-desktop-dashboard/backend/internal/addons/smart"
	"omv-desktop-dashboard/backend/internal/omv"
	"omv-desktop-dashboard/backend/internal/prefs"
	"omv-desktop-dashboard/backend/internal/server"
	"omv-desktop-dashboard/backend/internal/web"
)

// userAgent는 OMV를 호출할 때 쓰는 고정 User-Agent다. OMV는 세션을 User-Agent에 묶는다.
const userAgent = "omvdesk/0.2"

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv("OMVDESK_LISTEN"), "127.0.0.1:8090"),
		"수신 주소 (루프백만 허용, 환경 변수 OMVDESK_LISTEN)")
	omvURL := flag.String("omv-url", cmp.Or(os.Getenv("OMVDESK_OMV_URL"), "http://127.0.0.1"),
		"OMV 웹 UI 주소 (루프백만 허용, 환경 변수 OMVDESK_OMV_URL)")
	cookiePath := flag.String("cookie-path", cmp.Or(os.Getenv("OMVDESK_COOKIE_PATH"), "/desk"),
		"브라우저가 보는 대시보드 경로 (세션 쿠키 Path, 환경 변수 OMVDESK_COOKIE_PATH)")
	// systemd의 StateDirectory=를 쓰면 STATE_DIRECTORY가 설정된다.
	stateDir := flag.String("state-dir", cmp.Or(os.Getenv("OMVDESK_STATE_DIR"), os.Getenv("STATE_DIRECTORY")),
		"사용자 설정(위젯 배치 등)을 저장할 디렉터리. 비우면 메모리에만 둔다 (환경 변수 OMVDESK_STATE_DIR, STATE_DIRECTORY)")
	smartAt := flag.String("smart-refresh-at", cmp.Or(os.Getenv("OMVDESK_SMART_REFRESH_AT"), "00:00"),
		"SMART 정보를 하루 한 번 다시 가져오는 기준 시각 HH:MM, 서버 로컬 시간 (환경 변수 OMVDESK_SMART_REFRESH_AT)")
	flag.Parse()

	if err := checkLoopback(*listen); err != nil {
		log.Fatal(err)
	}
	if err := checkOMVURL(*omvURL); err != nil {
		log.Fatal(err)
	}

	var store prefs.Store
	if *stateDir == "" {
		log.Printf("no state directory: preferences will not persist across restarts")
		store = prefs.NewMemoryStore()
	} else {
		fs, err := prefs.NewFileStore(*stateDir)
		if err != nil {
			log.Fatalf("state directory: %v", err)
		}
		log.Printf("state directory: %s", *stateDir)
		store = fs
	}

	deps := builtin.HostDeps()
	hour, minute, err := smart.ParseRefreshAt(*smartAt)
	if err != nil {
		log.Fatal(err)
	}
	deps.Smart.RefreshHour, deps.Smart.RefreshMinute = hour, minute
	plugins := addons.NewDetector()
	plugins.Refresh()
	log.Printf("detected omv plugins: %v", slices.Sorted(maps.Keys(plugins.Plugins())))

	srv := &http.Server{
		Addr: *listen,
		Handler: server.New(server.Config{
			System:     deps.System,
			Static:     web.Handler(),
			OMV:        omv.NewClient(*omvURL, userAgent),
			Addons:     builtin.New(deps),
			Plugins:    plugins,
			Prefs:      store,
			CookiePath: *cookiePath,
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

	log.Printf("listening on http://%s (omv: %s)", *listen, *omvURL)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// checkLoopback은 루프백 IP가 아닌 수신 주소를 거부한다.
// 백엔드는 로컬 리버스 프록시를 통해서만 접근할 수 있어야 한다.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("listen address must be a loopback IP (e.g. 127.0.0.1:8090), got " + addr)
	}
	return nil
}

// checkOMVURL은 OMV 주소가 루프백 호스트의 http(s) URL인지 확인한다.
// OMV 세션 ID가 네트워크로 나가지 않게 한다.
func checkOMVURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("omv url must be http(s), got " + raw)
	}
	if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("omv url must point to a loopback address (e.g. http://127.0.0.1), got " + raw)
	}
	return nil
}
