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
- 보안 강화 정책 `files`(`api.SecurityPolicy.Files`)는 `/api/files/*` 전부(뿌리 포함)를 막는다. 적용 범위 `filesScope`가 `writes`면 읽기 라우트(`requireFiles`: 뿌리, 목록, 범위, find, prepare, 정보, 내려받기, 휴지통 목록·볼륨)는 열고 쓰기 라우트(`requireFilesWrite`: 올리기와 그 사전 확인, 만들기, 이름 바꾸기, 삭제, 복사, 휴지통 넣기·되돌리기·영구 삭제·비우기)만 막는다. 요청마다 보므로 정책을 올려도 따로 닫을 것이 없고, 이미 시작한 전송은 끝까지 간다

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
- 올리기(`PUT /api/files/upload?path=&overwrite=1`)는 원시 본문(`application/octet-stream`만, CSRF 보조 장치, `auth-omv.md`)과 `Content-Length`(없으면 411)를 받아 메모리에 모으지 않고 `fs.write`로 흘려 보낸다. 이름은 `proto.ValidName` + UTF-8 + `.onmaru-upload-` 접두사 금지. 핸들러와 사전 확인은 `internal/server/files_upload.go`
  - **본문 전 거절**: Go 서버는 핸들러가 본문을 읽기 전에 응답하고 남은 본문이 256KiB 이상이면 본문을 버리지 않고 500ms 뒤 연결을 닫는다. 브라우저(HTTP/1.1)와 일부 프록시는 본문을 끝까지 보낸 뒤에야 응답을 읽으므로 닫힌 연결에 쓰다가 RST를 받고, 거절이 502(Vite 프록시)나 늦은 응답·멈춤(Caddy)이 된다. 그래서 라우트를 `drainRejected`로 감싼다: 본문을 다 받기 전에 오류(400 이상)로 답하면 `EnableFullDuplex`로 응답을 길이와 함께 먼저 보내고(`Connection: close`) 남은 본문을 받은 뒤 닫는다. 세션이 있으면 `Content-Length`까지(60초 무진행이면 멈춤), 세션이 없는 401과 길이를 모르는 본문은 1MiB까지만 받는다. 401·보안 정책·형식·이름·범위 거절과 도우미의 도중 거절(`exists`, `permission_denied`, `no_space`)이 모두 이 경로다
  - **사전 확인**(`POST /api/files/upload/check {path}`, 204): 프록시에 따라 위 거절도 본문을 다 보낸 뒤에야 브라우저에 오므로, 프런트는 PUT 전에 본문 없이 같은 가드(`requireFilesWrite`)와 같은 검사(`uploadTarget`: 새 이름, 보이는 폴더 안)를 거친다. 도우미는 부르지 않는다(같은 이름·쓰기 권한은 PUT이 안다). 사용성을 위한 것이고 PUT은 모든 검사를 그대로 한다
- 프런트: 탐색기 창이 고른(끌어 놓은) 파일마다 `info`로 같은 이름을 먼저 확인하고(`uploads.ts`의 `checkUploads`, 있으면 덮어쓰기·건너뛰기, 남은 파일에 같게 적용) 그 파일들을 올리기 작업 하나로 파일 작업 목록에 더한다(`startUpload`, 아래 "파일 작업"). 작업은 파일을 하나씩 XHR로 올린다(`uploadFile`, 진행률). `uploadFile`은 본문을 보내기 전에 사전 확인(`checkUpload`)을 하므로, `withStepUp`이 보안 확인 대화상자를 본문 전에 띄우고 확인 뒤 둘 다 다시 한다. 탐색기 창을 닫아도 이어진다

## 만들기·이름 바꾸기·옮기기·삭제

