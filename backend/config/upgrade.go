package config

// 대시보드에서 업그레이드(.claude/docs/upgrade.md)에 쓰는 파일 경로와 형식(설치 기록, 업그레이드 계획). 설치기가 쓰고 백엔드가 읽는다.
// 확인·적용 요청과 상태 파일은 upgrade_request.go에 있다.

const (
	// UpgradeStateDir는 설치 기록과 업그레이드 상태 파일을 두는 디렉터리다(root 소유 0755, 파일은 0644).
	// 저장소 소유자(일반 사용자)로 실행하는 설치기의 --upgrade-plan과 onmaru가 읽으므로 다른 사용자도 읽을 수 있어야 한다.
	UpgradeStateDir = "/var/lib/onmaru-upgrade"
	// InstallRecordFile은 설치기가 설치할 때마다(수동 설치 포함) 쓰는 설치 기록이다.
	InstallRecordFile = UpgradeStateDir + "/installed.json"
	// UpgradeStatusFile은 업그레이드 서비스(onmaru-upgrade.service)가 확인·적용의 진행과 결과를 쓰는 상태 파일이다.
	// onmaru는 읽기만 하고, onmaru가 다시 시작되어도 남는다.
	UpgradeStatusFile = UpgradeStateDir + "/status.json"
	// UpgradeEnvFile은 대시보드에서 업그레이드의 root 쪽 설정 파일이다(저장소 경로, 빌드 계정, 빌드 PATH. root 소유 0644).
	// 설치기가 --web-upgrade일 때만 쓰므로 이 파일이 있으면 대시보드에서 업그레이드가 켜져 있다.
	UpgradeEnvFile = "/etc/default/onmaru-upgrade"
	// UpgradeCheckRequestName은 onmaru가 자기 상태 디렉터리에 남기는 확인 요청 파일의 이름이다. 설치한 onmaru의 상태
	// 디렉터리는 /var/lib/onmaru(StateDirectory=)이고 업그레이드 서비스의 경로 유닛이 그 안의 이 이름을 지켜본다.
	UpgradeCheckRequestName = "upgrade-check.json"
	// UpgradeRequestDir는 도우미의 root 연산 upgrade.apply가 적용 요청을 쓰는 디렉터리다(root 0700). 도우미 유닛의
	// RuntimeDirectory=라 도우미만 쓸 수 있다.
	UpgradeRequestDir = "/run/onmaru-upgrade"
	// UpgradeApplyRequestName은 UpgradeRequestDir 안의 적용 요청 파일 이름이다.
	UpgradeApplyRequestName = "apply.json"
	// InstalledStateDir는 설치한 onmaru의 상태 디렉터리(유닛의 StateDirectory=onmaru)다. 경로 유닛은 이 안의 확인
	// 요청만 지켜보므로, 상태 디렉터리를 옮긴 onmaru는 대시보드에서 업그레이드를 쓸 수 없다.
	InstalledStateDir = "/var/lib/onmaru"
)

// InstallRecordFormat은 InstallRecord의 형식 번호다. 필드를 빼거나 뜻을 바꾸면 올린다.
const InstallRecordFormat = 1

// InstallRecord는 지금 설치된 온마루다(InstallRecordFile).
// 업그레이드가 잘못 판단하지 않게, 설치기가 확인하지 못한 값은 빈 값으로 둔다.
type InstallRecord struct {
	Format int `json:"format"`
	// Version은 설치한 버전이다. 설치기와 실행 파일이 같은 커밋에서 빌드됐는지 확인하지 못하면 빈 값이다.
	Version string `json:"version"`
	// Revision은 설치한 실행 파일(onmaru)을 빌드한 커밋(40자리 16진수)이다. 알 수 없으면 빈 값이다.
	Revision string `json:"revision"`
	// RevisionTime은 그 커밋의 시각(RFC 3339)이다.
	RevisionTime string `json:"revisionTime"`
	// Modified는 커밋하지 않은 변경이 있는 작업 트리에서 빌드했는지다.
	Modified bool `json:"modified"`
	// InstalledAt은 설치한 시각(RFC 3339, UTC)이다.
	InstalledAt string `json:"installedAt"`
}

// UpgradePlanFormat은 UpgradePlan의 형식 번호다. 업그레이드를 진행하는 쪽은 모르는 형식이면 수동 재설치로 안내하므로,
// 필드를 빼거나 뜻을 바꾸면 올린다(읽는 쪽이 몰라도 되는 필드를 더할 때는 올리지 않는다).
const UpgradePlanFormat = 1

// 수동 재설치 명령. 저장소 루트에서 저장소 소유자 계정으로 실행한다.
const (
	UpgradeCommandInstall   = "./install.sh"
	UpgradeCommandReinstall = "./install.sh --reinstall"
)

// UpgradeReason.ID 값.
const (
	// UpgradeReasonNotInstalled: 설치되어 있지 않다(유닛이나 /etc/default/onmaru가 없다).
	UpgradeReasonNotInstalled = "not-installed"
	// UpgradeReasonNoRecord: 설치 기록이 없거나, 읽을 수 없거나, 버전·커밋이 비어 있다.
	UpgradeReasonNoRecord = "no-record"
	// UpgradeReasonDowngrade: 설치된 버전이 이 버전보다 높다.
	UpgradeReasonDowngrade = "downgrade"
	// UpgradeReasonManualList: 새 버전이 정한 수동 재설치 경계보다 설치된 버전이 낮다.
	UpgradeReasonManualList = "manual-list"
	// UpgradeReasonSetting: 웹에서 물을 수 없는 설정이 있다(Setting에 이름).
	UpgradeReasonSetting = "setting"
	// UpgradeReasonNode: Node.js가 없거나 이 버전이 요구하는 범위 밖이다.
	UpgradeReasonNode = "node"
)

