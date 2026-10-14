package sysinfo

import (
	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
)

// ID는 시스템 정보 애드온의 id다.
const ID = "system-info"

// Manifest는 시스템 정보 애드온의 선언이다. /proc만 읽으므로 플러그인·역할·RPC가 필요 없다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.systemInfo",
	Icon:  "utilities-system-monitor",
	Views: api.AddonViews{
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 420, Height: 360},
			// 평상시 내용이 스크롤 없이 들어가는 높이. Chrome 실측 265px을 10px 단위로 올렸다.
			// "연결 끊김" 배너(실측 311px 필요)는 예외 상황이므로 최소 크기에서는 스크롤을 허용한다.
			// 창 내용을 바꾸면 다시 잰다.
			MinSize: &api.Size{Width: 320, Height: 270},
		},
		// CPU·RAM·가동 시간 요약. 위젯 내용을 바꾸면 브라우저에서 다시 잰다.
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 2},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 3},
		},
	},
}

// New는 r을 provider로 쓰는 시스템 정보 애드온을 반환한다.
func New(r *Reader) addons.Addon {
	return addons.Addon{Manifest: Manifest, Providers: []addons.Provider{r}}
}

// Name은 provider 이름이다.
func (*Reader) Name() string { return "proc" }

// Requires는 provider 조건이다. /proc는 권한 없이 읽힌다.
func (*Reader) Requires() api.Requirements { return api.Requirements{} }
