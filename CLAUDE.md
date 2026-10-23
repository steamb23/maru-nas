# 마루 NAS (Maru NAS)

> A suspiciously normal desktop-like dashboard for openmediavault.

OpenMediaVault 8(OMV 8) 위에 올리는, 시놀로지 DSM처럼 창·작업표시줄·아이콘이 있는 데스크톱형 웹 대시보드.

영역별 상세 규칙은 `.claude/rules/`에 있고 해당 파일을 다룰 때 로드된다: `auth-omv.md`(OMV 8 동작, 인증 설계), `backend.md`, `helper.md`(권한 도우미), `terminal.md`(웹 터미널), `files.md`(파일 탐색기), `security.md`(보안 강화 모드), `frontend.md`, `frontend-shell.md`(창·작업표시줄·애니메이션), `i18n.md`, `icons.md`, `installer.md`(설치기). 작업 전 설계를 논의할 때는 해당 파일을 직접 읽는다. 설계 문서는 `.claude/docs/`(`addons.md`, `data-sources.md`, `helper.md`, `upgrade.md`)에, 나중에 개발할 기능 목록은 `.claude/docs/roadmap.md`에 있다.

이 파일과 규칙·설계 문서에서 사용자 문서를 가리킬 때는 한국어 판(`docs/ko/`)과 그 페이지의 한국어 제목을 쓴다. 영어 판에만 해당하는 사실(`README.md`가 거는 링크 등)을 적을 때만 `docs/en/`을 쓴다.

클라우드 세션(`CLAUDE_CODE_REMOTE=true`)에서 달라지는 규칙은 `.claude/rules/cloud.md`에 있다. SessionStart 훅(`.claude/hooks/cloud-session.sh`)이 클라우드에서만 불러오며, 문맥에 없으면 작업 전에 직접 읽는다.

## 목표와 범위

- 가동 현황 확인과 간단한 조작은 이 대시보드에서, 심층 설정은 OMV 기본 웹 UI(workbench)에서 한다
- OMV를 대체하지 않는다. OMV에 이미 있는 복잡한 설정 화면을 다시 만들지 말고 해당 OMV 페이지로 연결한다
- 로컬 네트워크 전용. 외부 인터넷 노출을 전제로 한 기능이나 설정을 만들지 않는다

## 실행 환경

- Debian + OMV 8 NAS. 개발과 실행 모두 NAS 안에서 한다
  - OMV 8의 모든 버전(8.0 이상)을 지원한다. 버전마다 RPC가 다른 곳(`Session.login` 응답 형식 등)은 `.claude/rules/auth-omv.md`의 "OMV 8 버전별 차이"에 두고, 새 RPC를 쓰면 옛 버전 소스에도 있는지 확인한다
- 프로젝트는 일반 사용자 계정 홈 아래에 있고, Claude Code도 일반 사용자 권한으로 실행된다

## 절대 규칙

- OMV가 관리하는 설정 파일(`/etc/` 아래 서비스 설정, OMV 설정 DB)을 직접 수정하지 않는다. OMV는 자기 DB를 기준으로 Salt로 설정 파일을 다시 생성하므로 직접 수정은 덮어써진다. 설정 변경은 반드시 로그인한 사용자의 세션으로 `rpc.php`를 호출해서 한다
- `omv-rpc` CLI는 쓰지 않는다. 엔진 소켓(`/var/lib/openmediavault/engined.sock`)에 직접 붙어 비밀번호 확인 없이 관리자로 실행되므로, root나 `openmediavault-engined` 그룹만 쓸 수 있다. 어떤 계정도 이 그룹에 추가하는 방안을 제안하지 않는다(사실상 root 권한 부여)
- 서비스 계정을 `docker` 그룹에도 넣지 않는다. root가 필요한 정보(SMART, SATA 디스크 온도, 컨테이너)는 OMV RPC로만 가져오며, Docker·Podman 소켓을 직접 읽는 코드는 만들지 않는다
- OMV RPC로 할 수 없는 권한 작업(사용자 자신의 파일 접근, root 작업)은 권한 도우미(`maru-helper`)의 정해진 연산으로만 한다. 임의 명령 실행 연산을 만들지 않고, root 연산을 더할 때는 `.claude/docs/helper.md`의 절차를 따른다
- `sudo`나 root 권한이 필요한 명령은 실행하지 않는다. 필요하면 명령과 이유를 제시하고 사용자가 직접 실행하게 한다
- 시스템 상태를 바꾸는 RPC(서비스 재시작, 디스크·파일시스템·공유 폴더 변경 등)는 코드로 작성만 하고, 실제 호출 테스트는 사용자에게 맡긴다
- 비밀번호, OMV 세션 ID, 토큰을 로그·파일·커밋·테스트 픽스처에 남기지 않는다
  - 예외: 보안 강화 모드의 인증 수단 기록(상태 디렉터리의 `security/<이름>.json`, `0600`)은 TOTP 비밀 키를 저장한다(TOTP는 서버가 비밀을 가져야 확인할 수 있다). 복구 코드는 해시만 둔다. 이 기록의 값도 로그·응답·커밋·테스트 픽스처에는 남기지 않는다(`.claude/rules/security.md`)
  - 예외: 패스키로 로그인(분할 키)은 브라우저 PRF로 암호화한 비밀번호 암호문을 그 브라우저의 localStorage에만 두고, 서버는 그 키의 나머지 조각(share)만 같은 기록에 둔다. share는 비밀번호도, 비밀번호에서 만든 값도 아니며 혼자서는 아무것도 풀 수 없다. share는 등록 응답과 패스키 서명을 확인한 뒤의 응답에만 담고, share·암호문·PRF 결과는 로그·커밋·테스트 픽스처에 남기지 않는다(`.claude/rules/security.md`의 "패스키로 로그인")
