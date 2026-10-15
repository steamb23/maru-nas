package addons

import (
	"context"
	"fmt"
	"net/http"

	"onmaru/backend/internal/api"
)

// applyLock은 이 백엔드의 OMV 설정 변경·적용(ApplyChange)을 하나씩 실행하게 한다. 적용이 끝나기 전에는 OMV가
// 적용 대기로 표시하므로, 겹쳐 실행하면 뒤의 요청이 앞 요청의 변경 때문에 config_dirty로 거절된다. 뒤의 요청은
// 앞의 것이 끝날 때까지 기다린다(기다리는 시간도 호출하는 쪽의 제한 시간에 들어간다). 이 프로세스 안의 변경만
// 막으며, 그사이 workbench에서 만든 변경은 아래 경쟁 조건과 같다. 서비스 켜기·끄기와 프로필 설명 저장이 함께 쓴다.
var applyLock = make(chan struct{}, 1)

// ApplyChange는 OMV 설정을 바꾸고 바로 적용한다(관리자 전용 RPC를 쓴다). OMV workbench에서 설정을 바꾸고
// "적용"을 누르는 것과 같다.
//
//  1. Config.isDirty: OMV에 적용하지 않은 변경이 있으면 아무것도 바꾸지 않고 거절한다(409 config_dirty).
//     적용은 표시된 모든 모듈을 배포하므로, 사용자가 workbench에서 만들다 만 변경을 대신 배포하지 않기 위해서다
//  2. change: 설정을 바꾼다. 바꿀 것이 없으면(이미 원하는 값) false를 돌려주고, 그러면 적용하지 않는다.
//     change의 오류는 그대로 돌려준다(서비스 켜기·끄기는 OMV가 저장을 거절하면 ErrActionFailed로 감싼다)
//  3. Config.applyChangesBg({modules: [], force: false}): 1에서 비어 있음을 확인했으므로 이번 변경과 OMV가
//     함께 표시한 모듈만 배포된다. 1과 2 사이에 다른 곳에서 바뀐 변경이 섞일 수 있는 것은 받아들인다
//  4. WaitBg로 끝날 때까지 기다린다
//
// 3·4가 실패하면 ErrActionFailed다. 이때 설정은 바뀐 채 적용 대기로 남는다(OMV workbench에 적용 대기 표시가 뜬다).
func ApplyChange(ctx context.Context, rpc RPCCaller, change func(context.Context) (bool, error)) error {
	select {
	case applyLock <- struct{}{}:
		defer func() { <-applyLock }()
	case <-ctx.Done():
		return ErrActionTimeout
	}
	var dirty bool
	if err := CallInto(ctx, rpc, "Config", "isDirty", map[string]any{}, &dirty); err != nil {
		return err
	}
	if dirty {
		return &ActionError{
			Status:  http.StatusConflict,
			Code:    api.ErrConfigDirty,
			Message: "omv has pending configuration changes",
		}
	}
	changed, err := change(ctx)
	if err != nil || !changed {
		return err
	}
	var filename string
	apply := map[string]any{"modules": []string{}, "force": false}
	if err := CallInto(ctx, rpc, "Config", "applyChangesBg", apply, &filename); err != nil {
		return fmt.Errorf("%w: %w", ErrActionFailed, err)
	}
	return WaitBg(ctx, rpc, filename, BgPollInterval)
}
