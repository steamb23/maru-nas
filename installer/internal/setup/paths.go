package setup

import "onmaru/backend/config"

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

	// 대시보드에서 업그레이드(--web-upgrade). root 쪽 설정 파일·상태 파일·요청 경로는 backend/config에 있다.
	upgradeName        = "onmaru-upgrade"
	upgradeServiceUnit = "/etc/systemd/system/onmaru-upgrade.service"
	upgradePathUnit    = "/etc/systemd/system/onmaru-upgrade.path"
	// upgradeInstallerDst는 업그레이드 서비스가 실행하는 root 소유 설치기 복사본이다(저장소 안의 실행 파일을 root로
	// 실행하지 않게). 설치할 때마다 지금 설치기로 바꾼다.
	upgradeInstallerDir = "/usr/local/lib/onmaru"
	upgradeInstallerDst = upgradeInstallerDir + "/onmaru-install"
	// upgradeCheckRequest는 onmaru가 자기 상태 디렉터리(StateDirectory=onmaru)에 남기는 확인 요청이다.
	upgradeCheckRequest = stateDir + "/" + config.UpgradeCheckRequestName
	// upgradeApplyRequest는 도우미의 upgrade.apply가 쓰는 적용 요청이다. 그 디렉터리는 도우미 유닛의 RuntimeDirectory=다.
	upgradeApplyRequest   = config.UpgradeRequestDir + "/" + config.UpgradeApplyRequestName
	upgradeRequestDirName = "onmaru-upgrade"
)

// 저장소 안의 경로(저장소 루트 기준).
const (
	binSrcRel       = "bin/onmaru"
	helperBinSrcRel = "bin/onmaru-helper"
	// installerBinSrcRel은 설치기 자신이다(install.sh가 빌드한다). 대시보드에서 업그레이드를 켜면 root 소유로 복사한다.
	installerBinSrcRel = "bin/onmaru-install"
	distIndexRel       = "backend/internal/web/static/dist/index.html"
	iconsRel           = "vendor/icons/colloid"
)
