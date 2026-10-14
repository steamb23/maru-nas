// Package smart는 SMART 애드온이다. OMV 코어의 Smart.getList(관리자 전용)를 부르고, 결과를 모든
// 세션이 함께 쓰는 캐시(Cache)에 보관한다. 온도 애드온도 이 캐시의 디스크 온도를 읽는다.
package smart

import (
	"context"
	"time"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

// ID는 SMART 애드온의 id다.
const ID = "smart"

// Source는 provider 이름이다.
const Source = "omv-smart"

// Manifest는 SMART 애드온의 선언이다. Smart.getList는 관리자 전용이다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.smart",
	Icon:  "org.gnome.DiskUtility",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정한다. 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 520, Height: 400},
			MinSize:     &api.Size{Width: 360, Height: 240},
		},
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 1},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 4},
		},
	},
	Requires: api.Requirements{Admin: true},
	RPC:      api.RPCMethods{Read: []string{"Smart.getList"}},
}

// Provider는 SMART 캐시를 provider로 선언한다. 온도 애드온도 같은 이름으로 쓴다(관리자 전용).
type Provider struct{}

func (Provider) Name() string               { return Source }
func (Provider) Requires() api.Requirements { return api.Requirements{Admin: true} }

// New는 cache를 쓰는 SMART 애드온을 반환한다.
func New(cache *Cache) addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{Provider{}},
		Fetch: func(ctx context.Context, req addons.Request) (any, error) {
			snap, err := cache.Get(ctx, req.RPC, req.Refresh)
			if err != nil {
				return nil, err
			}
			return api.SmartResponse{
				AddonSources: api.AddonSources{Sources: []string{Source}},
				CheckedAt:    snap.CheckedAt.Format(time.RFC3339),
				Disks:        snap.Disks,
			}, nil
		},
	}
}