- 라우트: `POST /api/files/mkdir {path}`, `POST /api/files/rename {from, to}`(이름 바꾸기, 끌어 놓기의 옮기기), `POST /api/files/delete {path, recursive}`(204). JSON 본문만 받고(CSRF 보조 장치) 도우미의 `fs.mkdir`·`fs.rename`·`fs.remove`를 부른다. 만들거나 바꾼 항목은 `fileItemOf`로 돌려준다
- **보이는 폴더 지키기**(`rootFolder`): 도우미는 자기 기준 디렉터리(홈, `/srv` 아래 마운트 지점)만 지키고 공유 폴더는 지키지 않는다. 그래서 onmaru가 보이는 폴더(홈, 스냅숏의 공유 폴더 전부, 디스크) 자신이거나 그것을 품은 경로의 이름 바꾸기·옮기기·삭제를 409 `root_folder`로 막는다(디스크 최상위 보기에서는 공유 폴더가 디스크의 한 폴더로 보인다). 스냅숏이 없는 대체 상태에서는 공유 폴더를 모르므로 파일 시스템 권한(디스크 맨 위는 보통 root 소유)에 맡긴다
- 새 이름(`splitNew`, `newNameOK`)은 올리기와 같은 검사(`proto.ValidName`, UTF-8, `.onmaru-upload-` 접두사 금지)를 거친다. 폴더를 자기 안으로 옮기는 요청(`to`가 `from` 안)과 같은 경로는 400이다. 프런트도 같은 검사(`paths.ts`의 `nameProblem`, `moveAllowed`)를 먼저 한다
- 옮기기는 이름 바꾸기 하나다. 다른 기준 디렉터리나 다른 파일시스템이면 도우미가 `cross_device`(409)를 주고 복사하지 않는다. 대상이 있으면 덮어쓰지 않는다(`file_exists`). 다른 디스크로 끌어 놓으면 아래 "복사"를 쓴다
- `POST /api/files/delete`는 SMB 휴지통(`.recycle`)을 거치지 않는 영구 삭제다(삭제의 기본은 아래 "휴지통"). `recursive`는 목록에서 폴더(`type`이 `dir`, 심볼릭 링크 아님)인 항목에만 보내고, 그 밖의 항목이 실제로는 비어 있지 않은 폴더면 `not_empty`(409)다. 목록이 낡아 파일로 보인 폴더를 통째로 지우지 않게 하기 위해서다. 도우미는 `fs.remove`를 10분까지 기다리고, 넘으면 일부만 지운 채 `transfer_timeout`이다(다시 지우면 이어서 지운다)
- `GET /api/files/list/{id}/find?name=&sort=&order=&q=`는 보기에서 그 이름의 자리다(`Listing.IndexOf`, 없으면 -1). 만들거나 이름을 바꾼 뒤 다시 받은 목록에서 그 줄을 고를 때 쓰며 도우미를 부르지 않는다
- 바꾼 뒤 프런트는 목록을 `refresh=1`로 다시 연다. 폴더 mtime이 거친 파일시스템에서도 낡은 목록을 다시 쓰지 않게 하기 위해서다

## 복사

- 다른 디스크(도우미의 기준 디렉터리. 홈과 `/srv` 아래 마운트 지점마다 하나, 휴지통 볼륨의 `top`과 같다)로 끌어 놓으면 옮기지 않고 복사한다. 원본은 그대로 둔다
- `POST /api/files/copy {dir, names, to}`(`internal/server/files_copy.go`, 쓰기 라우트): `dir` 안의 이름(512개까지, 새 이름과 같은 검사)을 `to`에 같은 이름으로 복사한다. 이름이 `to`를 품으면(폴더를 자기 안으로) 400이고, 인자 JSON이 도우미 요청 머리에 들어가지 않으면 400이다. 도우미 `fs.copy`(`.claude/docs/helper.md`의 "복사")가 하고, 진행은 요청이 열려 있는 동안 `text/event-stream`(`data:` 줄마다 `api.FilesCopyEvent`, `X-Accel-Buffering: no`)으로 보낸다. 서버에 백그라운드 작업은 없다
  - 시작하기 전의 실패(보안 정책, 범위, 대상이 폴더가 아님)는 보통의 오류 응답이라 `withStepUp`이 그대로 받는다. 첫 진행 뒤에 멈추면 마지막 사건이 `error`(보통의 응답이었다면 받았을 오류와 HTTP 상태, `writeFilesError`를 메모리에 받아 만든다)다. 이름마다의 실패는 결과의 `failed`(API 오류 코드)에 담긴다
  - 브라우저가 요청을 끊으면 도우미 연결이 닫히고, 도우미는 하던 항목의 임시 이름을 지우고 멈춘다(이미 복사한 항목은 남는다)
