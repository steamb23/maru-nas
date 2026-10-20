---
paths:
  - "backend/internal/files/**"
  - "backend/internal/server/files*.go"
  - "backend/cmd/fakeomv/files.go"
  - "frontend/src/files/**"
  - "frontend/src/settings/FilesSection.tsx"
---

# 파일 탐색기

홈 폴더와 OMV 공유 폴더를 둘러보는 셸 앱 `files`다. 파일은 권한 도우미의 사용자 권한 연산으로 다루고(`.claude/docs/helper.md`의 "파일 탐색기 연산"), onmaru는 보이는 폴더 밖의 경로를 거르고 목록을 정렬·캐시한다. 사용자 설명은 `README.md`의 "파일 탐색기", 남은 단계는 `ROADMAP.md`.

## 접근과 범위

- 접근은 파일 시스템 권한만 따른다. OMV 공유 폴더 권한(Privileges)은 따라 하지 않는다. 관리자도 자기 계정의 권한으로 보며, root로 둘러보는 기능은 만들지 않는다
- 보이는 폴더(`handler.roots`): 홈(도우미 `id`의 `home`, 세션별로 `files.Cache`에 기억) + 공유 폴더 스냅숏 + 디스크. 디스크는 스냅숏이 없을 때(`SharesUpdatedAt` 빈 값, 대체) 모두에게, 관리자가 "디스크 최상위 보기"(`SiteFiles.ShowDisks`)를 켜면 관리자에게 보인다. 스냅숏이 있는데 공유 폴더가 없으면(OMV에 공유 폴더가 없음) 디스크도 보이지 않으므로 사이드바가 그 사정을 안내한다(`files.nav.noShares`)
- `/api/files/*`는 보이는 폴더 밖 경로를 도우미에 보내기 전에 403 `outside_roots`로 거른다(`allowedPath`). 실제 범위는 도우미가 기준 디렉터리로 다시 정한다. `/srv/` 아래가 아닌 공유 폴더(sharerootfs)는 `reachable: false`이고 열 수 없다(도우미 추가 기준 경로는 로드맵)
- **공유 폴더 스냅숏**: `ShareMgmt.enumerateSharedFolders`는 관리자 전용이라 관리자 세션이 받아 `site.json`의 `files.shares`에 둔다. `startSession`이 관리자이고 도우미가 있으면 뒤에서(10초, 로그인 응답을 기다리게 하지 않음) 받고, 설정 앱의 "지금 받기"(`POST /api/site/files/refresh`)가 바로 받는다. 고정 메서드 호출(`sharesMethods`, `NoTouch`)이고, 성공했을 때만 바꾼다. 서버가 스스로 `site.json`을 쓰는 유일한 경우이며, 메모리(`handler.filesSite`)와 함께 바꾼다
  - 응답 해석은 `files.ParseShares`: `absdirpath`·`status`는 OMV 8.1.0부터 있으므로 없으면 `mntent.dir` + `reldirpath`로 만들고, `status`가 있고 `OK`가 아니면 뺀다(`auth-omv.md`의 "OMV 8 버전별 차이")
- 보안 강화 정책 `files`(`api.SecurityPolicy.Files`)는 `/api/files/*` 전부(뿌리 포함)를 `requireFiles`에서 막는다. 요청마다 보므로 정책을 올려도 따로 닫을 것이 없고, 이미 시작한 전송은 끝까지 간다

## 목록(큰 폴더)

