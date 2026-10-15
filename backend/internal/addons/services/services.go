// Package services는 OMV 서비스 애드온이다. Services.getStatus(관리자)로 OMV 서비스(SSH, SMB/CIFS, NFS 등)의
// 사용 설정과 실행 상태를 가져온다.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"onmaru/backend/internal/addons"
	"onmaru/backend/internal/api"
)

// ID는 서비스 애드온의 id다.
const ID = "services"

// Source는 provider 이름이다.
const Source = "omv-services"

// Manifest는 서비스 애드온의 선언이다. Services.getStatus가 관리자 전용이므로 애드온도 관리자 전용이다.
// 켜기·끄기는 adapters에 있는 서비스만 한다(서비스 설정의 enable을 바꾸고 OMV 설정을 적용).
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
	RPC: api.RPCMethods{
		Read: []string{
			"Services.getStatus", "Config.isDirty", "Exec.isRunning",
			"SSH.get", "SMB.getSettings", "NFS.getSettings", "Rsyncd.getSettings",
		},
		Write: []string{
			"Config.applyChangesBg",
			"SSH.set", "SMB.setSettings", "NFS.setSettings", "Rsyncd.setSettings",
		},
	},
	Actions: []api.ActionDecl{
		{Name: "enable", Admin: true},
		{Name: "disable", Admin: true, Confirm: true},
	},
}

// adapter는 서비스 하나의 설정 RPC다. 서비스마다 RPC 서비스·메서드 이름이 다르다.
type adapter struct {
	service, get, set string
	// fields는 set이 받는 속성이다(datamodels/rpc.<svc>.json의 set 스키마, 모두 필수). get 결과에서 이
	// 속성만 골라 보낸다. get은 다른 속성도 줄 수 있다(NFS.getSettings의 shares 등).
	fields []string
}

// adapters는 켜고 끌 수 있는 서비스다. 키는 Services.getStatus의 name(모듈 이름)이다.
// OMV 8의 engined/rpc/{ssh,smb,nfs,rsyncd}.inc와 datamodels를 보고 만들었다.
var adapters = map[string]adapter{
	"ssh": {service: "SSH", get: "get", set: "set", fields: []string{
		"enable", "port", "passwordauthentication", "pubkeyauthentication", "permitrootlogin",
		"tcpforwarding", "compression", "extraoptions",
	}},
	"samba": {service: "SMB", get: "getSettings", set: "setSettings", fields: []string{
		"enable", "workgroup", "serverstring", "loglevel", "usesendfile", "aio", "timeserver",
		"winssupport", "winsserver", "homesenable", "homesbrowseable", "extraoptions",
	}},
	// NFS.getSettings는 versions를 배열로 바꿔 주고 setSettings도 배열을 받는다.
	"nfs":    {service: "NFS", get: "getSettings", set: "setSettings", fields: []string{"enable", "versions"}},
	"rsyncd": {service: "Rsyncd", get: "getSettings", set: "setSettings", fields: []string{"enable", "port", "extraoptions"}},
}

// 상태는 사용자와 무관하므로(관리자 전용, 결과 같음) 세션 사이에 cacheTTL 동안 함께 쓴다.
// OMV가 모듈마다 systemctl을 실행하므로 탭·사용자마다 부르지 않게 하기 위해서다.
const (
	cacheTTL     = 5 * time.Second
	fetchTimeout = 20 * time.Second
)

// actionTimeout은 켜기·끄기 하나를 기다리는 시간이다. 설정 적용(Salt 배포)은 수십 초 걸릴 수 있다.
const actionTimeout = 3 * time.Minute

type provider struct{}

func (provider) Name() string               { return Source }
func (provider) Requires() api.Requirements { return api.Requirements{Admin: true} }

// New는 서비스 애드온을 반환한다. 부를 때마다 새 캐시를 만든다.
func New() addons.Addon {
	cache := addons.NewSharedCache[api.ServicesResponse](cacheTTL, fetchTimeout)
	return addons.Addon{
		Manifest:  Manifest,
		Providers: []addons.Provider{provider{}},
		Fetch: func(ctx context.Context, req addons.Request) (any, error) {
			return cache.Get(ctx, req.Refresh, func(ctx context.Context) (api.ServicesResponse, error) {
				raw, err := req.RPC.Call(ctx, "Services", "getStatus", nil)
				if err != nil {
					return api.ServicesResponse{}, err
				}
				return ParseStatus(raw)
			})
		},
		Actions: map[string]addons.Action{
			"enable":  {Timeout: actionTimeout, Run: toggle(true, cache)},
			"disable": {Timeout: actionTimeout, Run: toggle(false, cache)},
		},
	}
}

// toggle은 서비스를 켜거나 끄는 조작이다. OMV workbench에서 서비스 설정의 "사용"을 바꾸고 적용하는 것과 같다.
// 적용 대기 확인, 적용, 기다리기는 addons.ApplyChange가 하고(같은 백엔드의 다른 적용과 하나씩 실행), 여기서는 설정만 바꾼다.
//
//  1. get: 이미 원하는 값이면 바꾸지 않는다(적용도 하지 않음)
//  2. set: get 결과에서 set 스키마의 속성만 골라 enable만 바꿔 보낸다
//
// set이 실패하면 ErrActionFailed다. 설정을 바꾸기 시작했으면 끝난 뒤(실패해도) 상태 캐시를 비운다.
func toggle(enable bool, cache *addons.SharedCache[api.ServicesResponse]) func(context.Context, addons.ActionRequest) error {
	return func(ctx context.Context, req addons.ActionRequest) error {
		a, ok := adapters[req.Target]
		if !ok {
			return addons.ErrInvalidTarget
		}
		touched := false
		defer func() {
			if touched {
				cache.Invalidate()
			}
		}()
		return addons.ApplyChange(ctx, req.RPC, func(ctx context.Context) (bool, error) {
			var current map[string]json.RawMessage
			if err := addons.CallInto(ctx, req.RPC, a.service, a.get, nil, &current); err != nil {
				return false, err
			}
			var on flag
			if err := json.Unmarshal(current["enable"], &on); err != nil {
				return false, fmt.Errorf("%s.%s: enable: %w", a.service, a.get, err)
			}
			if bool(on) == enable {
				return false, nil
			}
			params := make(map[string]json.RawMessage, len(a.fields))
			for _, f := range a.fields {
				v, ok := current[f]
				if !ok {
					return false, fmt.Errorf("%s.%s: missing field %q", a.service, a.get, f)
				}
				params[f] = v
			}
			params["enable"] = json.RawMessage(fmt.Sprint(enable))
			touched = true
			if _, err := req.RPC.Call(ctx, a.service, a.set, params); err != nil {
				// OMV가 설정을 거절했다(SMB의 호스트 이름 길이 제한 등).
				return false, fmt.Errorf("%w: %w", addons.ErrActionFailed, err)
			}
			return true, nil
		})
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

// toggleable은 대시보드에서 켜고 끌 수 있는 서비스인지다.
func toggleable(name string) bool {
	_, ok := adapters[name]
	return ok
}
