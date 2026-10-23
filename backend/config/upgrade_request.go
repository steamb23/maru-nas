package config

import (
	"regexp"
	"strings"
)

// 대시보드에서 업그레이드의 요청과 상태(.claude/docs/upgrade.md). maru가 확인 요청을, 도우미의 upgrade.apply가 적용 요청을 쓰고,
// 업그레이드 서비스(설치기의 --upgrade-service)가 둘을 읽어 처리한 뒤 진행과 결과를 상태 파일에 쓴다.

// UpgradeRequestFormat은 확인·적용 요청의 형식 번호다. 필드를 빼거나 뜻을 바꾸면 올린다.
const UpgradeRequestFormat = 1

// 확인 방식(UpgradeCheckRequest.Mode, UpgradeCheck.Mode).
const (
	// UpgradeModeRemote는 저장소 소유자로 git fetch를 하고 지금 브랜치의 추적 브랜치를 대상으로 삼는다.
	UpgradeModeRemote = "remote"
	// UpgradeModeLocal은 지금 체크아웃(HEAD)을 대상으로 삼는다.
	UpgradeModeLocal = "local"
)

// 업데이트 채널(UpgradeCheckRequest.Channel, UpgradeCheck.Channel). 원격 확인이 따라갈 브랜치이며, 관리자가 설정 앱에서
// 고른 값을 maru가 site.json에 둔다. 채널 브랜치는 UpgradeChannelRemote에서 가져온다.
const (
	// UpgradeChannelCurrent(빈 값)는 지금 브랜치의 추적 브랜치를 따른다(채널을 고르지 않은 설치).
	UpgradeChannelCurrent = ""
	// UpgradeChannelStable은 릴리스 브랜치 stable을 따른다.
	UpgradeChannelStable = "stable"
	// UpgradeChannelMain은 개발 브랜치 main을 따른다.
	UpgradeChannelMain = "main"
)

// UpgradeChannelRemote는 채널 브랜치를 가져오는 원격 저장소 이름이다(클론한 저장소).
const UpgradeChannelRemote = "origin"

// ValidUpgradeChannel은 받을 수 있는 채널 값인지다. 채널은 브랜치 이름으로 git에 넘기므로 정한 값만 받는다.
func ValidUpgradeChannel(s string) bool {
	return s == UpgradeChannelCurrent || s == UpgradeChannelStable || s == UpgradeChannelMain
}

// UpgradeCheckRequest는 maru가 자기 상태 디렉터리에 남기는 확인 요청이다(UpgradeCheckRequestName). 업그레이드
// 서비스는 이 파일을 믿지 않고 확인 방식과 채널(정한 값 중 하나)만 읽는다.
type UpgradeCheckRequest struct {
	Format int    `json:"format"`
	Mode   string `json:"mode"`
	// Channel은 원격 확인이 따라갈 채널이다(UpgradeChannel*). 로컬 확인에서는 쓰지 않는다.
	Channel string `json:"channel,omitempty" tstype:"'' | 'stable' | 'main'"`
}

// UpgradeApplyRequest는 도우미의 root 연산 upgrade.apply가 UpgradeRequestDir에 쓰는 적용 요청이다.
type UpgradeApplyRequest struct {
	Format int `json:"format"`
	// Target은 확인 단계에서 정한 대상 커밋(40자리 16진수)이다. 업그레이드 서비스는 마지막 확인의 대상과 같을 때만 진행한다.
	Target string `json:"target"`
	// Settings는 업그레이드 계획이 물은 설정의 이름(설치기 옵션 이름)과 값이다. confirm 설정은 yes/no다.
	Settings map[string]string `json:"settings"`
	// User는 요청한 관리자다(도우미가 티켓에서 정한다).
	User string `json:"user"`
	// RequestedAt은 요청한 시각(RFC 3339, UTC)이다.
	RequestedAt string `json:"requestedAt"`
}

// 적용 요청의 설정 개수와 값 길이 상한.
const (
	MaxUpgradeSettings     = 32
	MaxUpgradeSettingValue = 512
)

var (
	upgradeTargetRE  = regexp.MustCompile(`^[0-9a-f]{40}$`)
	upgradeSettingRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)
)

// ValidUpgradeTarget은 대상 커밋이 40자리 소문자 16진수(SHA-1 git 커밋)인지다.
func ValidUpgradeTarget(s string) bool { return upgradeTargetRE.MatchString(s) }

