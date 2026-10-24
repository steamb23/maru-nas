package main

import (
	"os"
	"path/filepath"

	"maru/backend/internal/addons/storage"
)

// envDevUpgradeRoot는 개발 서버(make dev-fake) 전용 환경 변수다. 대시보드에서 업그레이드의 root 쪽 설정 파일과 상태
// 파일 경로 앞에 붙는 가짜 루트이며, 가짜 업그레이드 서비스(cmd/fakeomv)가 그 아래에 쓴다. 운영에서 바꿀 이유가 없는
// 경로이므로 플래그로 두지 않는다(가동 설정 목록에도 넣지 않는다).
const envDevUpgradeRoot = "MARU_DEV_UPGRADE_ROOT"

// envDevNotifyRoot는 개발 서버(make dev-fake) 전용 환경 변수다. APT 업데이트 목록(upgradeindex.json)과 재부팅 권장
// 파일을 읽는 가짜 루트이며, 가짜 OMV 프로세스(cmd/fakeomv)가 FAKEAPT 시나리오대로 그 아래에 쓴다. 운영에서는 언제나
// /이므로 플래그로 두지 않는다(가동 설정 목록·설치기 레지스트리에도 넣지 않는다).
const envDevNotifyRoot = "MARU_DEV_NOTIFY_ROOT"

// envDevFilesSrv는 개발 서버(make dev-fake) 전용 환경 변수다. 파일 탐색기가 데이터 디스크로 보는 디렉터리를
// 가짜 권한 도우미의 루트(<상태 디렉터리>/helper-root/srv)로 바꾸고, 그 바로 아래 폴더를 디스크로 본다(가짜 디스크는
// 마운트가 아니라 폴더다). 운영에서는 언제나 /srv이므로 플래그로 두지 않는다(가동 설정 목록에도 넣지 않는다).
const envDevFilesSrv = "MARU_DEV_FILES_SRV"

// devDisks는 개발 서버의 가짜 디스크(dir 바로 아래 폴더)다.
type devDisks struct{}

func (devDisks) MountsUnder(dir string) ([]storage.MountLabel, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []storage.MountLabel
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, storage.MountLabel{MountPoint: filepath.Join(dir, e.Name())})
		}
	}
	return out, nil
}
