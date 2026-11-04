// Package smart는 SMART 애드온이다. OMV 코어의 Smart.getList(관리자 전용)를 부르고, 결과를 모든
// 세션이 함께 쓰는 캐시(Cache)에 보관한다. 온도 애드온도 이 캐시의 디스크 온도를 읽는다.
// 디스크 하나의 상세(detail)와 smartctl 원문(raw)은 조작으로 묻는다(detail.go).
package smart

import (
	"context"
	"math"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

// ID는 SMART 애드온의 id다.
const ID = "smart"

// Source는 provider 이름이다.
const Source = "omv-smart"

// Manifest는 SMART 애드온의 선언이다. Smart 메서드는 모두 관리자 전용이다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.smart",
	Icon:  "org.gnome.DiskUtility",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정한다. 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 900, Height: 580},
			MinSize:     &api.Size{Width: 360, Height: 240},
		},
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 1},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 4},
		},
	},
	Requires: api.Requirements{Admin: true},
	RPC: api.RPCMethods{
		Read: []string{"Smart.getList"},
		// 상세 메서드는 데이터를 읽지만 smartctl을 실행해 잠든 디스크를 깨우므로, 브라우저가 고른 장치 파일로
		// /api/rpc가 부르지 못하게 쓰기로 선언해 조작으로만 부른다.
		Write: []string{"Smart.getInformation", "Smart.getAttributes", "Smart.getSelfTestLogs", "Smart.getExtendedInformation"},
	},
	Actions: []api.ActionDecl{
		{Name: "detail", Admin: true},
		{Name: "raw", Admin: true},
	},
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
				AddonSources:     api.AddonSources{Sources: []string{Source}},
				CheckedAt:        snap.CheckedAt.Format(time.RFC3339),
				RefreshInSeconds: int(math.Ceil(cache.RefreshIn(snap).Seconds())),
				Disks:            snap.Disks,
			}, nil
		},
		Actions: map[string]addons.Action{
			"detail": {Timeout: queryTimeout, Query: detailQuery(cache)},
			"raw":    {Timeout: queryTimeout, Query: rawQuery(cache)},
		},
	}
}