// ValidUpgradeSetting은 적용 요청의 설정 이름과 값이 받을 수 있는 꼴인지다. 이름은 설치기 옵션 이름 꼴이고, 값은
// 공백·제어 문자 없는 출력 가능한 ASCII다(빈 값 가능). 값은 셸을 거치지 않고 --이름=값 인자 하나로만 넘어가며,
// 이름이 계획의 질문인지와 값이 올바른지는 업그레이드 서비스와 새 설치기가 다시 검사한다.
func ValidUpgradeSetting(name, value string) bool {
	if !upgradeSettingRE.MatchString(name) || strings.HasPrefix(name, "no-") || len(value) > MaxUpgradeSettingValue {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] <= ' ' || value[i] > '~' {
			return false
		}
	}
	return true
}

// UpgradeStatusFormat은 UpgradeStatus의 형식 번호다. 필드를 빼거나 뜻을 바꾸면 올린다.
const UpgradeStatusFormat = 1

// UpgradeStatus.State 값.
const (
	UpgradeStateChecking    = "checking"
	UpgradeStateChecked     = "checked"
	UpgradeStateCheckFailed = "check-failed"
	UpgradeStateApplying    = "applying"
	UpgradeStateApplied     = "applied"
	UpgradeStateApplyFailed = "apply-failed"
)

// UpgradeStatus.Stage 값. 진행 중인 단계 또는 실패한 단계다.
const (
	// UpgradeStageInspect: 저장소 상태 확인(소유자, 작업 트리, 커밋 관계).
	UpgradeStageInspect = "inspect"
	// UpgradeStageFetch: 원격 확인의 git fetch.
	UpgradeStageFetch = "fetch"
	// UpgradeStagePlan: 대상 커밋의 설치기를 임시 빌드해 업그레이드 계획을 만든다(적용에서는 입력한 설정 값 검사).
	UpgradeStagePlan = "plan"
	// UpgradeStageUpdate: 원격 확인으로 진행할 때 작업 트리를 대상 커밋으로 fast-forward한다.
	UpgradeStageUpdate = "update"
	// UpgradeStageBuild: 저장소 소유자로 빌드한다.
	UpgradeStageBuild = "build"
	// UpgradeStageInstall: 새 설치기를 root로 실행해 설치한다(maru와 도우미가 다시 시작한다).
	UpgradeStageInstall = "install"
)

// 업그레이드 서비스가 확인 단계에서 더하는 수동 재설치 사유(UpgradeReason.ID). 계획(설치기)이 내는 사유는 upgrade.go에 있다.
const (
	// UpgradeReasonRepository: 저장소 경로가 없거나, 소유자가 설정한 빌드 계정이 아니거나 root다.
	UpgradeReasonRepository = "repository"
	// UpgradeReasonTools: 빌드 PATH에서 git·go·npm·make 중 하나를 찾을 수 없다.
	UpgradeReasonTools = "tools"
	// UpgradeReasonGo: Go가 대상 커밋이 요구하는 버전보다 낮다(도구 체인을 받아 올 수도 없다).
	UpgradeReasonGo = "go"
	// UpgradeReasonNoUpstream: 원격 확인인데 지금 브랜치에 추적 브랜치가 없다(태그·커밋을 체크아웃한 설치 포함).
	UpgradeReasonNoUpstream = "no-upstream"
	// UpgradeReasonDiverged: 원격 확인인데 지금 체크아웃에서 추적 브랜치로 fast-forward할 수 없다(로컬 커밋). 채널을
	// 바꿀 때는 그 채널의 로컬 브랜치에 원격에 없는 커밋이 있다.
	UpgradeReasonDiverged = "diverged"
	// UpgradeReasonDirty: 작업 트리에 커밋하지 않은 변경이 있다.
	UpgradeReasonDirty = "dirty"
	// UpgradeReasonCommitMissing: 설치된 커밋이 저장소에 없다.
	UpgradeReasonCommitMissing = "commit-missing"
	// UpgradeReasonNotDescendant: 대상이 설치된 커밋에서 이어진 커밋이 아니다(되돌아가는 설치). 채널을 바꾸는 원격
	// 확인은 덮어써서 설치하므로 이 사유를 쓰지 않는다(UpgradeCheck.Switch).
	UpgradeReasonNotDescendant = "not-descendant"
	// UpgradeReasonPlanFormat: 새 버전의 업그레이드 계획 형식을 이 업그레이드 서비스가 모른다.
	UpgradeReasonPlanFormat = "plan-format"
)

