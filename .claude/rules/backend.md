---
paths:
  - "backend/**"
---

# 백엔드 (Go)

- 표준 라이브러리(`net/http`)를 쓴다. 런타임 외부 의존성은 웹 터미널의 `golang.org/x/crypto/ssh`(`internal/terminal`)와 `github.com/coder/websocket`(`internal/server/terminal.go`)뿐이며 다른 곳에서 쓰지 않는다. 권한 도우미(`cmd/maru-helper`)에는 링크하지 않는다(도우미의 SSH 공개 키 확인은 `proto`가 직접 읽는다). tygo는 go.mod의 `tool` 지시어로 고정한 빌드 도구다
- `-listen`(`MARU_LISTEN`, 기본 `127.0.0.1:8090`)은 루프백 TCP 주소 또는 `unix:/절대/경로`를 받는다. 루프백이 아닌 TCP 주소는 `-allow-non-loopback`(`MARU_ALLOW_NON_LOOPBACK`)을 명시했을 때만 허용하고 시작 시 경고를 남긴다. 기본값이 거부인 동작을 유지한다
  - Unix 소켓은 권한 `0660`으로 만들고, 남은 소켓 파일은 지우되 소켓이 아닌 파일이나 사용 중인 소켓은 건드리지 않는다(`cmd/maru` `listenOn`)
  - 프록시를 거치지 않은 요청은 `X-Forwarded-*`를 위조할 수 있다. 루프백·소켓 수신은 이 헤더를 믿을 근거이므로 다른 경로를 기본으로 열지 않는다
- `-omv-url`(`MARU_OMV_URL`, 기본 `http://127.0.0.1`)은 루프백만 허용한다(OMV 세션 ID가 네트워크로 나가지 않게). 이 제한은 완화하지 않는다
- `-workbench-url`(`MARU_WORKBENCH_URL`, 기본 빈 값)은 브라우저가 OMV 웹 UI를 열 주소다. 백엔드는 접속하지 않고 `/api/capabilities`로 알려 주기만 한다. http(s) URL 또는 `/`·`./`·`../`로 시작하는 경로만 받고, scheme 없는 호스트 이름·query·fragment는 거부한다(`config.CheckWorkbenchURL`. 설정 앱에서 덮어쓰는 값과 설치기의 입력도 같은 함수로 검사한다)
- `backend/config`는 백엔드와 설치기(별도 모듈 `installer/`)가 함께 쓰는 공개 패키지다. 마루 NAS 버전(`Version`), 대시보드에서 업그레이드의 파일 형식(`upgrade.go`, `upgrade_request.go`. tygo가 `frontend/src/api/upgrade.gen.ts`로 내보낸다)과 서비스 상수(`upgrade_service.go`, 내보내지 않음), 환경 변수 키·기본값 상수와 설정 검사 함수(`CheckListen`, `CheckOMVURL`, `CheckHelperSocket`, `CheckWorkbenchURL`, `ParseRefreshAt`, `ParsePublicOrigin`, `ValidGroupName`)를 둔다. 플래그 정의와 `runtimeEntry` 목록은 이 상수를 쓴다. 표준 라이브러리만 쓰고 `backend/internal`을 import하지 않는다(`TestStandardLibraryOnly`). 여기서 규칙을 바꾸면 설치기에도 그대로 적용되므로 설치기 테스트도 돌린다(`.claude/rules/installer.md`)
- 대시보드 설정은 `internal/prefs`가 상태 디렉터리(`-state-dir`, `MARU_STATE_DIR`, 없으면 systemd의 `STATE_DIRECTORY`)에 저장한다: 사용자별 JSON(`users/`, `Store`), 전역 설정(`site.json`, `SiteStore`), 배경 이미지(`images/`, `ImageStore`), 보안 강화 모드의 수단 기록(`security/`, `security.Store`. 읽을 수 없는 파일은 `.corrupt`로 옮기지 않고 오류로 돌려준다, `.claude/rules/security.md`). 비우면 메모리에만 둔다. 대시보드 자신의 상태만 두며 OMV 설정이나 시스템 파일은 쓰지 않는다. `make dev`는 `.dev-state/real`, `make dev-fake`는 `.dev-state/fake`를 쓴다(커밋 안 함). git 워크트리에서 실행하거나 `BRANCH=<브랜치>`로 워크트리를 골라도 주 체크아웃의 `.dev-state`를 쓰므로, 상태 형식(`version`)을 바꾸는 브랜치이거나 주 체크아웃과 개발 서버를 동시에 띄울 때는 `MARU_STATE_DIR`로 따로 둔다
  - 설정 변경 API는 부분별로 둔다(`PUT /api/preferences/widgets`, `…/shortcuts`, `…/drawer`, `…/system-info`, `…/files`(파일 탐색기 보기), `…/notifications`(알림 읽음 상태), `…/appearance`, 전역은 `PUT /api/site/login`, `…/defaults`, `…/server`, `…/security`, 업데이트 채널은 `PUT /api/upgrade/channel`). 서로 덮어쓰지 않게 하며, 저장소는 설정 파일을 읽고 쓰는 동안 잠가 한쪽 저장이 다른 쪽을 지우지 않게 한다
