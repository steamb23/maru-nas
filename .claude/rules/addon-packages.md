---
paths:
  - "backend/internal/addonpkg/**"
  - "backend/internal/server/addon_*.go"
  - "backend/internal/addons/packagerpc*.go"
  - "backend/internal/addons/extend*.go"
  - "backend/internal/addons/reserved*.go"
  - "backend/config/addons*.go"
  - "frontend/src/addonpkg/**"
  - "frontend/src/settings/Addons*"
  - "sdk/**"
  - "installer/internal/setup/*addon*"
  - "installer/internal/setup/templates/maru-addon*"
---

# 애드온 패키지

설계는 `.claude/docs/addon-packages.md`. 사용자 설명은 `docs/ko/addons.md`(애드온 설치), 개발자 설명은 `docs/ko/addon-dev.md`(애드온 개발)부터.

- 패키지 코드는 믿지 않는다. 패키지 프론트는 `sandbox="allow-scripts"` iframe(`allow-same-origin` 없음)에서만, 백엔드는 설치기가 켠 systemd 템플릿(동적 uid)에서만 돈다. maru는 패키지 실행 파일을 직접 실행하지 않는다(maru의 자식은 maru uid라 상태 디렉터리·도우미 소켓에 닿는다). Go `plugin`은 쓰지 않는다
- 패키지 코드에 OMV 세션 ID, 도우미 티켓, 대시보드 세션 쿠키, root를 주지 않는다. 백엔드의 OMV 호출은 호스트 소켓과 요청마다 만든 호출 토큰으로만 하고, maru가 그 애드온이 선언한 메서드(`rpcCaller{addon: id}`)만 통과시킨다
- 브리지에서 애드온 id는 프레임 바인딩으로 정한다. 메시지의 id·출처 값을 믿지 않고 `event.source === iframe.contentWindow`만 받는다
- 사용자 조작이 필요한 브리지 요청(`action`, `openWorkbench`)은 포커스한 그 프레임의 활성화일 때만 받는다(`.claude/docs/addon-packages.md`의 "브리지"). 활성화만 보지 않는다
- 패키지가 보낸 문구를 호스트 화면(확인 대화상자, 토스트, 제목 표시줄, 검토 화면)에 보일 때는 패키지 배지와 이름을 붙인다. 호스트는 패키지 창 안에서 비밀번호를 묻지 않는다. 패키지 SVG는 `<img>`로만 그린다
- `/api/rpc` 중계는 내장 레지스트리만 본다. 패키지 메서드는 `POST /api/addons/{id}/rpc`로만 통과시킨다
- 금지 표(`internal/addons/packagerpc.go`)를 바꿀 때는 OMV 소스(`/usr/share/openmediavault/engined/rpc/*.inc`)에서 근거를 확인해 항목 주석과 설계 문서의 "금지 표"를 함께 고친다. 표를 줄이는 변경은 이슈에서 먼저 논의한다
- 프레임 응답(`internal/server/addon_ui.go`)은 모든 응답에 CSP(`sandbox allow-scripts` 포함), `nosniff`, `Referrer-Policy: no-referrer`를 붙이고 MIME은 고정 표로 정한다. 대시보드 문서의 `frame-src 'self'`를 빼지 않는다
- 토큰(프레임 토큰, 호출 토큰)·세션 ID를 로그·응답 본문·커밋·테스트 픽스처에 남기지 않는다. manifest와 브라우저가 보낸 값은 로그에 `%q`로 남긴다
- root 동기화(설치기 `--addon-sync`)는 패키지 파일을 읽지 않는다. maru가 쓴 요청·원하는 상태 파일은 믿지 않는 입력으로 보고 `O_NOFOLLOW`로 읽어 엄격하게 검사하며, 고정 템플릿 인스턴스의 `systemctl` 동사만 실행한다. 드롭인·유닛 파일을 쓰지 않는다
- 백엔드 유닛 템플릿은 `maru.service`보다 느슨하게 만들지 않는다(골든 테스트가 검사한다). `/run`·maru 상태 디렉터리·`/srv`·`/home`을 다시 열지 않는다
- 패키지 압축 파일은 `os.Root` 스테이징에 풀고 rename한다. 일반 파일·디렉터리만 받고 상한을 넘으면 멈춘다. 권한은 받은 값을 쓰지 않고 정한 값(디렉터리 0755, 파일 0644, `backend/run` 0755)으로 맞춘다
- SDK(`sdk/`)는 MIT다. GPL 코드(대시보드 `frontend/src`, `backend`)를 `sdk/`로 복사하지 않는다. 대시보드와 함께 쓰는 파일은 원본을 `sdk/` 안에 두고 대시보드가 불러온다
- `sdk/js` 소스는 npm 패키지를 import하지 않는다(의존성 없음, 락 파일 없음). 검사는 프론트 도구가 한다. `sdk/go`는 표준 라이브러리만 쓰고, `backend`는 `sdk/go`를 import하지 않는다(업그레이드 확인의 `git archive -- installer backend`에 `sdk/`가 없다)
- 패키지 manifest 형식(`internal/addonpkg/format`)을 바꾸면 `make types`로 `sdk/schema/manifest.schema.json`도 다시 만들고, 형식을 깨는 변경이면 `format` 또는 SDK 수준(`api.AddonSDKLevel`)을 올린다. SDK 수준은 하위 호환을 지키며 올린다