- 프런트: 탐색기 창이 `startCopy`로 파일 작업 목록에 작업을 더한다(아래 "파일 작업"). 계산은 `copyJobs.ts`
  - 이름이 많으면 512개·JSON 40KiB(`goJSONLength`, Go가 쓰는 길이의 상한)씩 요청을 나눈다(`copyChunks`). 진행률은 요청이 하나면 바이트, 여럿이면 끝난 요청 수에 지금 요청의 비율을 더한다. 남은 시간은 요청이 하나일 때만 보인다
  - 요청이 끝날 때마다 대상 폴더를 변경 알림으로 알리고, 취소·실패 뒤에는 2초 뒤 한 번 더 알린다(도우미가 연결이 끊긴 것을 안 뒤에 임시 이름을 지우므로)

## 파일 작업(복사, 올리기)

- 오래 걸리는 전송(복사, 올리기)은 파일 작업 목록(`transferStore.ts`)에 두고 파일 작업 창(셸 앱 `transfers`, `TransfersWindow.tsx`)이 그린다. 탐색기 창마다 상태를 따로 두는 원칙의 예외로 모듈 저장소에 둔다(작업은 그 창을 닫아도 이어지고 작업 창 하나에 모인다). 그 자리의 조작(삭제, 옮기기, 휴지통 작업)은 창의 고르기·목록에 묶여 있으므로 여기에 넣지 않고 창 안 배너(`runOp`)로 둔다. 둘은 진행 부품(`TaskProgress.tsx`의 `ProgressMeter`·`TaskCurrent`, 배너는 `TaskBanner`)과 실패 묶음(`groupFailures` + `FailuresBody`)을 함께 쓴다
- 작업은 종류와 관계없이 하나씩 차례로 한다. 올리기 작업 하나는 한 번에 고른 파일들이고(계산은 `uploadJobs.ts`), 진행률은 취소한 파일을 뺀 바이트다. 속도는 0.5초 간격 지수 평활(`nextRate`)이다
- 작업 창은 작업이 400ms를 넘을 때 열고(앱 목록·바로가기에는 보이지 않는다, `appAccess`), 모두 문제없이 끝나면 스스로 닫는다. 하지 못한 항목·건너뛴 특수 파일·멈춘 오류가 있거나 올리기를 취소해 다시 올릴 파일이 남았으면(`uploadJobs.ts`의 `unfinished`) (금방 끝났어도) 열어 둔다. 작업 창을 닫으면 남은 작업을 취소한다(창 저장소를 구독한다. 닫기 애니메이션 동안 목록이 그대로 보이게 목록은 다음 작업을 더할 때 비운다). 세션 만료(401)와 보안 확인 취소는 알림 대화상자 없이 취소로 두고, 다시 올릴 수 있게 작업 창에 남긴다
- 올리지 못한 파일은 이유별로 묶어 보이고, 끝난 작업에서 다시 올린다(`retryUpload`: 같은 이름이 생긴 파일은 "덮어써서 올리기", 그 밖의 실패와 취소한 파일은 "다시 올리기"). 파일 내용(`File`)은 작업이 목록에 있는 동안 둔다
- 보안 확인 대화상자는 작업을 더한 탐색기 창에 띄우고, 그 창이 닫혔으면 작업 창, 그것도 없으면 바탕화면에 띄운다(`liveScope`). 띄울 곳은 요청을 시작할 때가 아니라 `security_required`를 받은 때 고른다(`withStepUp`에 함수로 넘긴다). 사전 확인 뒤 PUT이 거절되면(그사이 정책이 바뀜) 프록시에 따라 거절이 본문을 다 보낸 뒤에야 와서 그사이 탐색기 창이 닫힐 수 있다
- 작업하는 동안 세션 활동(`auth/activity.ts`의 `reportActivity`, 세션 유지기가 60초에 한 번 연장)을 30초마다 알리고 `beforeunload`로 경고한다

