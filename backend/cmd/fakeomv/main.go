// fakeomv는 개발용 가짜 OMV rpc.php 서버다(internal/omv/omvfake).
//
// `make dev-fake`가 백엔드를 이 서버에 연결해, 실제 OMV 계정 없이 로그인 화면과
// 세션 만료를 브라우저에서 확인할 수 있게 한다. 계정은 모두 가짜 값이며 운영
// 바이너리(bin/onmaru)에는 포함되지 않는다.
//
// / 에는 가짜 workbench 안내 페이지를 둔다. 대시보드의 "OMV 워크벤치" 링크 앱이 여는 탭을
// 확인하는 용도이며, OMV 웹 UI를 흉내 내지는 않는다.
package main

import (
	"cmp"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"onmaru/backend/internal/omv/omvfake"
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

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("ignoring invalid %s=%q", key, v)
	}
	return def
}
