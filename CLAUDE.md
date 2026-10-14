# OMV 데스크톱 대시보드

<!-- 프로젝트 이름이 정해지면 위 제목을 바꿔주세요. HTML 주석은 Claude 컨텍스트에서 제외되므로 메모용으로 써도 됩니다. -->

OpenMediaVault 8(OMV 8) 위에 올리는, 시놀로지 DSM처럼 창·작업표시줄·아이콘이 있는 데스크톱형 웹 대시보드.

## 목표와 범위

- 가동 현황 확인과 간단한 조작은 이 대시보드에서, 심층 설정은 OMV 기본 웹 UI(workbench)에서 한다
- OMV를 대체하지 않는다. OMV에 이미 있는 복잡한 설정 화면을 다시 만들지 말고 해당 OMV 페이지로 연결한다
- 로컬 네트워크 전용. 외부 인터넷 노출을 전제로 한 기능이나 설정을 만들지 않는다

## 실행 환경

- Debian + OMV 8 NAS. 개발과 실행 모두 NAS 안에서 한다
- 프로젝트는 일반 사용자 계정 홈 아래에 있고, Claude Code도 일반 사용자 권한으로 실행된다

## 절대 규칙

- OMV가 관리하는 설정 파일(`/etc/` 아래 서비스 설정, OMV 설정 DB)을 직접 수정하지 않는다. OMV는 자기 DB를 기준으로 Salt로 설정 파일을 다시 생성하므로 직접 수정은 덮어써진다. 설정 변경은 반드시 로그인한 사용자의 세션으로 `rpc.php`를 호출해서 한다
- `omv-rpc` CLI는 쓰지 않는다. 엔진 소켓(`/var/lib/openmediavault/engined.sock`)에 직접 붙어 비밀번호 확인 없이 관리자로 실행되므로, root나 `openmediavault-engined` 그룹만 쓸 수 있다. 어떤 계정도 이 그룹에 추가하는 방안을 제안하지 않는다(사실상 root 권한 부여)
- `sudo`나 root 권한이 필요한 명령은 실행하지 않는다. 필요하면 명령과 이유를 제시하고 사용자가 직접 실행하게 한다
- 시스템 상태를 바꾸는 RPC(서비스 재시작, 디스크·파일시스템·공유 폴더 변경 등)는 코드로 작성만 하고, 실제 호출 테스트는 사용자에게 맡긴다
- 비밀번호, OMV 세션 ID, 토큰을 로그·파일·커밋·테스트 픽스처에 남기지 않는다
- 마지막 완료 단계의 바로 다음 단계가 허용하지 않는 기능(아래 "개발 단계")은 구현하지 않는다

## 아키텍처

- **배포 형태**: OMV와 같은 출처(same origin)로 서빙한다. 앞단의 Caddy가 HTTPS를 처리하고, 같은 도메인에서 `/desk/*`는 이 프로젝트 백엔드로, 나머지는 OMV로 리버스 프록시한다. OMV의 nginx 설정은 건드리지 않는다
  - 도메인 등 개인 환경 정보는 `CLAUDE.local.md`에 있다. 커밋되는 파일에 도메인·IP를 하드코딩하지 않는다
- **실행 방식**: systemd 서비스로 Go 실행 파일 하나를 전용 일반 사용자로 실행한다. Docker는 쓰지 않는다(호스트 `/proc`, `/sys`, 네트워크를 직접 봐야 하므로)
  - 프론트엔드 빌드 결과는 `go:embed`로 실행 파일에 포함한다
  - 백엔드는 127.0.0.1에만 바인딩한다
  - systemd 유닛 파일과 Caddy 설정 예시는 `deploy/`에 작성만 하고, 설치는 사용자가 직접 한다
- **프론트엔드**: 브라우저 안의 데스크톱 셸(창 열기·닫기·이동·포커스·z-order, 작업표시줄, 바탕화면 아이콘)
- **백엔드**: 얇은 API 서버. 역할은 두 가지뿐이다
  1. 현황 읽기: `/proc`, `/sys` 직접 읽기, root가 필요한 정보(SMART, 컨테이너 등)는 OMV RPC로 읽기
  2. 설정 변경 요청을 OMV `rpc.php`로 전달
- **OMV RPC는 항상 백엔드가 중계한다**: 브라우저 → `/desk/rpc` → 백엔드 → OMV `rpc.php`. 브라우저가 `rpc.php`를 직접 호출하지 않는다
  - 백엔드는 대시보드 세션마다 OMV 세션(쿠키 또는 `X-OpenMediaVault-SessionID`)을 메모리에 보관하고, 항상 같은 주소(127.0.0.1)와 고정 User-Agent로 OMV를 호출한다
  - 브라우저의 쿠키를 OMV에 그대로 전달하지 않는다. OMV는 IP·User-Agent가 다르면 그 세션을 파기하므로, 확인하려다 사용자를 로그아웃시키게 된다
  - 같은 출처 구성은 OMV 화면을 iframe으로 띄우는 실험을 위해 유지한다. 인증과는 무관하다