- `GET /api/files/list?path=`가 `fs.names`로 이름과 종류만 읽어 목록(`files.Listing`)을 만들고 ID를 준다. 같은 세션·경로의 목록이 있고 폴더 `ModTimeNs`가 같으면(`fs.stat`) 다시 쓴다. 폴더 mtime은 안의 파일 내용이 바뀌어도 그대로이므로 `refresh=1`(도구 막대의 새로 고침)은 언제나 다시 읽는다
- `GET /api/files/list/{id}`가 보기(정렬·방향·찾기어, `files.View`)의 범위를 준다. 정보가 없는 항목은 `fs.statmany`로 읽어 목록에 둔다(512개·JSON 48KiB씩 나눠 보냄, `fillStats`). `names=1`이면 도우미를 부르지 않고 이름만 준다(스크롤 위치 말풍선)
- 날짜·크기순은 모든 항목의 정보가 있어야 한다(아니면 409 `sort_pending`). 브라우저가 `POST …/prepare`(한 번에 2000개)를 진행 막대와 함께 되풀이하고, 취소는 부르지 않는 것이다. 서버에 백그라운드 작업은 없다
- 정렬(`files.CompareNames`): 폴더(`fs.names`의 `d` 종류) 먼저, 이름은 소문자로 바꾼 코드포인트 + 숫자 묶음 자연 순서 + 바이트 순서. 표준 라이브러리만 쓰므로 로캘 정렬이 아니다. 방향은 기준 값에만 적용하고 같으면 이름 오름차순
- 캐시(`files.Cache`): 세션별 목록 4개, 유휴 10분, 전체 이름 60만 개(이름당 최대 약 100바이트). 사용자 사이에 나누지 않고, 세션 삭제 훅(`OnRemove`)이 지운다. 목록이 사라지면 410 `listing_expired`이고 프런트가 다시 연다
- UTF-8이 아닌 이름은 JSON으로 바이트를 옮길 수 없으므로 정보를 읽지 않고 `badName`으로 표시한다. 프런트는 열기·내려받기를 막는다

## 내려받기·올리기

- 내려받기(`GET /api/files/download`)는 `fs.read` 스트림이다. 내용을 브라우저가 그리지 않게 언제나 `application/octet-stream`, `attachment`(ASCII `filename` + RFC 5987 `filename*`), `nosniff`, `CSP: default-src 'none'; sandbox`로 보낸다. 범위는 하나(`bytes=a-b`, `a-`, `-n`)와 ETag `If-Range`만 받고, 여러 범위는 200 전체다
- 프런트는 숨긴 `<a download>`로 받는다(`files/download.ts`). 401이면 브라우저가 오류 본문을 파일로 저장하므로 누르기 직전에 `GET /api/files/info`로 세션과 파일을 먼저 확인한다
- 올리기(`PUT /api/files/upload?path=&overwrite=1`)는 원시 본문(`application/octet-stream`만, CSRF 보조 장치, `auth-omv.md`)과 `Content-Length`(없으면 411)를 받아 메모리에 모으지 않고 `fs.write`로 흘려 보낸다. 이름은 `proto.ValidName` + UTF-8 + `.onmaru-upload-` 접두사 금지
- 프런트(`useUploads.ts`)는 고른 파일마다 `info`로 같은 이름을 먼저 확인하고(있으면 덮어쓰기·건너뛰기, 남은 파일에 같게 적용) 하나씩 XHR로 올린다(`uploadFile`, 진행률). 올리는 동안 30초마다 세션 활동(`auth/activity.ts`의 `reportActivity`, 세션 유지기가 60초에 한 번 연장)을 알리고 `beforeunload`로 경고하며, 창이 닫히면 그 창의 올리기를 취소한다

## 창(프런트)

