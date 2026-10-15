# 온마루 (Onmaru)

> A suspiciously normal desktop-like dashboard for openmediavault.

OpenMediaVault 8(OMV 8) 위에 올리는, 시놀로지 DSM처럼 창·작업표시줄·아이콘이 있는 데스크톱형 웹 대시보드.

영역별 상세 규칙은 `.claude/rules/`에 있고 해당 파일을 다룰 때 로드된다: `auth-omv.md`(OMV 8 동작, 인증 설계), `backend.md`, `frontend.md`, `frontend-shell.md`(창·작업표시줄·애니메이션), `i18n.md`, `icons.md`. 작업 전 설계를 논의할 때는 해당 파일을 직접 읽는다.

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
- 서비스 계정을 `docker` 그룹에도 넣지 않는다. root가 필요한 정보(SMART, SATA 디스크 온도, 컨테이너)는 OMV RPC로만 가져오며, Docker·Podman 소켓을 직접 읽는 코드는 만들지 않는다
- `sudo`나 root 권한이 필요한 명령은 실행하지 않는다. 필요하면 명령과 이유를 제시하고 사용자가 직접 실행하게 한다
- 시스템 상태를 바꾸는 RPC(서비스 재시작, 디스크·파일시스템·공유 폴더 변경 등)는 코드로 작성만 하고, 실제 호출 테스트는 사용자에게 맡긴다
- 비밀번호, OMV 세션 ID, 토큰을 로그·파일·커밋·테스트 픽스처에 남기지 않는다
- `ROADMAP.md`에 있는 기능은 사용자가 요청하기 전에 구현하지 않는다. 필요해 보이면 구현하지 말고 제안한다

## 아키텍처

- **배포**: 앞단 리버스 프록시(Caddy 등)가 HTTPS를 처리하고 대시보드를 둘 호스트·경로를 정한다(예: 별도 서브도메인 루트, 또는 OMV와 같은 도메인의 경로 접두사). 프론트는 상대 경로(`base: './'`)로 빌드하므로 경로 접두사에 묶이지 않고, 백엔드 라우트는 대시보드 루트(`/api/*`) 기준이다. OMV의 nginx 설정은 건드리지 않는다
  - 도메인 등 개인 환경 정보는 `*.local.md`(커밋 안 함)에 둔다. 커밋되는 파일에 도메인·IP를 하드코딩하지 않는다
  - OMV 웹 UI(workbench)는 대시보드 안 창이 아니라 이름 붙은 브라우저 탭으로 연다. 브라우저 기준 주소는 `-workbench-url`(`ONMARU_WORKBENCH_URL`)이며, 비우면 "OMV 워크벤치" 앱을 숨긴다(`README.md`의 "OMV 워크벤치 열기")
  - OMV 화면을 iframe으로 띄우는 실험은 OMV와 같은 출처(같은 도메인의 경로 접두사)에 둘 때만 가능하다(OMV가 `X-Frame-Options: SAMEORIGIN`을 보낸다). 인증과는 무관하다