- 전역 설정 쓰기(`/api/site/*`)는 `requireAdmin`(관리자가 아니면 403 `admin_required`)을 거친다. 로그인 화면이 쓰는 `GET /api/site`(화면 꾸밈 정보만, 가동 설정·주소는 넣지 않음)와 로그인 배경 이미지만 인증 없이 준다
- 배경 이미지는 자리(`prefs.SiteImage`, `prefs.UserWallpaper`)마다 하나다. 올리기는 원시 이미지 본문(`image/png|jpeg|webp`, 20MiB 이하, multipart 아님)이고 형식은 내용으로 다시 판별해 저장한다(SVG·HTML 거부). 보낼 때는 `serveImage`로 저장 형식, `nosniff`, `Content-Security-Policy: default-src 'none'; sandbox`, ETag(내용 해시 리비전)를 붙인다. 이미지를 지우면 그 이미지를 가리키던 배경 서술자도 지운다
- 웹에서 덮어쓰는 가동 설정은 재시작 없이 바뀌는 워크벤치 주소, SMART 확인 시각, 웹 터미널 허용(`-terminal`, 값은 `"true"`·`"false"` 문자열)뿐이다. 덮어쓴 값은 `site.json`에 두고 `handler.over`에 올려 바로 반영한다(`/api/capabilities`, `smart.Cache.SetRefreshAt`). 가동 설정 현황(값·출처)은 `main.go`가 `flag.Visit`·환경 변수로 만들어 `Config.Runtime`으로 넘긴다. 플래그를 더하면 그 목록에도 넣는다
- `cmd/fakeomv`와 `internal/omv/omvfake`, `internal/helper/helperfake`, `internal/terminal/sshfake`, `internal/upgrade/upgradefake`는 개발·테스트용이다. 운영 바이너리(`cmd/maru`, `cmd/maru-helper`)에서 import하지 않는다
- `-helper-socket`(`MARU_HELPER_SOCKET`, 기본 빈 값)은 권한 도우미 소켓의 절대 경로다. 비우면 `Config.Helper`가 nil이고 도우미 기능(`/api/helper*`)은 `disabled`·503이다. 시작할 때 도우미에 `reset`을 보낸다. 도우미 쪽 규칙은 `.claude/rules/helper.md`
- `/api/*` 경로는 대시보드 루트 기준이다. 대시보드를 경로 접두사 아래에 두면 리버스 프록시가 접두사를 제거한다. 쿠키 Path는 `-cookie-path`(기본 `/`)
- `internal/web/static/`에는 `.gitkeep`만 커밋한다. `dist/`는 빌드 결과다
  - 캐시: 해시가 붙은 `assets/*`는 `Cache-Control: public, max-age=31536000, immutable`, 그 밖의 파일(`index.html`)은 `no-cache`다. embed 파일은 수정 시각이 0이라 `Last-Modified`가 없으므로 시작할 때 내용 해시로 ETag를 만든다. 없는 파일(업그레이드 전 탭이 요청한 옛 청크)의 404에는 캐시 헤더를 붙이지 않는다(`internal/web`)
