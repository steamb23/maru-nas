# OMV 데스크톱 대시보드

<!-- 프로젝트 이름이 정해지면 위 제목을 바꿔주세요. HTML 주석은 Claude 컨텍스트에서 제외되므로 메모용으로 써도 됩니다. -->

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
- 마지막 완료 단계의 바로 다음 단계가 허용하지 않는 기능(아래 "개발 단계")은 구현하지 않는다

## 아키텍처

- **배포**: OMV와 같은 출처(same origin)로 서빙한다. 앞단의 Caddy가 HTTPS를 처리하고, 같은 도메인에서 `/desk/*`는 이 백엔드로, 나머지는 OMV로 리버스 프록시한다. OMV의 nginx 설정은 건드리지 않는다
  - 도메인 등 개인 환경 정보는 `CLAUDE.local.md`(커밋 안 함)에 둔다. 커밋되는 파일에 도메인·IP를 하드코딩하지 않는다
  - 같은 출처 구성은 OMV 화면을 iframe으로 띄우는 실험을 위해 유지한다. 인증과는 무관하다
- **실행**: systemd 서비스로 Go 실행 파일 하나를 전용 일반 사용자로 실행한다. 프론트엔드는 `go:embed`로 포함하고, 127.0.0.1에만 바인딩한다
  - Docker는 쓰지 않는다(호스트 `/proc`, `/sys`, 네트워크를 직접 봐야 하므로)
  - systemd 유닛과 Caddy 설정 예시는 `deploy/`에 작성만 하고, 설치는 사용자가 한다
- **백엔드는 얇은 API 서버**다. 역할은 (1) 현황 읽기: `/proc`·`/sys` 직접 읽기, root가 필요한 정보는 OMV RPC, (2) 설정 변경 요청을 OMV `rpc.php`로 전달, 두 가지뿐이다
- **OMV RPC는 항상 백엔드가 중계한다**: 브라우저 → `/desk/rpc` → 백엔드 → OMV `rpc.php`. 브라우저가 `rpc.php`를 직접 호출하지 않는다
  - 백엔드는 대시보드 세션마다 OMV 세션을 메모리에 보관하고, 항상 같은 주소(127.0.0.1)와 고정 User-Agent로 OMV를 호출한다
  - 브라우저의 쿠키를 OMV에 그대로 전달하지 않는다. OMV는 User-Agent가 다르면 세션을 파기하므로 사용자를 로그아웃시키게 된다
- **인증**: 자체 로그인 화면에서 백엔드가 OMV `Session.login`(필요하면 `Session.verify`)으로 위임하고, 대시보드 자체 세션 쿠키를 발급한다. 세션 만료는 OMV timeout 기준 브라우저 타이머로 처리한다. 상세는 `.claude/rules/auth-omv.md`

## 기술 스택

- **백엔드**: Go 표준 라이브러리만(`backend/`). 런타임 외부 의존성 없음
- **프론트엔드**: TypeScript + Vite + React(`frontend/`), 창 상태는 Zustand 단일 저장소(`windows[]`). 린트 oxlint, 테스트 Vitest(DOM 환경 없이 순수 함수·저장소만)
- **타입 공유**: tygo로 `backend/internal/api` → `frontend/src/api/types.gen.ts`. 생성물은 커밋하며 직접 수정하지 않는다
- **현지화**: `en`, `ko`. 화면 문구(aria-label·title 포함)는 모두 `t()`를 거치고, 새 키는 `en.json`과 `ko.json`에 함께 넣는다. 로그인 대기 이스터에그 문구는 현지화 제외(영어 고정)
- **아이콘**: Colloid 테마 하나만, 리졸버 `getIcon(name)`으로 freedesktop 이름을 써서 가져온다

## 애드온과 위젯 (3단계)

상세 설계는 `docs/addons.md`, 출처별 표와 플러그인 조사는 `docs/data-sources.md`.