- **실행**: systemd 서비스로 Go 실행 파일 하나를 전용 일반 사용자로 실행한다. 프론트엔드는 `go:embed`로 포함한다. 수신은 기본적으로 루프백 TCP(`127.0.0.1:8090`)이고, Docker 안의 프록시처럼 루프백에 닿지 못하는 경우를 위해 Unix 소켓(`unix:/경로`)을 지원한다. 루프백이 아닌 주소는 `-allow-non-loopback`을 명시해야만 허용한다
  - Docker는 쓰지 않는다(호스트 `/proc`, `/sys`, 네트워크를 직접 봐야 하므로)
  - 설치는 루트의 인터랙티브 `install.sh`(영어·한국어 안내)가 한다. systemd 유닛과 `/etc/default/onmaru`를 직접 만들며, Claude는 작성만 하고 실행은 사용자가 한다. 리버스 프록시는 설정하지 않는다
    - 2단계 구조: `./install.sh`가 설정 질문과 확인, 실행한 계정으로 빌드(1단계)를 한 뒤, root가 아니면 설정을 옵션으로 넘겨 `sudo ./install.sh --service-install ...`로 자신을 다시 실행해 사용자 생성·유닛 작성·시작(2단계)을 한다. 2단계는 받은 설정을 다시 검증하고, 설정이 모두 있으면 묻지 않는다. root만 있는 환경은 sudo 없이 이어서 진행한다. 일반 사용자가 sudo로 실행하면 root로 빌드하지 않는다
    - `--upgrade`는 기존 설치(유닛의 `User=`, `/etc/default/onmaru`)의 설정으로 질문·확인 없이 진행하고, 기존 설치에 없는 설정만 묻는다. 설치 설정을 새로 더하면 `load_installed_settings`에서도 읽게 한다
  - 사용자 대상 설치·설정·프록시 안내는 `README.md`에 둔다. 플래그를 추가·변경하면 `README.md`의 설정 표와 `install.sh`도 함께 고친다
- **백엔드는 얇은 API 서버**다. 역할은 (1) 현황 읽기: `/proc`·`/sys` 직접 읽기, root가 필요한 정보는 OMV RPC, (2) 설정 변경 요청을 OMV `rpc.php`로 전달, (3) 대시보드 설정(사용자별 위젯 배치·바로가기·화면 설정, 관리자의 전역 설정, 배경 이미지) 저장, 세 가지뿐이다
  - (3)은 대시보드 자신의 상태만 다룬다. 상태 디렉터리(`-state-dir`, systemd `StateDirectory=`)의 사용자별 JSON 파일, 관리자가 바꾸는 전역 설정(`site.json`), 올린 배경 이미지(`images/`)이며 OMV 설정·시스템 파일과 무관하다(`internal/prefs`)
  - 설정 창 앱(`src/settings/`)이 프로필(사진, 자기 OMV 계정의 전자 우편·태그), 화면(테마·바탕화면) 같은 사용자 설정과 관리자 전용 전역 설정(로그인 화면, 기본 화면, 서버), 정보 페이지를 맡는다. 전역에 적용되는 설정은 관리자만 바꾼다. 그 밖의 OMV·OS 설정은 넣지 않고, 넣게 되면 페이지 그룹으로 더한다
  - 가동 설정 중 재시작 없이 바뀌는 워크벤치 주소와 SMART 확인 시각만 웹에서 덮어쓴다(`site.json`, 플래그보다 우선). 나머지는 `/etc/default/onmaru`를 root가 고치며, 웹에서 sudo로 바꾸는 방식은 `ROADMAP.md`에 있다
- **OMV RPC는 항상 백엔드가 중계한다**: 브라우저 → `/api/rpc` → 백엔드 → OMV `rpc.php`. 브라우저가 `rpc.php`를 직접 호출하지 않는다
  - 백엔드는 대시보드 세션마다 OMV 세션을 메모리에 보관하고, 항상 같은 주소(127.0.0.1)와 고정 User-Agent로 OMV를 호출한다
  - 브라우저의 쿠키를 OMV에 그대로 전달하지 않는다. OMV는 User-Agent가 다르면 세션을 파기하므로 사용자를 로그아웃시키게 된다
- **인증**: 자체 로그인 화면에서 백엔드가 OMV `Session.login`(필요하면 `Session.verify`)으로 위임하고, 대시보드 자체 세션 쿠키를 발급한다. 세션 만료는 OMV timeout 기준 브라우저 타이머로 처리한다. 상세는 `.claude/rules/auth-omv.md`

## 기술 스택