- `internal/api`는 TS 타입 생성의 원본이다. 바꾸면 `make types`를 실행하고 `frontend/src/api/types.gen.ts`도 함께 커밋한다
- 주기 요청·세션 확인으로 부르는 OMV RPC는 `options.updatelastaccess=false`로 호출해 OMV 세션을 연장하지 않는다. 세션 연장은 로그인과 사용자 활동 신호만 한다(`auth-omv.md`의 "세션 만료")
- 애드온 계약은 `internal/addons`, 내장 애드온은 `internal/addons/<id>/`, 정적 목록은 `internal/addons/builtin`이다. manifest를 바꾸면 `make types`로 `frontend/src/addons/manifests.gen.ts`도 다시 만든다(`.claude/docs/addons.md`)
- OMV RPC는 `/api/rpc` 중계든 provider 내부 호출이든 manifest 허용 목록(`Registry.Classify`)과 `addons.WritesEnabled`를 거친다
  - 예외: 애드온이 아닌 서버 기능은 고정 메서드 목록(`rpcCaller.fixed`)만 부른다. 지금은 프로필(`internal/server/profile.go`의 `profileMethods`)과 파일 탐색기의 공유 폴더 스냅숏(`internal/server/files.go`의 `sharesMethods`, `ShareMgmt.enumerateSharedFolders`, 관리자 세션)이며, 쓰기는 여기서도 `writes`와 `WritesEnabled`를 따른다
- 쓰기 RPC는 애드온 조작(`Addon.Actions`, `POST /api/addons/{id}/actions/{action}`)과 프로필 저장(`PUT /api/profile`, 자기 계정의 전자 우편·태그를 `UserMgmt.setUserByContext`로)으로만 부른다. `/api/rpc`와 `Fetch`는 쓰기를 거부한다. 조작은 브라우저가 보낸 대상을 검사하고 OMV 파라미터를 직접 만든다(OMV에는 파라미터를 검사하지 않고 셸 명령을 만드는 메서드가 있다). 백그라운드 작업은 `addons.WaitBg`로 기다린다(`.claude/docs/addons.md`의 "조작")
- OMV 설정을 바꾸고 적용하는 흐름(적용 대기 확인 → 변경 → `Config.applyChangesBg` → `WaitBg`)은 `addons.ApplyChange` 하나로 한다. 이 백엔드 안의 적용은 그 안의 잠금으로 하나씩 실행된다(서비스 켜기·끄기, 관리자의 프로필 저장). 적용 대기가 이미 있으면 남이 만든 변경을 대신 배포하지 않게 409 `config_dirty`로 거절한다
- 프로필(`GET·PUT /api/profile`)은 로그인한 사용자 자신만 다룬다. 관리자는 `UserMgmt.getUser(자기 이름)`, 일반 사용자는 `getUserByContext`로 읽고, SSH 키·uid 등은 넘기지 않는다. OMV의 설명 칸(comment, GECOS)은 워크벤치가 "Tags"로 쓰므로 쉼표로 나눈 태그로 주고받는다. 저장은 보내지 않은 값을 지금 값으로 채워 보내며, 관리자는 `ApplyChange`로 적용까지, 일반 사용자는 적용 대기로 남긴다. 시스템 계정·수정 금지 계정은 409 `profile_readonly`
- 프로필 사진은 배경 이미지와 같은 이미지 저장소의 사용자 자리(`prefs.UserAvatar`)이고 상한은 2MiB다(브라우저가 512px로 잘라 줄여 보낸다)
- 내장 애드온의 현황 데이터는 `Addon.Fetch` → `GET /api/addons/{id}`로 준다. provider는 `Request.RPC`(그 애드온이 선언한 메서드만, `updatelastaccess=false`)로 OMV를 부르고, 일부 출처 실패는 응답의 `failedSources`에 담는다(`.claude/docs/addons.md`의 "애드온 데이터 API")
- `Smart.getList`는 대기 중인 디스크를 깨운다. 주기적으로 부르지 말고 `smart.Cache`를 거친다(온도는 `Peek`만)
- OMV가 매번 프로세스를 실행하는 느린 읽기 RPC(`Services.getStatus`, `Compose.getContainers`)는 `addons.SharedCache`로 세션 사이에 짧게(5초) 함께 쓴다. 결과가 사용자와 무관한 RPC에만 쓰고, `Request.Refresh`면 건너뛰며, 상태를 바꾸는 조작은 끝난 뒤(실패해도) `Invalidate`한다
- `/proc`·`/sys`를 읽는 코드는 파서(`io.Reader` 입력)와 읽기(`fs.FS` 주입)를 나눠 `testdata/`의 합성 픽스처로 테스트한다(`internal/addons/sysinfo` 참고)

