// Package builtin은 내장 애드온의 정적 목록이다. 새 애드온은 New에 추가한다.
package builtin

import (
	"maru/backend/internal/addons"
	"maru/backend/internal/addons/containers"
	"maru/backend/internal/addons/network"
	"maru/backend/internal/addons/services"
	"maru/backend/internal/addons/smart"
	"maru/backend/internal/addons/storage"
	"maru/backend/internal/addons/sysinfo"
	"maru/backend/internal/addons/temperature"
)

// Deps는 내장 애드온의 provider가 쓰는 입력이다.
type Deps struct {
	System *sysinfo.Reader
	// Processes는 시스템 정보 창의 프로세스 목록(관리자 전용)이다.
	Processes *sysinfo.Processes
	Storage   *storage.Reader
	Network   *network.Reader
	Hwmon     *temperature.Hwmon
	// Smart는 SMART 결과를 모든 세션이 함께 쓰는 캐시다. SMART·온도 애드온이 함께 쓴다.
	Smart *smart.Cache
}

// HostDeps는 호스트의 /proc, /sys 등을 읽는 입력이다. 만들 때 파일을 읽지 않는다.
func HostDeps() Deps {
	// 시스템 정보 창의 네트워크 합계는 네트워크 애드온과 같은 reader로 읽는다.
	net := network.NewReader()
	return Deps{
		System:    sysinfo.NewReader(net),
		Processes: sysinfo.NewProcesses(),
		Storage:   storage.NewReader(),
		Network:   net,
		Hwmon:     temperature.NewHwmon(),
		Smart:     smart.NewCache(0, 0),
	}
}

// New는 내장 애드온 레지스트리를 만든다. 목록은 컴파일 타임에 정해지므로
// 검사에 실패하면 프로그래밍 오류로 보고 panic한다(테스트가 먼저 잡는다).
func New(d Deps) *addons.Registry {
	r, err := addons.NewRegistry(
		sysinfo.New(d.System, d.Processes),
		storage.New(d.Storage),
		network.New(d.Network),
		temperature.New(d.Hwmon, d.Smart),
		containers.New(),
		smart.New(d.Smart),
		services.New(),
	)
	if err != nil {
		panic(err)
	}
	return r
}