## 기술 스택

- **백엔드**: Go(표준 라이브러리 `net/http`만 사용, 런타임 외부 의존성 없음). 모듈은 `backend/`
  - `cmd/omvdesk/`: 진입점. `-listen`(또는 `OMVDESK_LISTEN`, 기본 `127.0.0.1:8090`), 루프백이 아닌 주소는 거부. `-omv-url`(`OMVDESK_OMV_URL`, 기본 `http://127.0.0.1`, 루프백만 허용), `-cookie-path`(`OMVDESK_COOKIE_PATH`, 기본 `/desk`)
  - `cmd/fakeomv/`: 개발용 가짜 OMV 서버(`internal/omv/omvfake`). 운영 바이너리에는 들어가지 않는다
  - `internal/omv/`: rpc.php 클라이언트(고정 User-Agent, 세션 ID는 헤더로 보내고 응답 `Set-Cookie`로 갱신). `omvfake/`는 테스트·개발용 가짜 rpc.php
  - `internal/auth/`: 대시보드 세션 저장소(메모리)와 로그인 시도 제한(IP·사용자별)
  - `internal/api/`: JSON 응답 구조체. TS 타입 생성의 원본
  - `internal/sysinfo/`: `/proc` 파서(`io.Reader` 입력)와 `Reader`(`fs.FS` 주입). 픽스처는 `testdata/`
  - `internal/server/`: 라우팅. `/api/*`는 대시보드 루트 기준이며 `/desk` 접두사는 리버스 프록시(Vite·Caddy)가 제거한다. 인증 라우트와 `requireSession` 미들웨어는 `auth.go`
  - `internal/web/`: `go:embed`로 `static/dist/`(Vite 빌드 결과) 서빙. `static/.gitkeep`만 커밋한다