- **백엔드**: Go 표준 라이브러리만(`backend/`). 런타임 외부 의존성 없음
- **프론트엔드**: TypeScript + Vite + React(`frontend/`), 창 상태는 Zustand 단일 저장소(`windows[]`). 린트 oxlint, 테스트 Vitest(DOM 환경 없이 순수 함수·저장소만)
- **타입 공유**: tygo로 `backend/internal/api` → `frontend/src/api/types.gen.ts`. 생성물은 커밋하며 직접 수정하지 않는다
- **현지화**: `en`, `ko`. 화면 문구(aria-label·title 포함)는 모두 `t()`를 거치고, 새 키는 `en.json`과 `ko.json`에 함께 넣는다. 로그인 대기 이스터에그 문구와 로그인 화면의 브랜드 영역(한글 이름, 로마자 표기, 태그라인)은 현지화 제외
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

저장소 루트에서 실행한다. 처음 클론했다면 `git submodule update --init --depth 1`과 `npm --prefix frontend ci`를 먼저 실행한다.

| 명령 | 내용 |
|---|---|
| `make dev` | 개발 서버. 백엔드 `127.0.0.1:8090`(`go run`) + Vite `0.0.0.0:5173`. 접속은 `http://<NAS-IP>:5173/`. Ctrl+C로 둘 다 종료. 실제 OMV에 연결하므로 `frontend/.env.local`에 `ONMARU_OMV_URL`(이 NAS의 OMV 주소)을 둔다 |
| `make dev-fake` | `make dev`와 같지만 가짜 OMV(`127.0.0.1:8091`)를 함께 띄워 연결한다. `FAKEOMV_TIMEOUT=1m make dev-fake`처럼 세션 timeout을 분 단위로 바꿀 수 있다. 포트는 `ONMARU_LISTEN`(백엔드), `FAKEOMV_LISTEN`(가짜 OMV), `DEV_PORT`(Vite)로 바꾼다 |
| `make build` | 프론트 빌드 → `backend/internal/web/static/dist/` → `bin/onmaru` 단일 실행 파일 |
| `make lint` | `gofmt -l`, `go vet`, oxlint, `tsc -b` |
| `make test` | `go test -count=1 ./...`(모듈 밖 생성물 검사가 캐시로 통과하지 않게), `vitest run` |
| `make check` | lint + test |
| `make types` | Go 구조체에서 TS 다시 생성: `internal/api` → `src/api/types.gen.ts`, 내장 애드온 manifest → `src/addons/manifests.gen.ts`. 둘 중 하나라도 바꾸면 반드시 실행하고 생성물도 함께 커밋한다(manifest 생성물은 Go 테스트가 검사) |

개별 실행: `cd backend && go test ./internal/addons/sysinfo -run TestParseMemInfo`, `cd frontend && npx vitest run src/icons`

## 테스트 전략

로그인이 필요한 기능도 실제 비밀번호 없이 검증한다. Claude는 실제 OMV에 로그인 시도를 보내지 않는다(실패 로그·fail2ban 영향, 비밀번호 취급 금지).

1. **Go 테스트**: 가짜 OMV(`internal/omv/omvfake`)를 `httptest`로 띄워 실제 클라이언트·핸들러와 연결한다. 상세는 `.claude/rules/backend.md`
2. **Vitest**: 저장소·클라이언트·순수 함수
3. **브라우저(Playwright)**: Claude가 띄우는 개발 서버는 **기본 포트(8090·8091·5173)를 쓰지 않는다**. 사용자가 기본 포트로 자기 개발 서버를 띄울 수 있게, 저장소 루트에서 다른 포트와 상태 디렉터리로 띄운다
   ```sh
   DEV_PORT=5174 ONMARU_LISTEN=127.0.0.1:8190 FAKEOMV_LISTEN=127.0.0.1:8191 \
     ONMARU_STATE_DIR="$PWD/.dev-state/fake-claude" make dev-fake
   ```
   `http://<NAS-IP>:5174/`에 가짜 계정으로 로그인해 확인한다. 만료 확인은 앞에 `FAKEOMV_TIMEOUT=1m`을 더한다. 확인이 끝나면 서버를 내린다
   - 로그인하기 전에 백엔드 로그의 `omv:` 주소가 가짜 OMV(`127.0.0.1:8191`)인지 확인한다. `scripts/dev.sh`는 개발 서버 포트가 이미 쓰이고 있으면 시작하지 않는다
   - 상태 디렉터리를 나누는 것은 사용자의 `make dev-fake`와 같은 사용자 설정 파일(`.dev-state/fake`)을 동시에 쓰지 않기 위해서다
   - 탭을 여러 개 열면 뒤쪽 탭에 마우스 입력이 전달되지 않을 수 있다. 다른 탭 확인이 끝나면 닫거나 브라우저를 새로 연다
