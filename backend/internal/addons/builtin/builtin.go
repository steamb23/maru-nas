// Package builtin은 내장 애드온의 정적 목록이다. 새 애드온은 New에 추가한다.
package builtin

import (
	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/addons/sysinfo"
)

// Deps는 내장 애드온의 provider가 쓰는 입력이다.
type Deps struct {
	System *sysinfo.Reader
}

// New는 내장 애드온 레지스트리를 만든다. 목록은 컴파일 타임에 정해지므로
// 검사에 실패하면 프로그래밍 오류로 보고 panic한다(테스트가 먼저 잡는다).
func New(d Deps) *addons.Registry {
	r, err := addons.NewRegistry(
		sysinfo.New(d.System),
	)
	if err != nil {
		panic(err)
	}
	return r
}
