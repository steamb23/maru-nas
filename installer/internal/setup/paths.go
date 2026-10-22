package setup

import "maru/backend/config"

// 설치 경로. 바꾸면 README.md의 빠른 시작, docs/install.md·docs/reverse-proxy.md, 유닛 템플릿(templates/)도 함께 본다.
const (
	serviceName = "maru"
	binDst      = "/usr/local/bin/maru"
	unitFile    = "/etc/systemd/system/maru.service"
	envFile     = "/etc/default/maru"
	stateDir    = config.InstalledStateDir
	runtimeDir  = "/run/maru"
	// socketPath는 Unix 소켓 수신 주소다. 소켓 디렉터리는 systemd RuntimeDirectory=가 만들고
	// ProtectSystem=strict에서도 쓸 수 있는 곳이어야 하므로 이 경로만 받는다.
	socketPath  = runtimeDir + "/maru.sock"
	nginxWebGUI = "/etc/nginx/sites-enabled/openmediavault-webgui"

	helperName        = "maru-helper"
	helperBinDst      = "/usr/local/bin/maru-helper"
	helperSocketUnit  = "/etc/systemd/system/maru-helper.socket"
	helperServiceUnit = "/etc/systemd/system/maru-helper.service"
	helperEnvFile     = "/etc/default/maru-helper"
	// helperSocket은 도우미 소켓이다. Docker 프록시에 바인드 마운트하는 runtimeDir과 다른 디렉터리에 둔다.
	helperSocket = "/run/maru-helper/helper.sock"
	chkpwdPath   = "/usr/sbin/unix_chkpwd"

	// 대시보드에서 업그레이드(--web-upgrade). root 쪽 설정 파일·상태 파일·요청 경로는 backend/config에 있다.
	upgradeName        = "maru-upgrade"
	upgradeServiceUnit = "/etc/systemd/system/maru-upgrade.service"
	upgradePathUnit    = "/etc/systemd/system/maru-upgrade.path"
	// upgradeInstallerDst는 업그레이드 서비스가 실행하는 root 소유 설치기 복사본이다(저장소 안의 실행 파일을 root로
	// 실행하지 않게). 설치할 때마다 지금 설치기로 바꾼다.
	upgradeInstallerDir = "/usr/local/lib/maru"
	upgradeInstallerDst = upgradeInstallerDir + "/maru-install"
	// upgradeCheckRequest는 maru가 자기 상태 디렉터리(StateDirectory=maru)에 남기는 확인 요청이다.
	upgradeCheckRequest = stateDir + "/" + config.UpgradeCheckRequestName
	// upgradeApplyRequest는 도우미의 upgrade.apply가 쓰는 적용 요청이다. 그 디렉터리는 도우미 유닛의 RuntimeDirectory=다.
	upgradeApplyRequest   = config.UpgradeRequestDir + "/" + config.UpgradeApplyRequestName
	upgradeRequestDirName = "maru-upgrade"
)

// 저장소 안의 경로(저장소 루트 기준).
const (
	binSrcRel       = "bin/maru"
	helperBinSrcRel = "bin/maru-helper"
	// installerBinSrcRel은 설치기 자신이다(install.sh가 빌드한다). 대시보드에서 업그레이드를 켜면 root 소유로 복사한다.
	installerBinSrcRel = "bin/maru-install"
	distIndexRel       = "backend/internal/web/static/dist/index.html"
	iconsRel           = "vendor/icons/colloid"
)