- **타입 공유**: tygo(go.mod의 `tool` 지시어로 고정, 빌드 시에만 사용). `backend/internal/api` → `frontend/src/api/types.gen.ts`. 생성물은 커밋하며 직접 수정하지 않는다
- **프론트엔드**: TypeScript + Vite + React, 창 상태는 Zustand 단일 저장소. 린트는 oxlint, 테스트는 Vitest
  - Vite `base`는 `/desk/`. 개발 서버는 `/desk/api/*`를 `/desk`를 뗀 뒤 백엔드로 프록시한다
  - `server.fs.allow`는 `frontend/`와 `vendor/icons/`만 허용한다(개발 서버가 LAN에 열려 있으므로 저장소 루트 전체를 허용하지 않는다)
  - 개인 환경 값(`DEV_HOST`, `DEV_ALLOWED_HOSTS`, `OMVDESK_LISTEN`)은 커밋하지 않는 `frontend/.env.local`에 둔다
  - `src/windows/`: 창 상태 저장소(`store.ts`), 앱 목록(`apps.ts`), 창 위치·크기 제한(`geometry.ts`), 타일링 계산(`tiling.ts`). DOM에 의존하지 않아 node 환경에서 테스트한다
    - 타일링: 창을 끄는 포인터가 위 가장자리에 닿으면 최대화, 좌우 가장자리에 닿으면 반쪽 배치(모서리에서는 좌우 우선). 최대화·타일 창의 위치는 `tileRect`가 계산하며 가이드(`shell/TilePreview.tsx`)도 같은 함수를 쓴다. 좌우 타일 창은 `tileSplit` 하나를 공유하는 타일 그룹이라, 경계(창 안쪽 변 `TileResizeHandle`, 두 창 사이 간격 `TileDivider`, 공용 훅 `useTileSplitDrag.ts`)를 끌면 함께 크기가 바뀌고, 작업표시줄에서 누르면 짝 창도 함께 앞으로 온다(`raiseGroup`). 코드에서는 "타일링(tile)"이라는 용어만 쓴다
    - `windows[]` 순서가 작업표시줄 순서다. `Desktop`은 `stableOrder`로 창을 처음 연 순서대로 그려, 작업표시줄 순서를 바꿔도 창 DOM이 옮겨지지 않게 한다(옮겨지면 창 안의 스크롤 위치·iframe이 초기화된다)
  - `src/shell/`: 데스크톱 셸 컴포넌트(바탕화면, 창 프레임, 작업표시줄)
  - `src/shell/nativeGuard.ts`: 텍스트 선택·드래그·브라우저 컨텍스트 메뉴는 기본 금지(선택은 `index.css`의 `user-select`, 나머지는 문서 전체 리스너). `selectable` 클래스는 텍스트 선택이 꼭 필요한 영역에만 붙인다(시스템 정보 같은 현황 수치는 선택 불가). 텍스트 입력 요소(input, textarea, contenteditable)는 자동 허용
  - `src/shell/taskbarDrag.ts`: 작업표시줄 버튼 드래그 정렬의 배치 계산(DOM 없음). 드래그 중에는 transform으로만 움직이고 놓은 뒤 순서를 반영한다. 전환 효과는 부모가 아닌 각 `li` 클래스에 둔다
  - `src/shell/taskbarAnim.ts`: 작업표시줄 버튼 등장·퇴장 애니메이션 설정값과 그릴 순서 계산(`withLeaving`, DOM 없음). 등장은 `li`의 ref(`enterTask`)로, 퇴장은 `Taskbar`가 너비·투명도를 애니메이션한다. 닫힌 창의 버튼은 창과 같은 `useClosingWindows` 훅으로 마지막 상태를 받아 두고 `.leaving` 버튼으로 남긴다(드래그 정렬은 이 버튼을 빼고 잰다)
  - `src/shell/windowAnim.ts`: 창 열기·닫기·최대화·타일링·최소화 애니메이션 설정값과 FLIP 위치 계산(DOM 없음). 재생은 `useWindowAnimation.ts`(Web Animations API, transform·opacity만). 켜기/끄기는 `WINDOW_ANIMATIONS` 상수이며 설정 기능이 생기면 그쪽으로 옮긴다
    - 닫기: 저장소(`close`)에서는 창이 바로 지워져 작업표시줄·포커스는 즉시 바뀐다. `Desktop`이 `useClosingWindows`(저장소 구독)로 지워진 창의 마지막 상태와 타일 비율(`ClosingWindow`)을 받아 두고, 닫기 애니메이션이 끝나 `onClosed`가 불릴 때까지 그 상태로 그린다(`stableOrder`의 `keep`). 닫히는 창은 클릭을 받지 않고 창 내용의 주기 요청도 바로 멈춘다
  - `src/shell/screenAnim.ts`: 로그인 화면 ↔ 데스크톱 전환 애니메이션 설정값(DOM 없음). 로그인은 로그인 카드가 커지며 흐려져 사라진 뒤 데스크톱 내용이 커지며 또렷해지고, 로그아웃·세션 만료는 반대로 줄어든다. 배경(`.desktop`)은 그대로 두고 내용(`.login-card`, `.desktop-area`·`.taskbar`)만 움직인다. 재생은 `screenTransition.ts`이며, 나가는 애니메이션은 auth 저장소가 화면을 바꾸기 전에 `beforeScreenChange` 훅(`leaveScreen`)으로 재생하고, 들어오는 애니메이션은 각 화면이 처음 그려질 때 재생한다
  - `src/shell/Taskbar.tsx` 오른쪽 트레이: `[시계][프로필]` 순서이며 프로필 버튼이 오른쪽 끝에 고정된다. 시계는 `Clock.tsx`(서식·갱신 시점 계산은 `clock.ts`). 프로필 메뉴는 `ProfileMenu.tsx`이고 항목은 `profileMenu.ts`의 `PROFILE_MENU_ITEMS`에 데이터로 둔다(`requires: 'admin'`으로 권한별로 거름). 항목은 `action`(누르면 동작)과 `submenu`(하위 메뉴, 지금은 언어 선택) 두 종류다. action 항목을 추가하면 `ProfileMenu.tsx`의 `actions`에 동작도 넣는다. 아바타 그리기는 `ProfileAvatar` 한 곳에 모은다
  - `src/auth/`: 로그인 상태 저장소(`store.ts`, `createAuthStore`로 API·화면 전환 훅 주입. 로그아웃·만료로 떠나는 동안은 상태를 `loggedIn`으로 두고 다시 들어온 만료·로그아웃은 무시한다), 로그인·연결 끊김 화면(`LoginScreen.tsx`), 오류 문구(`messages.ts`), 세션 유지·만료 판단(`sessionKeeper.ts`의 순수 함수 `decide`, 실행은 `useSessionKeeper.ts`). API 클라이언트는 401을 받으면 `setUnauthorizedHandler`로 등록한 `expire()`를 부른다(로그인 관련 요청 제외)
  - `src/sysinfo/`: 시스템 정보 창. 주기 요청은 `poller.ts`(응답 후 다음 요청 예약, 중첩 없음)
  - `src/icons/`: 리졸버와 `IconImage`(컬러는 `<img>`, symbolic은 `mask-image`)
  - `src/i18n/`: 현지화. 아래 "현지화" 항목 참고
  - 컴포넌트 테스트용 DOM 환경(jsdom 등)은 쓰지 않는다. 로직을 순수 함수·저장소로 분리해 테스트하고, 화면 동작은 브라우저에서 확인한다
