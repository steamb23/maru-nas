package sysinfo

import (
	"context"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
)

// ID는 시스템 정보 애드온의 id다.
const ID = "system-info"

// Manifest는 시스템 정보 애드온의 선언이다. /proc, /sys만 읽으므로 플러그인·RPC가 필요 없다.
// 요약(개요)은 누구나 보고, 프로세스 목록은 provider(proc-processes)가 관리자에게만 준다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.systemInfo",
	Icon:  "utilities-system-monitor",
	Views: api.AddonViews{
		// 개요(그래프 카드)와 프로세스 표. 기본 크기는 관리자의 개요(탭 막대, 전체 사용률 그래프, 카드 2×2)가
		// 스크롤 없이 들어가는 높이다(Chrome 실측 511px, ko·en 같음, 10px 단위로 올림). 최소 크기에서는 카드가
		// 한 열이 되어 스크롤하고, 프로세스 표는 상태·사용자 열을 숨긴다. 창 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 720, Height: 520},
			MinSize:     &api.Size{Width: 440, Height: 360},
		},
		// CPU·RAM·가동 시간 요약. 위젯 내용을 바꾸면 브라우저에서 다시 잰다.
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 2},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 3},
		},
	},
}

// New는 r(요약·누적 카운터)과 procs(프로세스 목록)를 provider로 쓰는 시스템 정보 애드온을 반환한다.
// 위젯은 애드온 계약 이전부터 있던 /api/system(r.System)을, 창은 GET /api/addons/system-info를 쓴다.
func New(r *Reader, procs *Processes) addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{r, procs},
		Fetch: func(_ context.Context, req addons.Request) (any, error) {
			res, err := r.Stats()
			if err != nil {
				return nil, err
			}
			if req.Has(SourceProcesses) {
				if list, err := procs.List(); err != nil {
					res.FailedSources = append(res.FailedSources, SourceProcesses)
				} else {
					res.Sources = append(res.Sources, SourceProcesses)
					res.Processes = list
				}
			}
			return res, nil
		},
	}
}

// Name은 provider 이름이다.
func (*Reader) Name() string { return "proc" }

// Requires는 provider 조건이다. /proc와 /sys는 권한 없이 읽힌다.
func (*Reader) Requires() api.Requirements { return api.Requirements{} }