## 휴지통

- 도우미가 FreeDesktop 휴지통 규격을 trash-put과 같은 곳·형식으로 다룬다(`.claude/docs/helper.md`의 "휴지통"). 서버의 `trash-list`·`trash-restore`·`trash-empty`와 같은 휴지통이다
- 라우트(`internal/server/files_trash.go`): `POST /api/files/trash {path}`(넣기, 보이는 폴더는 `root_folder`), `GET /api/files/trash`(모든 휴지통 항목, 시각은 밀리초), `GET /api/files/trash/volumes`(홈·디스크마다 `home`·`shared`·`own`·`none`), `POST /api/files/trash/restore {trash, name, to}`, `POST /api/files/trash/purge {trash, name}`, `POST /api/files/trash/empty`. 넣을 곳이 없으면 409 `no_trash`
  - 되돌리기는 브라우저가 보낸 `to`(목록의 원래 경로나 같은 폴더의 새 이름)를 `splitNew`·`allowedPath`로 거른 뒤 도우미에 넘긴다. 도우미는 `to`로만 되돌리므로 보이는 폴더 밖으로 되돌리지 않는다
  - 휴지통 경로(`trash`)는 도우미가 직접 찾은 사용자의 휴지통 디렉터리와 같을 때만 받는다
- **디스크 휴지통**: OMV 데이터 디스크 맨 위는 보통 root 소유라 사용자가 `.Trash-<uid>`를 만들 수 없다. 관리자가 설정 앱 서버 페이지(`FilesSection`)에서 디스크마다 "휴지통 만들기"를 누르면 `POST /api/site/files/trash {disk}`(관리자, 디스크는 `MountsUnder`에 있어야 함)가 도우미 root 연산 `trash.prepare`로 맨 위에 `.Trash`(1777)를 만든다. 상승하지 않았으면 `elevation_required`이고 화면이 `requestElevation` 뒤 다시 요청한다. 상태는 `GET /api/site/files/trash`(관리자의 사용자 권한으로 `fs.trashdirs`)
- 프런트 삭제(`FilesWindow`의 `deleteItems`): 지금 폴더의 볼륨(`fetchTrashVolumes` + `paths.ts`의 `volumeOf`)이 `none`이면 영구 삭제 확인(`DeleteBody` `noTrash`), 아니면 시안 S-Delete의 세 단추(영구 삭제… / 취소 / 휴지통으로 옮기기, 휴지통 단추에 포커스)다. Shift+Delete는 영구 삭제 확인으로 바로 간다. 휴지통에 넣지 못한 항목(`no_trash`, `cross_device`)은 `runOp`의 `keep`으로 모아 영구 삭제를 따로 묻는다. 사이드바의 휴지통 항목은 `data-drop-path`가 `TRASH`(`'trash:'`)라 끌어 놓으면 휴지통으로 옮긴다
- **휴지통 보기**(`TrashView.tsx`, 시안 AC-Trash): `FilesWindow`의 `trashOpen`이면 목록 대신 그린다(둘러보던 폴더·기록은 그대로, 뒤로 단추와 사이드바 항목으로 나간다). 모든 휴지통 항목을 지운 시각 순으로 이름·원래 위치·지운 날짜·크기 열에 보이고(`virtual.ts`로 보이는 줄만, `selection.ts`로 고르기), 찾기는 도구 막대 찾기어로 이름을 거른다. 되돌리기·영구 삭제·휴지통 비우기와 정보 패널(원래 위치, 지운 날짜, 보관한 곳)을 둔다. 하나를 되돌릴 때 원래 자리에 같은 이름이 있으면 `askName`(`restore`)으로 새 이름을 묻는다. 휴지통이 없는 보이는 폴더는 위에 알린다
- 휴지통이 바뀌면 변경 알림의 `dirs`에 `TRASH`를 넣는다. 사이드바의 항목 수와 휴지통 보기가 다시 받는다