- `.claude/docs/roadmap.md`에 있는 기능은 사용자가 요청하기 전에 구현하지 않는다. 필요해 보이면 구현하지 말고 제안한다

## 아키텍처

- **배포**: 앞단 리버스 프록시(Caddy 등)가 HTTPS를 처리하고 대시보드를 둘 호스트·경로를 정한다(예: 별도 서브도메인 루트, 또는 OMV와 같은 도메인의 경로 접두사). 프론트는 상대 경로(`base: './'`)로 빌드하므로 경로 접두사에 묶이지 않고, 백엔드 라우트는 대시보드 루트(`/api/*`) 기준이다. OMV의 nginx 설정은 건드리지 않는다
  - 도메인 등 개인 환경 정보는 `*.local.md`(커밋 안 함)에 둔다. 커밋되는 파일에 도메인·IP를 하드코딩하지 않는다
  - OMV 웹 UI(workbench)는 대시보드 안 창(iframe)이 아니라 이름 붙은 브라우저 탭으로 연다. 브라우저 기준 주소는 `-workbench-url`(`MARU_WORKBENCH_URL`)이며, 비우면 "OMV 워크벤치" 앱을 숨긴다(`docs/ko/configuration.md`의 "OMV 워크벤치 열기")
- **실행**: systemd 서비스로 Go 실행 파일 하나를 전용 일반 사용자로 실행한다. 프론트엔드는 `go:embed`로 포함한다. 수신은 기본적으로 루프백 TCP(`127.0.0.1:8090`)이고, Docker 안의 프록시처럼 루프백에 닿지 못하는 경우를 위해 Unix 소켓(`unix:/경로`)을 지원한다. 루프백이 아닌 주소는 `-allow-non-loopback`을 명시해야만 허용한다
  - Docker는 쓰지 않는다(호스트 `/proc`, `/sys`, 네트워크를 직접 봐야 하므로)
  - 설치는 루트의 `install.sh`(영어·한국어 안내)가 한다. `install.sh`는 Go 설치기(`installer/` → `bin/maru-install`, huh TUI)를 빌드해 실행하는 부트스트랩이고, 설치기가 systemd 유닛과 `/etc/default/maru`를 직접 만든다. 리버스 프록시는 설정하지 않는다. Claude는 작성만 하고 실행은 사용자가 한다(확인은 Go 테스트와 `--dry-run`)
    - 흐름(1단계 빌드 → sudo 2단계), 설치 방식(새 설치, `--reinstall`, 업그레이드), 설정·단계를 더하는 절차는 `.claude/rules/installer.md`, 사용자 설명은 `docs/ko/install.md`·`docs/ko/install-options.md`·`docs/ko/upgrade.md`
    - 대시보드에서 업그레이드(`--web-upgrade`, 설계는 `.claude/docs/upgrade.md`): 설치기가 root 소유 복사본과 `maru-upgrade.service`·`.path`를 설치한다. 그 서비스(설치기의 `--upgrade-service`)가 저장소 소유자 권한으로 git·빌드를 하고 새 설치기를 root로 실행한다. maru는 확인 요청과 상태 파일만(설정 앱의 업데이트 페이지, `/api/upgrade*`), 도우미는 적용 요청(`upgrade.apply`)만 다룬다. 상세는 `.claude/rules/installer.md`의 "업그레이드 서비스", maru 쪽은 `.claude/rules/backend.md`의 "대시보드에서 업그레이드"
  - `README.md`는 처음 보는 사람을 위한 소개, 빠른 시작(빌드 도구가 없는 OMV에 SSH로 접속한 상태에서 로그인까지), 문서 목록, 기여 안내만 둔다. 그 밖의 사용자 대상 설치·설정·프록시·기능 안내는 `docs/`(Zensical 사이트, `zensical.toml`·`zensical.ko.toml`)에 둔다. 플래그를 추가·변경하면 `docs/ko/configuration.md`의 항목 표, 설치 옵션이면 `docs/ko/install-options.md`의 표(`TestDocsListEveryOption`)와 설치기 레지스트리도 함께 고친다. 문서의 표는 영어(`docs/en/`)에서도 고친다
  - 사용자 문서는 영어(`README.md`, `docs/en/`)와 한국어(`README.ko.md`, `docs/ko/`)를 함께 두고, 한쪽을 고치면 같은 커밋에서 다른 쪽도 고친다. 두 폴더의 페이지는 파일 이름이 같다. 다른 언어 판으로 가는 링크는 `README.md`·`README.ko.md`의 첫 줄에만 두고(`docs/` 페이지는 사이트의 언어 선택과 목차로 오간다), 영어 문서의 화면 이름·문구는 `en.json`과 설치기의 영어 문구에 맞춘다. 문서 사이트는 언어마다 따로 빌드한다. `zensical.toml`이 `docs/en/`을 `site/en/`에, `zensical.ko.toml`이 `docs/ko/`를 `site/ko/`에 빌드하고, 배포할 때 사이트 첫 주소에 브라우저 언어로 둘 중 하나를 고르는 `docs/index.html`과 영어 사이트의 404 페이지를 둔다. 두 설정의 목차는 같은 순서로 두고, 언어에 따른 항목(이름, 주소, 원본·빌드 위치, 목차, 화면 문구) 밖의 설정은 같게 둔다
  - `docs/`는 빌드하지 않아도 GitHub에서 읽혀야 한다. GitHub가 그리는 문법만 쓰고(알림은 `> [!NOTE]`·`[!TIP]`·`[!WARNING]`, Zensical 전용 `!!!`·`===`·속성 목록은 쓰지 않음), 중첩 목록과 목록 안 코드 블록은 4칸 들여쓰고, 표 칸에 `|`를 넣지 않는다. 페이지 사이 링크는 상대 경로 `.md`와 그 언어의 제목 앵커로 걸고(영어 페이지는 `docs/en/` 안에서, 한국어 페이지는 `docs/ko/` 안에서), `docs/` 밖 파일은 GitHub 절대 주소로 건다. 앵커가 GitHub와 사이트에서 같도록 한 페이지 안에 같은 제목을 두 번 쓰지 않는다
  - 사용자 문서에는 사용자가 할 일과 조건, 한도, 위험과 한계를 둔다. 내부 구현(파일 이름, 유닛 하드닝 목록, 서비스 구성 등)은 `.claude/`의 설계 문서에 둔다
