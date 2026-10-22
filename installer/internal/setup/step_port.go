package setup

import (
	"net"
	"strings"

	"maru/backend/config"
	"maru/installer/internal/prompt"
)

// stepPort는 TCP 수신이면 다른 프로세스(예: make dev의 백엔드)가 그 주소를 쓰고 있는지 미리 알린다.
// 서비스가 이미 돌고 있다면 그 포트는 자기 것이므로 건너뛴다.
func stepPort(c *Context) error {
	listen := c.Values.Get(nameListen)
	if strings.HasPrefix(listen, config.UnixPrefix) {
		return nil
	}
	if c.Sys.Check("systemctl", "is-active", "--quiet", serviceName+".service") == nil {
		return nil
	}
	if !c.Sys.PortInUse(listen) {
		return nil
	}
	_, port, _ := net.SplitHostPort(listen)
	c.Warn(L("port "+port+" is already in use by another process (for example a 'make dev' backend). The service will fail to start until it is free.",
		"포트 "+port+"을(를) 다른 프로세스가 쓰고 있습니다(예: 'make dev' 백엔드). 비워질 때까지 서비스가 시작되지 않습니다."))
	// 업그레이드는 묻지 않고 알리기만 한다. 포트가 비면 systemd가 다시 시작한다(Restart=on-failure).
	if c.upgrading() {
		return nil
	}
	ok, err := c.confirm(prompt.Confirm{ID: "port-continue", Title: c.L("Continue anyway?", "그래도 계속할까요?")})
	if err != nil {
		return err
	}
	if !ok {
		return quit(1)
	}
	return nil
}
