package temperature

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"os"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

// ID는 온도 애드온의 id다.
const ID = "temperature"

// provider 이름.
const (
	SourceHwmon   = "hwmon"
	SourceCPUTemp = "omv-cputemp"
)

const cpuTempPlugin = "openmediavault-cputemp"

// Manifest는 온도 애드온의 선언이다. hwmon은 권한 없이 읽히므로 애드온 자체에는 조건이 없다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.temperature",
	Icon:  "thermal-monitor",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정한다. 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 440, Height: 420},
			MinSize:     &api.Size{Width: 320, Height: 240},
		},
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 2, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 1},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 4},
		},
	},
	RPC: api.RPCMethods{Read: []string{"CpuTemp.get"}},
}

// Hwmon은 /sys/class/hwmon을 읽는 provider다.
type Hwmon struct {
	fs fs.FS
}

// NewHwmon은 호스트의 /sys/class/hwmon을 읽는 provider를 반환한다.
func NewHwmon() *Hwmon { return NewHwmonFS(os.DirFS("/sys/class/hwmon")) }

// NewHwmonFS는 hw(/sys/class/hwmon 형태)를 읽는 provider를 반환한다.
func NewHwmonFS(hw fs.FS) *Hwmon { return &Hwmon{fs: hw} }

func (*Hwmon) Name() string               { return SourceHwmon }
func (*Hwmon) Requires() api.Requirements { return api.Requirements{} }

// cpuTemp는 openmediavault-cputemp 플러그인의 CpuTemp.get(모든 사용자)이다. 사용자가 플러그인에
// 등록한 스크립트 값 하나를 주므로, hwmon에 CPU 센서가 없을 때만 쓴다(같은 센서 중복 방지).
type cpuTemp struct{}

func (cpuTemp) Name() string { return SourceCPUTemp }
func (cpuTemp) Requires() api.Requirements {
	return api.Requirements{Plugins: []string{cpuTempPlugin}}
}

// New는 온도 애드온을 반환한다.
func New(hw *Hwmon) addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{hw, cpuTemp{}},
		Fetch: func(ctx context.Context, req addons.Request) (any, error) {
			return fetch(ctx, req, hw)
		},
	}
}

func fetch(ctx context.Context, req addons.Request, hw *Hwmon) (api.TemperatureResponse, error) {
	res := api.TemperatureResponse{
		AddonSources: api.AddonSources{Sources: []string{}},
		Groups:       []api.SensorGroup{},
	}
	var errs []error
	fail := func(source string, err error) {
		res.FailedSources = append(res.FailedSources, source)
		errs = append(errs, err)
	}

	if req.Has(SourceHwmon) {
		if groups, err := ReadHwmon(hw.fs); err != nil {
			fail(SourceHwmon, err)
		} else {
			res.Sources = append(res.Sources, SourceHwmon)
			res.Groups = append(res.Groups, groups...)
		}
	}
	if req.Has(SourceCPUTemp) && !HasCPU(res.Groups) {
		var out struct {
			CPUTemp float64 `json:"cputemp"`
		}
		if err := addons.CallInto(ctx, req.RPC, "CpuTemp", "get", nil, &out); err != nil {
			fail(SourceCPUTemp, err)
		} else if c := out.CPUTemp; c > 0 && c < 150 {
			// 스크립트를 설정하지 않았으면 0을 준다.
			res.Sources = append(res.Sources, SourceCPUTemp)
			res.Groups = append(res.Groups, api.SensorGroup{
				ID: SourceCPUTemp, Kind: api.SensorCPU, Name: "cputemp", Source: SourceCPUTemp,
				Sensors: []api.Sensor{{Celsius: math.Round(c*10) / 10}},
			})
		}
	}
	SortGroups(res.Groups)
	// 쓸 수 있는 출처가 모두 실패했을 때만 요청 전체를 실패로 본다.
	if len(res.Sources) == 0 && len(errs) > 0 {
		return res, errors.Join(errs...)
	}
	return res, nil
}