- **백엔드는 얇은 API 서버**다. 역할은 (1) 현황 읽기: `/proc`·`/sys` 직접 읽기, root가 필요한 정보는 OMV RPC, (2) 설정 변경 요청을 OMV `rpc.php`로 전달, (3) 대시보드 설정(사용자별 위젯 배치·바로가기·화면 설정, 관리자의 전역 설정, 배경 이미지) 저장, 세 가지뿐이다. OMV RPC로 할 수 없는 권한 작업은 (4) 권한 도우미에 맡긴다. (5) 웹 터미널은 같은 호스트의 sshd에 SSH 클라이언트로 붙어 셸을 중계한다
  - (3)은 상태 디렉터리(`-state-dir`, `internal/prefs`)에 대시보드 자신의 상태만 두며 OMV 설정·시스템 파일을 쓰지 않는다. 내용은 `.claude/rules/backend.md`
  - 설정 앱(`src/settings/`)은 대시보드 설정, 자기 계정 프로필, 보안 수단, 정보만 다룬다. 전역에 적용되는 설정은 관리자만 바꾸고, 그 밖의 OMV·OS 설정은 넣지 않는다. 페이지 목록은 `docs/ko/settings-app.md`
  - 웹에서 덮어쓰는 가동 설정은 재시작 없이 바뀌는 워크벤치 주소, SMART 확인 시각, 웹 터미널 허용뿐이다(`site.json`, 플래그보다 우선). 나머지는 root가 `/etc/default/maru`를 고친다
