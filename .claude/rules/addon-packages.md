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
  - "frontend/src/settings/addons*"
  - "sdk/**"
  - "installer/internal/setup/*addon*"
  - "installer/internal/setup/templates/maru-addon*"
---

# 애드온 패키지

설계는 `.claude/docs/addon-packages.md`. 사용자 설명은 `docs/ko/addons.md`(애드온 설치), 개발자 설명은 `docs/ko/addon-dev.md`(애드온 개발)부터.

- 패키지 코드는 믿지 않는다. 패키지 프론트는 `sandbox="allow-scripts"` iframe(`allow-same-origin` 없음)에서만, 백엔드는 설치기가 켠 systemd 템플릿(동적 uid)에서만 돈다. maru는 패키지 실행 파일을 직접 실행하지 않는다(maru의 자식은 maru uid라 상태 디렉터리·도우미 소켓에 닿는다). Go `plugin`은 쓰지 않는다
- 패키지 코드에 OMV 세션 ID, 도우미 티켓, 대시보드 세션 쿠키, root를 주지 않는다. 백엔드의 OMV 호출은 호스트 소켓과 요청마다 만든 호출 토큰으로만 하고, maru가 그 애드온이 선언한 메서드(`rpcCaller{addon: id}`)만 통과시킨다
- 호스트 소켓(`internal/server/addon_host.go`)
    - 토큰은 바깥 요청(데이터·조작)마다 만들고 그 요청이 끝나면 지운다. 토큰 항목에는 바깥 요청의 `rpcCaller`를 둬서 범위(읽기·쓰기)와 OMV 401 처리를 바깥 요청과 함께 쓴다. 토큰이 다른 애드온의 소켓으로 오면 거부한다
    - `/v1/rpc`는 바깥 요청의 호출기만 쓴다(선언한 메서드만, 금지 표를 한 번 더 본다). 금지 표의 `Exec`·`Config`를 부르는 보조 호출기(`bgCaller`, `applyCaller`)는 `/v1/rpc-bg`·`/v1/apply` 처리기 안에서만 만들고, 정해진 메서드와 그 요청이 받은 상태 파일 이름만 부른다. 보조 호출기를 다른 곳에 넘기거나 고정 메서드를 늘리지 않는다
    - 계약(상태 코드, 본문 꼴, 헤더, 한도)은 `.claude/docs/addon-packages.md`의 "maru ↔ 백엔드"·"호스트 소켓"이고, Go SDK(`sdk/go`)와 다른 언어 백엔드가 따른다. 바꾸면 SDK와 사용자 문서(`docs/ko/addon-backend.md`)도 함께 고친다
    - 운영 소켓(0666)은 연결한 uid(`SO_PEERCRED`)를 `BackendPaths.HostPeer`(`addonpkg.InstalledHostPeer`: root와 systemd DynamicUser 범위 `config.AddonDynamicUIDMin`~`Max`)로 받은 연결에 바로 거른다. 거절한 연결은 동시 연결 칸(`limitListener`)을 차지하지 않고 닫는다(칸이 모두 차 있으면 받기 전이라 커널 대기열에서 기다린다). 개발 경로(0600)는 거르지 않는다. 동적 사용자로 도는 다른 서비스는 통과하므로, 패키지 사이를 나누는 것은 유닛의 `/run`(자기 디렉터리만)과 토큰이다. 유닛의 `PrivateUsers=yes`를 거쳐도 maru는 호스트 쪽 동적 uid를 본다(테스트 VM, systemd 257. 설계 문서의 "시험")
    - 운영 경로의 패키지별 디렉터리(`<config.AddonHostDir>/<id>/`)는 백엔드의 소켓 유닛이 root 소유 상위 디렉터리 안에 maru 사용자 소유로 만든다(`BackendPaths.HostDirFromUnit`). maru는 만들지도 지우지도 않고(유닛이 바인드하는 원본을 maru가 다른 곳으로 돌리지 못하게) 그 안에 소켓만 연다. 없거나 디렉터리가 아니면 열지 않았다가 레지스트리가 바뀔 때나 그 백엔드로 요청을 보내기 전(`addonHosts.ensure`)에 다시 연다. 열지 못한 이유는 바뀔 때만 로그에 남긴다