4. **실제 OMV**: 사용자가 `make dev`로 직접 확인한다. Claude는 기능마다 확인 체크리스트를 제시한다

가짜 계정(`omvfake.DefaultUsers`, 모두 가짜 값):

| 계정 | 비밀번호 | 역할 | 비고 |
|---|---|---|---|
| `admin` | `admin` | 관리자 | |
| `user` | `user` | 일반 사용자 | |
| `totp` | `totp` | 관리자 | 2차 인증 코드 `123456` |

## 작업 방식

- 세션에서 첫 작업을 시작하기 전에 `git status`로 커밋하지 않은 변경이 남아 있는지 확인한다. 남아 있으면 내용을 요약해 보여 주고 커밋할지 사용자에게 묻는다
- 작업을 시작하기 전에 `master`에서 작업 브랜치를 만들고 그 위에서 진행한다
- 작은 단위로 작업하고, 작업 단위 하나가 끝날 때마다 승인을 기다리지 않고 브랜치에 커밋한다
- 브랜치의 작업이 모두 끝나면 변경 요약을 제시하고, `master` 병합은 사용자가 승인한 뒤에만 한다
- 커밋 전에 `make check`를 실행한다. 프론트엔드나 embed 관련 변경이면 `make build`도 확인한다
- 새 의존성을 추가하기 전에 이유와 대안을 먼저 설명한다
- OMV RPC의 서비스·메서드 이름과 역할이 확실하지 않으면 추측하지 말고 OMV 소스(`/usr/share/openmediavault/engined/rpc/`, 로그인 관련은 `/var/www/openmediavault/rpc/session.inc`)를 읽는다
- 실제 시스템 값에 의존하는 로직은 샘플 데이터로도 테스트할 수 있게 입력부를 분리한다
- 테스트 픽스처(`testdata/` 등)에는 합성한 값만 쓴다. 이 NAS에서 뽑은 원본 출력을 그대로 넣지 않으며, 디스크 시리얼·MAC·IP·호스트 이름·사용자 이름 같은 식별 정보는 가짜 값으로 바꾼다
- 코드 주석은 한국어로 쓴다. 식별자·로그·에러 메시지는 영어를 유지한다

## 버전

- SemVer `0.y.z`. 원본은 `backend/internal/buildinfo`의 `Version` 상수 하나이고, OMV User-Agent와 정보 페이지(`/api/about`)가 이 값을 쓴다. `frontend/package.json`의 버전은 쓰지 않는다
- 기준은 업그레이드할 때 사용자가 할 일이 있는지다
  - 마이너(`0.y.0`): 새 기능·앱, 플래그·`install.sh` 설정 추가, 상태 디렉터리 형식 변경(`users/*.json`·`site.json`의 `version`, 이전 형식을 읽는 변환도 함께), 프록시 설정·지원 OMV 버전처럼 업그레이드 때 손봐야 하는 변경
  - 패치(`0.y.z`): 버그 수정, 번역·문구·문서 수정. 설정과 저장 형식은 그대로일 때
  - `1.0.0`: 깃허브에 공개하면서 설치·업그레이드 방법과 상태 디렉터리 형식의 호환을 약속할 수 있을 때
- 커밋마다 올리지 않는다(정보 페이지에 커밋 해시가 나온다). 올릴 때는 버전 커밋 하나를 만들고, 공개 배포부터는 그 커밋에 `v<버전>` 태그를 붙인다