- **현지화**: i18next + react-i18next. 지원 언어는 `en`, `ko`이며, 지원하지 않는 브라우저 언어와 번역이 빠진 키는 영어로 표시한다. 백엔드는 문구가 아닌 오류 `code`만 주고 번역은 프론트에서만 한다
  - 화면에 나오는 모든 문구(aria-label·title 포함)는 `t()`를 거친다. 카탈로그는 `src/i18n/locales/<언어>.json`(중첩 JSON, i18next JSON v4 형식: 치환자 `{{name}}`, 복수형 `_one`/`_other`). 새 키는 `en.json`과 `ko.json`에 함께 넣는다. 키 타입은 `en.json`에서 나오므로(`i18next.d.ts`) 없는 키는 tsc가 잡고, `catalog.test.ts`가 언어 사이의 키·치환자 일치를 검사한다
  - 저장소·데이터에는 문구가 아니라 키·코드를 둔다(`apps.ts`의 `titleKey`, `profileMenu.ts`의 `labelKey`, `auth/messages.ts`의 `LoginError.key`). 그래야 언어를 바꿀 때 이미 그려진 창 제목·오류도 바로 바뀐다
  - 숫자·날짜·시간 단위는 번역 키 대신 `Intl`로 표시하고(`sysinfo/format.ts`, `shell/clock.ts`), locale은 인자로 받는다. 컴포넌트는 `useLanguage()`로 현재 언어를 얻어 넘긴다
  - `src/i18n/index.ts`(초기화, `setLanguage`, `useLanguage`)는 document·localStorage를 만지므로 컴포넌트와 `main.tsx`에서만 import한다. node 테스트 대상 모듈은 `languages.ts`(지원 목록, `pickLanguage`, `languageName`)와 `i18next`의 타입만 import한다
  - 언어 선택: 저장값(localStorage `omvdesk.language`) → `navigator.languages` → 영어. 로그인 상태가 아닌 브라우저별 편의 설정이라 localStorage에 둔다. 바꾸는 곳은 로그인·연결 끊김 화면의 `shell/LanguageSelect.tsx`와 프로필 메뉴의 언어 하위 메뉴. 언어 이름은 번역하지 않고 `Intl.DisplayNames`로 그 언어 자신의 이름을 쓴다
  - 언어 추가: `locales/<code>.json`을 만들고 `languages.ts`의 `SUPPORTED_LANGUAGES`와 `index.ts`의 `resources`에 등록한다. 일부만 번역해도 된다(빠진 키는 영어). 언어가 많아지면 정적 번들 대신 동적 `import()` + `addResourceBundle`로 지연 로딩한다
- **아이콘**: Colloid를 git submodule(shallow)로 `vendor/icons/colloid/`에 둔다. 리졸버는 `frontend/src/icons/resolver.ts`이며, 매핑 테이블에 `?url` import로 한 줄씩 추가한 아이콘만 번들에 들어간다(디렉터리 glob 금지)

## 명령어

저장소 루트에서 실행한다. 처음 클론했다면 `git submodule update --init --depth 1`과 `npm --prefix frontend ci`를 먼저 실행한다.

| 명령 | 내용 |
|---|---|
| `make dev` | 개발 서버. 백엔드 `127.0.0.1:8090`(`go run`) + Vite `0.0.0.0:5173`. 접속은 `http://<NAS-IP>:5173/desk/`. Ctrl+C로 둘 다 종료. 실제 OMV에 연결하므로 `frontend/.env.local`에 `OMVDESK_OMV_URL`(이 NAS의 OMV 주소)을 둔다 |
| `make dev-fake` | `make dev`와 같지만 가짜 OMV(`127.0.0.1:8091`)를 함께 띄워 연결한다. 가짜 계정은 "테스트 전략" 참고. `FAKEOMV_TIMEOUT=1m make dev-fake`처럼 세션 timeout을 분 단위로 바꿀 수 있다 |
| `make build` | 프론트 빌드 → `backend/internal/web/static/dist/` → `bin/omvdesk` 단일 실행 파일 |
| `make lint` | `gofmt -l`, `go vet`, oxlint, `tsc -b` |
| `make test` | `go test ./...`, `vitest run` |
| `make check` | lint + test |
| `make types` | Go 구조체에서 TS 타입 다시 생성. `internal/api`를 바꾸면 반드시 실행하고 생성물도 함께 커밋한다 |

개별 실행: `cd backend && go test ./internal/sysinfo -run TestParseMemInfo`, `cd frontend && npx vitest run src/icons`

## 확인된 OMV 8 동작

OMV 공식 저장소 소스(2026-10 기준)에서 확인한 내용이다. OMV 업데이트 후 동작이 다르면 이 섹션을 갱신한다.