## 창(프런트)

- `src/files/`: `FilesWindow.tsx`(사이드바·도구 막대·상태 표시줄, 상태와 이동, 만들기·이름 바꾸기·삭제·옮기기), `FileList.tsx`(가상 목록, 고르기, 끌어서 옮기기), `InfoPane.tsx`(정보 패널), `ConflictBody.tsx`·`conflictDialog.tsx`(이름 충돌), `NameForm.tsx`·`nameDialog.tsx`(이름 입력), `DeleteBody.tsx`(삭제 확인, 하지 못한 항목 안내), `TrashView.tsx`(휴지통 보기), `TransfersWindow.tsx`·`transferStore.ts`(파일 작업 창과 작업 목록), `TaskProgress.tsx`(진행 막대·지금 항목·창 안 진행 배너), `uploads.ts`(올리기 전 이름 확인), `stepUp.ts`(쓰기 요청의 보안 확인), 순수 모듈 `paths.ts`, `copyJobs.ts`, `history.ts`, `virtual.ts`, `pageCache.ts`, `kinds.ts`, `uploadJobs.ts`, `messages.ts`, `selection.ts`, `ops.ts`(Vitest `files.test.ts`)
- 셸 앱이고 창을 여러 개 연다(`multiInstance`). 상태는 창마다 컴포넌트에 두며 모듈 저장소에 두지 않는다. 권한 도우미가 없으면(`capabilities.helper`) 앱은 그대로 보이고 창 안에 못 쓰는 이유를 안내한다(`appAccess`의 `helper_missing`, `AddonUnavailable`). 도우미는 설정되어 있는데 닿지 않거나 티켓이 없으면 창이 `/api/files/roots`의 오류(`helper_unavailable`·`helper_reauth`)를 보인다
- 시안: Design 캔버스의 AC-Main·AC-Selected(정보 패널은 파일을 눌렀을 때만, 폴더는 고르기만), AC-Large(가상 스크롤·자리 표시·위치 말풍선), AC-Large-Sort(날짜순 준비 배너), S-Locked(보안 확인), S-Fallback(대체 안내)
- 틀은 `SidebarLayout`의 `header`(도구 막대)를 쓰고 좁은 기준은 720px다. 정보 패널은 내용 너비 640px 아래(컨테이너 쿼리)에서 목록 위에 덮는다
- 가상 목록: 줄 높이 34px(`virtual.ts`의 `ROW_HEIGHT`, CSS `.files-row`와 맞춤), 스크롤 높이가 800만 px를 넘으면 비율로 줄인다(브라우저 요소 높이 한계). 200줄 페이지를 보기마다 캐시하고(60페이지), 스크롤이 90ms 멈춘 뒤 받는다(동시 2페이지). 받은 페이지는 `useMemo`로 만든 보기별 캐시 객체에 쌓고 `bump`로 다시 그린다. 요청을 끊는 컨트롤러는 마운트마다 effect에서 만든다(StrictMode가 정리를 한 번 더 불러도 끊긴 채 남지 않게)
- `role="grid"` + `aria-rowcount`/`aria-rowindex`, 포커스는 스크롤 영역 하나에 두고 고른 줄을 `aria-activedescendant`로 가리킨다. 화살표·PageUp/Down·Home/End로 고르고 Enter로 열며, Backspace는 상위 폴더다. 키보드로 폴더에 들어가면 새 목록에 포커스를 돌려준다
- 비동기 결과는 요청(뿌리 `try`, 목록 `path`, 정보 패널 `path`)과 함께 상태에 두고, 지금 요청과 다르면 받는 중으로 본다(effect 안에서 상태를 동기로 지우지 않는다, oxlint `set-state-in-effect`)
- 시안의 아이콘 보기 전환은 아직 그리지 않는다(`ROADMAP.md`)
- 쓰기 요청(이름 대화상자, `runOp`의 항목, 올리기, 휴지통 작업)은 `stepUp.ts`의 `withStepUp`으로 보낸다. `security_required`면 창을 잠그지 않고 확인 대화상자(`requestStepUp`, 동시에 막힌 요청은 대화상자 하나를 함께 쓴다)를 띄운 뒤 한 번 다시 한다. 창 전체의 확인 화면은 뿌리·목록이 막혔을 때(읽기까지 막는 정책)만 보인다

