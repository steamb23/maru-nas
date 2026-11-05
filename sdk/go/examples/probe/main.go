// SPDX-License-Identifier: MIT

// probe는 시험용 애드온 패키지 sdk/testdata/probe의 백엔드다. 시험 도구이며 예제가 아니다.
//
// 관리자 조작 run이 패키지 백엔드가 하지 못해야 하는 일(maru 상태 디렉터리·도우미 소켓·D-Bus·resolved 읽기, 공유
// 폴더·홈 목록, 루프백·LAN·인터넷 연결, 토큰 없는 호스트 호출)을 하나씩 시도하고 결과 표를 돌려준다. 표에는 닿았는지와
// 오류 종류만 담고 내용은 담지 않는다. 설치기가 켠 systemd 유닛(동적 uid, 네트워크 단계 4개)에서 샌드박스가 막는지
// 확인하는 데 쓴다(M3의 VM 확인). 샌드박스가 없는 개발 서버에서는 대부분 닿는 것이 정상이다.
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/steamb23/maru-nas/sdk/go/maruaddon"
)

func main() {
	if err := maruaddon.Serve(newHandler(osEnv(), hostSocket())); err != nil {
		log.Fatal(err)
	}
}

// hostSocket은 SDK가 쓰는 호스트 소켓 경로다(MARU_ADDON_HOST, 없으면 유닛 안의 기본 경로).
func hostSocket() string {
	if p := os.Getenv("MARU_ADDON_HOST"); p != "" {
		return p
	}
	return maruaddon.DefaultHostSocket
}

func newHandler(e env, host string) maruaddon.Handler {
	return maruaddon.Handler{Actions: map[string]maruaddon.HandlerFunc{
		"run": func(ctx context.Context, r *maruaddon.Request) (any, error) {
			// maru가 manifest의 admin으로 이미 막지만, 시험 도구라 백엔드도 한 번 더 본다.
			if !r.Admin {
				return nil, maruaddon.NewError(http.StatusUnprocessableEntity, "admin_required", "the probe is for administrators")
			}
			var in probeInput
			if err := r.DecodeInput(&in); err != nil {
				return nil, err
			}
			if err := in.validate(); err != nil {
				return nil, maruaddon.NewError(http.StatusBadRequest, "bad_input", err.Error())
			}
			// 누가 실행했는지만 남긴다(결과의 내용은 표에도 로그에도 없다).
			log.Printf("probe: run by %q, network %v", r.User, in.Network)
			return e.run(ctx, in, host), nil
		},
	}}
}
