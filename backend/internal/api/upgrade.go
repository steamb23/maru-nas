package api

import "maru/backend/config"

// 대시보드에서 업그레이드를 쓸 수 없는 이유(UpgradeInfo.DisabledReason).
const (
	// UpgradeNotEnabled: 설치기가 대시보드에서 업그레이드를 켜지 않았다(/etc/default/maru-upgrade가 없다).
	UpgradeNotEnabled = "not-enabled"
	// UpgradeNoHelper: 권한 도우미를 설정하지 않았다(-helper-socket). 업그레이드는 도우미의 root 연산으로 요청한다.
	UpgradeNoHelper = "no-helper"
	// UpgradeStateDir: 상태 디렉터리가 없거나 설치한 위치(/var/lib/maru)가 아니다. 업그레이드 서비스의 경로 유닛은
	// 그 안의 확인 요청만 지켜본다.
	UpgradeStateDir = "state-dir"
)

// UpgradeInfo는 GET /api/upgrade(관리자)의 응답이다. 설정 앱의 업데이트 페이지가 보여 준다.
// Status는 업그레이드 서비스가 쓴 상태 파일 그대로이고 형식은 backend/config(upgrade_request.go)에 있다.
type UpgradeInfo struct {
	// Enabled는 대시보드에서 업그레이드를 쓸 수 있다는 뜻이다. 아니면 DisabledReason에 이유가 있다.
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabledReason,omitempty" tstype:"'not-enabled' | 'no-helper' | 'state-dir'"`
	// Repo, User는 업그레이드할 저장소와 저장소 소유자(빌드 계정)다. 수동 재설치 안내에 쓴다. 모르면 없다.
	Repo string `json:"repo,omitempty"`
	User string `json:"user,omitempty"`
	// Pending은 확인 요청을 업그레이드 서비스가 아직 가져가지 않았다는 뜻이다. PendingStalled는 1분 넘게 남아 있다는
	// 뜻이다(경로 유닛 maru-upgrade.path가 돌지 않는다).
	Pending        bool `json:"pending"`
	PendingStalled bool `json:"pendingStalled"`
	// Interrupted는 상태 파일이 확인·업그레이드 중인 채로 업그레이드 서비스의 시간 상한(2시간)보다 오래 바뀌지 않았다는
	// 뜻이다. 서비스가 중단됐으므로 다시 확인할 수 있다.
	Interrupted bool `json:"interrupted"`
	// Version, Revision은 지금 실행 중인 maru의 버전과 커밋이다(Revision은 모르면 없다).
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	// Installed는 설치기가 남긴 설치 기록이다(/var/lib/maru-upgrade/installed.json). 없거나 읽을 수 없으면 null이다.
	// 실행 파일에 커밋이 없을 때(go run) 화면이 이 기록의 커밋을 보인다.
	Installed *config.InstallRecord `json:"installed"`
	// Status는 업그레이드 서비스의 상태 파일이다. 없거나 읽을 수 없으면 null이다.
	Status *config.UpgradeStatus `json:"status"`
	// Channel은 관리자가 고른 업데이트 채널이다(site.json). 빈 값이면 지금 브랜치의 추적 브랜치를 따른다.
	Channel string `json:"channel" tstype:"'' | 'stable' | 'main'"`
}

// StartUpgradeCheckRequest는 POST /api/upgrade/check의 본문이다.
type StartUpgradeCheckRequest struct {
	// Mode는 확인 방식이다. remote는 git fetch 뒤 업데이트 채널(고르지 않았으면 추적 브랜치), local은 지금
	// 체크아웃(HEAD)을 대상으로 삼는다. 채널은 site.json의 값을 쓴다.
	Mode string `json:"mode" tstype:"'remote' | 'local'"`
}

// SetUpgradeChannelRequest는 PUT /api/upgrade/channel의 본문이다. 원격 확인이 따라갈 업데이트 채널을 site.json에 둔다.
type SetUpgradeChannelRequest struct {
	Channel string `json:"channel" tstype:"'' | 'stable' | 'main'"`
}

// StartUpgradeRequest는 POST /api/upgrade/apply의 본문이다. root 권한 상승이 필요하다.
type StartUpgradeRequest struct {
	// Target은 확인이 정한 대상 커밋(40자리 16진수)이다.
	Target string `json:"target"`
	// Settings는 업그레이드 계획이 물은 설정의 이름과 값이다(confirm은 yes/no).
	Settings map[string]string `json:"settings"`
}