## 알림

화면 쪽은 `.claude/rules/frontend-shell.md`의 "알림", 작업 결과 알림은 아래 "서버 백그라운드 작업".

- 서버 알림은 `internal/notify`가 출처(`Source`: `Name`, `Admin`, `Collect`)를 모아 만든다. `Center`는 결과를 모든 세션이 함께 쓰도록 30초 보관하고, 지난 뒤 처음 묻는 요청에서 다시 읽는다(`addons.Detector`와 같은 지연 캐시이며 고루틴이 없다. 서버 백그라운드 작업이 아니다). 읽지 못한 출처는 직전 결과를 유지한다. 수집은 요청이 끊겨도 끝까지 하고(`context.WithoutCancel`) 출처마다 5초 제한이다
  - 출처가 준 알림은 형식을 검사해 맞지 않으면 버린다: id(`ValidID`), 리비전(`ValidRevision`), 수준, 코드(`knownCodes`, `api.Notice.Code`의 tstype과 같아야 한다. `TestCodesMatchTSType`), RFC 3339 `since`, 동작(`app`은 앱·페이지 id 형식, `workbench`는 앞 슬래시 없는 라우트, `power`는 값 없음). 다른 출처와 겹치는 id는 앞 출처의 것을 남기고, 출처마다 20개·`items` 20개까지 둔다
  - 리비전은 내용이 바뀔 때만 달라져야 한다(`Revision`: 정렬한 조각의 해시). 파일 수정 시각을 쓰지 않는다(내용이 같아도 매일 다시 쓰는 파일이 있다)
  - 운영 출처는 APT 업데이트(`aptUpdates`)와 재부팅 권장(`rebootRequired`)이다(관리자 전용, `internal/aptindex`가 `upgradeindex.json`·`/run/reboot-required(.pkgs)`를 직접 읽음, `main.go`에 등록). 출처를 더할 때는 문구 키(프론트 `notifications/messages.ts`)와 코드 유니온(프론트 `api/client.ts`의 `NOTICE_CODES`도, 빠지면 tsc가 잡는다)을 함께 더한다. 직접 읽는 출처는 `/proc`·`/sys`처럼 파서와 `fs.FS` 읽기를 나눈다
  - 출처는 지울 수 있는 알림(`dismissible`)을 줄 수 없다(`Center.accept`가 버린다). 지울 수 있는 것은 사용자별 작업 결과뿐이고 핸들러가 합친다(아래 "서버 백그라운드 작업")
- `GET /api/notifications`(`requireSession`)는 이 세션이 볼 알림(관리자 전용 출처는 관리자에게만, 서버가 거른다)과 이 사용자의 작업 결과 알림, 그중 읽은 것(`seen`, 지금 알림의 id만), 진행 중인 작업(`tasks`, 관리자만)을 준다. 관리자 요청은 기다리는 작업 감시자를 깨운다(`Jobs.Offer`). `DELETE /api/notifications/{id}`는 이 사용자의 작업 결과 알림만 지운다(아니면 404). `DELETE /api/notifications?through=<id>`(알림 센터의 "모두 지우기", `Jobs.DismissThrough`)는 그 작업 결과와 그보다 먼저 끝난 이 사용자의 결과를 모두 지운다. 보이지 않던 오래된 결과(최신 5개 밖)도 지워 다음 응답에 다시 나오지 않게 하고, 기준보다 나중에 끝난 결과(브라우저가 아직 받지 못했을 수 있음)는 남긴다. 기준이 이 사용자의 결과가 아니면 404, 없으면 400이다. `PUT /api/preferences/notifications`는 읽음 상태(id → 리비전, 128개까지, 형식 검사) 전체를 바꾼다. 프론트는 지금 목록에 있는 id만 보내므로 저장 크기가 묶인다. 빈 목록이면 `seen`은 `{}`다
- 개발 서버: `MARU_DEV_NOTIFY_ROOT`(개발 전용 환경 변수, 플래그·가동 설정 목록·설치기 레지스트리에 넣지 않음)가 있으면 APT 목록·재부팅 파일을 그 가짜 루트에서 읽는다. `make dev-fake`가 상태 디렉터리의 `notify-root`로 정하고 가짜 OMV 프로세스가 합성 목록을 쓴다(`FAKEAPT_SCENARIO`, 아래 "서버 백그라운드 작업"). `MARU_DEV_NOTICES`(개발 전용 환경 변수, 플래그·가동 설정 목록·설치기 레지스트리에 넣지 않음)가 있으면 그 파일(`{admin: [...], everyone: [...]}`)을 읽는 출처 둘을 두고 보관 시간을 2초로 줄인다(`cmd/maru/devnotices.go`, 리비전이 비면 내용으로 채움). `make dev-fake`가 상태 디렉터리의 `dev-notices.json`으로 정하고, 가짜 OMV 프로세스가 `FAKENOTICES`(sample, none, many, rotate) 시나리오대로 쓴다. rotate는 `FAKENOTICES_STEP`(기본 30s)마다 개수를 바꿔 토스트를 확인하게 한다

