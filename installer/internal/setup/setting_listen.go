package setup

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"onmaru/backend/config"
	"onmaru/installer/internal/prompt"
)

// settingListen은 onmaru의 수신 주소다. 리버스 프록시가 접속하는 방식(루프백 TCP, Unix 소켓, 모든 주소의 TCP)을
// 골라 정하고, 모든 주소의 TCP는 위험 동의를 받아 allow-non-loopback을 함께 정한다.
var settingListen = &Setting{
	Name:  nameListen,
	Flag:  FlagValue,
	Arg:   "ADDR",
	Usage: L(config.DefaultListen+", unix:"+socketPath+", or host:port", config.DefaultListen+", unix:"+socketPath+", 또는 host:port"),
	Env:   []EnvLine{{File: OnmaruEnv, Key: config.EnvListen}},
	// 루프백 외 주소 허용은 수신 주소와 짝이므로 수신 주소를 기존 설치에서 읽을 때만 함께 읽는다.
	Load: func(c *Context) (string, bool) {
		v := c.Installed.Env.Value(config.EnvListen)
		if v == "" {
			return "", false
		}
		if c.Installed.Env.Value(config.EnvAllowNonLoopback) == "true" {
			c.Values.SetBool(nameAllowNonLoopback, true)
		}
		return v, true
	},
	Ask:      askListen,
	Validate: validateListen,
	Summary: &SummarySpec{
		Label: L("listen", "수신"),
		Value: func(c *Context, v string) string {
			if c.Values.Bool(nameAllowNonLoopback) {
				return v + "  (" + c.L("non-loopback allowed", "루프백 외 주소 허용") + ")"
			}
			return v
		},
	},
}

// settingAllowNonLoopback은 루프백이 아닌 수신 주소를 허용하는지다. 질문은 listen이 함께 한다.
var settingAllowNonLoopback = &Setting{
	Name:  nameAllowNonLoopback,
	Flag:  FlagBool,
	Usage: L("allow a non-loopback --listen address (dangerous)", "루프백이 아닌 --listen 주소를 허용합니다(위험)"),
	Env: []EnvLine{{
		File:  OnmaruEnv,
		Key:   config.EnvAllowNonLoopback,
		Value: func(c *Context) string { return strconv.FormatBool(c.Values.Bool(nameAllowNonLoopback)) },
	}},
}

// validPort는 1-65535의 십진수 포트다(앞자리 0·부호 없음).
func validPort(p string) bool {
	if p == "" || p[0] < '1' || p[0] > '9' || strings.Trim(p, "0123456789") != "" {
		return false
	}
	n, err := strconv.Atoi(p)
	return err == nil && n <= 65535
}

// isLoopbackTCP는 루프백 IP:포트인지다. 백엔드 규칙(config.CheckListen)에 더해 포트는 숫자만 받는다.
func isLoopbackTCP(addr string) bool {
	if strings.HasPrefix(addr, config.UnixPrefix) || config.CheckListen(addr, false) != nil {
		return false
	}
	_, port, err := net.SplitHostPort(addr)
	return err == nil && validPort(port)
}