- **권한 도우미**(`maru-helper`, `-helper-socket`, 설치기가 기본으로 설치): root로 도는 별도 실행 파일이 소켓 활성화로 maru 서비스 사용자의 연결만 받는다. 로그인할 때 비밀번호를 `unix_chkpwd`로 확인해 티켓을 주고, 연산을 그 사용자 권한으로 강등해 실행한다. root 연산은 권한 상승한 관리자 세션에서 5분 동안, 정해진 목록만 쓴다(지금은 `id`, 대시보드에서 업그레이드의 `upgrade.apply`, 파일 탐색기 디스크 휴지통의 `trash.prepare`). 상세는 `.claude/docs/helper.md`, 규칙은 `.claude/rules/helper.md`
- **파일 탐색기**(`internal/files`, `src/files/`, 셸 앱): 홈 폴더와 공유 폴더를 도우미의 사용자 권한 연산으로 둘러보고 내려받고 올리고 하위 폴더까지 찾는다. 공유 폴더 목록은 관리자 세션이 받은 스냅숏(`site.json`)이고, 접근은 파일 시스템 권한을 따르며 관리자가 켜면 OMV 공유 폴더 권한도 따른다. 큰 폴더는 이름을 먼저 읽고 보이는 범위의 정보만 읽는다. 파일 미리보기는 따로 여는 셸 앱(`src/viewer/`)이다. 상세는 `.claude/rules/files.md`
- **웹 터미널**(`internal/terminal`, 기본 꺼짐): 백엔드가 루프백 sshd에 대시보드 세션 사용자로만 접속해 WebSocket(`/api/terminal`)으로 셸을 중계한다. TTY를 직접 띄우지 않으며, 누가 들어갈 수 있는지는 OMV의 SSH 설정이 정한다. 상세는 `.claude/rules/terminal.md`
- **보안 강화 모드**(`internal/security`): 웹 터미널·root 권한 상승을 세션의 보증 수준(로그인 < 비밀번호 재확인 < 2차 인증)으로 막는다. 2차 인증(패스키·TOTP·복구 코드·OMV 2차 인증)의 확인은 maru가 하며 도우미에 넣지 않는다. 상세는 `.claude/rules/security.md`
- **OMV RPC는 항상 백엔드가 중계한다**: 브라우저 → `/api/rpc` → 백엔드 → OMV `rpc.php`. 브라우저가 `rpc.php`를 직접 호출하지 않는다
  - 백엔드는 대시보드 세션마다 OMV 세션을 메모리에 보관하고, 항상 같은 주소(127.0.0.1)와 고정 User-Agent로 OMV를 호출한다
  - 브라우저의 쿠키를 OMV에 그대로 전달하지 않는다. OMV는 User-Agent가 다르면 세션을 파기하므로 사용자를 로그아웃시키게 된다
- **인증**: 자체 로그인 화면에서 백엔드가 OMV `Session.login`(필요하면 `Session.verify`)으로 위임하고, 대시보드 자체 세션 쿠키를 발급한다. 세션 만료는 OMV timeout 기준 브라우저 타이머로 처리한다. 상세는 `.claude/rules/auth-omv.md`

## 기술 스택

- **백엔드**: Go 표준 라이브러리 위주(`backend/`). 런타임 외부 의존성은 웹 터미널의 `golang.org/x/crypto/ssh`와 `github.com/coder/websocket`뿐이며, 권한 도우미 실행 파일에는 링크하지 않는다
- **설치기**: 별도 Go 모듈 `installer/`(`go.work` 없이 `replace`로 로컬 `backend`). TUI 의존성 `charm.land/huh/v2`는 이 모듈에만 있다. 백엔드와 공유하는 검증 함수·환경 변수 키·기본값·버전·업그레이드 파일 형식은 공개 패키지 `backend/config`(표준 라이브러리만)에 둔다
- **프론트엔드**: TypeScript + Vite + React(`frontend/`), 창 상태는 Zustand 단일 저장소(`windows[]`). 웹 터미널의 xterm.js와 인증 앱 QR의 uqr은 쓰는 화면의 지연 청크에만 싣는다. 린트 oxlint, 테스트 Vitest(DOM 환경 없이 순수 함수·저장소만)
- **타입 공유**: tygo로 `backend/internal/api` → `frontend/src/api/types.gen.ts`. 생성물은 커밋하며 직접 수정하지 않는다
- **현지화**: 1차 지원 `en`, `ko`는 항상 완전하게 두고, 나머지 언어(`languages.ts`의 `SUPPORTED_LANGUAGES`)는 기여나 일괄 번역으로 채운다. 화면 문구(aria-label·title 포함)는 모두 `t()`를 거치고, 새 키는 `en.json`과 `ko.json`에 함께 넣는다. 로그인 대기 이스터에그 문구와 로그인 화면의 브랜드 영역(한글 이름, 로마자 표기, 태그라인)은 현지화 제외
- **아이콘**: Colloid 테마 하나만, 리졸버 `getIcon(name)`으로 freedesktop 이름을 써서 가져온다

## 애드온과 위젯

상세 설계는 `.claude/docs/addons.md`, 출처별 표와 플러그인 조사는 `.claude/docs/data-sources.md`.