## 나중에 확인할 것

- provider 내부 RPC 경로(`GET /api/addons/{id}` → OMV)를 실제 OMV로 확인한다: 컨테이너(`Compose.getContainers`), SMART(`Smart.getList`, 관리자), cputemp(hwmon에 CPU 센서가 없을 때만). 지금은 Go 테스트(omvfake)로만 검증했다. `/api/rpc` 중계 통과 경로도 아직 실제 OMV로 확인하지 않았다
- Podman 모드의 컨테이너 목록(미검증 표시 중)
- 컨테이너·서비스 조작을 실제 OMV로 확인한다(사용자가 `make dev`로). 컨테이너: `Compose.doContainerCommand` → `Exec.isRunning` 흐름, 실패한 docker 명령이 `Exec.isRunning` 예외로 오는지. 서비스: `Services.getStatus`의 실제 이름·값, 어댑터별 get → set(스키마 속성만) → `Config.applyChangesBg` → `Exec.isRunning` 흐름, 적용 대기 중 거절. 지금은 omvfake로만 검증했다

- 설정 앱을 실제 OMV·설치본으로 확인한다(사용자가). 유닛 하드닝 아래에서 `site.json`·`images/` 쓰기와 이미지 올리기(리버스 프록시의 본문 크기 제한이 20MB보다 작으면 413), 워크벤치 주소·SMART 시각 덮어쓰기가 재시작 뒤에도 유지되는지. 지금은 가짜 OMV와 `make dev-fake`로만 확인했다
- 프로필 저장을 실제 OMV로 확인한다(사용자가 `make dev`로). 관리자(일반 계정)는 저장 후 `/etc/passwd`의 GECOS·OMV 전자 우편이 바뀌고 적용 대기가 남지 않는지, 일반 사용자는 적용 대기가 생기는지, 내장 `admin`은 읽기 전용으로 보이는지, "OMV에서 사용자 편집"이 실제 편집 화면(`#/usermgmt/users/edit/<이름>`)을 여는지. 지금은 omvfake로만 확인했다
- 강제 테마(라이트·다크)에서 묶음 로고(`<img>` 안 SVG의 `prefers-color-scheme`) 글자색이 따라가는지 Firefox·Safari에서 확인한다. Chromium에서는 따라간다. 따르지 않으면 로고를 `?raw`로 인라인한다
- `install.sh`로 실제 설치한 뒤(사용자가 실행): 유닛 하드닝(`ProtectSystem=strict`, `DevicePolicy=closed`, `RestrictAddressFamilies` 등. `PrivateDevices=yes`는 `/dev/disk/by-label`을 숨겨 파일시스템 라벨이 사라지므로 쓰지 않는다) 아래에서 모든 애드온 데이터가 나오는지, Docker Caddy의 `/run/onmaru` 바인드 마운트가 서비스 재시작·재부팅 후에도 유지되는지. 지금은 `systemd-run --user`로 비슷한 제한을 걸어 가짜 OMV로만 확인했다
- OMV 워크벤치 링크 앱을 실제 OMV로 확인한다(사용자가 `make dev`에 `ONMARU_WORKBENCH_URL`을 두고). 같은 탭을 다시 쓸 때 브라우저가 그 탭으로 전환하는지(Chrome·Firefox), 그 탭의 OMV 로그인이 유지되는지. 지금은 가짜 OMV와 같은 출처 주소로만 확인했다
- (보류: 새 탭 방식을 택했다) 같은 출처에서 OMV workbench를 iframe 창으로 띄울 수 있는지
- (보류) 같은 탭 iframe은 sessionStorage를 공유하므로, 대시보드 로그인 후 OMV 창도 로그인 상태로 보이게 할 수 있는지. OMV 내부 저장 키(`username`, `permissions`)에 의존하는 실험이라 OMV 업데이트 시 깨질 수 있다. 실험 브랜치에서만 다룬다
