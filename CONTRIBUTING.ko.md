# 마루 NAS에 기여하기

English: [CONTRIBUTING.md](CONTRIBUTING.md)

openmediavault 8용 데스크톱형 웹 대시보드 마루 NAS에 관심을 가져 주셔서 감사합니다. 이 문서는 이슈나 풀 리퀘스트를 올리기 전에 알아야 할 것을 정리합니다. 영역별 설계 규칙은 `CLAUDE.md`와 `.claude/rules/`에 있습니다.

## 범위

- 마루 NAS는 가동 현황 확인과 간단한 조작을 맡습니다. 심층 설정은 OMV 웹 UI(워크벤치)에서 합니다. OMV 화면을 다시 만들지 않고 해당 화면으로 연결합니다.
- 로컬 네트워크 전용입니다. 인터넷 노출을 전제로 한 기능이나 설정은 추가하지 않습니다.
- OMV 8.x 모든 버전을 지원합니다. 새 RPC 호출은 OMV 8.0 소스에도 있어야 합니다.
- `ROADMAP.md`에 예정된 기능이 있습니다. 그 기능을 시작하기 전에 이슈를 열어 설계를 먼저 맞춰 주세요.

## 개발 환경 준비

Go 1.27.1 이상, Node.js 22.12 이상, npm, git, make가 필요합니다. 이 도구가 하나도 없는 Debian 13(그 위에 만든 OMV 8 포함)에서는 다음처럼 설치합니다.

```sh
sudo apt update
sudo apt install git make nodejs npm
sudo apt install -t trixie-backports golang-go
```