- **로그인 흐름은 두 단계**: `Session.login`(`username`, `password`) 응답의 `status`가 `authenticated`면 완료, `challengeRequired`면 TOTP 등 2차 인증이 필요하다. 이때 같은 세션으로 `Session.verify`(`challengeresponse`)를 호출한다. 2FA를 쓰지 않더라도 이 분기를 항상 구현한다
- **2차 인증 시 세션 ID**: `challengeRequired`일 때 OMV는 세션 ID를 바꾸고(`session_regenerate_id`) 새 ID를 응답 본문 없이 `Set-Cookie`로만 준다. `Session.verify`는 그 ID로 호출해야 한다. `challengeresponse`의 형식은 2FA 플러그인이 정한다(이 NAS에는 2FA 플러그인이 없어 미검증)
- **대기 중 세션 주의**: 인증되지 않은 세션으로 엔진 RPC(Session 밖의 서비스)를 부르면 OMV가 세션을 파기하므로, 2차 인증 대기 중인 세션으로는 다른 RPC를 부르지 않는다
- **세션 쿠키**: 이름 `OPENMEDIAVAULT-SESSIONID`, HttpOnly, SameSite=Strict, HTTPS일 때 Secure. 쿠키 대신 `X-OpenMediaVault-SessionID` 헤더도 받는다
- **세션 바인딩**: 세션은 User-Agent에 묶이며 다르면 서버가 세션을 파기한다(`Session.logout` 같은 로컬 서비스 호출 시 검사). IP 검사(`validateIpAddress`)는 OMV 소스에서 꺼져 있다. 한 주체가 받은 세션을 다른 주체에게 넘겨 쓰는 설계는 하지 않는다
- **클라이언트 IP**: 인증 로그의 IP는 `X-Forwarded-For` 등 프록시 헤더를 그대로 믿어 정한다(세션 검증에는 쓰지 않음). 백엔드는 브라우저 IP를 `X-Forwarded-For`로 넘긴다
- **세션 만료**: OMV 웹 관리 설정의 timeout 값(기본 5분)을 따른다. 인증된 호출마다 마지막 접근 시각이 갱신되며, `options.updatelastaccess=false`면 갱신하지 않는다. RPC가 401을 반환하면 로그인 화면으로 보낸다
- **세션 확인용 RPC**: `WebGui.getAutoLogoutSettings`. 엔진 RPC라 인증된 세션이 필요하고 역할 제한이 없으며, 자동 로그아웃 시간(`timeout`, 분, 0이면 꺼짐)을 돌려준다. OMV workbench도 이 값으로 브라우저에서 자동 로그아웃 타이머를 돈다. `System.noop`은 admin 전용이다
- **로그인 실패 제한**: 비밀번호 실패에는 OMV 쪽 지연이 없다(2차 인증 실패만 제한, 429). 그래서 백엔드가 자체 제한을 둔다
- **새 브라우저 로그인 메일**: 로그인 성공 시 `OPENMEDIAVAULT-LOGIN-*` 쿠키가 없으면 "새 브라우저 로그인" 알림 메일을 보낸다. 백엔드가 이 쿠키(비밀 아님)를 사용자별로 메모리에 보관해 다음 로그인에 실어 보내므로, 백엔드 재시작 후 첫 로그인에만 메일이 간다
- **권한**: 로그인 응답의 `permissions.role`에 admin 포함 여부가 들어 있다. 관리자가 아니면 설정 관련 창과 버튼을 숨긴다
- **workbench의 탭별 로그인**: OMV workbench는 사용자 이름과 권한을 sessionStorage에 저장하고 그것으로 로그인 여부를 판단해서, 새 탭마다 다시 로그인해야 한다
- **보안 헤더**: 기본값이 `X-Frame-Options: SAMEORIGIN`, CSP `default-src 'self'`. OMV 화면을 iframe에 넣는 건 같은 출처에서만 가능하다. 보안 헤더를 끄는 방식은 쓰지 않는다

## 인증 설계