- **애드온 = manifest + provider + 보기(창, 위젯)**. 시스템 정보를 포함한 모든 기능을 같은 계약으로 만든다. 보기는 창과 바탕화면 위젯 중 하나 이상이며, 둘은 같은 provider·API를 쓰고 화면만 다르다
- **위젯은 창 시스템과 분리한다**. 바탕화면 위 별도 레이어에 그리드 칸 단위로 붙이고, 창 저장소·창 컴포넌트에 위젯용 분기를 넣지 않는다. 공유는 provider, `/api/*`, `poller.ts`, 공통 훅까지만 한다
  - 그리드는 오른쪽 위 기준이고, 배치는 백엔드에 사용자별로 저장한다(`/api/preferences`). 추가·제거는 컨텍스트 메뉴(`shell/contextMenu.ts`)로 한다. 상세는 `.claude/docs/addons.md`의 "위젯 시스템"
- `/api/rpc` 중계는 manifest 허용 목록의 read 메서드만 통과시킨다. write는 애드온 조작(`POST /api/addons/{id}/actions/{action}`)으로만 부르며, 조작이 대상을 검사하고 OMV 파라미터를 직접 만든다
- 내장 애드온만 정적 목록으로 둔다(백엔드 `internal/addons/<id>/` + `internal/addons/builtin`, 프론트 `src/addons/<id>/` + `src/addons/registry.ts`). 컴파일 타임 타입(`AppId`)을 유지하고, 별도 패키지·런타임 로딩·Go `plugin`은 쓰지 않는다. 프론트 보기를 처음 쓸 때 내려받는 것(`lazyView`)은 이 정적 목록 안의 지연 로딩이다
- manifest 원본은 Go 구조체이고 프론트 목록(`manifests.gen.ts`)은 `make types`로 생성한다. 백엔드·프론트 id 일치는 Go 골든 테스트와 tsc(`Record<AddonId, …>`)가 검사한다
- 플러그인 설치 여부는 `/usr/share/openmediavault/engined/rpc/<name>.inc`와 `/var/lib/dpkg/status`로 감지해 `/api/capabilities`로 내려준다
- provider는 권한이 필요 없는 직접 읽기(`/proc`, `/sys`)를 1차로, OMV RPC를 보강으로 쓰고 응답에 출처를 담는다
- 컨테이너는 compose 플러그인(omv-extras) RPC로 가져오며 Docker 런타임 기준으로 검증한다. Podman 모드는 미검증으로 표시한다

## 명령어

저장소 루트에서 실행한다. 처음 클론했다면 `git submodule update --init --depth 1`과 `npm --prefix frontend ci`를 먼저 실행한다. 도구 버전은 Go 1.27.1 이상(`backend/go.mod`), Node.js 20.19 이상 또는 22.12 이상(Vite 8)이고, Vitest 5는 Node.js 22.12 이상을 공식 지원한다. 도구 설치 방법은 `docs/ko/install.md`의 "빌드 도구"에 있다.

| 명령 | 내용 |
|---|---|
| `make dev` | 개발 서버. 백엔드 `127.0.0.1:8090`(`go run`) + Vite `0.0.0.0:5173`(`http://<NAS-IP>:5173/`), Ctrl+C로 둘 다 종료. 실제 OMV에 연결하므로 `frontend/.env.local`에 `MARU_OMV_URL`(이 NAS의 OMV 주소)을 둔다 |
| `make dev-fake` | `make dev`에 가짜 OMV(`127.0.0.1:8091`), 가짜 권한 도우미(상태 디렉터리의 `helper.sock`, 파일 연산은 `helper-root/` 아래만), 가짜 sshd(`127.0.0.1:8092`, 장난감 셸)를 함께 띄워 연결한다 |
| `make dev-pick` | 워크트리와 OMV 연결(실제·가짜)을 번호로 골라 개발 서버를 띄운다(`scripts/dev-pick.sh`). 입력을 기다리므로 Claude는 쓰지 않는다 |
| `make build` | 대상 없이 `make`만 실행해도 같다. 프론트 빌드 → `backend/internal/web/static/dist/` → `bin/maru` 단일 실행 파일, 권한 도우미 `bin/maru-helper`(`CGO_ENABLED=0`), 설치기 `bin/maru-install` |
| `make lint` | `gofmt -l`, `go vet`(backend·installer 두 모듈), oxlint, `tsc -b` |
| `make test` | `go test -count=1 ./...`(backend·installer 두 모듈. 모듈 밖 생성물 검사가 캐시로 통과하지 않게), `vitest run` |
| `make check` | lint + test |
| `make types` | Go 구조체에서 TS 다시 생성: `internal/api` → `src/api/types.gen.ts`, 내장 애드온 manifest → `src/addons/manifests.gen.ts`. 둘 중 하나라도 바꾸면 반드시 실행하고 생성물도 함께 커밋한다(manifest 생성물은 Go 테스트가 검사) |

개별 실행: `cd backend && go test ./internal/addons/sysinfo -run TestParseMemInfo`, `cd installer && go test ./internal/setup -run TestGolden`, `cd frontend && npx vitest run src/icons`

