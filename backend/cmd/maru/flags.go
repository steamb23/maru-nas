package main

import (
	"cmp"
	"flag"
	"log"
	"os"
	"strconv"

	"maru/backend/config"
	"maru/backend/internal/api"
)

// options는 명령줄 플래그(기본값은 환경 변수)로 정한 가동 설정이다.
type options struct {
	listen           string
	allowNonLoopback bool
	omvURL           string
	cookiePath       string
	stateDir         string
	workbenchURL     string
	smartAt          string
	helperSocket     string
	terminalEnabled  bool
	sshAddr          string
	sshHostKeys      string
	publicOrigin     string
}

// parseFlags는 플래그를 정의하고 해석한다. 플래그를 더하면 options.runtime의 목록에도 넣는다.
func parseFlags() options {
	var o options
	flag.StringVar(&o.listen, "listen", cmp.Or(os.Getenv(config.EnvListen), config.DefaultListen),
		"수신 주소. host:port(기본은 루프백만 허용) 또는 unix:/절대/경로.sock (환경 변수 MARU_LISTEN)")
	flag.BoolVar(&o.allowNonLoopback, "allow-non-loopback", envBool(config.EnvAllowNonLoopback),
		"루프백이 아닌 TCP 주소(0.0.0.0 등)에서도 수신한다. 위험: 리버스 프록시를 거치지 않은 접속을 받는다 (환경 변수 MARU_ALLOW_NON_LOOPBACK)")
	flag.StringVar(&o.omvURL, "omv-url", cmp.Or(os.Getenv(config.EnvOMVURL), config.DefaultOMVURL),
		"OMV 웹 UI 주소 (루프백만 허용, 환경 변수 MARU_OMV_URL)")
	flag.StringVar(&o.cookiePath, "cookie-path", cmp.Or(os.Getenv(config.EnvCookiePath), config.DefaultCookiePath),
		"브라우저가 보는 대시보드 경로 (세션 쿠키 Path, 환경 변수 MARU_COOKIE_PATH)")
	// systemd의 StateDirectory=를 쓰면 STATE_DIRECTORY가 설정된다.
	flag.StringVar(&o.stateDir, "state-dir", cmp.Or(os.Getenv(config.EnvStateDir), os.Getenv(config.EnvSystemdStateDir)),
		"사용자 설정(위젯 배치 등)을 저장할 디렉터리. 비우면 메모리에만 둔다 (환경 변수 MARU_STATE_DIR, STATE_DIRECTORY)")
	flag.StringVar(&o.workbenchURL, "workbench-url", os.Getenv(config.EnvWorkbenchURL),
		"브라우저에서 OMV 웹 UI(workbench)를 열 주소. http(s) URL 또는 대시보드 기준 경로(/, ../). 비우면 OMV 워크벤치 앱을 숨긴다 (환경 변수 MARU_WORKBENCH_URL)")
	flag.StringVar(&o.smartAt, "smart-refresh-at", cmp.Or(os.Getenv(config.EnvSmartRefreshAt), config.DefaultSmartRefreshAt),
		"SMART 정보를 하루 한 번 다시 가져오는 기준 시각 HH:MM, 서버 로컬 시간 (환경 변수 MARU_SMART_REFRESH_AT)")
	flag.StringVar(&o.helperSocket, "helper-socket", os.Getenv(config.EnvHelperSocket),
		"권한 도우미(maru-helper) 소켓의 절대 경로. 비우면 도우미 기능을 끈다 (환경 변수 MARU_HELPER_SOCKET)")
	flag.BoolVar(&o.terminalEnabled, "terminal", envBool(config.EnvTerminal),
		"웹 터미널(같은 호스트의 sshd에 SSH로 접속)을 허용한다. 설정 앱에서 덮어쓸 수 있다 (환경 변수 MARU_TERMINAL)")
	flag.StringVar(&o.sshAddr, "ssh-addr", cmp.Or(os.Getenv(config.EnvSSHAddr), config.DefaultSSHAddr),
		"웹 터미널이 접속할 sshd 주소. auto는 /etc/ssh/sshd_config의 Port로 127.0.0.1에 접속한다. 루프백 IP:포트만 허용 (환경 변수 MARU_SSH_ADDR)")
	flag.StringVar(&o.sshHostKeys, "ssh-host-keys", cmp.Or(os.Getenv(config.EnvSSHHostKeys), config.DefaultSSHHostKeys),
		"웹 터미널이 받을 sshd 호스트 공개 키 파일의 glob 패턴 (환경 변수 MARU_SSH_HOST_KEYS)")
	flag.StringVar(&o.publicOrigin, "public-origin", os.Getenv(config.EnvPublicOrigin),
		"브라우저가 대시보드에 접속하는 공개 HTTPS 주소(https://호스트[:포트], 도메인 이름). 보안 강화 모드의 패스키에 쓴다. 비우면 패스키를 끈다 (환경 변수 MARU_PUBLIC_ORIGIN)")
	flag.Parse()
	return o
}

// runtime은 설정의 서버 페이지에 보여 줄 가동 설정 현황(값·출처)이다. flag.Parse 뒤에 부른다.
func (o options) runtime() []api.RuntimeSetting {
	return runtimeSettings(explicitFlags(), os.LookupEnv, []runtimeEntry{
		{flag: "listen", env: config.EnvListen, value: o.listen},
		{flag: "allow-non-loopback", env: config.EnvAllowNonLoopback, value: strconv.FormatBool(o.allowNonLoopback)},
		{flag: "omv-url", env: config.EnvOMVURL, value: o.omvURL},
		{flag: "cookie-path", env: config.EnvCookiePath, value: o.cookiePath},
		{flag: "state-dir", env: config.EnvStateDir, value: o.stateDir, systemdEnv: config.EnvSystemdStateDir},
		// 빈 값은 "끔"이라는 뜻으로 일부러 넣는 값이다(설치기, installer/).
		{flag: "workbench-url", env: config.EnvWorkbenchURL, value: o.workbenchURL, emptyIsSet: true},
		{flag: "smart-refresh-at", env: config.EnvSmartRefreshAt, value: o.smartAt},
		{flag: "helper-socket", env: config.EnvHelperSocket, value: o.helperSocket},
		{flag: "terminal", env: config.EnvTerminal, value: strconv.FormatBool(o.terminalEnabled)},
		{flag: "ssh-addr", env: config.EnvSSHAddr, value: o.sshAddr},
		{flag: "ssh-host-keys", env: config.EnvSSHHostKeys, value: o.sshHostKeys},
		// 빈 값은 "패스키 끔"이라는 뜻으로 일부러 넣는 값이다(설치기, installer/).
		{flag: "public-origin", env: config.EnvPublicOrigin, value: o.publicOrigin, emptyIsSet: true},
	})
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

// runtimeSettings는 가동 설정 값마다 출처를 정한다. 명령줄 플래그가 환경 변수보다 우선한다(parseFlags의 플래그 기본값이
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
