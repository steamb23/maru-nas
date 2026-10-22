package storage

import (
	"context"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

// ID는 디스크 용량 애드온의 id다.
const ID = "storage"

// Manifest는 디스크 용량 애드온의 선언이다. /proc와 statfs만 쓰므로 플러그인·역할·RPC가 필요 없다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.storage",
	Icon:  "drive-harddisk",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정했다(위젯 2×2에 3줄, 창은 넘치면 스크롤). 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 560, Height: 400},
			MinSize:     &api.Size{Width: 360, Height: 240},
		},
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 2},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 4},
		},
	},
}

// New는 r을 provider로 쓰는 디스크 용량 애드온을 반환한다.
func New(r *Reader) addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{r},
		Fetch: func(context.Context, addons.Request) (any, error) {
			return r.Storage()
		},
	}
}

// Name은 provider 이름이다.
func (*Reader) Name() string { return "proc" }

// Requires는 provider 조건이다. /proc/mounts와 statfs는 권한 없이 읽힌다.
func (*Reader) Requires() api.Requirements { return api.Requirements{} }