// UpgradeQuestion.Kind와 Reason 값.
const (
	// UpgradeQuestionInput은 한 줄 입력이다. 설치기에 --<이름> <값>으로 넘긴다.
	UpgradeQuestionInput = "input"
	// UpgradeQuestionConfirm은 예·아니요(값 yes/no)다. 설치기에 --<이름> 또는 --no-<이름>으로 넘긴다.
	UpgradeQuestionConfirm = "confirm"
	// UpgradeQuestionNew: 기존 설치에 없는 설정(새 버전에서 생긴 설정)이다.
	UpgradeQuestionNew = "new"
	// UpgradeQuestionInvalid: 기존 설치(또는 넘긴 옵션)의 값이 이 버전의 검사를 통과하지 못했다.
	UpgradeQuestionInvalid = "invalid"
)

// LocalizedText는 설치기 문구의 영어·한국어 한 쌍이다(설치기 문구는 두 언어만 있다).
type LocalizedText struct {
	En string `json:"en"`
	Ko string `json:"ko"`
}

// UpgradePlan은 새 버전의 설치기가 내는 업그레이드 계획이다(onmaru-install --upgrade-plan). 설치기는 기존 설치를
// 읽기만 하고 계획을 만든다. 설정 옵션을 함께 주면 그 값을 쓰고 검사하므로, 입력한 값을 확인하는 데에도 쓴다.
type UpgradePlan struct {
	Format int `json:"format"`
	// Version은 계획을 낸 설치기(업그레이드 대상)의 버전이다.
	Version string `json:"version"`
	// Revision은 계획을 낸 설치기를 빌드한 커밋이다. 알 수 없으면 빈 값이다.
	Revision string `json:"revision"`
	// Installed는 설치 기록이다. 없거나 읽을 수 없으면 null이다.
	Installed *InstallRecord `json:"installed"`
	// Web은 대시보드에서 업그레이드할 수 있는지다(Manual이 비어 있다).
	Web bool `json:"web"`
	// Command는 수동 재설치 명령이다(Manual이 있을 때만). 사유 중 하나라도 --reinstall이면 --reinstall이다.
	Command string `json:"command,omitempty"`
	// Manual은 대시보드에서 업그레이드할 수 없는 사유다.
	Manual []UpgradeReason `json:"manual"`
	// Questions는 업그레이드하기 전에 물어야 하는 설정이다.
	Questions []UpgradeQuestion `json:"questions"`
	// Notes는 업그레이드한 뒤 할 일이다.
	Notes []LocalizedText `json:"notes"`
	// Requires는 빌드에 필요한 도구 버전이다.
	Requires UpgradeRequires `json:"requires"`
}

// UpgradeReason은 수동 재설치 사유 하나다.
type UpgradeReason struct {
	// ID는 사유의 종류다(UpgradeReason* 상수).
	ID string `json:"id"`
	// Setting은 ID가 setting일 때 그 설정 이름이다.
	Setting string        `json:"setting,omitempty"`
	Text    LocalizedText `json:"text"`
	// Command는 이 사유를 넘는 명령이다(UpgradeCommand* 상수).
	Command string `json:"command"`
}

// UpgradeQuestion은 업그레이드하기 전에 묻는 설정 하나다. 문구와 기본값은 설치기의 설정 정의에서 가져온다.
type UpgradeQuestion struct {
	// Name은 설정 이름(설치기 옵션 이름)이다.
	Name string `json:"name"`
	// Kind는 입력 형태다(UpgradeQuestionInput, UpgradeQuestionConfirm).
	Kind  string        `json:"kind"`
	Title LocalizedText `json:"title"`
	// Help는 질문 앞의 설명이다. 여러 줄일 수 있고 비어 있을 수 있다.
	Help LocalizedText `json:"help"`
	// Default는 기본값이다. confirm이면 yes 또는 no다.
	Default string `json:"default"`
	// Dash면 빈 값을 "-"로 보여 주고 "-" 입력을 빈 값으로 받는다(빈 값이 "끔"인 설정).
	Dash bool `json:"dash"`
	// EmptyOK면 빈 값도 정한 값이다.
	EmptyOK bool `json:"emptyOK"`
	// Reason은 묻는 이유다(UpgradeQuestionNew, UpgradeQuestionInvalid).
	Reason string `json:"reason"`
	// Error는 Reason이 invalid일 때 검사 오류다.
	Error *LocalizedText `json:"error,omitempty"`
}

// UpgradeRequires는 빌드에 필요한 도구 버전이다.
type UpgradeRequires struct {
	// Go는 최소 Go 버전이다. 계획을 낸 설치기를 빌드할 수 있었다면 충분하므로 따로 확인하지 않는다.
	Go string `json:"go"`
	// Node는 Node.js 버전 범위다(npm engines 꼴).
	Node string `json:"node"`
	// NodeFound는 PATH에서 찾은 Node.js 버전이다(앞의 v 없이). 없으면 빈 값이다.
	NodeFound string `json:"nodeFound"`
}
