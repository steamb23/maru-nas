package main

import (
	"testing"

	"maru/backend/config"
	"maru/backend/internal/upgrade"
)

func TestUpgradePaths(t *testing.T) {
	installed := upgrade.Paths{EnvFile: config.UpgradeEnvFile, StatusFile: config.UpgradeStatusFile, RecordFile: config.InstallRecordFile}
	withRequest := installed
	withRequest.CheckRequest = "/var/lib/maru/upgrade-check.json"
	cases := []struct {
		devRoot, stateDir string
		want              upgrade.Paths
	}{
		// 설치한 상태 디렉터리에서만 확인 요청을 쓴다(경로 유닛이 그곳만 본다).
		{stateDir: "/var/lib/maru", want: withRequest},
		{stateDir: "/var/lib/maru/", want: withRequest},
		{stateDir: "/srv/maru-state", want: installed},
		{want: installed},
		// 개발 서버는 가짜 루트 아래를 읽고 지금 상태 디렉터리에 쓴다.
		{devRoot: "/tmp/dev", stateDir: "/tmp/state", want: upgrade.Paths{EnvFile: "/tmp/dev" + config.UpgradeEnvFile,
			StatusFile: "/tmp/dev" + config.UpgradeStatusFile, RecordFile: "/tmp/dev" + config.InstallRecordFile,
			CheckRequest: "/tmp/state/upgrade-check.json"}},
	}
	for _, c := range cases {
		if got := upgradePaths(c.devRoot, c.stateDir); got != c.want {
			t.Errorf("upgradePaths(%q, %q) = %+v, want %+v", c.devRoot, c.stateDir, got, c.want)
		}
	}
}