문서 사이트: `zensical serve`(영어 미리보기), `zensical serve -f zensical.ko.toml -a localhost:8001`(한국어 미리보기), `zensical build --strict`와 `zensical build --strict -f zensical.ko.toml`(깨진 링크·앵커 검사, CI의 `docs` 잡과 같다). 두 빌드는 각자의 `site_dir`(`site/en/`, `site/ko/`) 밖의 파일을 건드리지 않는다. 사이트(`https://steamb23.github.io/maru-nas/`)는 `stable`을 push하면 `.github/workflows/docs.yml`이 GitHub Pages에 배포하므로 최신 릴리스 기준이다. 릴리스 사이에 문서만 먼저 올릴 때는 사용자가 Actions 화면에서 main을 골라 그 워크플로를 수동으로 실행하고, 다음 `stable` push가 다시 덮어쓴다. Zensical은 Python 도구라 `make`에 넣지 않았고, 설치 방법과 고정 버전은 `CONTRIBUTING.md`의 "Documentation"에 있다

개발 서버 환경 변수와 워크트리:

- 포트: `MARU_LISTEN`(백엔드), `FAKEOMV_LISTEN`(가짜 OMV), `FAKESSH_LISTEN`(가짜 sshd), `DEV_PORT`(Vite). 상태 디렉터리는 `MARU_STATE_DIR`
- `make dev-fake` 전용: `FAKEOMV_TIMEOUT=1m`(세션 timeout, 분 단위), `FAKEOMV_VERSION=8.3`(옛 OMV 응답, `omvfake.Version`), `FAKEFILES_LARGE=60000`(파일 탐색기의 큰 폴더, 가짜 디스크의 사진/휴대폰 백업), `FAKESSH_AUTH=publickey`(가짜 sshd가 받는 인증 방법), `FAKEUPGRADE_SCENARIO=manual`(가짜 업그레이드 서비스의 시나리오: web, manual, latest, check-fail, apply-fail, disabled), `FAKEUPGRADE_STEP=20s`(가짜 업그레이드 서비스의 단계 사이 지연, 기본 1.5s)
- 웹 터미널 허용은 설정 앱이나 `MARU_TERMINAL=true`로 켠다
- git 워크트리에서 실행하면 주 체크아웃의 `frontend/.env.local`(워크트리에 있으면 그 값이 우선)과 `.dev-state`를 쓴다. 주 체크아웃에서 `make dev BRANCH=<브랜치>`(`make dev-fake`도 같다)로 그 브랜치가 체크아웃된 워크트리의 코드를 실행할 수 있다(실행은 부른 쪽의 `scripts/dev.sh`)

## 테스트 전략

로그인이 필요한 기능도 실제 비밀번호 없이 검증한다. Claude는 실제 OMV에 로그인 시도를 보내지 않는다(실패 로그·fail2ban 영향, 비밀번호 취급 금지).

- 예외: 가상 머신으로 테스트 환경을 구축한 경우, 그 VM의 OMV와 maru에는 VM 전용 가짜 계정으로 로그인해 확인해도 된다. VM의 주소·계정·전달 경로는 `CLAUDE.local.md`에 두고, 로그인하기 전에 그 주소가 실제로 VM으로 이어지는지 확인한다(실제 NAS의 OMV로 가지 않게)

1. **Go 테스트**: 가짜 OMV(`internal/omv/omvfake`)를 `httptest`로 띄워 실제 클라이언트·핸들러와 연결한다. 상세는 `.claude/rules/backend.md`
2. **Vitest**: 저장소·클라이언트·순수 함수
3. **브라우저(Playwright)**: Claude가 띄우는 개발 서버는 **기본 포트(8090·8091·8092·5173)를 쓰지 않는다**. 사용자가 기본 포트로 자기 개발 서버를 띄울 수 있게, 저장소 루트에서 다른 포트와 상태 디렉터리로 띄운다
   ```sh
   DEV_PORT=5174 MARU_LISTEN=127.0.0.1:8190 FAKEOMV_LISTEN=127.0.0.1:8191 FAKESSH_LISTEN=127.0.0.1:8192 \
     MARU_STATE_DIR="$PWD/.dev-state/fake-claude" make dev-fake
   ```
   `http://<NAS-IP>:5174/`에 가짜 계정으로 로그인해 확인한다. 만료 확인은 앞에 `FAKEOMV_TIMEOUT=1m`을 더한다. 확인이 끝나면 서버를 내린다
   - 로그인하기 전에 백엔드 로그의 `omv:` 주소가 가짜 OMV(`127.0.0.1:8191`)인지 확인한다. `scripts/dev.sh`는 개발 서버 포트가 이미 쓰이고 있으면 시작하지 않는다
   - git 워크트리(`.claude/worktrees/<이름>`)에서는 `$PWD/.dev-state/…`가 길어 가짜 도우미 소켓(`helper.sock`)이 Unix 소켓 경로 길이 한도(108바이트)를 넘어 묶이지 않는다. 주 체크아웃의 `.dev-state` 아래처럼 짧은 경로를 쓴다
   - 상태 디렉터리를 나누는 것은 사용자의 `make dev-fake`와 같은 사용자 설정 파일(`.dev-state/fake`)을 동시에 쓰지 않기 위해서다
   - 탭을 여러 개 열면 뒤쪽 탭에 마우스 입력이 전달되지 않을 수 있다. 다른 탭 확인이 끝나면 닫거나 브라우저를 새로 연다