## 고르기와 작업(프런트)

- **고르기**(`selection.ts`): 보기 안 자리의 구간(`spans`)과 기준(`anchor`), 위치(`focus`)다. 누르기, Ctrl(⌘)·Shift·Ctrl+Shift 누르기, Shift+화살표, Ctrl+A로 고르고 Esc나 빈 곳 누르기로 지운다. 보기·목록이 바뀌면 지운다. 받지 않은 줄까지 고를 수 있으므로 작업할 때 `FileListHandle.items`가 받은 줄은 그대로 쓰고 빠진 구간은 이름만(`names=1`, 2000개씩) 받아 채운다. 이름이 UTF-8이 아닌 항목은 경로로 가리킬 수 없어 작업에서 뺀다
- 정보 패널은 하나만 골랐고 그 줄이 파일일 때만 연다(여러 개를 고르거나 고르기를 지우면 닫는다). 패널의 이름 바꾸기·삭제 단추는 패널의 파일에 한다
- 메뉴(`FilesWindow`의 `menuItems`): 줄은 (하나일 때) 열기 또는 내려받기 / 이름 바꾸기(F2) / 삭제…(Delete, 여럿이면 "항목 N개 삭제…") / (파일 하나) 정보, 빈 곳은 폴더 만들기·올리기·모두 선택·새로 고침이다. 고르지 않은 줄에서 열면 그 줄 하나를 고른다. 쓸 수 없는 폴더에서는 비활성 이유로 "쓰기 권한 없음"을 보인다
- 이름 대화상자(`NameForm`)는 `PasskeyAdd`처럼 대화상자 안에서 요청하고 실패(`ops.ts`의 `opErrorKey`)를 입력란 아래에 보인다. 처음 포커스를 받을 때 이름 바꾸기는 확장자 앞까지(`stemLength`) 고른다. 새 폴더 이름은 `find`로 "새 폴더", "새 폴더 (2)"…에서 없는 것을 고른다
- 여러 항목 작업(삭제, 옮기기, 휴지통 넣기·되돌리기·영구 삭제·비우기, `ops.ts`의 `RunOp`)은 `runEach`로 하나씩 차례로 한다(도우미의 사용자당 동시 연산 4개 안에서 목록에 자리를 남긴다). 300ms가 지나도 끝나지 않으면 진행 배너(지금 항목 이름과 함께)와 "중지"(하던 항목까지 하고 멈춤)를 보이고, 하지 못한 항목은 이유별로 묶어(`groupFailures`) 알린다. 세션이 끝났거나(401) 보안 확인을 취소했거나 `stopOn`이면 남은 항목도 안 되므로 멈춘다. 옮기기는 미리 알지 못한 `cross_device`(같은 기준 안의 다른 마운트)를 만나면 거기서 멈추고 그 항목부터 복사로 넘긴다
- **끌어서 옮기기·복사하기**: HTML5 끌기는 `nativeGuard`가 `dragstart`에서 막으므로 포인터로 한다(마우스·펜만. 터치는 스크롤·길게 누르기 메뉴와 겹친다). 누른 줄에서 6px 넘게 움직이면 그때 pointer capture를 잡는다(그냥 누르기의 click은 줄로 간다). 고르지 않은 줄을 끌면 그 줄 하나를 고르되 정보 패널은 열지 않는다(끄는 도중 목록 폭이 바뀌면 놓을 곳이 움직인다)
  - 놓을 곳은 `elementFromPoint` + `[data-drop-path]`(폴더 줄, 경로 단계, 사이드바 항목, 다른 탐색기 창의 것 포함)다. 목록 자신(`.files-list`)도 쓸 수 있는 폴더면 그 폴더를 가리키므로, 다른 창의 빈 곳이나 파일 줄에 놓으면 그 창의 폴더로 간다(목록 둘레에 점선으로 표시). 지금 폴더와, 끄는 폴더 자신·그 안은 놓을 곳이 아니다. 표시(`DROP_TARGET`)와 이름표(`.files-drag-ghost`, body에 붙이고 포인터를 받지 않음)는 렌더 없이 DOM으로 바꾼다
  - 놓으면 할 일은 창의 `dropMode`(`copyJobs.ts`)가 정하고 이름표에 "옮기기"·"복사"·"휴지통으로"로 보인다. 원본 폴더와 대상이 다른 디스크(휴지통 볼륨의 `top`이 다름)면 복사, 같으면 옮기기다. 옮기기·휴지통은 원본 폴더에 쓸 수 있어야 하므로 읽기 전용 폴더에서도 끌 수 있지만 다른 디스크로만 놓인다. 볼륨을 받지 못했으면 옮겨 본다
  - 목록 위·아래 32px 띠에서는 스크롤하고, Esc나 창 포커스를 잃으면 취소한다. 끌기를 끝낸 뒤 따라오는 click 하나는 무시한다(Esc로 취소하면 놓기 전에 끝나므로 다음 누름까지 남긴다)