## 서버 백그라운드 작업

시스템 업데이트 앱(`packages`)의 확인·설치처럼 브라우저 없이 서버에서 끝까지 도는 OMV 작업이다(`internal/jobs`). 브라우저가 진행하는 파일 복사·올리기는 대상이 아니다.

- **한 번에 하나**: `jobs.Manager.Start`가 OMV를 부르기 전에 슬롯을 잡고(겹치면 409 `job_busy`), OMV가 거절하면 푼다. 작업 종류는 `jobs.Kinds` 표(`apt-update`, `apt-upgrade`)다
- **실행·감시는 `jobs.Runner`**: 기록에 Runner 이름과 불투명한 handle을 두므로 나중의 상태 파일 서비스는 Runner 하나를 더하는 일이다. 지금 Runner는 `omvjob`(이름 `omv-session`)이다
  - `Start`는 `Apt.update`·`upgrade`를 조작의 세션으로 부르고 상태 파일 이름(`^/tmp/bgstatus[A-Za-z0-9]+$`)을 handle(`{filename, pos}`)로 준다. 응답을 잃어도 다시 부르지 않는다(작업이 둘 생긴다). 잠금 문구는 `ErrLocked`(409 `apt_locked`), 적용 대기 문구는 `ErrConfigDirty`(409 `config_dirty`)
  - `Watch`는 1초마다 `Exec.getOutput{filename, pos, length: 2MiB}`(`pendingOutput`이면 4번까지 이어 받음)를 부른다. `Exec.isRunning`은 쓰지 않는다(둘 다 끝나면 상태 파일을 지워 출력을 잃는다). `Exec.stop`은 쓰지 않는다
  - **세션 옮겨 가기**: 상태 파일은 세션에 묶이지 않으므로 관리자 대시보드 세션 아무것으로나 읽는다(`server/jobs.go`의 `jobSessions`, 시작한 세션 먼저, 나머지는 최근 순). 호출은 `rpcCaller`(그 애드온의 허용 목록, 늘 `NoTouch`)이고 `h.expire`를 부르지 않는다. OMV 401이면 그 대시보드 세션을 지우고(OMV가 이미 파기함) 다음 세션으로, 403이면 건너뛴다. 세션이 없으면 기다리고(`waiting`), 관리자 요청(`/api/notifications`, `/api/jobs*`)이나 30초마다 다시 본다. 로그에는 사용자 이름만 남긴다
  - **오류 판정**(OMV 예외 코드는 늘 0이라 문구로 가린다, `omvjob.classify`와 테스트): 네트워크·rpc.php 밖 응답(`omv.Error.FromRPC`)·엔진 소켓 문구는 일시 오류(물러났다 다시, 1시간 넘게 이어지면 `job_timeout`). "Failed to open file (filename=<우리 파일>"은 사라짐: 바로 전 폴링이 성공했고 업데이트 목록이 시작 뒤에 다시 만들어졌으면 출력이 불완전한 성공, 아니면 `job_lost`. openmediavault 제거 보호 문구는 `apt_core_removal`, "Failed to execute command"는 `apt_failed`, 그 밖의 500은 10초 넘게 3번 같으면 `job_failed`. 성공 폴링이 `KindSpec.MaxRun`을 넘어도 실행 중이면 `job_timeout`