4. **실제 OMV**: 사용자가 `make dev`로 직접 확인한다. Claude는 기능마다 확인 체크리스트를 제시한다

가짜 계정(`omvfake.DefaultUsers`, 모두 가짜 값):

| 계정 | 비밀번호 | 역할 | 비고 |
|---|---|---|---|
| `admin` | `admin` | 관리자 | |
| `user` | `user` | 일반 사용자 | |
| `totp` | `totp` | 관리자 | 2차 인증 코드 `123456` |

가짜 도우미(`helperfake.DefaultUsers`)는 같은 계정·비밀번호를 쓴다. `admin`은 OMV 내장 관리자처럼 sudo 그룹이 아니라 권한 상승을 할 수 없고, `totp`만 상승할 수 있다. 권한을 바꾸지 않으므로 `id`는 가짜 계정의 uid·그룹을 돌려준다.

## 작업 방식

- 세션에서 첫 작업을 시작하기 전에 `git status`로 커밋하지 않은 변경이 남아 있는지 확인한다. 남아 있으면 내용을 요약해 보여 주고 커밋할지 사용자에게 묻는다
- 별도 지시가 없으면 작업을 시작하기 전에 `main`에서 작업 브랜치와 git 워크트리를 만들고 그 워크트리에 들어가서 진행한다. 주 체크아웃은 다른 세션이나 사용자가 쓰고 있을 수 있다. 주 체크아웃에서 하라는 지시가 있으면 브랜치만 만든다
  - 브랜치는 `main` 하나를 트렁크로 두고 짧게 사는 작업 브랜치만 만든다. `develop`·`release-*`·`hotfix-*` 같은 장기 브랜치는 두지 않는다. 릴리스를 가리키는 `stable`은 "버전"의 릴리스 절차로만 움직인다
  - 작업 브랜치 이름은 `<종류>/<짧은-설명>`(영어 소문자, 하이픈)으로 짓는다. 종류는 `feat`(새 기능·앱), `fix`(버그 수정), `docs`(문서), `i18n`(번역), `chore`(빌드·CI·정리), `refactor`(동작 변화 없는 구조 변경)
  - 주 체크아웃에서 `git worktree add -b <브랜치> .claude/worktrees/<이름> main`으로 만들고 `EnterWorktree`의 `path`로 들어간다. `EnterWorktree`의 `name`은 로컬 `main`보다 뒤처질 수 있는 `origin/main`에서 갈라지므로 쓰지 않는다
  - 워크트리 안에서 `git submodule update --init --depth 1`과 `npm --prefix frontend ci`를 먼저 실행한다
  - 사용자는 주 체크아웃에서 `make dev BRANCH=<브랜치>`나 `make dev-pick`으로 그 워크트리의 개발 서버를 띄운다
  - `main` 병합(rebase 후 fast-forward)은 `ExitWorktree`(keep)로 주 체크아웃에 돌아간 뒤 `git merge --ff-only <브랜치>`로 한다. `EnterWorktree`로 들어간 워크트리 안에서는 주 체크아웃을 대상으로 한 git 명령이 거부된다. 병합한 뒤에는 워크트리 디렉터리를 `trash-put`으로 지우고 `git worktree prune`을 실행한다
  - 처음부터 워크트리에서 시작한 세션(앱이 만든 `.claude/worktrees/<이름>-<해시>`, 브랜치 `claude/<이름>-<해시>`)은 새 워크트리를 만들지 않고 그 브랜치에서 작업한다. 이 세션은 `EnterWorktree`를 거치지 않았으므로 `ExitWorktree`로 주 체크아웃에 돌아갈 수 없다(아무 일도 하지 않는다). `main` 병합은 워크트리 안에서 `main` 위로 rebase한 뒤 `git -C <주 체크아웃> merge --ff-only <브랜치>`로 한다. 워크트리는 세션이 쓰고 있는 디렉터리이므로 `trash-put`·`git worktree prune`으로 지우지 않으며, 세션을 보관(archive)하면 앱이 정리한다
