// SPDX-License-Identifier: MIT

// hello-backend는 예제 애드온 패키지 sdk/examples/hello-backend의 백엔드다.
//
//   - 데이터(GET /v1/data): OMV에서 읽은 시스템 정보(System.getInformation, 관리자에게는 성능 통계 설정 PerfStats.get도)와
//     백엔드가 스스로 두는 값(지금까지 답한 데이터 요청 수, 관리자에게는 마지막으로 바꾼 기록)
//   - 조작 perfstats(관리자, 확인): OMV의 성능 통계를 끄거나 켠다. Host.Apply로 바꾸고 바로 적용하므로 적용 대기로 남지
//     않는다. 같은 창에서 다시 되돌릴 수 있다
//
// 빌드와 실행은 sdk/examples/hello-backend/README.md에 있다.
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

// actionPerfStats는 manifest의 조작 이름이다.
const actionPerfStats = "perfstats"

func main() {
	st, err := openStore(maruaddon.StateDir())
	if err != nil {
		log.Fatal(err)
	}
	if err := maruaddon.Serve(newHandler(&backend{state: st, now: time.Now})); err != nil {
		log.Fatal(err)
	}
}

// backend는 요청 처리기가 함께 쓰는 상태다. 처리기는 여러 고루틴에서 동시에 불린다.
type backend struct {
	state *store
	now   func() time.Time
}

func newHandler(b *backend) maruaddon.Handler {
	return maruaddon.Handler{
		Data:    b.data,
		Actions: map[string]maruaddon.HandlerFunc{actionPerfStats: b.setPerfStats},
	}
}

// systemInfo는 System.getInformation 결과 가운데 이 예제가 쓰는 부분이다. OMV는 일반 사용자에게 시각과 호스트
// 이름만 주고, 버전 등은 관리자에게만 준다.
type systemInfo struct {
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
}

// perfStats는 PerfStats.get 결과이자 PerfStats.set 파라미터다(OMV의 rpc.perfstats.set은 enable 하나다).
type perfStats struct {
	Enable bool `json:"enable"`
}

// dataReply는 프레임의 app.data()가 받는 값이다.
type dataReply struct {
	Hostname string `json:"hostname"`
	Version  string `json:"version,omitempty"`
	// PerfStats는 관리자에게만 준다(PerfStats.get은 관리자 전용 메서드다).
	PerfStats *perfStats `json:"perfStats,omitempty"`
	// DataRequests는 이 백엔드가 답한 데이터 요청 수다. 백엔드는 모든 사용자가 함께 쓰므로 모두에게 같은 수다.
	DataRequests int64 `json:"dataRequests"`
	// Persistent는 값을 데이터 디렉터리에 두는지(아니면 메모리에만 있어 다시 시작하면 처음부터 센다)다.
	Persistent bool `json:"persistent"`
	// LastChange는 누가 언제 성능 통계를 바꿨는지다. 다른 관리자의 이름이 들어 있으므로 관리자에게만 준다.
	LastChange *change `json:"lastChange,omitempty"`
}

// data는 데이터 요청이다. 읽기 토큰이라 manifest의 rpc.read에 선언한 메서드만 부를 수 있다.
func (b *backend) data(ctx context.Context, r *maruaddon.Request) (any, error) {
	var info systemInfo
	if err := r.Host.RPCInto(ctx, "System", "getInformation", nil, &info); err != nil {
		return nil, err
	}
	st := b.state.update(func(s *savedState) { s.DataRequests++ })
	reply := dataReply{
		Hostname:     info.Hostname,
		Version:      info.Version,
		DataRequests: st.DataRequests,
		Persistent:   b.state.persistent(),
	}
	// 관리자 전용 메서드를 일반 사용자 요청에서 부르면 OMV가 거절하고(maru가 502로 바꾼다) 화면 전체가 실패한다.
	// 그래서 r.Admin으로 가른다.
	if r.Admin {
		var ps perfStats
		if err := r.Host.RPCInto(ctx, "PerfStats", "get", nil, &ps); err != nil {
			return nil, err
		}
		reply.PerfStats = &ps
		reply.LastChange = st.LastChange
	}
	return reply, nil
}

// perfStatsInput은 조작 perfstats의 입력이다. 켤지 끌지를 프레임이 정하므로 같은 조작으로 되돌릴 수 있다.
type perfStatsInput struct {
	Enable *bool `json:"enable"`
}

// setPerfStats는 OMV의 성능 통계를 끄거나 켠다. manifest가 관리자 조작(admin)이고 확인(confirm)을 요구하므로 maru가
// 관리자인지 보고 호스트가 실행할 때마다 먼저 묻는다. 쓰기를 선언한 패키지라 세션에 비밀번호 재확인(root 권한 상승
// 정책이 2차 인증이면 2차 인증)이 없으면 maru가 첫 실행 전에 한 번 받는다. 보증 수준은 세션이 끝날 때까지 남는다.
// 결과가 없는 조작이라 (nil, nil)로 204를 돌려주고, 프레임은 데이터를 다시 읽어 바뀐 값을 보인다. 이미 원하는 값이면
// 바꾸지도 적용하지도 않는다.
func (b *backend) setPerfStats(ctx context.Context, r *maruaddon.Request) (any, error) {
	var in perfStatsInput
	if err := r.DecodeInput(&in); err != nil {
		return nil, err
	}
	if in.Enable == nil {
		return nil, maruaddon.NewError(http.StatusBadRequest, "bad_input", "input.enable (true or false) is required")
	}
	var cur perfStats
	if err := r.Host.RPCInto(ctx, "PerfStats", "get", nil, &cur); err != nil {
		return nil, err
	}
	if cur.Enable == *in.Enable {
		return nil, nil
	}
	// Apply는 쓰기를 부른 뒤 OMV의 "변경 적용"까지 기다린다. Host.RPC로 쓰기만 부르면 적용 대기로 남는다.
	_, err := r.Host.Apply(ctx, maruaddon.Call{Service: "PerfStats", Method: "set", Params: perfStats{Enable: *in.Enable}})
	if maruaddon.IsCode(err, "config_dirty") {
		// 남이 만든 적용 대기를 대신 배포하지 않게 maru가 거절했다. 그대로 돌려주면 500(maru는 502)이 되므로 프레임이
		// 알아볼 코드로 바꾼다.
		return nil, maruaddon.NewError(http.StatusConflict, "config_dirty",
			"OMV has pending configuration changes; apply or revert them in the OMV workbench first")
	}
	if err != nil {
		return nil, err
	}
	b.state.update(func(s *savedState) {
		s.LastChange = &change{User: r.User, At: b.now().UTC().Truncate(time.Second), Enable: *in.Enable}
	})
	return nil, nil
}