- **오류 문구의 경계**: 작업 출력(apt가 낸 내용)은 워크벤치가 보이는 것과 같은 내용이라 브라우저에 준다. OMV 예외 문구는 로그에만 남기고 코드로 바꾼다
- **저장**(`prefs.FileStore`의 `jobs.Store`, 상태 디렉터리가 없으면 `jobs.MemoryStore`): `jobs/current.json`(진행 기록, 세션 ID 없음, 시작·10초마다·종료 때 씀), `jobs/results/<사용자>.json`(최근 10개·30일), `jobs/output/<id>.log`(조각마다 덧붙이고 fsync하지 않음, 4MiB를 넘으면 뒤 2MiB만 남김). 자체 `version: 1`이고 `users/*.json`은 건드리지 않는다. 디렉터리는 쓸 때만 만든다
  - 다시 시작하면(`Resume`) 출력 파일을 기록의 길이로 자르고 기록한 자리부터 이어 받는다. 부트 id가 바뀌었으면 `job_reboot`, 모르는 Runner면 `job_lost`, 결과를 저장한 뒤 멈췄으면 결과를 믿는다. `main.go`가 종료할 때 `Close`로 진행 기록을 남긴다
- **결과 알림**: 끝나면 시작한 사용자의 결과로 남기고 `notify.Center.Invalidate`로 APT·재부팅 알림을 다시 읽게 한다. 결과 알림(`jobSucceeded`·`jobFailed`·`jobUnknown`, id `job-<작업 id>`, `dismissible`, `job`)은 지우지 않은 최신 5개이고 `notify.Check`로 검사한다. 요약(`count`, `items`)은 확인이면 찾은 업데이트, 설치면 시작할 때 목록에서 사라진 패키지다
- **API**(`requireAdmin`): `GET /api/jobs`(진행 중인 작업은 누가 시작했든, 결과는 이 사용자의 것, `last`는 마지막으로 끝난 작업이라 다른 관리자가 시작한 작업의 결과도 창이 보인다), `GET /api/jobs/{id}`, `GET /api/jobs/{id}/output?from=N`(논리 위치, 64KiB, UTF-8 경계, `more`면 바로 다시). 진행 중인 작업과 마지막으로 끝난 작업의 출력은 어느 관리자나, 그 밖의 결과는 시작한 사용자만 읽는다
- 서버 테스트는 `envOptions{jobs}`와 `Config.JobInterval`(5ms)을 쓴다. 가짜 OMV는 `Exec.getOutput`을 실제 의미대로(오류 먼저·실패 작업은 지우지 않음) 흉내 내고 `SetAptConfig`·`SetGlitch`·`DropBg`로 상황을 만든다. 개발 서버는 `FAKEAPT_SCENARIO`(success, slow, fail, core-removal, locked, dirty)이고 가짜 작업은 폴링마다 출력 조각 하나씩 나아간다(maru가 1초마다 읽으므로 조각 수가 곧 초)

## 파일 탐색기

규칙은 `.claude/rules/files.md`, 도우미 쪽 연산은 `.claude/docs/helper.md`의 "파일 탐색기 연산".