// isAnyTCP는 모든 주소 모드의 IP:포트인지다. 호스트는 IP만 받고(백엔드도 IP만 받는다) 비우면 모든 주소다.
func isAnyTCP(addr string) bool {
	if strings.HasPrefix(addr, config.UnixPrefix) || config.CheckListen(addr, true) != nil {
		return false
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || (host != "" && net.ParseIP(host) == nil) {
		return false
	}
	return validPort(port)
}

func validateListen(c *Context, v string) error {
	switch {
	case strings.HasPrefix(v, config.UnixPrefix):
		// 소켓 디렉터리는 systemd RuntimeDirectory=가 만들고 ProtectSystem=strict에서도 쓸 수 있는 곳이어야 한다.
		if v != config.UnixPrefix+socketPath {
			return errors.New(c.L("the Unix socket must be unix:"+socketPath+", got "+v,
				"Unix 소켓은 unix:"+socketPath+" 이어야 합니다: "+v))
		}
	case isLoopbackTCP(v):
	case !isAnyTCP(v):
		return errInvalidAddress(c, v)
	case !c.Values.Bool(nameAllowNonLoopback):
		return errors.New(c.L(v+" is not a loopback address; add --allow-non-loopback to accept the risk",
			v+" 은 루프백 주소가 아닙니다. 위험을 감수하려면 --allow-non-loopback을 추가하세요"))
	}
	return nil
}

func errInvalidAddress(c *Context, v string) error {
	return errors.New(c.L("invalid address: "+v+" (use IP:port, e.g. 0.0.0.0:8090)",
		"주소가 올바르지 않습니다: "+v+" (IP:포트 형식, 예: 0.0.0.0:8090)"))
}

// currentListen은 수신 방식 질문의 기본값이 될 지금 수신 주소다. "설정 바꾸기"면 지금 값, 기존 설치의 값이 잘못됐으면
// 없음(기본 루프백), 아니면 기존 설치의 값이다.
func currentListen(c *Context) string {
	if v, ok := c.prefill[nameListen]; ok {
		return v
	}
	if _, bad := c.invalid[nameListen]; bad {
		return ""
	}
	return c.Installed.Env.Value(config.EnvListen)
}

// 수신 방식 선택지
const (
	listenLoopback = "loopback"
	listenUnix     = "unix"
	listenAny      = "any"
)

// askListen은 리버스 프록시가 onmaru에 접속하는 방식을 고르게 하고 수신 주소를 정한다.
// 기본 선택은 기존 설치의 수신 주소를 따른다.
func askListen(c *Context) error {
	cur := currentListen(c)
	mode := listenLoopback
	switch {
	case strings.HasPrefix(cur, config.UnixPrefix):
		mode = listenUnix
	case cur != "" && !isLoopbackTCP(cur):
		mode = listenAny
	}
	choice, err := c.P.Select(prompt.Select{
		ID:    "listen-mode",
		Title: c.L("How should the reverse proxy reach onmaru?", "리버스 프록시가 onmaru에 어떻게 접속하나요?"),
		Options: []prompt.Option{
			{Value: listenLoopback,
				Label: c.L("TCP on loopback ("+config.DefaultListen+")", "루프백 TCP ("+config.DefaultListen+")"),
				Help:  c.L("The proxy runs on this host (not in a container).", "프록시가 이 호스트에서 직접 실행될 때(컨테이너가 아닐 때).")},
			{Value: listenUnix,
				Label: c.L("Unix socket ("+socketPath+")", "Unix 소켓 ("+socketPath+")"),
				Help: c.L("The proxy runs in a Docker container: mount "+runtimeDir+" into it.",
					"프록시가 Docker 컨테이너일 때: "+runtimeDir+" 을 컨테이너에 마운트합니다.")},
			{Value: listenAny,
				Label: c.L("TCP on any address (DANGEROUS)", "모든 주소의 TCP (위험)"),
				Help:  c.L("Anyone who can reach the address talks to onmaru directly.", "그 주소에 닿는 누구나 onmaru에 직접 접속할 수 있습니다.")},
		},
		Default: mode,
	})
	if err != nil {
		return err
	}
	listenTitle := c.L("Listen address", "수신 주소")
	switch choice {
	case listenUnix:
		c.Values.Set(nameListen, config.UnixPrefix+socketPath)
		c.Values.SetBool(nameAllowNonLoopback, false)
	case listenAny:
		def := "0.0.0.0:8090"
		if mode == listenAny {
			def = cur
		}
		v, err := c.P.Input(prompt.Input{ID: "listen", Title: listenTitle, Default: def, Validate: func(s string) error {
			if !isAnyTCP(s) {
				return errInvalidAddress(c, s)
			}
			return nil
		}})
		if err != nil {
			return err
		}
		ans, err := c.P.Input(prompt.Input{
			ID:    "listen-risk",
			Title: c.L("Type 'yes' to accept the risk", "위험을 감수하려면 'yes'를 입력하세요"),
			Help: c.L(`WARNING: listening on a non-loopback address.
- Clients that connect directly bypass the reverse proxy and HTTPS. Passwords travel in plain HTTP.
- X-Forwarded-For / X-Forwarded-Proto can be forged: the login rate limit can be bypassed
  and OMV auth logs (fail2ban) may record a fake IP.
Restrict access with a firewall if you choose this.`, `경고: 루프백이 아닌 주소에서 수신합니다.
- 직접 접속하는 클라이언트는 리버스 프록시와 HTTPS를 거치지 않습니다. 비밀번호가 평문 HTTP로 오갑니다.
- X-Forwarded-For / X-Forwarded-Proto를 위조할 수 있습니다. 로그인 시도 제한을 우회할 수 있고
  OMV 인증 로그(fail2ban)에 가짜 IP가 남을 수 있습니다.
이 방식을 쓴다면 방화벽으로 접근을 제한하세요.`),
		})
		if err != nil {
			return err
		}
		if ans != "yes" {
			return fail(c.L("aborted", "중단했습니다"))
		}
		c.Values.Set(nameListen, v)
		c.Values.SetBool(nameAllowNonLoopback, true)
	default:
		def := config.DefaultListen
		if isLoopbackTCP(cur) {
			def = cur
		}
		v, err := c.P.Input(prompt.Input{ID: "listen", Title: listenTitle, Default: def, Validate: func(s string) error {
			if !isLoopbackTCP(s) {
				return errors.New(c.L("not a loopback address: "+s+" (use option 3 for other addresses)",
					"루프백 주소가 아닙니다: "+s+" (다른 주소는 3번을 고르세요)"))
			}
			return nil
		}})
		if err != nil {
			return err
		}
		c.Values.Set(nameListen, v)
		c.Values.SetBool(nameAllowNonLoopback, false)
	}
	return nil
}
