package addons

import (
	"fmt"

	"onmaru/backend/internal/api"
)

const (
	// MaxShortcuts는 한 사용자가 바탕화면에 둘 수 있는 바로가기 수의 상한이다.
	MaxShortcuts = 64
	// maxAppIDLen은 바로가기가 가리키는 앱 id 길이의 상한이다.
	maxAppIDLen = 64
)

// ValidateShortcuts는 저장할 바로가기 목록을 검사한다. 앱 id는 애드온 id와 같은 형식이고,
// 앱마다 하나여야 한다.
//
// 레지스트리로 검사하지 않는다. 준비 중 앱(아직 애드온이 없는 앱)은 프론트에만 있고, 모르는 id는
// 프론트가 불러올 때 뺀다. capability(역할·플러그인)도 보지 않는다(ValidateWidgets와 같은 이유).
func ValidateShortcuts(list []api.Shortcut) error {
	if len(list) > MaxShortcuts {
		return fmt.Errorf("too many shortcuts (max %d)", MaxShortcuts)
	}
	seen := make(map[string]bool, len(list))
	for i, s := range list {
		if len(s.App) > maxAppIDLen || !idPattern.MatchString(s.App) {
			return fmt.Errorf("shortcut %d: invalid app id %q", i, s.App)
		}
		if seen[s.App] {
			return fmt.Errorf("shortcut %d: duplicate app %q", i, s.App)
		}
		seen[s.App] = true
	}
	return nil
}