- **바깥 파일 끌어 놓기**: `.files-content`가 `dragover`·`drop`을 받는다(`nativeGuard`는 `dragstart`만 막는다). 폴더 줄 위면 그 폴더, 아니면 지금 폴더(쓸 수 있을 때, 목록 자신의 `data-drop-path`는 폴더 줄로 보지 않는다)에 올린다(`uploadFiles`). 폴더(`webkitGetAsEntry().isDirectory`)는 아직 올리지 않고 이름을 알린다
- **창 사이 변경 알림**(`ops.ts`의 `notifyFilesChanged`·`onFilesChanged`): 창마다 상태를 따로 두므로 모듈 저장소 대신 알림만 나눈다. 바뀐 폴더(`dirs`)를 보거나 없어진 경로(`gone`) 아래를 보는 창이 목록을 다시 열고, 없어진 파일의 정보 패널을 닫는다. 올리기가 끝날 때도 같은 알림을 쓴다
  - 같은 세션의 목록 캐시는 경로마다 하나라 여러 창이 함께 `refresh=1`로 읽으면 서로 밀어내고(410) 고르기를 잃는다. 그래서 바꾼 창(`origin`)만 바로 새로 읽고(`refreshed`), 같은 폴더를 보는 다른 창은 300ms 뒤 `refresh` 없이 그 목록을 함께 쓴다. 만든 항목 고르기(`pendingSelect`)는 `listing_expired`면 다음 목록에서 다시 찾는다

## 테스트