- **애드온 = manifest + provider + 보기(창, 위젯)**. 시스템 정보를 포함한 모든 기능을 같은 계약으로 만든다. 보기는 창과 바탕화면 위젯 중 하나 이상이며, 둘은 같은 provider·API를 쓰고 화면만 다르다
- **위젯은 창 시스템과 분리한다**. 바탕화면 위 별도 레이어에 그리드 칸 단위로 붙이고, 창 저장소·창 컴포넌트에 위젯용 분기를 넣지 않는다. 공유는 provider, `/api/*`, `poller.ts`, 공통 훅까지만 한다
- `/desk/rpc` 중계는 manifest 허용 목록(read·write 구분)에 있는 메서드만 통과시키고, 현재 단계가 허용하지 않는 write는 백엔드가 거부한다
- 내장 애드온만 정적 목록으로 둔다(백엔드 `internal/addons/<id>/`, 프론트 `src/addons/<id>/`). 컴파일 타임 타입(`AppId`)을 유지하고, 별도 패키지·런타임 로딩·Go `plugin`은 쓰지 않는다
- 플러그인 설치 여부는 `/usr/share/openmediavault/engined/rpc/<name>.inc`와 `/var/lib/dpkg/status`로 감지해 `/api/capabilities`로 내려준다
- provider는 권한이 필요 없는 직접 읽기(`/proc`, `/sys`)를 1차로, OMV RPC를 보강으로 쓰고 응답에 출처를 담는다
- 컨테이너는 compose 플러그인(omv-extras) RPC로 가져오며 Docker 런타임 기준으로 검증한다. Podman 모드는 미검증으로 표시한다

## 명령어

저장소 루트에서 실행한다. 처음 클론했다면 `git submodule update --init --depth 1`과 `npm --prefix frontend ci`를 먼저 실행한다.

| 명령 | 내용 |
|---|---|
| `make dev` | 개발 서버. 백엔드 `127.0.0.1:8090`(`go run`) + Vite `0.0.0.0:5173`. 접속은 `http://<NAS-IP>:5173/desk/`. Ctrl+C로 둘 다 종료. 실제 OMV에 연결하므로 `frontend/.env.local`에 `OMVDESK_OMV_URL`(이 NAS의 OMV 주소)을 둔다 |
| `make dev-fake` | `make dev`와 같지만 가짜 OMV(`127.0.0.1:8091`)를 함께 띄워 연결한다. `FAKEOMV_TIMEOUT=1m make dev-fake`처럼 세션 timeout을 분 단위로 바꿀 수 있다 |
| `make build` | 프론트 빌드 → `backend/internal/web/static/dist/` → `bin/omvdesk` 단일 실행 파일 |
| `make lint` | `gofmt -l`, `go vet`, oxlint, `tsc -b` |
| `make test` | `go test ./...`, `vitest run` |
| `make check` | lint + test |
| `make types` | Go 구조체에서 TS 타입 다시 생성. `internal/api`를 바꾸면 반드시 실행하고 생성물도 함께 커밋한다 |

개별 실행: `cd backend && go test ./internal/sysinfo -run TestParseMemInfo`, `cd frontend && npx vitest run src/icons`

## 개발 단계

마지막 완료 단계: **2단계**

3단계는 세부 단계(3-1 → 3-2 → 3-3) 순서로 진행하고, 완료 표시도 세부 단계로 적는다(예: **3-1단계**). "바로 다음 단계"는 다음 세부 단계를 뜻한다.

1. 데스크톱 셸 + 시스템 정보 창(`/proc`의 CPU·RAM·가동 시간을 2초마다 갱신). 완료
2. 로그인: OMV 위임 로그인, 세션 복원·만료, 로그인 시도 제한. 작업표시줄 시계·프로필 메뉴와 현지화(ko·en)도 이 무렵 추가했다. 로그인 대기 이스터에그(`frontend/src/auth/easterEgg/`)는 단계와 무관하게 사용자 요청으로 추가한 예외다. 완료
3. 애드온과 위젯(읽기 전용): 세 세부 단계 모두 시스템 상태를 바꾸는 코드(쓰기 RPC 포함) 금지
   - 3-1. 애드온 구현: 애드온 계약(manifest, provider, RPC 허용 목록)과 capability 감지(`/api/capabilities`)를 만든다
     - 기존 시스템 정보 창을 첫 내장 애드온(창 보기)으로 옮겨 계약을 검증한다
     - 위젯 레이어와 새 기능 애드온은 만들지 않는다
   - 3-2. 위젯 시스템 구현: 바탕화면 위젯 레이어(그리드 배치, 추가·제거·이동·크기 조절, 배치 저장)와 manifest의 위젯 보기 선언을 만든다
     - 시스템 정보의 CPU 사용률·RAM·가동 시간을 첫 위젯으로 옮겨 위젯 시스템을 검증한다
     - 정할 것은 `docs/addons.md`의 "3-2단계에서 정할 것"에 있다
   - 3-3. 위젯 세부 구현: 디스크 용량, SMART, 온도, 네트워크, 컨테이너 목록을 내장 애드온의 위젯으로 구현한다
     - 현재 설치된 플러그인 기준으로 먼저 구현한다. 컨테이너는 Docker 런타임 기준으로 검증한다
     - 위젯으로 부족한 항목은 같은 애드온에 창 보기를 둔다
   - 시스템 정보 창은 위젯으로 요약을 옮긴 뒤 나중에 프로세스 모니터로 확장한다(시점 미정, 사용자가 정한다). 프로세스 목록 읽기는 읽기 전용이지만 프로세스 종료 같은 조작은 4단계 이후다
