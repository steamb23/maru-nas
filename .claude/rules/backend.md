---
paths:
  - "backend/**"
---

# 백엔드 (Go)

- 표준 라이브러리(`net/http`)만 쓴다. 런타임 외부 의존성을 두지 않는다. tygo는 go.mod의 `tool` 지시어로 고정한 빌드 도구다
- `-listen`(`ONMARU_LISTEN`, 기본 `127.0.0.1:8090`)은 루프백 TCP 주소 또는 `unix:/절대/경로`를 받는다. 루프백이 아닌 TCP 주소는 `-allow-non-loopback`(`ONMARU_ALLOW_NON_LOOPBACK`)을 명시했을 때만 허용하고 시작 시 경고를 남긴다. 기본값이 거부인 동작을 유지한다
  - Unix 소켓은 권한 `0660`으로 만들고, 남은 소켓 파일은 지우되 소켓이 아닌 파일이나 사용 중인 소켓은 건드리지 않는다(`cmd/onmaru` `listenOn`)
  - 프록시를 거치지 않은 요청은 `X-Forwarded-*`를 위조할 수 있다. 루프백·소켓 수신은 이 헤더를 믿을 근거이므로 다른 경로를 기본으로 열지 않는다
- `-omv-url`(`ONMARU_OMV_URL`, 기본 `http://127.0.0.1`)은 루프백만 허용한다(OMV 세션 ID가 네트워크로 나가지 않게). 이 제한은 완화하지 않는다
- 대시보드 사용자 설정(위젯 배치, 바로가기 등)은 `internal/prefs`가 상태 디렉터리(`-state-dir`, `ONMARU_STATE_DIR`, 없으면 systemd의 `STATE_DIRECTORY`)에 사용자별 JSON으로 저장한다. 비우면 메모리에만 둔다. 대시보드 자신의 상태만 두며 OMV 설정이나 시스템 파일은 쓰지 않는다. `make dev`는 `.dev-state/real`, `make dev-fake`는 `.dev-state/fake`를 쓴다(커밋 안 함)
  - 설정 변경 API는 부분별로 둔다(`PUT /api/preferences/widgets`, `PUT /api/preferences/shortcuts`). 서로 덮어쓰지 않게 하며, 저장소는 설정 파일을 읽고 쓰는 동안 잠가 한쪽 저장이 다른 쪽을 지우지 않게 한다
- `cmd/fakeomv`와 `internal/omv/omvfake`는 개발·테스트용이다. 운영 바이너리(`cmd/onmaru`)에서 import하지 않는다
- `/api/*` 경로는 대시보드 루트 기준이다. 대시보드를 경로 접두사 아래에 두면 리버스 프록시가 접두사를 제거한다. 쿠키 Path는 `-cookie-path`(기본 `/`)
- `internal/web/static/`에는 `.gitkeep`만 커밋한다. `dist/`는 빌드 결과다
- `internal/api`는 TS 타입 생성의 원본이다. 바꾸면 `make types`를 실행하고 `frontend/src/api/types.gen.ts`도 함께 커밋한다
- 주기 요청·세션 확인으로 부르는 OMV RPC는 `options.updatelastaccess=false`로 호출해 OMV 세션을 연장하지 않는다. 세션 연장은 로그인과 사용자 활동 신호만 한다(`auth-omv.md`의 "세션 만료")
- 애드온 계약은 `internal/addons`, 내장 애드온은 `internal/addons/<id>/`, 정적 목록은 `internal/addons/builtin`이다. manifest를 바꾸면 `make types`로 `frontend/src/addons/manifests.gen.ts`도 다시 만든다(`.claude/docs/addons.md`)
- OMV RPC는 `/api/rpc` 중계든 provider 내부 호출이든 manifest 허용 목록(`Registry.Classify`)과 `addons.WritesEnabled`를 거친다
- 쓰기 RPC는 애드온 조작(`Addon.Actions`, `POST /api/addons/{id}/actions/{action}`)으로만 부른다. `/api/rpc`와 `Fetch`는 쓰기를 거부한다. 조작은 브라우저가 보낸 대상을 검사하고 OMV 파라미터를 직접 만든다(OMV에는 파라미터를 검사하지 않고 셸 명령을 만드는 메서드가 있다). 백그라운드 작업은 `addons.WaitBg`로 기다린다(`.claude/docs/addons.md`의 "조작")
- 내장 애드온의 현황 데이터는 `Addon.Fetch` → `GET /api/addons/{id}`로 준다. provider는 `Request.RPC`(그 애드온이 선언한 메서드만, `updatelastaccess=false`)로 OMV를 부르고, 일부 출처 실패는 응답의 `failedSources`에 담는다(`.claude/docs/addons.md`의 "애드온 데이터 API")
- `Smart.getList`는 대기 중인 디스크를 깨운다. 주기적으로 부르지 말고 `smart.Cache`를 거친다(온도는 `Peek`만)
- OMV가 매번 프로세스를 실행하는 느린 읽기 RPC(`Services.getStatus`, `Compose.getContainers`)는 `addons.SharedCache`로 세션 사이에 짧게(5초) 함께 쓴다. 결과가 사용자와 무관한 RPC에만 쓰고, `Request.Refresh`면 건너뛰며, 상태를 바꾸는 조작은 끝난 뒤(실패해도) `Invalidate`한다
- `/proc`·`/sys`를 읽는 코드는 파서(`io.Reader` 입력)와 읽기(`fs.FS` 주입)를 나눠 `testdata/`의 합성 픽스처로 테스트한다(`internal/addons/sysinfo` 참고)

## 테스트

- 서버 테스트는 `internal/server/server_test.go`의 `newEnv`·`browser`를 쓴다. `omvfake`를 `httptest`로 띄우고 실제 `omv.Client`와 서버 핸들러를 연결한다
- 시계는 주입한다. OMV 세션 만료는 `omvfake.Expire(user)`로 흉내 낸다
- 새 RPC를 쓰는 기능은 `omvfake`에 그 RPC의 합성 응답을 추가해 같은 방식으로 테스트한다. 응답 형식은 OMV 소스(`/usr/share/openmediavault/engined/rpc/`)를 읽고 만들며 NAS 실제 출력을 넣지 않는다
- `omvfake`는 인증된 모든 호출에서 User-Agent를 검사한다(실제 OMV보다 엄격). 백엔드가 고정 User-Agent를 지키는지 확인하기 위해서다
