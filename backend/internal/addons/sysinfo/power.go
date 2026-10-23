package sysinfo

import (
	"context"
	"time"

	"maru/backend/internal/addons"
	"maru/backend/internal/api"
)

// 전원 조작(시스템 종료, 다시 시작, 절전)과 절전 방식 조회. OMV 8의 engined/rpc/system.inc와 powermgmt.inc를
// 보고 만들었고, 모두 관리자 전용이다.
//
//   - System.shutdown·reboot·standby는 {delay}를 받는다. delay가 0보다 크면 engined가 fork해 바로 응답하고,
//     자식이 그만큼 기다린 뒤 systemctl poweroff·reboot·(대기 방식)을 실행한다
//   - System.standby는 OMV 전원 관리의 대기 방식(standbymode: poweroff, suspend, hibernate, suspendhybrid)을
//     따르고, 그 방식을 지원하지 않으면 전원을 끈다. 그래서 프론트가 PowerMgmt.get과
//     PowerMgmt.enumerateStandbyModes로 미리 확인한다

// SourcePower는 절전 방식을 읽는 provider 이름이다(관리자 전용).
const SourcePower = "omv-power"

// powerDelaySeconds는 OMV에 넘기는 delay다. 조작 응답(204)이 브라우저에 닿은 뒤에 시스템이 내려가게 한다.
const powerDelaySeconds = 2

// powerTimeout은 전원 조작 하나를 기다리는 시간이다. delay가 있으면 OMV가 바로 돌려준다.
const powerTimeout = 30 * time.Second

// powerProvider는 OMV 전원 관리 설정을 읽는다. PowerMgmt RPC가 관리자 전용이다.
type powerProvider struct{}

func (powerProvider) Name() string               { return SourcePower }
func (powerProvider) Requires() api.Requirements { return api.Requirements{Admin: true} }

// powerAction은 System.<method>를 부르는 조작이다. 대상이 없는 조작이라 빈 대상만 받는다.
func powerAction(method string) addons.Action {
	return addons.Action{
		Timeout: powerTimeout,
		Run: func(ctx context.Context, req addons.ActionRequest) error {
			if req.Target != "" {
				return addons.ErrInvalidTarget
			}
			_, err := req.RPC.Call(ctx, "System", method, map[string]int{"delay": powerDelaySeconds})
			return err
		},
	}
}

// readPower는 OMV 전원 관리의 대기 방식과 이 기기가 지원하는 대기 방식을 읽는다.
func readPower(ctx context.Context, rpc addons.RPCCaller) (*api.PowerInfo, error) {
	var settings struct {
		StandbyMode string `json:"standbymode"`
	}
	if err := addons.CallInto(ctx, rpc, "PowerMgmt", "get", nil, &settings); err != nil {
		return nil, err
	}
	var modes []string
	if err := addons.CallInto(ctx, rpc, "PowerMgmt", "enumerateStandbyModes", nil, &modes); err != nil {
		return nil, err
	}
	if modes == nil {
		modes = []string{}
	}
	return &api.PowerInfo{StandbyMode: settings.StandbyMode, StandbyModes: modes}, nil
}