- 백엔드 응답(`addonpkg.Translate`)은 정한 상태(200·204·400·404·409·422·429)만 통과시키고 헤더는 넘기지 않는다. 백엔드의 401·403·5xx와 코드를 그대로 브라우저에 주지 않는다(`addon_error` + 검사한 `addonCode`)
- 개발 백엔드는 `MARU_DEV_ADDON_BACKENDS=1`(개발 전용 환경 변수, `cmd/maru/devaddons.go`)로만 켠다. maru의 플래그·가동 설정·설치기 레지스트리에 이 변수를 넣지 않는다
- 운영 백엔드는 설치기 설정 `--addon-backends`가 켠다. maru는 시작할 때 개발 변수를 먼저 보고, 아니면 상태 디렉터리가 정확히 `config.InstalledStateDir`이고 `config.AddonSyncStatusFile`이 일반 파일(Lstat)일 때만 운영 경로(`InstalledBackendPaths`)와 root 동기화(`InstalledSyncPaths`)를 켠다(`cmd/maru/devaddons.go`의 `addonBackends`). 개발 서버의 `.dev-state`는 운영 파일을 쓰지 않는다. 패키지 관리자의 `BackendsEnabled`와 서버의 `Config.AddonHost`는 같은 값에서 정한다
- root 동기화의 maru 쪽(`internal/addonpkg/backend_sync.go`의 `Sync`, 서버는 `internal/server/addon_sync.go`)은 원하는 상태(`config.AddonBackendsFile`)와 요청(`config.AddonSyncRequest`)을 쓰고 결과(`config.AddonSyncStatusFile`)는 `O_NOFOLLOW`·크기 상한으로 읽기만 한다. systemctl을 부르지 않는다
    - 원하는 상태는 레지스트리가 바뀔 때마다(`Manager.OnChange`, 호스트 소켓을 맞춘 다음, 등록할 때 한 번) 받은 설치본(개발본 제외)의 백엔드 패키지로 만들고(id 순, `instance`는 기록의 `installUID`), 쓰기 전에 `Validate`로 검사하며, 바이트가 바뀔 때만 임시 파일 + fsync + rename(0600)으로 쓴다. 레지스트리는 `Sync`가 받은 함수(`Manager.Active`, 잠금 없음)로 `Sync.mu`를 쥔 채 읽는다(다시 동기화·purge가 그 전에 읽은 목록으로 새 상태를 덮어쓰지 않게)
    - 백엔드 설치본은 `config.AddonBackendsMax`개까지 켠다(`Manager.wouldAccept`의 `too_many_backends`, 개발본은 세지 않음). 그래도 넘거나 항목 검사에 실패한 백엔드(손으로 고친 기록)는 파일에서 빼고 `failed`로 보인다
    - 요청은 결과의 digest가 지금 파일과 다르고(전체 실패한 결과는 맞춘 것이 아니다) 대기 중인 요청이 없을 때만 쓴다. 관리자의 다시 동기화(`POST /api/site/addons/sync`)는 결과와 상관없이 쓰되 대기 중인 요청은 두고, 1분(`addonpkg.SyncStall`)이 넘은 요청만 덮어쓴다. 화면의 대기(`Pending`)는 요청 파일이 있을 때와, 요청을 쓴 뒤 1분 안이고 결과 파일이 그대로일 때다(동기화는 요청을 먼저 지우고 일한다)
    - purge는 제거가 성공한 뒤에만 더한다(`?purge=1`, 백엔드 패키지만, 감사 로그의 `purge`는 실제로 넣었는지). 지난 파일의 purge를 이어 가고, 지금 디스크의 파일을 맞춘 결과(digest가 같음)의 `purged`와 다시 켜진 id는 뺀다. 파일에는 상한만큼만 넣고 나머지는 메모리에서 기다리며, 남은 목록이 이미 상한이면 제거하기 전에 409 `addon_purge_full`로 거절한다(`Sync.PurgeFull`)
    - 결과에는 설치 인스턴스가 없으므로, maru는 자기가 쓴 파일의 항목(지금 파일과 지난 8개, 시작할 때 디스크의 파일)을 digest로 기억한다. 백엔드 상태는 결과의 digest가 가리키는 파일의 그 id 항목이 지금 rev·설치 인스턴스와 같고 결과가 그 id를 같은 rev로 켰으면 `running`, 켜지 못했으면 `failed`, 그 밖(결과 없음·읽지 못함, 전체 실패, 모르는 파일, 다른 설치)은 `pending`이다
    - capability `backend_unavailable`은 백엔드가 꺼진 설치와 `failed`다. `pending`은 capability를 막지 않지만 maru가 그 백엔드로 보내지 않는다(`backendReady`, 503 `addon_backend_unavailable`). 그 소켓을 아직 이전 설치·rev의 유닛이 쥐고 있을 수 있어서다
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
- 예제·시험 패키지의 백엔드는 `sdk/go/examples/<id>/`(UI는 `sdk/examples/<id>/` 또는 `sdk/testdata/<id>/`)에 두고 `sdk/go` 모듈 안에 둔다(중첩 `go.mod` 없음, 모든 `.go` 첫 줄 `// SPDX-License-Identifier: MIT`). 실행 파일은 `make sdk-examples`가 만들고 커밋하지 않는다. 예제의 OMV 설정 변경은 `Host.Apply`로 적용까지 하고(쓰기 토큰의 `/v1/rpc`는 적용 대기를 남긴다), 되돌릴 수 있고 접속·인증·저장소에 닿지 않는 코어 메서드만 쓴다. 시험 도구(`probe`)는 닿았는지와 오류 종류만 돌려주고 읽은 내용을 담지 않는다
- `scripts/dev.sh`는 `FAKEADDONS=examples`일 때만 `sdk/examples/<id>`의 백엔드를 띄운다(`sdk/testdata`·`MARU_DEV_ADDONS`의 백엔드는 띄우지 않는다). 가짜 OMV의 `FAKEOMV_EXTRA_RPC`(`dev/fake-rpc.json`)는 가짜 OMV가 구현하지 않은 메서드에만 답한다(`omvfake/extra.go`)
- SDK CLI(`sdk/js/cli`)는 상수를 다시 적지 않고 스키마 파일의 `x-maru`를 읽는다. 압축 파일 규칙(`archive.go`)이나 manifest 검사를 바꾸면 `x-maru`(`addonpkg.Schema`), 공유 벡터(`sdk/testdata/manifests/`의 `valid`·`invalid`·`server-only`), 고정 파일(`sdk/testdata/packed/`)을 함께 고친다
- 패키지 manifest 형식(`internal/addonpkg/format`)을 바꾸면 `make types`로 `sdk/js/schema/manifest.schema.json`도 다시 만들고, 형식을 깨는 변경이면 `format` 또는 SDK 수준(`api.AddonSDKLevel`)을 올린다. SDK 수준은 하위 호환을 지키며 올린다