- 로그인 화면은 이 대시보드에서 자체 디자인하고, 인증은 백엔드가 OMV `Session.login`(필요하면 `Session.verify`)을 호출해 위임한다. 계정·비밀번호를 따로 저장하지 않는다
- 로그인 성공 시 백엔드가 대시보드 자체 세션 쿠키(`omvdesk_session`, HttpOnly, SameSite=Strict, Path `/desk`)를 발급한다. Secure는 HTTPS로 들어온 요청(`X-Forwarded-Proto: https`)에만 붙인다(평문 HTTP 개발 서버에서는 브라우저가 Secure 쿠키를 저장하지 않으므로). 세션 저장소는 메모리이며 백엔드 재시작 시 모두 재로그인한다
- 백엔드의 `/api/*`는 대시보드 세션이 없으면 401을 반환한다(`/api/health`, 로그인 라우트 제외)
- 라우트: `POST /api/login`, `POST /api/login/verify`(2차 인증, 대기 쿠키 `omvdesk_pending`), `POST /api/logout`, `GET /api/session`(세션 복원), `POST /api/session/activity`(사용자 활동 신호). POST는 `Content-Type: application/json`만 받는다(CSRF 보조 장치)
- **세션 만료는 브라우저 타이머로 처리한다**
  - 백엔드는 로그인 직후와 확인 때마다 `WebGui.getAutoLogoutSettings`로 timeout을 읽는다. OMV 세션의 마지막 접근 시각을 갱신하는 것은 이 백엔드뿐이므로(로그인, 활동 신호) 마지막 갱신 시각 + timeout이 만료 시각이다. `/api/session`, `/api/session/activity`, 로그인 응답의 `expiresInSeconds`(남은 초. 시계 차이를 피하려고 절대 시각을 쓰지 않음)로 알려 준다
  - 프론트(`sessionKeeper.ts`)는 사용자 입력이 있으면 최대 60초에 한 번 `/api/session/activity`로 연장한다. 아직 보내지 않은 입력이 있으면 만료 10초 전에는 반드시 보낸다(timeout이 1분이어도 조작 중에는 만료되지 않게). 입력 없이 만료 시각이 지나면 2초 뒤 `/api/session`으로 확인하고, 401이면 로그인 화면으로 보낸다(다른 탭이 연장했으면 새 만료 시각을 받는다)
  - 주기 요청(시스템 정보 갱신 등)과 세션 확인은 `updatelastaccess=false`라 OMV 세션을 연장하지 않는다
  - 안전장치로 백엔드는 요청 처리 중 마지막 확인에서 60초가 지났거나 예상 만료 시각이 지났으면 OMV에 확인한다(OMV 설정 변경, OMV 재시작 등). OMV에 연결할 수 없을 때는 세션을 유지한다
  - JWT는 쓰지 않는다. OMV 세션 ID 때문에 서버 쪽 세션 저장소가 어차피 필요하고, HttpOnly 쿠키 안의 `exp`는 브라우저가 읽을 수 없으며, 조작할 때마다 만료가 늘어나는 구조와 맞지 않는다
- 로그인 상태를 localStorage·sessionStorage에 저장하지 않는다. 앱 시작 시 백엔드에 세션 유효 여부를 물어 상태를 복원한다 (OMV workbench처럼 탭마다 로그인하지 않게 하기 위함)
- OMV가 중계 호출에 401을 반환하면 해당 대시보드 세션을 지우고 브라우저를 로그인 화면으로 보낸다
- 로그인 시도 횟수 제한과 실패 시 지연을 둔다: 실패 응답 전 1초 지연, IP·사용자 이름별로 15분 안에 5번째 실패부터 30초에서 두 배씩 최대 15분 잠금(429, `Retry-After`), 한 IP의 동시 시도는 2개까지
- 로그에는 사용자 이름·IP·결과만 남긴다. 비밀번호, 대시보드·OMV 세션 ID, OMV 오류의 `trace`는 남기지 않고 브라우저에도 넘기지 않는다
- 로그아웃하거나 다른 사용자가 로그인하면 창을 모두 닫는다. 세션 만료 후 같은 사용자가 다시 로그인하면 창 배치를 남긴다

## 애드온과 데이터 출처

3단계에서 도입한다. 출처별 상세 표와 플러그인 조사는 `docs/data-sources.md`, 애드온 구조는 `docs/addons.md`에 있다.

- **애드온 = manifest + provider + 창 컴포넌트**. 시스템 정보를 포함한 모든 기능 창을 같은 계약으로 만든다
  - manifest: id, 제목, 아이콘, 창 기본·최소 크기, `requires`(필요한 OMV 플러그인 패키지, admin 여부), 사용할 OMV RPC 허용 목록(read·write 구분)
  - `/desk/rpc` 중계는 manifest 허용 목록에 있는 메서드만 통과시킨다. 현재 단계가 허용하지 않는 write는 백엔드가 거부한다
- **내장 애드온만 둔다**. 정적 목록으로 등록한다(백엔드 `internal/addons/<id>/`, 프론트 `src/addons/<id>/`). 지금의 `windows/apps.ts`처럼 컴파일 타임 타입(`AppId`)을 유지한다. 별도 패키지, 런타임 로딩, Go `plugin`은 쓰지 않는다. 기능 추가는 포크·기여로 한다
- **capability 감지**: 플러그인 설치 여부는 `/usr/share/openmediavault/engined/rpc/<name>.inc` 존재와 `/var/lib/dpkg/status`로 확인한다(둘 다 root 없이 읽힘). 세션 역할과 합쳐 `/api/capabilities`로 내려주고, 프론트는 쓸 수 없는 창을 숨기거나 "플러그인 필요"로 안내한다. RPC 404(`RPC service '...' not found.`)는 마지막 안전장치로만 쓴다
- **provider**: 한 항목에 출처가 여러 개면 권한이 필요 없는 직접 읽기(`/proc`, `/sys`)를 1차로, OMV RPC를 보강으로 쓴다. 응답에 출처를 함께 담는다
- **권한 원칙**: 서비스 계정을 `docker`·`openmediavault-engined` 그룹에 넣지 않고 sudo도 쓰지 않는다. root가 필요한 정보(SMART, SATA 디스크 온도, 컨테이너)는 OMV RPC로만 가져온다. Docker·Podman 소켓을 직접 읽는 provider는 만들지 않는다
- **컨테이너**: compose 플러그인(omv-extras) RPC로 가져오며 Docker 런타임 기준으로 검증한다. compose의 Podman 모드는 미검증으로 표시한다
- **런타임 애드온**(선언형 YAML, 서드파티 코드)은 범위 밖이다. 막히지 않게만 설계하며, 고려 사항은 `docs/addons.md`의 "향후 고려"에 있다

