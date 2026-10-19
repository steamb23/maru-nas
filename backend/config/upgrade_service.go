package config

import "time"

// 업그레이드 서비스(onmaru-upgrade.service)의 설정 파일 키와 실행 시간 상한. 파일 형식(upgrade.go, upgrade_request.go)과
// 달리 프론트엔드 타입으로 내보내지 않는다(tygo.yaml).

// UpgradeEnvFile의 키. 설치기가 쓰고 업그레이드 서비스가 읽는다. onmaru는 수동 재설치 안내에 저장소 경로와 계정을 읽는다.
const (
	EnvUpgradeRepo = "ONMARU_UPGRADE_REPO"
	EnvUpgradeUser = "ONMARU_UPGRADE_USER"
	EnvUpgradePath = "ONMARU_UPGRADE_PATH"
)

// UpgradeServiceTimeout은 업그레이드 서비스 한 번의 실행 시간 상한(유닛의 TimeoutStartSec)이다. 상태 파일이 확인·적용
// 중인 채로 이보다 오래 바뀌지 않았으면 onmaru는 서비스가 중단된 것으로 본다(서비스가 죽으면 상태를 고치지 못한다).
const UpgradeServiceTimeout = 2 * time.Hour