Debian의 Node.js는 20.19입니다. 빌드도 되고 테스트도 통과하지만 테스트 도구 Vitest 5는 22.12 이상만 공식 지원하므로, 할 수 있다면 [nodejs.org](https://nodejs.org/)의 배포본을 권합니다. 다른 시스템에서는 Go를 [go.dev/dl](https://go.dev/dl/)에서, Node.js를 nodejs.org에서 설치합니다. 도구에 대한 더 자세한 내용(기본 Debian 저장소의 Go 1.24 등)은 [docs/install.ko.md](docs/install.ko.md#빌드-도구)에 있습니다.

그다음 코드를 받아 개발 서버를 띄웁니다.

```sh
git clone https://github.com/steamb23/maru-nas.git
cd maru-nas
git submodule update --init --depth 1
npm --prefix frontend ci
make dev-fake
```

`make dev-fake`는 백엔드, Vite 개발 서버, 가짜 OMV를 함께 띄우므로 실제 OMV 없이 개발할 수 있습니다. `http://<호스트>:5173/`에 접속해 가짜 계정으로 로그인합니다.

| 계정 | 비밀번호 | 역할 | 비고 |
|---|---|---|---|
| `admin` | `admin` | 관리자 | |
| `user` | `user` | 일반 사용자 | |
| `totp` | `totp` | 관리자 | 2차 인증 코드 `123456` |

실제 OMV로 확인하려면 `frontend/.env.local`에 `MARU_OMV_URL=<OMV 주소>`를 두고 `make dev`를 실행합니다. 이 파일은 커밋하지 않습니다.

개발 서버의 백엔드도 기본으로 `127.0.0.1:8090`을 써서 설치한 maru 서비스와 주소가 같습니다. 같은 머신에 maru를 설치했다면 서비스를 Unix 소켓이나 다른 포트로 설치하거나(`install.sh`는 포트가 사용 중이면 경고합니다), 개발 서버를 다른 포트로 띄웁니다. 예: `MARU_LISTEN=127.0.0.1:8190 DEV_PORT=5174 make dev-fake`

자주 쓰는 명령(저장소 루트에서 실행):

| 명령 | 내용 |
|---|---|
| `make check` | 린트(`gofmt`, `go vet`, oxlint, `tsc`)와 테스트(`go test`, Vitest) |
| `make build` | 기본 대상(대상 없이 `make`). 프로덕션 빌드(`bin/maru`, 권한 도우미 `bin/maru-helper`, 설치기 `bin/maru-install`) |
| `make types` | Go 구조체와 애드온 manifest에서 TypeScript 다시 생성 |

## 문서

`README.md`에는 소개, 빠른 시작, 문서 목록, 기여 안내만 둡니다. 사용자 문서는 `docs/`에 있고 [Zensical](https://zensical.org/)로 사이트를 만듭니다. README와 모든 페이지는 영어(`<이름>.md`)와 한국어(`<이름>.ko.md`)로 있습니다. 언어마다 목차와 검색이 따로인 사이트를 만듭니다. `zensical.toml`이 영어 페이지를, `zensical.ko.toml`이 한국어 페이지를 빌드하고, 한국어 사이트는 `ko/` 아래에 올라갑니다. 사이트(<https://steamb23.github.io/maru-nas/>)는 `stable` 브랜치에서 배포하므로, `main`에서 고친 문서는 다음 릴리스 때 사이트에 반영됩니다. 유지자가 Documentation 워크플로를 수동으로 실행해 `main`을 먼저 올릴 때도 있습니다.

- 영어 페이지와 한국어 페이지를 함께 고칩니다. 한국어를 쓰지 않는다면 영어 페이지만 고치고 풀 리퀘스트에 그렇다고 적어 주세요. 병합하기 전에 유지자가 한국어 페이지를 고칩니다.
- 사이트를 빌드하지 않아도 GitHub에서 읽혀야 합니다. GitHub가 그리는 마크다운만 씁니다. GitHub 알림(`> [!NOTE]`, `> [!TIP]`, `> [!WARNING]`)은 되고, Zensical 전용 문법(`!!!` 알림, `===` 탭, 속성 목록)은 쓰지 않습니다.
- 중첩 목록과 목록 안 코드 블록은 4칸 들여씁니다. GitHub는 2칸도 받지만 사이트의 마크다운 엔진은 4칸이 필요합니다.
- 표 칸 안에는 코드 안이라도 `|`를 넣지 않습니다.
- 페이지 사이는 상대 경로 `.md`와 제목 앵커로 겁니다. 영어 페이지는 영어 페이지로(`configuration.md#opening-the-omv-workbench`), 한국어 페이지는 한국어 페이지로(`configuration.ko.md#omv-워크벤치-열기`) 겁니다. `docs/` 밖의 파일은 GitHub 절대 주소로 겁니다. 한 페이지 안에 같은 제목을 두 번 쓰면 GitHub와 사이트의 앵커가 달라지므로 쓰지 않습니다.
- 사용자 문서에는 사용자가 할 일과 조건, 한도, 위험을 씁니다. 내부 구현은 `.claude/`의 설계 문서에 둡니다.
- 설치기 옵션이나 플래그를 더하거나 바꾸면 `docs/install-options.md`(테스트가 검사합니다)와 `docs/configuration.md`의 표를 두 언어 모두 고칩니다.
- 페이지를 더하면 `zensical.toml`과 `zensical.ko.toml` 두 곳의 목차에 넣습니다.

사이트를 미리 보고 검사하려면 고정한 Zensical 버전을 pipx나 가상 환경에 설치합니다(Debian과 Ubuntu는 시스템 전체 `pip install`을 거부합니다).

```sh
python3 -m venv .venv
.venv/bin/pip install zensical==0.0.71
.venv/bin/zensical serve                                   # 영어 미리보기: http://localhost:8000/
.venv/bin/zensical serve -f zensical.ko.toml -a localhost:8001  # 한국어 미리보기: http://localhost:8001/
# CI가 하는 검사. 깨진 링크·앵커가 있으면 실패
.venv/bin/zensical build --strict
.venv/bin/zensical build --strict -f zensical.ko.toml
```

## 브랜치와 풀 리퀘스트

- `main`에서 `<종류>/<짧은-설명>`(영어 소문자, 하이픈) 브랜치를 만들어 작업합니다. 종류는 `feat`(새 기능·앱), `fix`(버그 수정), `docs`(문서), `i18n`(번역), `chore`(빌드·CI·정리), `refactor`(동작 변화 없는 구조 변경)입니다.
- 저장소를 포크한 뒤 `main`을 대상으로 풀 리퀘스트를 올립니다. 풀 리퀘스트 하나에 변경 하나만 담습니다.
- 풀 리퀘스트는 머지 커밋 없이 `main`에 rebase로 병합됩니다. 자기 풀 리퀘스트 브랜치에는 force-push해도 됩니다.
- 모든 풀 리퀘스트에서 CI가 `make check`와 `make build`, 문서 빌드(`zensical build --strict`)를 실행하며 모두 통과해야 합니다. push하기 전에 `make check`를 먼저 돌리고, `docs/`나 `zensical.toml`을 바꿨다면 문서 빌드도 돌려 주세요.
- `main`은 개발 브랜치입니다. 릴리스에는 `v<버전>` 태그를 붙이고, `stable` 브랜치는 항상 최신 릴리스를 가리킵니다.

## 커밋 메시지와 언어

- 커밋 메시지와 풀 리퀘스트 설명은 영어 또는 한국어로 써 주세요. 다른 언어도 받지만, 둘 중 하나면 검토가 쉽습니다.
- 유지자는 커밋과 코드 주석을 한국어로 씁니다. 그래서 이력과 코드 곳곳에 한국어가 있습니다. 기여자의 주석은 영어나 한국어 어느 쪽이든 괜찮습니다.
- 커밋 메시지 첫 줄에는 무엇을 왜 바꿨는지 한 문장으로 적습니다.

## 코드 규칙

- 백엔드는 Go 표준 라이브러리를 쓰고, 웹 터미널에만 `golang.org/x/crypto/ssh`와 `github.com/coder/websocket`을 씁니다. 권한 도우미에는 둘 다 링크하지 않습니다. 설치기(`installer/`, `install.sh`가 실행하는 별도 Go 모듈)는 터미널 화면에 `charm.land/huh/v2`를 쓰며, 이 의존성은 `installer/go.mod`에만 있고 설치기는 백엔드에서 `backend/config`만 씁니다. 프론트엔드는 TypeScript, Vite, React, Zustand이고 웹 터미널에 xterm.js, 인증 앱 QR 코드에 uqr를 씁니다.
- 의존성을 추가하기 전에 왜 필요한지와 대안이 무엇이었는지 풀 리퀘스트에 설명해 주세요.
- `backend/internal/api`의 구조체나 애드온 manifest를 바꾸면 `make types`를 실행하고 생성물(`frontend/src/api/types.gen.ts`, `frontend/src/addons/manifests.gen.ts`)을 함께 커밋합니다. 테스트가 최신 상태인지 검사합니다.
- 식별자, 로그, 에러 메시지는 영어로 유지합니다.
- 화면에 나오는 모든 문구(`aria-label`, `title` 포함)는 `t()`를 거치고, 키를 `en.json`과 `ko.json`에 함께 넣습니다. 한국어를 쓰기 어렵다면 `ko.json`의 같은 키에 영어 문구를 넣고 풀 리퀘스트에 알려 주세요. 병합 전에 유지자가 번역합니다.

## 권한과 보안

마루 NAS는 권한 없는 시스템 사용자로 실행되며, 그 상태를 유지해야 합니다.

- 서비스 프로세스는 권한을 올리지 않습니다. root가 필요한 정보와 변경은 로그인한 사용자 자신의 OMV 세션으로 OMV RPC API를 거칩니다.
- `omv-rpc`를 호출하거나, OMV 엔진 소켓에 붙거나, Docker·Podman 소켓을 직접 읽는 코드는 추가하지 않습니다. 서비스 사용자를 `docker`나 `openmediavault-engined` 그룹에 넣는 풀 리퀘스트는 받지 않습니다.
- OMV RPC로 할 수 없는 권한 작업(사용자 자신의 파일, root 작업)은 권한 도우미(`backend/cmd/maru-helper`)의 정해진 연산으로만 합니다. 임의 명령을 실행하는 기능은 없습니다. root 연산을 더하려면 이슈에서 먼저 상의해 주세요.
- 비밀번호, OMV 세션 ID, 토큰을 로그·테스트 픽스처·커밋에 남기지 않습니다. 보안 강화 모드의 인증 앱 비밀 키와 복구 코드도 비밀 값입니다.
- 테스트 픽스처(`testdata/` 등)에는 합성한 값만 씁니다. 디스크 시리얼, MAC·IP 주소, 호스트 이름, 사용자 이름은 가짜 값으로 바꿉니다.
- 도메인, IP 주소 같은 개인 환경 정보는 git이 무시하는 `*.local.md` 파일에 둡니다. 커밋되는 파일에 하드코딩하지 않습니다.

## 번역

번역은 가장 쉽게 기여할 수 있는 길입니다.

- 카탈로그는 `frontend/src/i18n/locales/<언어>.json`입니다. `en`과 `ko`는 유지자가 관리하며 항상 완전합니다. 그 밖의 언어는 일부만 번역되어 있어도 되고, 빠진 키는 영어로 표시됩니다.
- `en.json`을 원문으로 옮깁니다. 제품·기술 이름(OpenMediaVault, OMV, SMART, SSH, SMB/CIFS, NFS, systemd, Go), 경로, 명령, 단위, 예시 도메인은 옮기지 않습니다.
- 복수형은 i18next v4 접미사(`key_one`, `key_other` 등)를 씁니다. 범주의 종류는 언어마다 다릅니다. 복수형 키를 번역하면 그 언어가 쓰는 범주를 모두 넣어 주세요.
- 언어별 용어와 어조는 `.claude/rules/i18n.md`의 "번역 기준" 표에 있습니다.
- 카탈로그 테스트로 확인합니다.

  ```sh
  cd frontend && npx vitest run src/i18n
  ```

- 언어를 추가하려면 `locales/<코드>.json`(BCP 47 태그, 예: `pt-PT`, `zh-Hant`)을 만들고 `frontend/src/i18n/languages.ts`의 `SUPPORTED_LANGUAGES`에 넣습니다.
- 소프트 하이픈 같은 JSON 이스케이프는 파일에 이스케이프 그대로 둡니다. 보이지 않는 문자 자체를 붙여 넣지 마세요. 카탈로그 테스트가 그런 문자를 행 번호와 함께 알려 줍니다.

## 아이콘과 제3자 에셋

- 아이콘은 Colloid 테마만 쓰고, `getIcon(name)`으로 freedesktop 이름을 써서 가져옵니다. 다른 아이콘 테마나 아이콘 경로 직접 참조는 추가하지 않습니다.
- 제3자 에셋(예: 브랜드 로고)은 라이선스가 GPL-3.0과 호환될 때만 받습니다. 원본을 수정하지 않고 `vendor/icons/<출처>/`에 `LICENSE` 또는 `NOTICE`와 함께 두고, `vendor/icons/README.md`에 절을 더하고, `frontend/src/icons/sources/`에 출처 테이블을 만듭니다. 상세는 `.claude/rules/icons.md`에 있습니다.

## 라이선스

마루 NAS는 GNU General Public License v3.0으로 배포됩니다(`LICENSE` 참고). 기여하시면 기여 내용도 같은 조건으로 제공하는 데 동의하는 것으로 봅니다.