## 아이콘

- 아이콘 테마는 **Colloid**(vinceliuice/Colloid-icon-theme, GPL-3.0) 하나만 쓴다. 다른 테마와 섞지 않는다
- 아이콘은 경로를 직접 쓰지 말고 리졸버(`getIcon(name)`)를 통해 freedesktop 아이콘 이름(`folder-music`, `drive-harddisk` 등)으로 가져온다. 테마 교체는 리졸버 설정만 바꾸면 되게 한다
- 컬러 아이콘(폴더·앱·파일 형식)은 그대로, 단색 `-symbolic` 아이콘은 CSS `mask-image` 또는 인라인 SVG + `fill: currentColor`로 색을 입힌다. `<img>`로 넣으면 색을 바꿀 수 없다
- 테마별로 이름이 다른 아이콘(창 최소화 등)은 리졸버의 매핑 테이블에서 처리한다
- 테마에 없는 대시보드 전용 아이콘은 Colloid 폴더·앱 SVG 위에 MIT 라이선스 라인 아이콘(Tabler, Lucide 등)을 얹어 합성한다
- 브랜드 앱 아이콘(Discord, Steam 등)은 실제 그 앱을 가리킬 때만 쓴다. 장식이나 대표 이미지로 쓰지 않는다
- 아이콘 원본은 `vendor/icons/colloid/`에 두고, 테마의 `LICENSE`와 출처를 함께 보관한다

## 개발 단계

마지막 완료 단계: **2단계**

1. 데스크톱 셸 + 시스템 정보 창
   - 바탕화면 아이콘, 창 열기·닫기·최소화·최대화·드래그 이동·포커스(z-order), 작업표시줄
   - "시스템 정보" 창 하나만 실제 데이터: CPU 사용률, RAM, 가동 시간을 백엔드가 `/proc`에서 읽어 2초마다 갱신
   - 창 상태는 `windows[]` 단일 저장소로 관리한다
   - 로그인, 창 크기 조절, 창 배치 저장, 다른 기능 창은 만들지 않는다(빈 "준비 중" 창만). 창 크기 조절은 1단계 범위 밖이었고 이후 추가됨
   - 완료 기준: 겹친 창 중 클릭한 창이 항상 맨 앞 / 창을 화면 밖으로 끌어도 제목 표시줄은 잡을 수 있게 남음 / 최소화한 창이 원래 위치·크기로 복원 / 시스템 정보 창을 닫으면 갱신 요청도 멈춤 / 백엔드가 꺼져도 창이 깨지지 않고 "연결 끊김" 표시
2. 로그인: "인증 설계" 섹션대로 OMV 위임 로그인을 붙인다. 이 단계부터 로그인 필수
   - 자체 로그인 화면, `Session.login` → `challengeRequired`면 `Session.verify`, 로그아웃(`Session.logout`)
   - 대시보드 세션 쿠키 발급, 앱 시작 시 세션 복원, 세션 없는 `/api/*` 요청은 401, OMV가 401을 반환하면 세션 삭제 후 로그인 화면
   - 로그인 시도 제한·실패 지연, 세션별 역할(admin 여부)을 보관해 프론트에 알림
   - 새 위젯·창, 시스템 상태를 바꾸는 코드는 만들지 않는다(작업표시줄 시계와 프로필 메뉴(로그아웃만)는 사용자 요청으로 2단계에 포함)
   - 현지화(ko·en, 로그인 화면·프로필 메뉴의 언어 선택)는 사용자 요청으로 2단계와 3단계 사이에 넣었다
   - 완료 기준: 잘못된 비밀번호는 오류 표시와 지연 / 새로고침·새 탭에서도 로그인 유지 / 로그아웃 후 `/api/system`이 401 / OMV 세션 만료 시 로그인 화면으로 이동 / 비밀번호·세션 ID가 로그에 남지 않음
3. 애드온 기반 읽기 전용 위젯: "애드온과 데이터 출처" 섹션의 애드온 계약과 capability 감지를 먼저 만든 뒤, 디스크 용량, SMART, 온도, 네트워크, 컨테이너 목록을 내장 애드온으로 구현한다
   - 현재 설치된 플러그인 기준으로 먼저 구현한다. 컨테이너는 Docker 런타임 기준으로 검증한다
   - 기존 시스템 정보 창도 내장 애드온으로 옮긴다
   - 시스템 상태를 바꾸는 코드(쓰기 RPC 포함) 금지
