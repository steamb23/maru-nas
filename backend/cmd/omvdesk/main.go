// omvdesk는 OMV 데스크톱 대시보드의 백엔드다.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"omv-desktop-dashboard/backend/internal/server"
	"omv-desktop-dashboard/backend/internal/sysinfo"
	"omv-desktop-dashboard/backend/internal/web"
)

func main() {
	listen := flag.String("listen", cmp.Or(os.Getenv("OMVDESK_LISTEN"), "127.0.0.1:8090"),
		"수신 주소 (루프백만 허용, 환경 변수 OMVDESK_LISTEN)")
	flag.Parse()

	if err := checkLoopback(*listen); err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              *listen,
		Handler:           server.New(sysinfo.NewReader(), web.Handler()),
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

	log.Printf("listening on http://%s", *listen)
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