- `src/files/`: `FilesWindow.tsx`(사이드바·도구 막대·상태 표시줄, 상태와 이동), `FileList.tsx`(가상 목록), `InfoPane.tsx`(정보 패널), `UploadPanel.tsx`, `ConflictBody.tsx`·`conflictDialog.tsx`(이름 충돌), 순수 모듈 `paths.ts`, `history.ts`, `virtual.ts`, `pageCache.ts`, `kinds.ts`, `uploadQueue.ts`, `messages.ts`(Vitest `files.test.ts`)
- 셸 앱이고 창을 여러 개 연다(`multiInstance`). 상태는 창마다 컴포넌트에 두며 모듈 저장소에 두지 않는다. 권한 도우미가 없으면(`capabilities.helper`) 앱은 그대로 보이고 창 안에 못 쓰는 이유를 안내한다(`appAccess`의 `helper_missing`, `AddonUnavailable`). 도우미는 설정되어 있는데 닿지 않거나 티켓이 없으면 창이 `/api/files/roots`의 오류(`helper_unavailable`·`helper_reauth`)를 보인다
- 시안: Design 캔버스의 AC-Main·AC-Selected(정보 패널은 파일을 눌렀을 때만, 폴더는 고르기만), AC-Large(가상 스크롤·자리 표시·위치 말풍선), AC-Large-Sort(날짜순 준비 배너), S-Menu(올리기 목록), S-Locked(보안 확인), S-Fallback(대체 안내)
- 틀은 `SidebarLayout`의 `header`(도구 막대)를 쓰고 좁은 기준은 720px다. 정보 패널은 내용 너비 640px 아래(컨테이너 쿼리)에서 목록 위에 덮는다
- 가상 목록: 줄 높이 34px(`virtual.ts`의 `ROW_HEIGHT`, CSS `.files-row`와 맞춤), 스크롤 높이가 800만 px를 넘으면 비율로 줄인다(브라우저 요소 높이 한계). 200줄 페이지를 보기마다 캐시하고(60페이지), 스크롤이 90ms 멈춘 뒤 받는다(동시 2페이지). 받은 페이지는 `useMemo`로 만든 보기별 캐시 객체에 쌓고 `bump`로 다시 그린다. 요청을 끊는 컨트롤러는 마운트마다 effect에서 만든다(StrictMode가 정리를 한 번 더 불러도 끊긴 채 남지 않게)
- `role="grid"` + `aria-rowcount`/`aria-rowindex`, 포커스는 스크롤 영역 하나에 두고 고른 줄을 `aria-activedescendant`로 가리킨다. 화살표·PageUp/Down·Home/End로 고르고 Enter로 열며, Backspace는 상위 폴더다. 키보드로 폴더에 들어가면 새 목록에 포커스를 돌려준다
- 비동기 결과는 요청(뿌리 `try`, 목록 `path`, 정보 패널 `path`)과 함께 상태에 두고, 지금 요청과 다르면 받는 중으로 본다(effect 안에서 상태를 동기로 지우지 않는다, oxlint `set-state-in-effect`)
- 2단계 기능(폴더 만들기, 이름 바꾸기, 삭제, 끌어 놓기)과 시안의 아이콘 보기 전환은 아직 그리지 않는다

## 테스트

- Go: `internal/files`(정렬, 스냅숏 해석 8.0·8.1+, 보기·찾기, 캐시 상한·유휴), `internal/server/files_test.go`(`envOptions{files: true}`가 가짜 도우미 디스크를 디스크·`FilesSrvDir`로 둔다: 뿌리·스냅숏·관리자 로그인 갱신, 범위 밖 403, 목록·범위·prepare·410, 내려받기 헤더·Range·If-Range, 올리기 형식·길이·이름·덮어쓰기, 보안 정책)
- `make dev-fake`: 가짜 OMV가 가짜 도우미 디스크(`helper-root/srv/disk1`)에 합성 파일과 공유 폴더(문서, 미디어, 사진, 폴더 없는 백업, 루트 파일시스템의 kvm)를 만든다(`cmd/fakeomv/files.go`). `FAKEFILES_LARGE=<n>`이면 사진/휴대폰 백업에 빈 파일 n개를 만든다. onmaru는 개발 전용 환경 변수 `ONMARU_DEV_FILES_SRV`(플래그·가동 설정 목록에 넣지 않음)로 그 아래 폴더를 디스크로 본다
- 브라우저: Playwright 컨테이너는 문자셋 때문에 한글 이름 내려받기를 `download`로 저장한다(Blob도 같다). 한글 파일 이름은 실제 브라우저에서 확인한다
