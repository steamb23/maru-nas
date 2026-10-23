// fakeomv는 개발용 가짜 OMV rpc.php 서버다(internal/omv/omvfake).
//
// `make dev-fake`가 백엔드를 이 서버에 연결해, 실제 OMV 계정 없이 로그인 화면과
// 세션 만료를 브라우저에서 확인할 수 있게 한다. 계정은 모두 가짜 값이며 운영
// 바이너리(bin/maru)에는 포함되지 않는다.
//
// / 에는 가짜 workbench 안내 페이지를 둔다. 대시보드의 "OMV 워크벤치" 링크 앱이 여는 탭을
// 확인하는 용도이며, OMV 웹 UI를 흉내 내지는 않는다.
//
// FAKEHELPER_SOCKET이 있으면 같은 가짜 계정으로 가짜 권한 도우미(internal/helper/helperfake)도
// 그 소켓에 띄운다. 권한을 바꾸지 않으며, 파일 연산은 FAKEHELPER_ROOT(기본: 소켓 옆 helper-root)
// 아래의 가짜 홈과 디스크에서만 한다.
//
// FAKESSH_LISTEN이 있으면 웹 터미널용 가짜 sshd(internal/terminal/sshfake)도 띄운다. 같은 가짜 계정으로
// 인증하고(공개 키는 가짜 도우미의 가짜 홈에 쓴 authorized_keys로 확인), 장난감 셸만 있다. 호스트 공개 키는
// FAKESSH_HOST_KEY 경로에 쓰고, FAKESSH_AUTH로 받을 인증 방법을 고른다(예: publickey면 공개 키만 받는 sshd).
//
// 가짜 도우미가 있으면 파일 탐색기용 합성 파일과 공유 폴더를 만든다(files.go, FAKEFILES_LARGE로 큰 폴더).
//
// 가짜 도우미가 있고 MARU_DEV_UPGRADE_ROOT·MARU_STATE_DIR가 있으면 대시보드에서 업그레이드의 가짜 업그레이드
// 서비스(internal/upgrade/upgradefake)도 띄운다. maru가 그 상태 디렉터리에 남긴 확인 요청과 가짜 도우미가 쓴
// 적용 요청을 처리해 가짜 루트의 상태 파일에 쓴다. FAKEUPGRADE_SCENARIO로 시나리오(web, manual, latest, check-fail,
// apply-fail, disabled)를, FAKEUPGRADE_STEP으로 단계 사이 지연(기본 1.5s)을 고른다.
//
// MARU_DEV_NOTICES가 있으면 maru의 개발용 알림 출처가 읽는 가짜 알림 파일을 그 경로에 쓴다(notices.go).
// FAKENOTICES로 시나리오(sample, none, many, rotate)를, FAKENOTICES_STEP으로 rotate 주기(기본 30s)를 고른다.
package main

