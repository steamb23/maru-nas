package setup

// 설치 경로. 바꾸면 README.md의 설치 설명, 리버스 프록시 예시, 유닛 템플릿(templates/)도 함께 본다.
const (
	serviceName = "onmaru"
	binDst      = "/usr/local/bin/onmaru"
	unitFile    = "/etc/systemd/system/onmaru.service"
	envFile     = "/etc/default/onmaru"
	stateDir    = "/var/lib/onmaru"
	runtimeDir  = "/run/onmaru"
	// socketPath는 Unix 소켓 수신 주소다. 소켓 디렉터리는 systemd RuntimeDirectory=가 만들고
	// ProtectSystem=strict에서도 쓸 수 있는 곳이어야 하므로 이 경로만 받는다.
	socketPath  = runtimeDir + "/onmaru.sock"
	nginxWebGUI = "/etc/nginx/sites-enabled/openmediavault-webgui"

	helperName        = "onmaru-helper"
	helperBinDst      = "/usr/local/bin/onmaru-helper"
	helperSocketUnit  = "/etc/systemd/system/onmaru-helper.socket"
	helperServiceUnit = "/etc/systemd/system/onmaru-helper.service"
	helperEnvFile     = "/etc/default/onmaru-helper"
	// helperSocket은 도우미 소켓이다. Docker 프록시에 바인드 마운트하는 runtimeDir과 다른 디렉터리에 둔다.
	helperSocket = "/run/onmaru-helper/helper.sock"
	chkpwdPath   = "/usr/sbin/unix_chkpwd"
)

// 저장소 안의 경로(저장소 루트 기준).
const (
	binSrcRel       = "bin/onmaru"
	helperBinSrcRel = "bin/onmaru-helper"
	distIndexRel    = "backend/internal/web/static/dist/index.html"
	iconsRel        = "vendor/icons/colloid"
)