- Go: `internal/files`(정렬, 스냅숏 해석 8.0·8.1+, 보기·찾기·자리, 캐시 상한·유휴), `internal/server/files_test.go`(`envOptions{files: true}`가 가짜 도우미 디스크를 디스크·`FilesSrvDir`로 둔다: 뿌리·스냅숏·관리자 로그인 갱신, 범위 밖 403, 목록·범위·prepare·find·410, 내려받기 헤더·Range·If-Range, 올리기 형식·길이·이름·덮어쓰기, 만들기·이름 바꾸기·옮기기(같은 디스크, 홈으로는 `cross_device`)·삭제(`not_empty`)와 `root_folder`(디스크 최상위 보기 포함), 보안 정책)
- Go(휴지통): `TestFilesTrash`(홈·디스크 휴지통, 볼륨 종류, 되돌리기 범위·충돌, 영구 삭제, 비우기), `TestSiteTrashPrepare`(관리자, 상승 필요, 만든 뒤 `.Trash/<uid>`), `TestFilesSecurityWritesOnly`·`TestFilesScopeChangeNeedsStepUp`(쓰기 전용 범위)
- Go(올리기): `TestFilesUploadCheck`(가드·이름·범위, 만들지 않음), `TestDrainRejected`(남은 본문을 받는 양과 상한, 성공·본문 뒤 오류는 그대로), `TestFilesUploadEarlyRejectNoReset`(본문을 끝까지 보내는 클라이언트가 실제 TCP에서 RST 없이 403을 받음, 루프백 버퍼보다 큰 32MiB)
- Go(복사): `worker/copy_test.go`(다른 디스크로 파일·폴더·링크, 특수 파일 건너뛰기, 시각·실행 비트, 이름마다의 실패, 시작 전 실패, 끊으면 임시 이름 지우기), `TestFilesCopy`(사건 흐름, 같은 이름, 시작 전 오류 응답, 인자 크기)
- Vitest: 고르기 구간, 새 이름 검사·확장자 앞 고르기, `moveAllowed`, `volumeOf`, `runEach` 중지·실패 묶기, 변경 알림, 끌어 놓기의 할 일(`dropMode`), 복사 요청 나누기·진행률·속도, 올리기 작업(차례, 다시 올리기·덮어쓰기, 멈추기, 양), 복사 응답 사건 읽기와 올리기 사전 확인(거절하면 본문을 보내지 않음, `client.test.ts`)
- 브라우저(복사): 가짜 홈과 가짜 디스크는 기준 디렉터리가 달라 홈 ↔ 디스크가 복사다. 작업 창을 보려면 파일 수만 개짜리 폴더를 쓴다(큰 파일 하나는 로컬 디스크에서 금방 끝난다)
- 브라우저(휴지통): 가짜 디스크 맨 위(`helper-root/srv/disk1`)는 개발 사용자 소유라 처음에는 `own`이다. `none`(휴지통 없음 안내)은 맨 위를 잠시 `chmod 0555`로 바꿔 보고, 디스크 휴지통 만들기는 상승할 수 있는 `totp`로 확인한다. 서버의 `trash-list --trash-dir=<휴지통>`으로 형식 호환을 볼 수 있다
- `make dev-fake`: 가짜 OMV가 가짜 도우미 디스크(`helper-root/srv/disk1`)에 합성 파일과 공유 폴더(문서, 미디어, 사진, 폴더 없는 백업, 루트 파일시스템의 kvm)를 만든다(`cmd/fakeomv/files.go`). `FAKEFILES_LARGE=<n>`이면 사진/휴대폰 백업에 빈 파일 n개를 만든다. onmaru는 개발 전용 환경 변수 `ONMARU_DEV_FILES_SRV`(플래그·가동 설정 목록에 넣지 않음)로 그 아래 폴더를 디스크로 본다
- 브라우저: Playwright 컨테이너는 문자셋 때문에 한글 이름 내려받기를 `download`로 저장한다(Blob도 같다). 한글 파일 이름은 실제 브라우저에서 확인한다
  - 끌어서 옮기기는 `page.mouse`(down → 여러 단계 move → up)로, 바깥 파일 끌어 놓기는 페이지 안에서 `DataTransfer`에 `File`을 넣어 `dragover`·`drop` 이벤트를 보내 확인한다
