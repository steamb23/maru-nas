---
paths:
  - "backend/**"
---

# 백엔드 (Go)

- 표준 라이브러리(`net/http`)만 쓴다. 런타임 외부 의존성을 두지 않는다. tygo는 go.mod의 `tool` 지시어로 고정한 빌드 도구다
- 주소 플래그는 루프백만 허용한다: `-listen`(`OMVDESK_LISTEN`, 기본 `127.0.0.1:8090`), `-omv-url`(`OMVDESK_OMV_URL`, 기본 `http://127.0.0.1`). 루프백이 아닌 값은 거부하는 동작을 유지한다
- `cmd/fakeomv`와 `internal/omv/omvfake`는 개발·테스트용이다. 운영 바이너리(`cmd/omvdesk`)에서 import하지 않는다
- `/api/*` 경로는 대시보드 루트 기준이다. `/desk` 접두사는 리버스 프록시(Vite·Caddy)가 제거한다. 쿠키 Path는 `-cookie-path`(기본 `/desk`)
- `internal/web/static/`에는 `.gitkeep`만 커밋한다. `dist/`는 빌드 결과다
- `internal/api`는 TS 타입 생성의 원본이다. 바꾸면 `make types`를 실행하고 `frontend/src/api/types.gen.ts`도 함께 커밋한다
- 주기 요청·세션 확인으로 부르는 OMV RPC는 `options.updatelastaccess=false`로 호출해 OMV 세션을 연장하지 않는다. 세션 연장은 로그인과 사용자 활동 신호만 한다(`auth-omv.md`의 "세션 만료")
- 애드온 계약은 `internal/addons`, 내장 애드온은 `internal/addons/<id>/`, 정적 목록은 `internal/addons/builtin`이다. manifest를 바꾸면 `make types`로 `frontend/src/addons/manifests.gen.ts`도 다시 만든다(`docs/addons.md`)
- OMV RPC는 `/api/rpc` 중계든 provider 내부 호출이든 manifest 허용 목록(`Registry.Classify`)과 `addons.WritesEnabled`를 거친다
- `/proc`·`/sys`를 읽는 코드는 파서(`io.Reader` 입력)와 읽기(`fs.FS` 주입)를 나눠 `testdata/`의 합성 픽스처로 테스트한다(`internal/addons/sysinfo` 참고)

## 테스트

- 서버 테스트는 `internal/server/server_test.go`의 `newEnv`·`browser`를 쓴다. `omvfake`를 `httptest`로 띄우고 실제 `omv.Client`와 서버 핸들러를 연결한다
- 시계는 주입한다. OMV 세션 만료는 `omvfake.Expire(user)`로 흉내 낸다
- 새 RPC를 쓰는 기능은 `omvfake`에 그 RPC의 합성 응답을 추가해 같은 방식으로 테스트한다. 응답 형식은 OMV 소스(`/usr/share/openmediavault/engined/rpc/`)를 읽고 만들며 NAS 실제 출력을 넣지 않는다
- `omvfake`는 인증된 모든 호출에서 User-Agent를 검사한다(실제 OMV보다 엄격). 백엔드가 고정 User-Agent를 지키는지 확인하기 위해서다
