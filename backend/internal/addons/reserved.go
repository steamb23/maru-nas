package addons

import (
	"slices"

	"maru/backend/config"
)

// MaxPackageIDLen은 애드온 패키지 id 길이의 상한이다. 패키지 백엔드의 동적 사용자 이름(maru-a-<id>)이
// systemd의 사용자 이름 한도(31자) 안에 들어야 한다(config.AddonIDMaxLen).
const MaxPackageIDLen = config.AddonIDMaxLen

// reservedAppIDs는 애드온 패키지가 쓸 수 없는 id다. 프론트에만 있는 앱(셸 앱, 링크 앱, 준비 중 앱)은
// 레지스트리에 없으므로 여기에 둔다. 프론트 표의 키가 모두 이 목록에 있는지는 Vitest가 본다
// (manifests.gen.ts의 RESERVED_APP_IDS). 나머지는 앞으로 셸이 쓸 수 있는 이름이다.
var reservedAppIDs = []string{
	// 셸 앱(frontend/src/windows/apps.ts의 SHELL_APPS)
	"files", "terminal", "transfers", "viewer", "settings",
	// 링크 앱(LINK_APPS)
	"workbench",
	// 앞으로 쓸 수 있는 이름
	"addon", "addons", "app", "apps", "dashboard", "desktop", "maru", "omv", "package", "shell", "system",
}

// ReservedAppIDs는 애드온 패키지가 쓸 수 없는 앱 id 목록이다(정렬됨).
func ReservedAppIDs() []string {
	out := slices.Clone(reservedAppIDs)
	slices.Sort(out)
	return out
}

// IsReservedAppID는 id를 애드온 패키지가 쓸 수 없는지다.
func IsReservedAppID(id string) bool { return slices.Contains(reservedAppIDs, id) }
