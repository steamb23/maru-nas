package network

import (
	"context"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

// ID는 네트워크 애드온의 id다.
const ID = "network"

// Manifest는 네트워크 애드온의 선언이다. /proc, /sys만 읽으므로 플러그인·역할·RPC가 필요 없다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.network",
	Icon:  "preferences-system-network-ethernet",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정한다. 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 520, Height: 400},
			MinSize:     &api.Size{Width: 360, Height: 240},
		},
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 2},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 4},
		},
	},
}

// New는 r을 provider로 쓰는 네트워크 애드온을 반환한다.
func New(r *Reader) addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{r},
		Fetch: func(context.Context, addons.Request) (any, error) {
			return r.Network()
		},
	}
}

// Name은 provider 이름이다.
func (*Reader) Name() string { return "proc" }

// Requires는 provider 조건이다. /proc/net/dev와 /sys/class/net은 권한 없이 읽힌다.
func (*Reader) Requires() api.Requirements { return api.Requirements{} }
