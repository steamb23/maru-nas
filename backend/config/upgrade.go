package config

// 대시보드에서 업그레이드(ROADMAP.md)에 쓰는 파일 경로와 형식. 설치기가 쓰고 백엔드가 읽는다.

const (
	// UpgradeStateDir는 설치 기록을 두는 디렉터리다(root 소유 0755, 파일은 0644).
	// 저장소 소유자(일반 사용자)로 실행하는 설치기의 --upgrade-plan이 읽으므로 다른 사용자도 읽을 수 있어야 한다.
	UpgradeStateDir = "/var/lib/onmaru-upgrade"
	// InstallRecordFile은 설치기가 설치할 때마다(수동 설치 포함) 쓰는 설치 기록이다.
	InstallRecordFile = UpgradeStateDir + "/installed.json"
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