- 작은 단위로 작업하고, 작업 단위 하나가 끝날 때마다 승인을 기다리지 않고 브랜치에 커밋한다
- 브랜치의 작업이 모두 끝나면 변경 요약을 제시하고, `main` 병합은 사용자가 승인한 뒤에만 한다
- 커밋 전에 `make check`를 실행한다. 프론트엔드나 embed 관련 변경이면 `make build`도 확인한다. `docs/`나 `zensical.toml`·`zensical.ko.toml`을 바꿨으면 두 사이트의 `zensical build --strict`도 확인한다
- `main`으로의 push와 모든 PR은 GitHub Actions(`.github/workflows/ci.yml`)가 `make check`와 `make build`로, 문서는 두 사이트의 `zensical build --strict`로 검사한다. 외부 기여는 fork → PR로 받고, 유지자가 `main`에 rebase 병합한다(머지 커밋 없음)
- 커밋 메시지는 한국어로 쓴다(사용자와 Claude의 작업 언어). 외부 기여자의 커밋·주석 언어는 자유이고 영어 또는 한국어를 권장한다. 기여자용 안내는 `CONTRIBUTING.md`(영어 원본)와 `CONTRIBUTING.ko.md`이며, 기여자에게 보이는 규칙(브랜치 이름, 권한 규칙, 번역 절차 등)을 바꾸면 두 파일을 함께 고친다
- 새 의존성을 추가하기 전에 이유와 대안을 먼저 설명한다
- OMV RPC의 서비스·메서드 이름과 역할이 확실하지 않으면 추측하지 말고 OMV 소스(`/usr/share/openmediavault/engined/rpc/`, 로그인 관련은 `/var/www/openmediavault/rpc/session.inc`)를 읽는다
- 실제 시스템 값에 의존하는 로직은 샘플 데이터로도 테스트할 수 있게 입력부를 분리한다
- 테스트 픽스처(`testdata/` 등)에는 합성한 값만 쓴다. 이 NAS에서 뽑은 원본 출력을 그대로 넣지 않으며, 디스크 시리얼·MAC·IP·호스트 이름·사용자 이름 같은 식별 정보는 가짜 값으로 바꾼다
- 코드 주석은 한국어로 쓴다. 식별자·로그·에러 메시지는 영어를 유지한다

## 버전

- SemVer `0.y.z`. 원본은 `backend/config`의 `Version` 상수 하나이고, OMV User-Agent와 정보 페이지(`/api/about`), 설치기가 이 값을 쓴다. `frontend/package.json`의 버전은 쓰지 않는다
- 기준은 업그레이드할 때 사용자가 할 일이 있는지다
  - 마이너(`0.y.0`): 새 기능·앱, 플래그·`install.sh` 설정 추가, 상태 디렉터리 형식 변경(`users/*.json`·`site.json`의 `version`, 이전 형식을 읽는 변환도 함께), 프록시 설정·지원 OMV 버전처럼 업그레이드 때 손봐야 하는 변경
  - 패치(`0.y.z`): 버그 수정, 번역·문구·문서 수정. 설정과 저장 형식은 그대로일 때
  - `1.0.0`: 깃허브에 공개하면서 설치·업그레이드 방법과 상태 디렉터리 형식의 호환을 약속할 수 있을 때
- 커밋마다 올리지 않는다(정보 페이지에 커밋 해시가 나온다). 올릴 때는 버전 커밋 하나를 만들고, 공개 배포부터는 그 커밋에 `v<버전>` 태그를 붙인다
- 릴리스 절차(공개 배포부터). 태그는 고정이고 `stable` 브랜치가 "최신 릴리스"를 가리키는 움직이는 이름이다. `README.md`·`README.ko.md`의 빠른 시작과 `docs/en/install.md`·`docs/ko/install.md`는 `stable`을 받는다
  1. `main`에 버전 커밋을 만든다(`Version` 상수, 이전 버전 태그 이후의 변경 요약)
  2. 그 커밋에 주석 태그를 붙인다: `git tag -a v<버전> -m "v<버전>"`. 붙인 태그는 옮기지 않는다
  3. `stable`을 그 태그로 fast-forward한다: `git branch -f stable v<버전>`. `stable`은 앞으로만 가고 되감지 않으며, 태그 없는 커밋을 가리키게 하지 않는다
  4. `main`, 태그, `stable`을 push하고(`git push origin main v<버전> stable`) GitHub Release에 변경 요약을 올린다. `stable` push가 문서 사이트를 다시 배포한다(`.github/workflows/docs.yml`. 저장소 설정에서 Pages의 Source는 GitHub Actions, `github-pages` 환경은 `stable`과 `main`에서 배포를 허용해야 한다)
  - 첫 공개 릴리스에서 `stable`을 처음 만든다. 그 전에는 `stable`이 없으므로 빠른 시작과 설치 문서의 `git clone -b stable`은 동작하지 않는다
  - 릴리스 뒤에 급한 수정이 필요하면 `main`에서 고치고 패치 버전으로 위 절차를 반복한다. 릴리스 브랜치를 따로 만들지 않는다