4. 간단한 조작: 컨테이너 시작·정지, 서비스 켜기·끄기를 OMV RPC로
5. 파일 탐색기 창: 공유 폴더 둘러보기(우선 읽기 전용)
6. "심층 설정" 연결: 각 창에서 해당 OMV 페이지로 이동

단계를 넘어갈지는 사용자가 정한다. 다음 단계 기능이 필요해 보이면 구현하지 말고 제안한다.

## 테스트 전략

로그인이 필요한 기능도 실제 비밀번호 없이 검증한다. Claude는 실제 OMV에 로그인 시도를 보내지 않는다(실패 로그·fail2ban 영향, 비밀번호 취급 금지).

1. **Go 테스트**: `internal/omv/omvfake`를 `httptest`로 띄우고 실제 `omv.Client`와 서버 핸들러를 연결해 검증한다(`internal/server/server_test.go`의 `newEnv`·`browser`). 시계는 주입하며 `omvfake.Expire(user)`로 OMV 세션 만료를 흉내 낸다. 새 RPC를 쓰는 기능은 `omvfake`에 그 RPC의 합성 응답을 추가하고 같은 방식으로 테스트한다. 응답 형식은 OMV 소스(`/usr/share/openmediavault/engined/rpc/`)를 읽고 만들며 NAS 실제 출력을 넣지 않는다
2. **Vitest**: 저장소·클라이언트·순수 함수. 인증 저장소는 `createAuthStore`에 가짜 API를 주입한다
3. **브라우저(Playwright)**: `make dev-fake`로 띄우고 `http://<NAS-IP>:5173/desk/`에 가짜 계정으로 로그인해 확인한다. 만료 확인은 `FAKEOMV_TIMEOUT=1m make dev-fake`. 사용자가 `make dev`(실제 OMV 연결)를 띄워 두었을 수 있으므로, 브라우저로 로그인하기 전에 백엔드 로그의 `omv:` 주소가 가짜 OMV(`127.0.0.1:8091`)인지 확인한다. `scripts/dev.sh`는 개발 서버 포트가 이미 쓰이고 있으면 시작하지 않는다 Playwright에서 탭을 여러 개 열면 뒤쪽 탭에 마우스 입력이 전달되지 않을 수 있으니, 다른 탭 확인이 끝나면 닫거나 브라우저를 새로 연다
4. **실제 OMV**: 사용자가 `make dev`로 직접 확인한다. Claude는 단계마다 확인 체크리스트를 제시한다

가짜 계정(`omvfake.DefaultUsers`, 모두 가짜 값):

| 계정 | 비밀번호 | 역할 | 비고 |
|---|---|---|---|
| `admin` | `admin` | 관리자 | |
| `user` | `user` | 일반 사용자 | |
| `totp` | `totp` | 관리자 | 2차 인증 코드 `123456` |

`omvfake`는 User-Agent 검사를 인증된 모든 호출에서 한다(실제 OMV보다 엄격). 백엔드가 고정 User-Agent를 지키는지 확인하기 위해서다.

## 작업 방식

- 작은 단위로 작업하고 의미 있는 단위마다 git 커밋한다. 단, 커밋은 사용자가 변경 내용을 검토하고 승인한 뒤에만 한다. 작업이 끝나면 변경 요약을 제시하고 승인을 기다린다
- 새 의존성을 추가하기 전에 이유와 대안을 먼저 설명한다
- OMV RPC의 서비스·메서드 이름이 확실하지 않으면 추측하지 말고 OMV 소스(`/usr/share/openmediavault/engined/rpc/`, 로그인 관련은 `/var/www/openmediavault/rpc/session.inc`)를 읽는다
- 실제 시스템 값에 의존하는 로직은 샘플 데이터로도 테스트할 수 있게 입력부를 분리한다
- 테스트 픽스처(`testdata/` 등)는 소스와 함께 커밋되므로 합성한 값만 쓴다. 이 NAS에서 뽑은 원본 출력을 그대로 넣지 않으며, 디스크 시리얼·MAC·IP·호스트 이름·사용자 이름 같은 식별 정보는 가짜 값으로 바꾼다
- 코드 주석은 한국어로 쓴다. 식별자·로그·에러 메시지는 영어를 유지한다
- 커밋 전에 `make check`(lint + test)를 실행한다. 프론트엔드나 embed 관련 변경이면 `make build`도 확인한다

## 나중에 확인할 것

- 같은 출처에서 OMV workbench를 iframe 창으로 띄울 수 있는지
- 같은 탭 iframe은 sessionStorage를 공유하므로, 대시보드 로그인 후 OMV 창도 로그인 상태로 보이게 할 수 있는지. OMV 내부 저장 키(`username`, `permissions`)에 의존하는 실험이라 OMV 업데이트 시 깨질 수 있다. 실험 브랜치에서만 다룬다