- 라우트(`internal/server/files.go`): `GET /api/files/roots`, `GET /api/files/list`, `GET /api/files/list/{id}`, `GET /api/files/list/{id}/find`, `POST /api/files/list/{id}/prepare`, `GET /api/files/info`, `GET /api/files/download`, `GET /api/files/preview`(미리보기 창, 형식 허용 목록·inline), `GET /api/files/search`(하위 폴더 검색, `text/event-stream`), `PUT /api/files/upload`(본문 전 거절은 `drainRejected`), `POST /api/files/upload/check`(올리기 사전 확인), `POST /api/files/mkdir`, `POST /api/files/rename`, `POST /api/files/delete`, `POST /api/files/copy`(진행을 `text/event-stream`으로 보낸다), 휴지통 `GET·POST /api/files/trash`, `GET /api/files/trash/volumes`, `POST /api/files/trash/restore`·`purge`·`empty`(모두 보안 정책 `files` → 도우미 티켓. 읽기는 `requireFiles`, 쓰기는 `requireFilesWrite`이고 정책 범위가 `writes`면 쓰기만 막는다), 관리자 `GET·PUT /api/site/files`, `POST /api/site/files/refresh`, `GET·POST /api/site/files/trash`(디스크 휴지통, 만들기는 root 상승 필요). 경로 계산·정렬·목록 캐시는 `internal/files`
- 이름 바꾸기·옮기기·삭제는 사이드바에 보이는 폴더(홈, 공유 폴더, 디스크) 자신과 그것을 품은 경로를 409 `root_folder`로 막는다(`rootFolder`. 도우미는 공유 폴더를 지키지 않는다, `.claude/rules/files.md`)
- 공유 폴더 권한 따르기(`SiteFiles.FollowPrivileges`)를 켜면 `roots`가 권한 없는 공유 폴더를 숨기고(`withinRoots`가 그 아래를 403 `outside_roots`), 쓰기 라우트는 바꿀 곳을 `allowedWritePaths`로 걸러 읽기 전용 공유 폴더 안이면 403 `share_read_only`다(`.claude/rules/files.md`의 "공유 폴더 권한 따르기")
- 도우미 파일 오류 코드는 `writeFilesError`가 HTTP로 바꾼다(`not_found` 404, `permission_denied`·`outside_roots` 403, `exists` 409 `file_exists`, `not_empty`·`cross_device`·`no_trash` 409, `no_space` 507, `busy` 429 `helper_busy`, `timeout` 504 `transfer_timeout` 등). 나머지는 `writeHelperError`
- 디스크(`Config.Mounts`, `storage.Reader.MountsUnder`)와 데이터 디스크 디렉터리(`Config.FilesSrvDir`, 기본 `/srv`). 개발 서버는 `MARU_DEV_FILES_SRV`(개발 전용, 플래그·가동 설정 목록·설치기 레지스트리에 넣지 않음)로 가짜 도우미 디스크를 가리킨다
- HTTP 서버에는 `ReadTimeout`·`WriteTimeout`을 두지 않는다(6시간 전송). 전송 상한과 무진행 끊기는 도우미가 한다

## 대시보드에서 업그레이드

설계는 `.claude/docs/upgrade.md`, 업그레이드 서비스(설치기)는 `.claude/rules/installer.md`의 "업그레이드 서비스".

- maru는 업그레이드 서비스의 파일만 다룬다(`internal/upgrade`): root 쪽 설정 파일(`config.UpgradeEnvFile`, 있으면 켜짐, 저장소 경로·계정은 수동 안내에 쓴다), 상태 파일(`config.UpgradeStatusFile`), 설치 기록(`config.InstallRecordFile`, 실행 파일에 커밋이 없을 때 화면이 쓴다)을 O_NOFOLLOW·크기 상한으로 읽고, 확인 요청(`<상태 디렉터리>/upgrade-check.json`)을 같은 디렉터리 임시 파일에서 이름을 바꿔 쓴다. 저장소·git·빌드에는 닿지 않는다(`ProtectHome=yes`)
  - 상태 파일이 `checking`·`applying`인 채로 `config.UpgradeServiceTimeout`(유닛의 `TimeoutStartSec`)보다 오래되면 중단으로, 확인 요청이 1분 넘게 남아 있으면 멈춤으로 보고, 둘 다 다시 요청할 수 있다. 같은 방식(원격이면 같은 채널)으로 성공한 확인(`checked`)이 나온 지(`CheckedAt`) 30초(`checkReuse`)가 지나지 않았으면 확인 요청을 쓰지 않고 그 결과를 그대로 돌려준다(`Snapshot.RecentCheck`, 오류나 안내 없음). 실패한 확인은 원인을 고친 뒤 바로 다시 확인할 수 있게 다시 쓰지 않는다
  - 업데이트 채널(`.claude/docs/upgrade.md`의 "업데이트 채널")은 `site.json`의 `upgrade.channel`(`api.SiteUpgrade`)이다. 원격 확인 요청에 담고 `UpgradeInfo.channel`로 알린다. 모르는 값(손으로 고친 파일)은 빈 값(지금 브랜치)으로 본다
  - 경로 유닛은 `config.InstalledStateDir`(`/var/lib/maru`) 안의 확인 요청만 보므로, 상태 디렉터리가 비었거나 그곳이 아니면 확인 요청 경로를 비워 `state-dir`로 끈다(`main.go`의 `upgradePaths`)
