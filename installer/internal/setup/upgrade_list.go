package setup

// 업그레이드 계획(--upgrade-plan)이 쓰는 목록. 유지자가 버전을 낼 때 여기에 더한다(.claude/rules/installer.md의
// "수동 재설치 항목과 업그레이드 뒤 할 일"). 이 기능 전의 설치는 설치 기록이 없어 어차피 수동이므로 처음엔 비어 있다.

// manualUpgrade는 대시보드에서 업그레이드로 넘을 수 없는 버전 경계다. 설치된 버전이 Below보다 낮으면 이 버전으로
// 올리는 일을 수동 재설치로 안내한다. 예: 서비스 사용자나 소켓 경로가 바뀔 때, 새 root 구성 요소(유닛 등)가 생길 때,
// 업그레이드 서비스 자체의 동작 방식이 바뀔 때.
type manualUpgrade struct {
	// Below는 0.y.z 버전이다. 보통 그 변경이 들어간 버전이다.
	Below string
	// Reason은 화면에 보일 이유다.
	Reason Text
	// Command는 config.UpgradeCommandInstall 또는 config.UpgradeCommandReinstall이다.
	Command string
}

// upgradeNote는 업그레이드한 뒤 할 일이다. 설치된 버전이 Since보다 낮으면 보여 준다.
type upgradeNote struct {
	Since string
	Text  Text
}

// manualUpgrades는 수동 재설치 경계 목록이다(오래된 것부터).
var manualUpgrades = []manualUpgrade{}

// upgradeNotes는 업그레이드 뒤 할 일 목록이다(오래된 것부터).
var upgradeNotes = []upgradeNote{}