// UpgradeStatus는 업그레이드 서비스의 상태 파일(UpgradeStatusFile)이다.
type UpgradeStatus struct {
	Format int    `json:"format"`
	State  string `json:"state"`
	// Stage는 진행 중이거나 실패한 단계다(UpgradeStage*). 끝났으면 빈 값이다.
	Stage string `json:"stage,omitempty"`
	// UpdatedAt은 마지막으로 쓴 시각(RFC 3339, UTC)이다.
	UpdatedAt string `json:"updatedAt"`
	// Check는 마지막으로 끝난 확인 결과다. 새 확인을 시작하면 지우고, 적용하는 동안과 적용한 뒤에도 남긴다.
	Check *UpgradeCheck `json:"check"`
	// Apply는 진행 중이거나 마지막으로 한 적용이다.
	Apply *UpgradeApply `json:"apply"`
	// Error는 마지막 확인·적용이 실패한 이유다. 성공하면 null이다.
	Error *UpgradeError `json:"error"`
	// Branch는 업그레이드 서비스가 마지막으로 본 저장소의 브랜치다(확인할 때, 적용이 저장소를 갱신한 뒤에 쓴다).
	// Detached면 브랜치가 아닌 체크아웃(태그·커밋)이다. 둘 다 비었으면 모른다(아직 보지 않았거나 이전 서비스).
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached,omitempty"`
}

// UpgradeCheck는 확인 결과다.
type UpgradeCheck struct {
	Mode string `json:"mode"`
	// Channel은 원격 확인이 따른 채널이다(UpgradeChannel*). 지금 브랜치를 따른 원격 확인과 로컬 확인은 빈 값이다.
	Channel string `json:"channel,omitempty" tstype:"'' | 'stable' | 'main'"`
	// CheckedAt은 확인을 마친 시각(RFC 3339, UTC)이다.
	CheckedAt string `json:"checkedAt"`
	// Upstream은 원격 확인의 추적 브랜치(예: origin/stable)다. 채널을 따르면 그 채널의 원격 브랜치다.
	Upstream string `json:"upstream,omitempty"`
	// Head는 확인할 때의 저장소 HEAD 커밋이다.
	Head string `json:"head"`
	// Branch는 확인할 때 체크아웃한 브랜치다. 브랜치가 아니면(태그·커밋) 빈 값이다.
	Branch string `json:"branch,omitempty"`
	// Switch는 업그레이드하면 저장소를 채널 브랜치로 바꾼다는 뜻이다(Channel이 있고 Branch와 다르다). 이때는 대상이
	// 설치된 커밋에서 이어지지 않아도(이전 버전이어도) 덮어써서 설치한다.
	Switch bool `json:"switch,omitempty"`
	// Dropped는 설치된 커밋에는 있지만 대상에는 없는 커밋 수다(Switch로 이전 버전을 덮어쓸 때).
	Dropped int `json:"dropped,omitempty"`
	// Target은 대상 커밋이다. 정하지 못했으면(추적 브랜치 없음 등) 빈 값이다.
	Target string `json:"target,omitempty"`
	// UpToDate는 대상이 설치된 커밋과 같다는 뜻이다.
	UpToDate bool `json:"upToDate"`
	// Installed는 확인할 때의 설치 기록이다.
	Installed *InstallRecord `json:"installed"`
	// Commits는 설치된 커밋에서 대상까지의 커밋이다(새것부터, 최대 MaxUpgradeCommits개).
	Commits []UpgradeCommit `json:"commits"`
	// CommitsTruncated는 커밋이 더 있어 일부만 담았다는 뜻이다.
	CommitsTruncated bool `json:"commitsTruncated"`
	// Plan은 대상 커밋의 설치기가 낸 업그레이드 계획이다. 만들지 않았거나 모르는 형식이면 null이다.
	Plan *UpgradePlan `json:"plan"`
	// Web은 대시보드에서 업그레이드할 수 있는지다(대상이 있고, 최신이 아니고, Manual이 비어 있다).
	Web bool `json:"web"`
	// Manual은 업그레이드 서비스의 사유와 계획의 사유를 합친 수동 재설치 사유다.
	Manual []UpgradeReason `json:"manual"`
	// Command는 수동 재설치 명령이다(Manual이 있을 때만).
	Command string `json:"command,omitempty"`
}

// MaxUpgradeCommits는 확인 결과에 담는 커밋 수의 상한이다.
const MaxUpgradeCommits = 200

// UpgradeCommit은 커밋 하나다.
type UpgradeCommit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Time    string `json:"time"`
	Subject string `json:"subject"`
}

// UpgradeApply는 적용 하나다.
type UpgradeApply struct {
	Target      string `json:"target"`
	User        string `json:"user"`
	RequestedAt string `json:"requestedAt"`
	StartedAt   string `json:"startedAt"`
	// FinishedAt은 끝난 시각이다. 진행 중이면 빈 값이다.
	FinishedAt string `json:"finishedAt,omitempty"`
	// Version은 설치한 버전이다(성공했을 때 설치 기록에서 읽는다).
	Version string `json:"version,omitempty"`
}

// UpgradeError는 실패 이유다.
type UpgradeError struct {
	Text LocalizedText `json:"text"`
	// Log는 실패한 명령 출력의 끝부분이다. 전체 로그는 journalctl -u maru-upgrade에 있다.
	Log string `json:"log,omitempty"`
}