- 라우트(모두 `requireAdmin`): `GET /api/upgrade`(`api.UpgradeInfo`, 상태 파일은 `config.UpgradeStatus` 그대로), `PUT /api/upgrade/channel`(업데이트 채널 저장, 상승 필요 없음, 꺼짐·바쁨이면 거절), `POST /api/upgrade/check`(상승 필요 없음), `POST /api/upgrade/apply`(대상·설정 꼴을 먼저 400으로 거르고, 켜짐·바쁨·끝난 확인과 같은 대상인지, 원격 확인이면 지금 채널로 한 확인인지 본 뒤 도우미 `upgrade.apply`. 상승하지 않았으면 도우미의 `elevation_required`). 오류 코드 `upgrade_disabled`·`upgrade_busy`·`upgrade_outdated`
  - 여기서 하는 검사는 빠른 거절과 안내다. 진행할 수 있는지는 업그레이드 서비스가 저장소·설치 상태로 다시 확인한다
  - 로그에는 사용자·대상·설정 이름만 남기고 설정 값은 남기지 않는다
- `Config.Upgrade`가 nil이면 빈 경로라 꺼짐(`not-enabled`)이다(테스트가 실제 `/etc/default/maru-upgrade`를 읽지 않게). 서버 테스트는 `envOptions{upgrade: true}`가 임시 디렉터리를 쓴다
- 개발 서버: `MARU_DEV_UPGRADE_ROOT`(개발 전용 환경 변수, 플래그·가동 설정 목록·설치기 레지스트리에 넣지 않음)가 있으면 설정·상태 파일 경로 앞에 붙이고 상태 디렉터리 검사를 건너뛴다. `make dev-fake`가 상태 디렉터리의 `upgrade-root`로 정하고, 가짜 OMV 프로세스의 가짜 업그레이드 서비스(`internal/upgrade/upgradefake`, 운영 바이너리에서 import하지 않음)가 그 아래에 쓴다. 가짜 저장소는 `stable` 브랜치에서 시작해 채널을 따라 옮긴다. 시나리오는 `FAKEUPGRADE_SCENARIO`(web, manual, latest, check-fail, apply-fail, disabled), 단계 지연은 `FAKEUPGRADE_STEP`(기본 1.5s)이며 maru를 다시 시작하지 않는다

## 테스트

- 서버 테스트는 `internal/server/server_test.go`의 `newEnv`·`browser`를 쓴다. `omvfake`를 `httptest`로 띄우고 실제 `omv.Client`와 서버 핸들러를 연결한다. 도우미가 필요하면 `envOptions{helper: true}`가 `helperfake`를 임시 소켓에 띄워 실제 `client.Client`로 연결한다. `files: true`면 그 가짜 디스크를 파일 탐색기의 디스크로 둔다
- 시계는 주입한다. OMV 세션 만료는 `omvfake.Expire(user)`로 흉내 낸다
- 옛 OMV 8 버전의 응답은 `omvfake.SetVersion`(`Version82`, `Version81`, `Version80`)으로 흉내 낸다. 버전마다 RPC가 다른 기능은 해당 버전대도 테스트한다(`auth-omv.md`의 "OMV 8 버전별 차이")
- 새 RPC를 쓰는 기능은 `omvfake`에 그 RPC의 합성 응답을 추가해 같은 방식으로 테스트한다. 응답 형식은 OMV 소스(`/usr/share/openmediavault/engined/rpc/`)를 읽고 만들며 NAS 실제 출력을 넣지 않는다
- `omvfake`는 인증된 모든 호출에서 User-Agent를 검사한다(실제 OMV보다 엄격). 백엔드가 고정 User-Agent를 지키는지 확인하기 위해서다
