package setup

import (
	"net"
	"strings"
	"time"

	"maru/backend/config"
)

func stepDaemonReload(c *Context) error { return c.Sys.Run("systemctl", "daemon-reload") }

// stepStart는 maru를 켜고, 돌고 있으면 다시 시작한다.
func stepStart(c *Context) error {
	unit := serviceName + ".service"
	if err := c.Sys.RunQuiet("systemctl", "enable", unit); err != nil {
		return err
	}
	if c.Sys.Check("systemctl", "is-active", "--quiet", unit) == nil {
		c.Info(L("Restarting "+serviceName, serviceName+" 재시작"))
		return c.Sys.Run("systemctl", "restart", unit)
	}
	c.Info(L("Starting "+serviceName, serviceName+" 시작"))
	return c.Sys.Run("systemctl", "start", unit)
}

// stepHealth는 시작한 뒤 상태를 본다. Type=simple이라 start는 곧바로 죽는 프로세스에도 성공하므로 잠시 뒤
// 실제 상태를 보고, /api/health를 몇 번 확인한다. 실패해도 설치는 끝낸다(경고만).
func stepHealth(c *Context) error {
	listen := c.Values.Get(nameListen)
	if c.Sys.DryRun() {
		return c.Sys.HealthCheck(listen)
	}
	c.Sys.Sleep(2 * time.Second)
	if c.Sys.Check("systemctl", "is-active", "--quiet", serviceName+".service") != nil {
		c.Warn(L(serviceName+" is not running. See: systemctl status "+serviceName+"; journalctl -u "+serviceName+" -e",
			serviceName+" 이(가) 실행 중이 아닙니다. 확인: systemctl status "+serviceName+"; journalctl -u "+serviceName+" -e"))
		return nil
	}
	for i := range 5 {
		if c.Sys.HealthCheck(listen) == nil {
			c.Info(L("Health check OK", "상태 확인 성공"))
			return nil
		}
		if i < 4 {
			c.Sys.Sleep(time.Second)
		}
	}
	c.Warn(L("health check failed. See: journalctl -u "+serviceName+" -e", "상태 확인 실패. 로그: journalctl -u "+serviceName+" -e"))
	return nil
}

// stepNextSteps는 끝난 뒤의 안내다. 리버스 프록시 설정이 그대로 맞으면(수신 주소·서비스 사용자를 바꾸지 않은
// 업그레이드) 로그 위치만 알린다.
func stepNextSteps(c *Context) error {
	c.Print("")
	c.Info(L("Done. Logs: journalctl -u "+serviceName+" -f", "완료. 로그: journalctl -u "+serviceName+" -f"))
	if c.proxyUnchanged {
		return nil
	}
	c.Print(c.L("Next: point your reverse proxy (HTTPS) at maru. See docs/reverse-proxy.md.",
		"다음 단계: 리버스 프록시(HTTPS)가 maru를 가리키게 설정하세요. docs/reverse-proxy.md를 참고하세요."))
	listen := c.Values.Get(nameListen)
	if path, ok := strings.CutPrefix(listen, config.UnixPrefix); ok {
		c.Print("  - Caddy:  reverse_proxy unix/" + path)
		c.Print("  - " + c.L("Proxy in Docker: bind-mount "+runtimeDir+":"+runtimeDir+" into the container.",
			"Docker 안의 프록시: 컨테이너에 "+runtimeDir+":"+runtimeDir+" 을 바인드 마운트하세요."))
		c.Print("  - " + c.L("Proxy on this host: its user must be in the '"+c.group+"' group (socket mode 0660).",
			"호스트의 프록시: 프록시 사용자가 '"+c.group+"' 그룹에 있어야 합니다(소켓 권한 0660)."))
		return nil
	}
	// 모든 주소에서 수신하면 프록시에는 실제로 닿는 주소를 적어야 한다.
	host, port, _ := net.SplitHostPort(listen)
	addr := net.JoinHostPort(host, port)
	switch host {
	case "", "0.0.0.0", "::":
		addr = "<" + c.L("this-host-IP", "이-호스트-IP") + ">:" + port
	}
	c.Print("  - Caddy:  reverse_proxy http://" + addr)
	return nil
}
