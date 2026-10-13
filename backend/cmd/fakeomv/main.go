// fakeomv는 개발용 가짜 OMV rpc.php 서버다(internal/omv/omvfake).
//
// `make dev-fake`가 백엔드를 이 서버에 연결해, 실제 OMV 계정 없이 로그인 화면과
// 세션 만료를 브라우저에서 확인할 수 있게 한다. 계정은 모두 가짜 값이며 운영
// 바이너리(bin/omvdesk)에는 포함되지 않는다.
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

	"omv-desktop-dashboard/backend/internal/omv/omvfake"
)

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv("FAKEOMV_LISTEN"), "127.0.0.1:8091"),
		"수신 주소 (루프백만 허용, 환경 변수 FAKEOMV_LISTEN)")
	timeout := flag.Duration("timeout", envDuration("FAKEOMV_TIMEOUT", 5*time.Minute),
		"세션 timeout, 분 단위, 0이면 만료 없음 (환경 변수 FAKEOMV_TIMEOUT, 예: 2m)")
	flag.Parse()

	if *timeout < 0 || *timeout%time.Minute != 0 {
		log.Fatalf("timeout must be whole minutes like OMV's setting, got %v", *timeout)
	}

	host, _, err := net.SplitHostPort(*listen)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		log.Fatalf("listen address must be a loopback IP, got %s", *listen)
	}

	srv := omvfake.New(omvfake.DefaultUsers(), *timeout)
	log.Printf("fake omv listening on http://%s (session timeout %v)", *listen, *timeout)
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
	hs := &http.Server{Addr: *listen, Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	if err := hs.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
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
