// Package services는 OMV 서비스 애드온이다. Services.getStatus(관리자)로 OMV 서비스(SSH, SMB/CIFS, NFS 등)의
// 사용 설정과 실행 상태를 가져온다.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"omv-desktop-dashboard/backend/internal/addons"
	"omv-desktop-dashboard/backend/internal/api"
)

// ID는 서비스 애드온의 id다.
const ID = "services"

// Source는 provider 이름이다.
const Source = "omv-services"

// Manifest는 서비스 애드온의 선언이다. Services.getStatus가 관리자 전용이므로 애드온도 관리자 전용이다.
var Manifest = api.Manifest{
	ID:    ID,
	Title: "apps.services",
	Icon:  "preferences-system-network-server",
	Views: api.AddonViews{
		// 크기는 브라우저 실측으로 정한다. 내용을 바꾸면 다시 잰다.
		Window: &api.WindowView{
			DefaultSize: api.Size{Width: 440, Height: 420},
			MinSize:     &api.Size{Width: 340, Height: 240},
		},
		Widget: &api.WidgetView{
			DefaultSize: api.GridSize{Cols: 3, Rows: 2},
			MinSize:     &api.GridSize{Cols: 2, Rows: 1},
			MaxSize:     &api.GridSize{Cols: 4, Rows: 5},
		},
	},
	Requires: api.Requirements{Admin: true},
	RPC:      api.RPCMethods{Read: []string{"Services.getStatus"}},
}

type provider struct{}

func (provider) Name() string               { return Source }
func (provider) Requires() api.Requirements { return api.Requirements{Admin: true} }

// New는 서비스 애드온을 반환한다.
func New() addons.Addon {
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{provider{}},
		Fetch: func(ctx context.Context, req addons.Request) (any, error) {
			raw, err := req.RPC.Call(ctx, "Services", "getStatus", nil)
			if err != nil {
				return nil, err
			}
			return ParseStatus(raw)
		},
	}
}

// flag는 OMV가 bool 대신 숫자·문자열로 줄 수 있는 참거짓 값이다(설정 DB 값을 그대로 넘기는 플러그인).
type flag bool

func (f *flag) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case bool:
		*f = flag(x)
	case float64:
		*f = flag(x != 0)
	case string:
		*f = flag(x == "1" || strings.EqualFold(x, "true") || strings.EqualFold(x, "yes"))
	default:
		*f = false
	}
	return nil
}

type statusRow struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Enabled flag   `json:"enabled"`
	Running flag   `json:"running"`
}

// ParseStatus는 Services.getStatus 응답({total, data})을 읽는다.
//
// 행은 IServiceStatus를 구현한 OMV 모듈마다 하나다. name은 대개 모듈 이름(ssh, samba 등)이지만
// compose 플러그인처럼 다른 값을 쓰는 모듈도 있다. title은 OMV가 번역한 문구라 표시에만 쓴다.
func ParseStatus(raw json.RawMessage) (api.ServicesResponse, error) {
	var res struct {
		Data []statusRow `json:"data"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return api.ServicesResponse{}, fmt.Errorf("services status: %w", err)
	}
	out := api.ServicesResponse{
		AddonSources: api.AddonSources{Sources: []string{Source}},
		Services:     []api.Service{},
	}
	for _, r := range res.Data {
		if r.Name == "" {
			continue
		}
		title := r.Title
		if title == "" {
			title = r.Name
		}
		out.Services = append(out.Services, api.Service{
			Name:       r.Name,
			Title:      title,
			Enabled:    bool(r.Enabled),
			Running:    bool(r.Running),
			Toggleable: toggleable(r.Name),
		})
	}
	slices.SortFunc(out.Services, func(a, b api.Service) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// toggleable은 대시보드에서 켜고 끌 수 있는 서비스인지다. 켜기·끄기는 다음 커밋에서 추가한다.
func toggleable(string) bool { return false }