4. 간단한 조작: 컨테이너 시작·정지, 서비스 켜기·끄기를 OMV RPC로
5. 파일 탐색기 창: 공유 폴더 둘러보기(우선 읽기 전용)
6. "심층 설정" 연결: 각 창에서 해당 OMV 페이지로 이동

단계를 넘어갈지는 사용자가 정한다. 다음 단계 기능이 필요해 보이면 구현하지 말고 제안한다.

## 테스트 전략

로그인이 필요한 기능도 실제 비밀번호 없이 검증한다. Claude는 실제 OMV에 로그인 시도를 보내지 않는다(실패 로그·fail2ban 영향, 비밀번호 취급 금지).

1. **Go 테스트**: 가짜 OMV(`internal/omv/omvfake`)를 `httptest`로 띄워 실제 클라이언트·핸들러와 연결한다. 상세는 `.claude/rules/backend.md`
2. **Vitest**: 저장소·클라이언트·순수 함수
3. **브라우저(Playwright)**: `make dev-fake`로 띄우고 `http://<NAS-IP>:5173/desk/`에 가짜 계정으로 로그인해 확인한다. 만료 확인은 `FAKEOMV_TIMEOUT=1m make dev-fake`
   - 사용자가 `make dev`(실제 OMV 연결)를 띄워 두었을 수 있다. 로그인하기 전에 백엔드 로그의 `omv:` 주소가 가짜 OMV(`127.0.0.1:8091`)인지 확인한다. `scripts/dev.sh`는 개발 서버 포트가 이미 쓰이고 있으면 시작하지 않는다
   - 탭을 여러 개 열면 뒤쪽 탭에 마우스 입력이 전달되지 않을 수 있다. 다른 탭 확인이 끝나면 닫거나 브라우저를 새로 연다
4. **실제 OMV**: 사용자가 `make dev`로 직접 확인한다. Claude는 단계마다 확인 체크리스트를 제시한다

가짜 계정(`omvfake.DefaultUsers`, 모두 가짜 값):

| 계정 | 비밀번호 | 역할 | 비고 |
|---|---|---|---|
| `admin` | `admin` | 관리자 | |
| `user` | `user` | 일반 사용자 | |
| `totp` | `totp` | 관리자 | 2차 인증 코드 `123456` |

## 작업 방식

- 작은 단위로 작업하고 의미 있는 단위마다 git 커밋한다. 단, 커밋은 사용자가 변경 내용을 검토하고 승인한 뒤에만 한다. 작업이 끝나면 변경 요약을 제시하고 승인을 기다린다
- 커밋 전에 `make check`를 실행한다. 프론트엔드나 embed 관련 변경이면 `make build`도 확인한다
- 새 의존성을 추가하기 전에 이유와 대안을 먼저 설명한다
- OMV RPC의 서비스·메서드 이름과 역할이 확실하지 않으면 추측하지 말고 OMV 소스(`/usr/share/openmediavault/engined/rpc/`, 로그인 관련은 `/var/www/openmediavault/rpc/session.inc`)를 읽는다
- 실제 시스템 값에 의존하는 로직은 샘플 데이터로도 테스트할 수 있게 입력부를 분리한다
- 테스트 픽스처(`testdata/` 등)에는 합성한 값만 쓴다. 이 NAS에서 뽑은 원본 출력을 그대로 넣지 않으며, 디스크 시리얼·MAC·IP·호스트 이름·사용자 이름 같은 식별 정보는 가짜 값으로 바꾼다
- 코드 주석은 한국어로 쓴다. 식별자·로그·에러 메시지는 영어를 유지한다

## 나중에 확인할 것

- 같은 출처에서 OMV workbench를 iframe 창으로 띄울 수 있는지
- 같은 탭 iframe은 sessionStorage를 공유하므로, 대시보드 로그인 후 OMV 창도 로그인 상태로 보이게 할 수 있는지. OMV 내부 저장 키(`username`, `permissions`)에 의존하는 실험이라 OMV 업데이트 시 깨질 수 있다. 실험 브랜치에서만 다룬다
