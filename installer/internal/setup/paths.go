package setup

import "maru/backend/config"

// 설치 경로. 바꾸면 README.md의 빠른 시작, docs/en/install.md·docs/en/reverse-proxy.md(한국어 docs/ko/도), 유닛 템플릿(templates/)도 함께 본다.
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
	// upgradeInstallerDst는 root 서비스(업그레이드 서비스, 애드온 백엔드의 root 동기화)가 실행하는 root 소유 설치기
	// 복사본이다(저장소 안의 실행 파일을 root로 실행하지 않게). 둘 중 하나라도 켜면 설치할 때마다 지금 설치기로 바꾸고,
	// 둘 다 끄면 지운다(step_installer_copy.go).
	upgradeInstallerDir = "/usr/local/lib/maru"
	upgradeInstallerDst = upgradeInstallerDir + "/maru-install"
	// upgradeCheckRequest는 maru가 자기 상태 디렉터리(StateDirectory=maru)에 남기는 확인 요청이다.
	upgradeCheckRequest = stateDir + "/" + config.UpgradeCheckRequestName
	// upgradeApplyRequest는 도우미의 upgrade.apply가 쓰는 적용 요청이다. 그 디렉터리는 도우미 유닛의 RuntimeDirectory=다.
	upgradeApplyRequest   = config.UpgradeRequestDir + "/" + config.UpgradeApplyRequestName
	upgradeRequestDirName = "maru-upgrade"

	// 애드온 백엔드(--addon-backends). 경로·형식은 backend/config(addons.go, addons_units.go)에 있다.
	addonSyncPathUnit    = "/etc/systemd/system/" + config.AddonSyncName + ".path"
	addonSyncServiceUnit = "/etc/systemd/system/" + config.AddonSyncName + ".service"
	// addonSyncRequest는 maru가 원하는 상태를 쓴 뒤 자기 상태 디렉터리에 남기는 동기화 요청이다(경로 유닛이 지켜본다).
	addonSyncRequest = stateDir + "/" + config.AddonSyncRequest
	// addonBackendsFile은 maru가 자기 상태 디렉터리에 쓰는 원하는 상태다(root 동기화가 읽기만 한다).
	addonBackendsFile = stateDir + "/" + config.AddonBackendsFile
	// addonDataDir·addonPrivateDataDir는 백엔드 데이터다(StateDirectory=maru-addon/<id>, DynamicUser=yes라 systemd가
	// private 아래에 두고 바깥에 심볼릭 링크를 둔다). --no-addon-backends는 남기고, 제거는 확인을 받은 뒤 지운다.
	addonDataDir        = "/var/lib/" + config.AddonStateDirName
	addonPrivateDataDir = "/var/lib/private/" + config.AddonStateDirName
	// addonWantsDir는 소켓 유닛을 켜면(WantedBy=sockets.target) systemctl이 인스턴스 링크를 두는 곳이다. 기록을 잃어도
	// 켜진 인스턴스를 끌 수 있게 끌 때 이 디렉터리도 본다.
	addonWantsDir = "/etc/systemd/system/sockets.target.wants"
)

// 저장소 안의 경로(저장소 루트 기준).
const (
	binSrcRel       = "bin/maru"
	helperBinSrcRel = "bin/maru-helper"
	// installerBinSrcRel은 설치기 자신이다(install.sh가 빌드한다). 대시보드에서 업그레이드나 애드온 백엔드를 켜면 root
	// 소유로 복사한다(upgradeInstallerDst).
	installerBinSrcRel = "bin/maru-install"
	distIndexRel       = "backend/internal/web/static/dist/index.html"
	iconsRel           = "vendor/icons/colloid"
)