import (
	"cmp"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"maru/backend/config"
	"maru/backend/internal/helper/helperfake"
	"maru/backend/internal/omv/omvfake"
	"maru/backend/internal/terminal/sshfake"
	"maru/backend/internal/upgrade/upgradefake"
)

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv("FAKEOMV_LISTEN"), "127.0.0.1:8091"),
		"수신 주소 (루프백만 허용, 환경 변수 FAKEOMV_LISTEN)")
	timeout := flag.Duration("timeout", envDuration("FAKEOMV_TIMEOUT", 5*time.Minute),
		"세션 timeout, 분 단위, 0이면 만료 없음 (환경 변수 FAKEOMV_TIMEOUT, 예: 2m)")
	omvVersion := flag.String("omv-version", cmp.Or(os.Getenv("FAKEOMV_VERSION"), "8.5"),
		"흉내 낼 OMV 버전, 8.0 ~ 8.4는 옛 로그인 응답 (환경 변수 FAKEOMV_VERSION, 예: 8.3)")
	flag.Parse()

	if *timeout < 0 || *timeout%time.Minute != 0 {
		log.Fatalf("timeout must be whole minutes like OMV's setting, got %v", *timeout)
	}
	version, err := omvfake.ParseVersion(*omvVersion)
	if err != nil {
		log.Fatal(err)
	}

	host, _, err := net.SplitHostPort(*listen)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		log.Fatalf("listen address must be a loopback IP, got %s", *listen)
	}

	srv := omvfake.New(omvfake.DefaultUsers(), *timeout)
	srv.SetVersion(version)
	log.Printf("fake omv listening on http://%s (OMV %s, session timeout %v)", *listen, version, *timeout)
	for _, u := range omvfake.DefaultUsers() {
		role := "user"
		if u.Admin {
			role = "admin"
		}
		if u.TOTP != "" {
			log.Printf("  account %s / %s (%s, 2FA code %s)", u.Name, u.Password, role, u.TOTP)
		} else {
			log.Printf("  account %s / %s (%s)", u.Name, u.Password, role)
		}
	}
	var helper *helperfake.Server
	if sock := os.Getenv("FAKEHELPER_SOCKET"); sock != "" {
		root := cmp.Or(os.Getenv("FAKEHELPER_ROOT"), filepath.Join(filepath.Dir(sock), "helper-root"))
		h, err := helperfake.Start(sock, root, helperfake.DefaultUsers(), helperfake.Options{})
		if err != nil {
			log.Fatalf("fake helper: %v", err)
		}
		defer h.Close()
		helper = h
		log.Printf("fake helper listening on %s (files under %s)", sock, root)
		// 파일 탐색기: 가짜 디스크에 합성 파일을 만들고 가짜 OMV의 공유 폴더가 그 아래를 가리키게 한다.
		seedFiles(h.Disk())
		d := omvfake.DefaultPluginData()
		d.SharedFolders = devShares(h.Disk())
		srv.SetPluginData(d)
		for _, u := range helperfake.DefaultUsers() {
			log.Printf("  helper account %s uid %d groups %v", u.Name, u.UID, u.Groups)
		}
	}
	if addr := os.Getenv("FAKESSH_LISTEN"); addr != "" {
		startFakeSSH(addr, helper)
	}
	if root, state := os.Getenv("MARU_DEV_UPGRADE_ROOT"), os.Getenv("MARU_STATE_DIR"); helper != nil && root != "" && state != "" {
		u, err := upgradefake.Start(upgradefake.Options{
			Root:         root,
			CheckRequest: filepath.Join(state, config.UpgradeCheckRequestName),
			ApplyDir:     helper.UpgradeDir(),
			Scenario:     os.Getenv("FAKEUPGRADE_SCENARIO"),
			Step:         envDuration("FAKEUPGRADE_STEP", 1500*time.Millisecond),
			Logf:         log.Printf,
		})
		if err != nil {
			log.Fatalf("fake upgrade service: %v", err)
		}
		defer u.Close()
		log.Printf("fake upgrade service under %s (scenario %s)", root, cmp.Or(os.Getenv("FAKEUPGRADE_SCENARIO"), upgradefake.ScenarioWeb))
	}
	if path := os.Getenv("MARU_DEV_NOTICES"); path != "" {
		scenario := cmp.Or(os.Getenv("FAKENOTICES"), noticesSample)
		if err := startNotices(path, scenario, envDuration("FAKENOTICES_STEP", 30*time.Second)); err != nil {
			log.Fatalf("fake notices: %v", err)
		}
		log.Printf("fake notices at %s (scenario %s)", path, scenario)
	}
	mux := http.NewServeMux()
	mux.Handle("/rpc.php", srv)
	mux.HandleFunc("GET /{$}", workbenchPage)
	mux.HandleFunc("GET /index.html", workbenchPage)
	hs := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// workbenchPage는 가짜 workbench 안내 페이지다. OMV처럼 다른 출처의 iframe에 들어가지 않게 한다.
// OMV는 hash 라우터라 경로는 #/ 아래에 있으므로, 지금 hash를 보여 줘 탭을 다시 열었을 때의 이동을 확인한다.
func workbenchPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(workbenchHTML))
}

const workbenchHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>Fake OMV Workbench (dev)</title>
<style>body{font:16px system-ui,sans-serif;margin:3rem;color:#222}code{background:#eee;padding:.1rem .3rem}</style>
</head>
<body>
<h1>Fake OMV Workbench (dev)</h1>
<p>Served by <code>cmd/fakeomv</code> for <code>make dev-fake</code>. This is not the OMV web UI.</p>
<p>Route: <code id="route"></code> · loaded at <code id="loaded"></code></p>
<script>
const show = () => { document.getElementById('route').textContent = location.hash || '(none)' }
document.getElementById('loaded').textContent = new Date().toLocaleTimeString()
addEventListener('hashchange', show)
show()
</script>
</body>
</html>
`

// startFakeSSH는 가짜 sshd를 addr에 띄우고 호스트 공개 키를 FAKESSH_HOST_KEY에 쓴다.
func startFakeSSH(addr string, helper *helperfake.Server) {
	host, _, err := net.SplitHostPort(addr)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		log.Fatalf("FAKESSH_LISTEN must be a loopback IP, got %s", addr)
	}
	opts := sshfake.Options{Users: sshfake.DefaultUsers(), Logf: log.Printf}
	if helper != nil {
		opts.Home = helper.Home
	}
	if err := sshfake.ParseMethods(os.Getenv("FAKESSH_AUTH"), &opts); err != nil {
		log.Fatal(err)
	}
	s, err := sshfake.Start(addr, opts)
	if err != nil {
		log.Fatalf("fake sshd: %v", err)
	}
	if path := os.Getenv("FAKESSH_HOST_KEY"); path != "" {
		if err := s.WriteHostKey(path); err != nil {
			log.Fatalf("fake sshd host key: %v", err)
		}
	}
	log.Printf("fake sshd listening on %s (publickey %t, password %t, keyboard-interactive %t)",
		s.Addr, !opts.NoPublicKey, !opts.NoPassword, !opts.NoKeyboardInteractive)
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("ignoring invalid %s=%q", key, v)
	}
	return def
}
