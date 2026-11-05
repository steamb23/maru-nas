package config

import (
	"path"
	"testing"
)

// 이름 상수는 경로 상수와 같은 곳을 가리킨다.
func TestAddonUnitNames(t *testing.T) {
	if path.Dir(AddonSyncLockFile) != AddonSyncDir {
		t.Errorf("lock file %s is not in %s", AddonSyncLockFile, AddonSyncDir)
	}
	// 동적 사용자 이름(User=maru-a-<id>)은 31자를 넘지 않는다.
	if n := len(AddonUnitUserPrefix) + AddonIDMaxLen; n > 31 {
		t.Errorf("dynamic user name can be %d characters", n)
	}
	if AddonDynamicUIDMin > AddonDynamicUIDMax || AddonDynamicUIDMin <= 0 {
		t.Errorf("dynamic uid range %d-%d", AddonDynamicUIDMin, AddonDynamicUIDMax)
	}
}
