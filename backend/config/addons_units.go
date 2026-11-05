package config

// 애드온 패키지 백엔드의 systemd 유닛(설치기 --addon-backends가 쓰는 템플릿)과 설치 디렉터리 이름. 유닛 템플릿이
// 패키지 파일 경로를 maru와 같은 이름으로 가리키게 둘 다 이 값을 쓴다(.claude/docs/addon-packages.md의 "백엔드 프로세스").

// 상태 디렉터리 안의 패키지 파일 이름. 패키지는 <state>/addons/<id>/<rev>/에 풀리고, current가 지금 rev를 가리키며,
// 백엔드 실행 파일은 <rev>/backend/run이다. 유닛의 ExecStart는 InstalledStateDir 아래의 이 경로다.
const (
	// AddonsDirName은 상태 디렉터리 안의 패키지 디렉터리 이름이다.
	AddonsDirName = "addons"
	// AddonCurrentLink는 패키지 디렉터리 안에서 지금 rev를 가리키는 심볼릭 링크 이름이다.
	AddonCurrentLink = "current"
	// AddonBackendExec는 rev 디렉터리 안의 백엔드 실행 파일이다(실행 중인 아키텍처의 것만 이 이름으로 둔다).
	AddonBackendExec = "backend/run"
)

// 백엔드 유닛의 이름.
const (
	// AddonUnitUserPrefix는 백엔드 유닛의 동적 사용자 이름 앞부분이다(User=maru-a-<id>). 이름은 31자를 넘지 않는다.
	AddonUnitUserPrefix = "maru-a-"
	// AddonStateDirName은 백엔드 데이터(StateDirectory=maru-addon/<id>)의 상위 디렉터리 이름이다. DynamicUser=yes이므로
	// systemd는 데이터를 /var/lib/private/maru-addon/<id>에 두고 /var/lib/maru-addon/<id>에 심볼릭 링크를 둔다.
	AddonStateDirName = "maru-addon"
	// AddonSyncName은 root 동기화 유닛(maru-addons.path, maru-addons.service)의 이름이다.
	AddonSyncName = "maru-addons"
	// AddonSyncLockFile은 root 동기화 기록을 쓰는 root 쪽(설치기의 --service-install·--uninstall, --addon-sync)이
	// 모두 잡는 잠금 파일(flock)이다.
	AddonSyncLockFile = AddonSyncDir + "/lock"
)

// systemd가 DynamicUser=yes 유닛에 주는 uid 범위(systemd의 DYNAMIC_UID_MIN·DYNAMIC_UID_MAX)다. 운영의 호스트 소켓(0666)은
// 받은 연결의 상대 uid(SO_PEERCRED)가 0이거나 이 범위일 때만 받는다. 백엔드 유닛은 PrivateUsers=yes라서 유닛 안에서는
// 다른 uid로 보이지만, 커널이 maru에 주는 uid는 이 범위의 실제 uid일 것으로 본다(테스트 VM에서 확인한다).
const (
	AddonDynamicUIDMin = 61184
	AddonDynamicUIDMax = 65519
)
