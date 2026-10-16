// onmaru는 온마루(Onmaru) 대시보드의 백엔드다.
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
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/addons/builtin"
	"onmaru/backend/internal/addons/smart"
	"onmaru/backend/internal/api"
	"onmaru/backend/internal/buildinfo"
	"onmaru/backend/internal/helper/client"
	"onmaru/backend/internal/omv"
	"onmaru/backend/internal/prefs"
	"onmaru/backend/internal/server"
	"onmaru/backend/internal/web"
)

// userAgent는 OMV를 호출할 때 쓰는 고정 User-Agent다. OMV는 세션을 User-Agent에 묶는다.
const userAgent = "onmaru/" + buildinfo.Version

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv("ONMARU_LISTEN"), "127.0.0.1:8090"),
		"수신 주소. host:port(기본은 루프백만 허용) 또는 unix:/절대/경로.sock (환경 변수 ONMARU_LISTEN)")
	allowNonLoopback := flag.Bool("allow-non-loopback", envBool("ONMARU_ALLOW_NON_LOOPBACK"),
		"루프백이 아닌 TCP 주소(0.0.0.0 등)에서도 수신한다. 위험: 리버스 프록시를 거치지 않은 접속을 받는다 (환경 변수 ONMARU_ALLOW_NON_LOOPBACK)")
	omvURL := flag.String("omv-url", cmp.Or(os.Getenv("ONMARU_OMV_URL"), "http://127.0.0.1"),
		"OMV 웹 UI 주소 (루프백만 허용, 환경 변수 ONMARU_OMV_URL)")
	cookiePath := flag.String("cookie-path", cmp.Or(os.Getenv("ONMARU_COOKIE_PATH"), "/"),
		"브라우저가 보는 대시보드 경로 (세션 쿠키 Path, 환경 변수 ONMARU_COOKIE_PATH)")
	// systemd의 StateDirectory=를 쓰면 STATE_DIRECTORY가 설정된다.
	stateDir := flag.String("state-dir", cmp.Or(os.Getenv("ONMARU_STATE_DIR"), os.Getenv("STATE_DIRECTORY")),
		"사용자 설정(위젯 배치 등)을 저장할 디렉터리. 비우면 메모리에만 둔다 (환경 변수 ONMARU_STATE_DIR, STATE_DIRECTORY)")
	workbenchURL := flag.String("workbench-url", os.Getenv("ONMARU_WORKBENCH_URL"),
		"브라우저에서 OMV 웹 UI(workbench)를 열 주소. http(s) URL 또는 대시보드 기준 경로(/, ../). 비우면 OMV 워크벤치 앱을 숨긴다 (환경 변수 ONMARU_WORKBENCH_URL)")
	smartAt := flag.String("smart-refresh-at", cmp.Or(os.Getenv("ONMARU_SMART_REFRESH_AT"), "00:00"),
		"SMART 정보를 하루 한 번 다시 가져오는 기준 시각 HH:MM, 서버 로컬 시간 (환경 변수 ONMARU_SMART_REFRESH_AT)")
	helperSocket := flag.String("helper-socket", os.Getenv("ONMARU_HELPER_SOCKET"),
		"권한 도우미(onmaru-helper) 소켓의 절대 경로. 비우면 도우미 기능을 끈다 (환경 변수 ONMARU_HELPER_SOCKET)")
	flag.Parse()

	network, address, err := parseListen(*listen, *allowNonLoopback)
	if err != nil {
		log.Fatal(err)
	}
	if _, _, err := parseListen(*listen, false); err != nil {
		log.Printf("WARNING: listening on non-loopback address %s. Requests can bypass the reverse proxy: "+
			"traffic is plain HTTP and X-Forwarded-For/X-Forwarded-Proto can be forged by any client "+
			"(login rate limit, OMV auth log, cookie Secure flag)", address)
	}
	if err := checkOMVURL(*omvURL); err != nil {
		log.Fatal(err)
	}
	if err := server.CheckWorkbenchURL(*workbenchURL); err != nil {
		log.Fatal(err)
	}
	if err := checkHelperSocket(*helperSocket); err != nil {
		log.Fatal(err)
	}

	// store는 사용자 설정, 전역 설정, 배경 이미지를 함께 저장한다.
	var store interface {
		prefs.Store
		prefs.SiteStore
		prefs.ImageStore
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
	hour, minute, err := smart.ParseRefreshAt(*smartAt)
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
		{flag: "listen", env: "ONMARU_LISTEN", value: *listen},
		{flag: "allow-non-loopback", env: "ONMARU_ALLOW_NON_LOOPBACK", value: strconv.FormatBool(*allowNonLoopback)},
		{flag: "omv-url", env: "ONMARU_OMV_URL", value: *omvURL},
		{flag: "cookie-path", env: "ONMARU_COOKIE_PATH", value: *cookiePath},
		{flag: "state-dir", env: "ONMARU_STATE_DIR", value: *stateDir, systemdEnv: "STATE_DIRECTORY"},
		// 빈 값은 "끔"이라는 뜻으로 일부러 넣는 값이다(install.sh).
		{flag: "workbench-url", env: "ONMARU_WORKBENCH_URL", value: *workbenchURL, emptyIsSet: true},
		{flag: "smart-refresh-at", env: "ONMARU_SMART_REFRESH_AT", value: *smartAt},
		{flag: "helper-socket", env: "ONMARU_HELPER_SOCKET", value: *helperSocket},
	})

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

// unixPrefix는 -listen 값에서 Unix 소켓 경로를 나타내는 접두사다.
const unixPrefix = "unix:"

// parseListen은 -listen 값을 net.Listen 인자로 바꾼다.
// 백엔드는 로컬 리버스 프록시를 통해서만 접근하는 것이 기본이므로, allowNonLoopback이 아니면
// 루프백 IP가 아닌 TCP 주소를 거부한다. Unix 소켓은 파일 권한으로 접근을 제한한다.
func parseListen(addr string, allowNonLoopback bool) (network, address string, err error) {
	if path, ok := strings.CutPrefix(addr, unixPrefix); ok {
		if !filepath.IsAbs(path) {
			return "", "", errors.New("unix socket path must be absolute (e.g. unix:/run/onmaru/onmaru.sock), got " + addr)
		}
		return "unix", path, nil
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", err
	}
	if !allowNonLoopback {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return "", "", errors.New("listen address must be a loopback IP (e.g. 127.0.0.1:8090) or unix:/path, got " +
				addr + " (use -allow-non-loopback to listen on other addresses)")
		}
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

// checkHelperSocket은 도우미 소켓 경로가 비었거나 절대 경로인지 확인한다.
func checkHelperSocket(path string) error {
	if path != "" && (!filepath.IsAbs(path) || filepath.Clean(path) != path) {
		return errors.New("helper socket must be an absolute path (e.g. /run/onmaru-helper/helper.sock), got " + path)
	}
	return nil
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
