# 애드온 구조

기능(시스템 정보, 저장소, 온도, 네트워크, 컨테이너 등)을 같은 계약으로 만들기 위한 설계다. 각 기능은 창과 바탕화면 위젯 중 하나 이상의 보기로 보여 주고, 필요하면 상태를 바꾸는 조작(컨테이너, OMV 서비스, 전원)을 둔다. 항목별 데이터 출처는 [data-sources.md](data-sources.md)를 본다.

데이터·조작이 없는 셸 기능("모든 앱", 바로가기, OMV 웹 UI를 새 탭으로 여는 링크 앱)은 애드온이 아니다. 링크 앱은 `windows/apps.ts`의 `kind: 'link'`이며 `.claude/rules/frontend-shell.md`의 "바로가기와 모든 앱"을 본다.

범위는 아래 두 층이다.

1. **애드온 계약**: 애드온이 무엇을 필요로 하고 무엇을 호출하는지 선언하는 manifest, provider, capability
2. **내장 애드온**: 저장소 안에 소스로 들어 있고 함께 빌드되는 애드온. 기능을 추가하려면 포크해서 기여하거나 직접 빌드해서 쓴다

런타임에 불러오는 애드온은 만들지 않는다. 아래 "향후 고려"에 나중을 위한 제약만 적어 둔다.

## 애드온 계약

### manifest

애드온마다 하나씩 둔다. Go 코드 곳곳에 흩어 두지 않고 직렬화할 수 있는 데이터 구조(`api.Manifest`)로 정의한다. 런타임 선언형 애드온이 생기더라도 같은 구조를 그대로 읽을 수 있게 하려는 것이다.

| 필드 | 내용 |
|---|---|
| `id` | 고유 이름(`storage`, `containers` 등). 창·위젯 종류 식별자와 같다 |
| `title`, `icon` | 창 제목·위젯 이름의 번역 키(`en.json`), freedesktop 아이콘 이름(리졸버로 해석). 없는 키·아이콘은 tsc가 잡는다 |
| `views.window` | 창 보기. 기본(`defaultSize`)·최소(`minSize`) 크기(픽셀) |
| `views.widget` | 위젯 보기. 기본(`defaultSize`)·최소(`minSize`)·최대(`maxSize`) 크기(그리드 칸, `cols`·`rows`). 최소·최대가 없으면 그 방향으로는 기본 크기에 고정된다. 한 변은 1~12칸(`addons.MaxWidgetSpan`) |
| `requires.plugins` | 필요한 OMV 플러그인 패키지 이름. 비어 있으면 플러그인 없이 동작한다. `internal/addons`의 `knownPlugins` 표에 있는 이름만 쓸 수 있다 |
| `requires.admin` | admin만 쓸 수 있는지 여부 |
| `rpc.read` | 호출하는 읽기 전용 OMV RPC (`Service.method`) |
| `rpc.write` | 호출하는 상태 변경 OMV RPC. 조작(`actions`)만 부른다 |
| `actions` | 상태를 바꾸는 조작(`name`, `admin`, `confirm`). 구현은 `Addon.Actions`. 아래 "조작" |

레지스트리(`addons.NewRegistry`)는 등록할 때 manifest를 검사한다: id 형식(소문자·숫자·`-`)과 중복, 보기 1개 이상(창 또는 위젯), 크기(위젯은 최소 ≤ 기본 ≤ 최대), `Service.method` 형식, 같은 메서드를 읽기와 쓰기로 함께 선언하지 않기, `Session.*` 금지(로그인·로그아웃은 백엔드만 부른다), 조작 선언과 구현의 1:1 일치(조작이 있으면 `rpc.write` 1개 이상), provider 1개 이상.

한 애드온 안에서도 출처마다 조건이 다를 수 있다(예: 온도는 hwmon은 누구나, SMART 디스크 온도는 admin). 그래서 `requires`는 "창·위젯을 쓸 수 있는 최소 조건"이고, provider별 조건은 provider가 따로 선언한다.

### provider

- 한 항목의 데이터를 한 출처에서 가져오는 구현이다. 인터페이스는 대략 다음 형태다
  - `Name()`: 출처 이름(`hwmon`, `omv-cputemp` 등). 응답에 출처로 담긴다
  - `Requires()`: 이 provider에 필요한 플러그인과 역할
  - 데이터 메서드: 요청 컨텍스트와 세션(OMV RPC 클라이언트, 역할)을 받는다
- 애드온은 provider를 우선순위대로 갖는다
  - 쓸 수 있는 것 중 첫 번째를 쓰거나, 항목에 따라 결과를 합친다
  - 합칠 때는 같은 센서·장치가 중복되지 않게 한다
- 직접 읽기 provider는 `fs.FS`를 주입받는다. `internal/addons/sysinfo`의 `Reader`(provider 이름 `proc`)와 같은 방식이다
- OMV RPC provider는 RPC 클라이언트 인터페이스를 주입받는다. 테스트에서는 가짜 클라이언트를 넣는다

### capability 감지

- 플러그인 설치 여부
  - `/usr/share/openmediavault/engined/rpc/<name>.inc` 파일이 있는지와 `/var/lib/dpkg/status`의 패키지 상태로 판단한다. 둘 다 root 없이 읽힌다
  - 백엔드 시작 시 확인하고, 로그인할 때와 일정 주기마다 다시 확인한다. 실행 중에 플러그인을 설치하거나 지운 경우를 반영하기 위해서다
  - 구현은 `addons.Detector`다. 결과를 5분 보관하고, 지난 뒤 처음 묻는 요청에서 다시 확인한다(백그라운드 고루틴 없음). 읽지 못하면 직전 결과를 유지한다
  - `.inc` 파일 이름은 패키지마다 `knownPlugins` 표에 둔다. 표에 넣을 때 `dpkg -S`로 소속을 확인한다
- OMV 버전: 같은 `addons.Detector`가 같은 주기로 `/var/lib/dpkg/status`의 `openmediavault` 패키지 버전과 `/usr/share/openmediavault/productinfo.xml`의 코드명(`versionname`)을 읽는다. OMV `ProductInfo`와 같은 출처다. `/api/about`이 관리자에게만 주고 설정의 정보 페이지가 "버전 (코드명)"으로 보여 준다(OMV `System.getInformation`도 `version`을 관리자에게만 준다)
- 세션 역할: 로그인 응답의 `permissions.role`에서 얻은 admin 여부
- `/api/capabilities`는 두 정보를 합쳐 애드온마다 다음 값을 돌려준다. 함께 `plugins`(감지한 알려진 플러그인 목록)도 준다
  - `available`: manifest의 `requires`를 만족하고, 쓸 수 있는 provider가 하나 이상 있다
  - `reason`: `admin_required`, `plugin_missing`. 관리자 조건을 먼저 본다(일반 사용자에게 플러그인 설치 안내는 쓸모없다)
  - `sources`: 쓸 수 있는 provider 이름 목록(우선순위 순서)
- 프론트(`src/addons/capabilities.ts`)는 로그인한 동안 5분마다 가져온다. 관리자 전용은 바로가기와 "모든 앱"에서 숨기고(저장한 바로가기 목록에는 남는다), 플러그인 없음은 아이콘을 둔 채 창 안에 안내한다(`AddonUnavailable`). 아직 받지 못했거나 요청이 실패하면 모두 그린다. 실제 차단은 백엔드가 한다
- 실제 호출이 OMV에서 `RPC service '...' not found.`(HTTP 404)로 실패하면 provider를 쓸 수 없는 상태로 바꾼다. 이 경로는 마지막 안전장치일 뿐, 감지의 기본 수단이 아니다

### RPC 허용 목록과 쓰기 허용

- 백엔드의 OMV RPC 호출(`/api/rpc` 중계, provider의 내부 호출, 조작)은 등록된 manifest의 `rpc.read`·`rpc.write`에 있는 메서드만 통과시킨다(`Registry.Classify`, 대소문자까지 정확히 일치)
- 쓰기 RPC 허용 여부는 백엔드 한 곳(상수 `addons.WritesEnabled`)에서 정한다. 지금은 켜져 있다
- 켜져 있어도 **쓰기는 조작으로만** 부른다. `/api/rpc` 중계와 `Fetch`는 쓰기를 거부한다. `Compose.doContainerCommand`처럼 파라미터를 검사하지 않고 셸 명령을 만드는 OMV 메서드가 있어서, 쓰기 파라미터는 브라우저가 아니라 애드온 Go 코드가 만들어야 한다
- 테스트(`builtin`)로 "쓰기를 선언한 내장 애드온은 조작이 있다"를 확인한다
- 역할 검사는 OMV가 최종적으로 한다. 백엔드는 `requires.admin`과 조작의 `admin`으로 불필요한 호출을 미리 막을 뿐이다

### `/api/rpc` 중계

브라우저 경로는 `/api/rpc`. 본문은 `{service, method, params}`(64KB 이하)이고, 성공하면 OMV 응답의 `response` 값을 그대로 돌려준다.

| 상황 | 응답 |
|---|---|
| 허용 목록에 없음, 쓰기 메서드 | 403 `rpc_forbidden` (OMV에 보내지 않음) |
| 메서드를 선언한 애드온을 이 사용자가 쓸 수 없음 | 403 `admin_required` / `plugin_missing` (OMV에 보내지 않음) |
| OMV 401 | 대시보드 세션을 지우고 401 |
| OMV 404 (RPC 서비스 없음) | 플러그인 감지를 다시 하도록 표시하고 502 `omv_error` |
| 그 밖의 OMV 오류, 연결 실패 | 502 `omv_error`. OMV 메시지는 로그에만 남긴다 |

- OMV에는 `updatelastaccess=false`로 호출한다. 창·위젯의 주기 요청이 OMV 세션을 연장하면 안 되기 때문이다
- params는 다시 직렬화하지 않고 그대로 넘긴다(큰 정수의 정밀도 유지)
- 내장 애드온은 이 중계를 쓰지 않고 provider 내부 호출(아래 "애드온 데이터 API")로 OMV를 부른다. 중계는 나중의 선언형 애드온 등을 위해 남겨 둔다
- 통과 경로(허용된 메서드가 실제 OMV까지 가는 경우)는 Go 테스트(omvfake)로만 검증했다

### 애드온 데이터 API

내장 애드온의 현황 데이터는 브라우저가 OMV RPC를 직접 조합하지 않고 백엔드 provider가 만든다. 출처 합치기·중복 제거(온도), 세션 사이에 공유하는 캐시(SMART)가 백엔드에 있어야 하기 때문이다.

- `addons.Addon.Fetch(ctx, Request)`가 응답을 만든다. 없으면 `GET /api/addons/{id}`는 404다. 시스템 정보 위젯은 애드온 계약 이전부터 있던 `/api/system`을 그대로 쓰고, 창은 `GET /api/addons/system-info`를 쓴다
- `Request`: `Admin`, `Sources`(capability로 계산한 쓸 수 있는 provider, 우선순위 순서. `Has(name)`), `Refresh`(`?refresh=1`, 캐시를 쓰는 애드온만 본다), `Query`(조회 파라미터. 브라우저가 보낸 값이므로 애드온이 자기가 정한 이름·값만 읽고 OMV에 넘기지 않는다. 시스템 정보의 `processes=1`·`power=1`), `RPC`
- `RPC`(`addons.RPCCaller`)는 서버가 요청마다 사용자 세션으로 만든다
  - **호출한 애드온 자신이** manifest `rpc.read`에 선언한 메서드만 부른다(`Registry.Classify`로 확인, 다른 애드온이 선언한 메서드는 거부). 쓰기는 부를 수 없다(조작에서만)
  - 항상 `updatelastaccess=false`로 부른다. 위젯 주기 요청이 OMV 세션을 연장하면 안 된다
  - OMV 401이면 provider가 오류를 삼켜도 대시보드 세션을 지우고 401로 응답한다. 404(RPC 서비스 없음)면 플러그인 감지를 다시 하게 한다
- 서버(`internal/server/addondata.go`)는 capability를 먼저 확인한다(403 `admin_required`/`plugin_missing`, OMV에 보내지 않음). `Fetch`가 실패하면 OMV 호출 실패는 502 `omv_error`, 그 밖은 500
- 세션 사이 캐시: SMART는 `smart.Cache`(하루 한 번), 서비스·컨테이너는 `addons.SharedCache`(5초, 동시 요청은 OMV 호출 하나를 함께 기다림, 조작 뒤 `Invalidate`). 사용자와 무관한 결과에만 쓴다. 프론트의 `refresh()`가 `?refresh=1`을 보내면 건너뛴다
- 응답은 모두 `api.AddonSources`(`sources`: 값을 보탠 provider, `failedSources`: 쓸 수 있었지만 실패한 provider)를 품는다. 일부 출처만 실패하면 나머지 값으로 응답하고, 모두 실패했을 때만 오류로 본다
- 프론트: `fetchAddon(id, guard, {refresh})`(`api/client.ts`)와 응답 형태 검사(`api/guards.ts`), 엔드포인트별 공유 폴러 `createSharedPoll`과 그것에 붙는 훅 `usePolledData`(`refresh()` 포함, 결과는 같은 엔드포인트의 모든 보기가 받는다)

### 조작

상태를 바꾸는 일(컨테이너 시작·정지, 서비스 켜기·끄기)은 애드온 조작으로만 한다.

- manifest `actions`: `name`(소문자·숫자·`-`), `admin`(관리자만), `confirm`(실행 전 확인). `admin`은 애드온의 `requires.admin`과 따로 정한다(컨테이너 목록은 누구나 보고 조작은 관리자만)
- 구현 `Addon.Actions[name]`: `Timeout`(0이면 1분)과 `Run(ctx, ActionRequest{Admin, Target, RPC})`
  - `Target`은 브라우저가 보낸 값이다. OMV에 넘기기 전에 애드온이 형식과 존재를 검사하고, 잘못되면 `ErrInvalidTarget`
  - OMV 파라미터는 고정값과 검사한 대상으로 애드온이 만든다. 브라우저가 보낸 다른 값은 넘기지 않는다
  - 시작하지 않고 거절할 이유가 있으면 `*ActionError{Status, Code}`(예: 409 `config_dirty`)
  - `RPC`는 그 애드온이 선언한 읽기·쓰기를 부를 수 있다(`updatelastaccess=false`)
- OMV 백그라운드 작업: `execBgProc`·`callMethodBg`를 쓰는 메서드는 상태 파일 이름만 바로 돌려준다. `addons.WaitBg`가 `Exec.isRunning`(관리자 전용, 끝난 상태 파일을 지움)을 500ms마다 불러 끝날 때까지 기다린다. 실패는 OMV가 `Exec.isRunning`에서 예외로 알려 주므로 `ErrActionFailed`, 시간 초과는 `ErrActionTimeout`이다. 시간 초과나 백엔드 재시작 때 OMV 쪽 작업은 계속 돌고 상태 파일은 남는다
- API: `POST /api/addons/{id}/actions/{action}`, 본문 `{target}`(4KB 이하), 성공하면 204

| 상황 | 응답 |
|---|---|
| 애드온·조작 없음 | 404 |
| capability 없음, 조작의 `admin`인데 관리자가 아님 | 403 `admin_required` / `plugin_missing` (OMV에 보내지 않음) |
| 같은 (애드온, 대상)에 진행 중인 조작 | 409 `action_busy` |
| 잘못된 대상 | 400 `bad_request` |
| `ActionError` | 그 상태와 코드 |
| 백그라운드 작업 실패 | 502 `action_failed` |
| 시간 초과 | 504 `action_timeout` |
| OMV 401 | 대시보드 세션을 지우고 401 |
| 그 밖의 OMV 오류 | 502 `omv_error` |

- 브라우저 연결이 끊겨도 백엔드는 `Timeout`까지 기다린다. OMV 쪽 작업은 어차피 계속 돌므로, 끝날 때까지 진행 중 표시를 유지해 같은 조작이 겹치지 않게 한다
- 로그는 `user/addon/action/target/result`만 남긴다. OMV 오류 메시지는 별도 줄로 남기고 브라우저에 넘기지 않는다(trace는 클라이언트가 버린다)
- 프론트(`addons/common/`)
  - 상태: `actionState.ts`(대상별 running → settling → 해제 또는 error, 확인은 한 번에 하나). 성공한 대상은 새 현황 응답(`settle`)까지 잠가 둔다. `settle`에 결과 판정(`SettleCheck`)을 넘기면 결과 상태에 이른 대상만 풀고, 이르지 않아도 `SETTLE_TIMEOUT_MS`(15초)가 지나면 푼다
  - 실행: `useAddonActions(id, onDone)`. manifest의 `confirm`이면 확인을 열고, 아니면 바로 실행한다. 성공·실패 모두 `onDone`(보통 `refresh`)을 부른다
  - 화면: `RowActions.tsx`
    - 행 버튼(`RowActionButtons`)과 우클릭 메뉴(`actionMenuItems`) 모두 같은 확인 모달(`ActionConfirmDialog`)을 연다. 창 범위 `Modal`이라 그 창 본문만 막고, 취소에 포커스하며 Esc·배경 누르기는 취소다. 확인 중인 행 버튼은 눌린 모양으로 둔다
    - 실패는 행 아래(`RowActionPanel`)에 오류 코드별 문구로 보여 준다
  - 조작 버튼은 관리자에게만 보여 준다. 위젯에서는 조작하지 않는다(위젯은 끌어서 옮기는 요소라 잘못 누르기 쉽다)

## 보기: 창과 위젯

애드온은 창과 바탕화면 위젯 중 하나 이상의 보기를 갖는다. 위젯은 한눈에 보는 요약이고, 창은 자세한 화면이다. 위젯을 누르면 같은 애드온의 창을 연다(창 보기가 있을 때).

### 위젯은 창 시스템과 분리한다

- 위젯은 바탕화면 위의 별도 레이어에 그리드 칸 단위로 붙인다. 위젯끼리 겹치지 않으며 작업표시줄·z-order·타일링·최소화가 없다
- 창 저장소(`windows[]`)와 창 컴포넌트에 위젯용 분기를 넣지 않는다
- 공유는 아래층에서만 한다: provider, `/api/*`, `poller.ts`, 포인터 드래그 같은 공통 훅
- 검토 후 쓰지 않은 방식: 창을 "고정"하면 작업표시줄에서 빠지고 창 버튼이 숨겨져 위젯처럼 쓰는 방식
  - 작업표시줄 순서와 드래그 정렬, 등장·퇴장 애니메이션, z-order, 타일링·타일 그룹, 로그아웃 시 창 닫기 규칙마다 예외가 생긴다
  - 창은 자유 배치와 겹침을 전제로 하므로 그리드 배치·겹침 방지를 창 시스템 안에 넣기 어렵다
  - 창 화면을 작게 줄이면 위젯처럼 보이지 않아 결국 작은 화면을 따로 만들게 된다

### 위젯 시스템

#### 그리드와 화면 크기

- 칸은 80px, 칸 사이 8px, 데스크톱 영역 가장자리와 12px(`widgets/grid.ts`의 `CELL`, `GAP`, `PADDING`)
- 그리드는 데스크톱 영역(작업표시줄 제외)의 **오른쪽 위**를 기준으로 한다. 배치는 `{addon, right, top, cols, rows}`이고 `right`는 오른쪽 끝에서 위젯 오른쪽 끝까지의 칸 수다
  - 브라우저 너비가 바뀌어도 위젯은 오른쪽에 붙어 있고, 바탕화면 아이콘(왼쪽)과 떨어져 있다
  - 위젯끼리 겹치는지가 화면 크기와 무관하게 정해지므로 백엔드도 겹침을 검사할 수 있다
- **저장 배치와 화면 배치를 나눈다**(`widgets/layout.ts`)
  - 저장 배치: 사용자가 놓은 자리. 서로 겹치지 않는다
  - 화면 배치(`fitLayout`): 지금 그리드와 아이콘 영역에 맞춘 자리. 저장 자리에 그대로 놓을 수 있는 위젯을 먼저 놓고, 나머지는 저장 자리에서 가장 가까운 빈칸에 놓는다. 빈칸이 없으면 숨긴다. 저장 배치는 바꾸지 않으므로 화면을 다시 넓히면 원래 자리로 돌아간다
  - 사용자가 옮기거나 크기를 바꾸거나 추가한 위젯만 저장 배치를 바꾼다(`commitPlacement`). 그 자리가 화면이 좁아 밀려난 위젯의 저장 자리를 덮으면, 밀려난 위젯은 지금 보이는 자리로, 그 자리도 다른 저장 자리와 겹치면 그리드 바로 아래로 옮긴다

#### 바탕화면 아이콘과의 관계

- 바탕화면 아이콘은 사용자가 고른 바로가기다(`.claude/rules/frontend-shell.md`의 "바로가기와 모든 앱")
- 아이콘 목록(`.desktop-icons`)의 크기를 ResizeObserver로 재서, 조금이라도 걸치는 칸을 예약 칸으로 뺀다(`reservedCells`). 위젯은 예약 칸에 놓이지 않는다
- 바로가기 수가 바뀌거나(추가·제거, capability로 숨김) 화면 높이가 바뀌어 아이콘 열 수가 바뀌어도 다시 잰다
- 위젯은 바로가기 목록을 불러온 뒤에 그린다(`iconsReady`). 아이콘 영역을 모르는 채로 그리면 위젯이 아이콘 자리에 놓였다가 밀려난다

#### 편집 방식

- 편집 모드 없이 평소에 바로 다룬다
- 잠금: 설정의 화면 페이지에서 켜는 사용자 설정(화면 설정의 `widgetsLocked`, 관리자 기본값 없음)이다. 배치를 바꾸는 조작을 모두 막는다
  - 옮기기·크기 조절: 끌어도 움직이지 않고 놓아도 창을 열지 않으며, 크기 조절 핸들을 그리지 않는다
  - 추가·제거: 바탕화면 메뉴의 "위젯 추가 ▸"와 위젯 메뉴의 "위젯 제거"를 비활성으로 두고 "잠김"을 보여 준다
  - 끌지 않고 누르면 창을 여는 것과 위젯 메뉴의 "열기"는 그대로다
  - 프론트만 지키는 편의 설정이다. 백엔드는 잠겨 있어도 위젯 배치 저장을 거절하지 않는다
- 이동: 위젯 안쪽을 4px 넘게 끈다. 그 전에 놓으면 클릭이며, 창 보기가 있으면 창을 연다(키보드 Enter·Space도 같다)
- 크기 조절: 창과 같은 8방향 변·모서리 핸들(`.resize-handle`, 변 6px, 모서리 14px). 끄는 변만 움직이고 반대쪽 변은 그대로다. manifest의 최소·최대와 그리드 안으로 제한한다
- 끄는 동안 위젯은 포인터를 따라가고, 놓을 칸은 가이드(`WidgetPreview`)로 보여 준다. 가이드의 모양과 등장 비율은 타일링 가이드를 따르고, 칸이 바뀌면 같은 요소가 임계 감쇠 스프링으로 새 칸을 따라간다(FLIP). 칸마다 애니메이션을 새로 시작하면 속도가 끊겨 빠르게 끌 때 멈칫거리므로, 위치와 속도를 이어 가며 requestAnimationFrame으로 transform만 바꾸고, 끄는 동안 React 렌더 없이 핸들(`moveTo`)로 움직인다. 놓을 수 없는 자리(겹침, 예약 칸)에서는 가이드를 지우지 않고 숨겨 두었다가 다시 보일 때 마지막 자리에서 이어서 미끄러지며, 이때 놓으면 원래 자리로 돌아간다
- 끄는 동안에는 위젯 레이어가 창 위로 올라온다. 평소에는 창 아래에 있다
- 추가·제거는 컨텍스트 메뉴로 한다: 빈 바탕화면 → "위젯 추가 ▸"(이미 있는 위젯은 체크·비활성, 자리가 없으면 비활성과 "자리 없음"), 위젯 → "열기"·"위젯 제거". 터치는 길게 누르기(500ms)
- 애드온마다 위젯은 하나다(백엔드도 같은 애드온의 중복을 거부한다)

#### 배치 저장

- 백엔드 상태 디렉터리의 사용자별 JSON 파일(`internal/prefs`, `<state-dir>/users/<사용자 이름>.json`). 로그아웃해도 남고 기기 사이에 공유된다
  - 새 의존성을 피하려고 SQLite 대신 JSON 파일을 썼다. `prefs.Store` 인터페이스 뒤에 두었으므로, 지표 기록 같은 시계열 데이터가 필요해지면 그때 다시 검토한다
  - 사용자 이름은 영문자·숫자·`_`·`-` 밖의 바이트를 `%XX`로 바꿔 파일 이름으로 쓴다(경로 탈출 방지). 쓰기는 임시 파일 + fsync + rename. 읽을 수 없는 파일은 `.corrupt`로 옮겨 두고 빈 설정으로 시작한다
  - 상태 디렉터리가 없으면(`-state-dir` 미지정) 메모리에만 둔다
- `GET /api/preferences`: 한 번도 저장하지 않았으면 `widgets`가 없고, 프론트는 기본 배치(시스템 정보 위젯을 오른쪽 위에)를 쓴다. 모두 지웠으면 빈 목록이다. 바탕화면 바로가기(`shortcuts`)도 같은 규칙이다(기본 목록: 시스템 정보·파일·저장소)
- `PUT /api/preferences/widgets`: 위젯 배치 전체를 바꾼다. 다른 설정은 그대로 둔다
  - 검사(`Registry.ValidateWidgets`): 위젯 보기가 있는 등록된 애드온, 애드온마다 하나, 크기는 manifest 범위 안, 좌표는 0~200칸, 서로 겹치지 않음, 64개 이하. 본문 64KB 이하, `application/json`만
  - capability(역할·플러그인)는 검사하지 않는다. 역할이나 플러그인이 바뀌어도 배치를 잃지 않게 하고, 표시 여부는 프론트가 정한다(관리자 전용은 그리지 않고, 플러그인 없음은 위젯 안에 짧은 안내)
- `PUT /api/preferences/shortcuts`: 바로가기 목록 전체(`[{app}]`, 바탕화면 순서)를 바꾼다. 다른 설정은 그대로 둔다
  - 검사(`addons.ValidateShortcuts`)는 형식만 본다: 앱 id는 애드온 id와 같은 형식, 앱마다 하나, 64개 이하. 셸 앱(`files`, `terminal`, `settings`)과 링크 앱은 프론트에만 있으므로 레지스트리로 검사하지 않고, 모르는 id는 프론트가 불러올 때 뺀다. capability도 위젯과 같은 이유로 보지 않는다
  - 객체 목록으로 둔 것은 나중에 폴더·OMV 페이지 바로가기를 필드만 더해 담기 위해서다
- `PUT /api/preferences/appearance`: 화면 설정(`{theme, wallpaper, animations, taskbarAlign, widgetsLocked}`) 전체를 바꾼다. 빈 값은 관리자 기본 화면을 따른다. 바탕화면 이미지는 `…/wallpaper-image`로 따로 올리며, 응답의 `wallpaperImage`(리비전)는 저장하지 않고 응답할 때 붙인다
- `PUT /api/preferences/drawer`: "모든 앱" 화면 설정(`{sort}`) 전체를 바꾼다. `sort`는 `recommended`(추천순, 대시보드가 정한 순서)나 `name`(화면 언어의 이름순)만 받는다. 저장한 적 없거나 프론트가 모르는 값이면 추천순이다
- `PUT /api/preferences/system-info`: 시스템 정보 창 보기 설정(`{cpuScale, cpuGraph}`) 전체를 바꾼다. `cpuScale`은 `machine`(전체 대비)·`core`(코어당 100%), `cpuGraph`는 `overall`·`cores`만 받는다. 저장한 적 없거나 프론트가 모르는 값이면 `machine`·`overall`이다
- 설정 파일은 설정 하나를 바꿀 때 읽기부터 쓰기까지 잠그므로(`prefs` `update`), 위젯과 바로가기를 동시에 저장해도 서로를 지우지 않는다
- 프론트 저장소(`widgets/store.ts`, `shortcuts/store.ts`, "모든 앱" 설정, 시스템 정보 창 설정)는 같은 저장 큐(`api/saveQueue.ts`)로 저장 요청을 하나씩 보내고, 보내는 동안 바뀐 값은 마지막 것만 이어서 보낸다. 실패하면 로컬 값을 유지하고 다음 변경 때 다시 보낸다. 각 저장소는 따로 `GET /api/preferences`를 부르지만 동시에 부른 요청은 클라이언트가 하나로 합친다(`fetchPreferences`)

#### 주기 요청

- 탭이 보이지 않으면(`document.visibilityState`) 위젯과 창 모두 주기 요청을 멈추고, 다시 보이면 바로 한 번 가져온다(`addons/common/usePageVisible.ts`). 창·위젯 틀이 보기의 `active`에 넣으므로 애드온은 따로 처리하지 않는다
- 위젯은 늘 떠 있으므로 창보다 길게 갱신한다(시스템 정보: 창 1초, 위젯 5초)
- 예외: 시스템 정보 창은 창이 열려 있는 동안 탭을 숨기거나 창을 최소화해도 2초마다 계속 가져와 사용량 기록을 이어 간다(사용자 요청). 숨긴 탭의 메인 스레드 타이머는 브라우저가 늦추므로(크롬은 5분 뒤 1분에 한 번) Worker 타이머(`addons/common/workerTimer.ts`)를 쓰고, 보이지 않는 동안에는 다시 그리지 않는다(`recorder.ts`). 아래 "시스템 정보"
- 알려진 한계: 시스템 정보 위젯의 CPU 사용률(`/api/system`)은 백엔드가 직전 샘플과 비교해 계산하므로, 다른 탭·세션의 위젯이 함께 부르면 5초 평균이 아니라 직전 요청 이후의 평균이다. 창은 누적 카운터로 보기마다 계산하므로 이 한계가 없다

### 현황 내장 애드온

| id | 이름 | requires | provider (우선순위) | 위젯 / 창 갱신 |
|---|---|---|---|---|
| `system-info` | 시스템 정보 | 없음(프로세스 목록·절전 방식은 admin) | `proc`, `proc-processes`(admin), `omv-power`(admin) | 5초 / 1초(숨기면 2초) |
| `storage` | 디스크 용량 | 없음 | `proc` | 1분 / 15초 |
| `network` | 네트워크 | 없음 | `proc` | 5초 / 2초 |
| `temperature` | 온도 | 없음 | `hwmon`, `omv-cputemp`(cputemp 플러그인), `omv-smart`(admin, 캐시만) | 10초 / 5초 |
| `containers` | 컨테이너 | compose 플러그인 | `omv-compose` | 30초 / 15초 |
| `smart` | 디스크 건강 | admin | `omv-smart` | 10분(캐시 확인만) / 같음 + 새로고침 |

위젯 목록이 위젯에 다 들어가지 않으면 마지막 줄을 "외 N개"로 쓴다(`addons/common/widgetRows.ts`의 `widgetCapacity`·`fitList`, 줄 높이는 각 위젯의 CSS와 맞춘다). 크기는 브라우저 실측(ko·en)으로 정했다.

#### 디스크 용량 (`storage`)

- `/proc/mounts`에서 장치가 `/dev/`로 시작하는 것과 mergerfs·zfs 풀(zfs는 최상위 데이터셋만)을 고른다. overlay·tmpfs·squashfs·네트워크 파일시스템(statfs가 오래 걸릴 수 있음)은 뺀다
- 같은 장치가 여러 곳에 마운트되어 있으면(bind mount, 컨테이너) `/proc/mounts`에서 처음 나온 것을 쓴다. 원래 마운트가 bind mount보다 앞에 있다
- 용량은 statfs(주입 가능한 `StatfsFunc`), 라벨은 `/dev/disk/by-label` 심볼릭 링크로 얻는다. 둘 다 root가 필요 없다
- 이름: 라벨 → 루트(시스템) → OMV 마운트 경로(`/srv/dev-disk-by-…`)면 장치 이름 → 마운트 경로 끝. 사용률은 df와 같다(사용 / (사용 + 일반 사용자 남은 공간)), 80% 주의, 90% 위험. 위젯은 `/boot` 아래를 뺀다
- 준비 중 앱 `storage`를 이 애드온으로 바꿨다

#### 네트워크 (`network`)

- `/proc/net/dev` 누적 카운터와 `/sys/class/net/<if>/operstate`·`speed`. `lo`와 `veth*`는 뺀다. `device` 링크가 있으면 물리 인터페이스이고, 나머지(브리지, docker0, VPN)는 `virtual`로 창에서만 접어 둔다. MAC·IP는 보여 주지 않는다
- 속도는 **프론트가 계산**한다. 백엔드는 누적 카운터와 단조 시각(`sampledAtMs`, 백엔드 프로세스 시작 기준)을 주고, 보기마다 직전 응답과의 차이로 구한다(`computeRates`). 그래서 창과 위젯이 서로의 샘플에 영향을 주지 않는다(시스템 정보 위젯 CPU 사용률의 한계를 피함). 첫 값은 두 번째 응답부터 나오고, 간격이 30초를 넘거나 카운터가 줄면 다음 샘플을 기다린다

#### 온도 (`temperature`)

- `hwmon`: `/sys/class/hwmon/*`의 `name`, `tempN_input`·`_label`·`_max`·`_crit`. 칩 이름으로 CPU(coretemp, k10temp 등)·디스크(nvme, drivetemp)·그 밖으로 나눈다. 경고·위험 온도는 0 초과 200 미만만 쓴다(NVMe 센서가 "없음"을 큰 값으로 준다)
- `omv-cputemp`: hwmon에 CPU 센서가 없을 때만 `CpuTemp.get`을 부른다(같은 센서 중복 방지). 스크립트를 설정하지 않으면 0을 주므로 버린다
- `omv-smart`(관리자): SMART 캐시에 값이 있을 때만 디스크 온도를 더한다. **SMART 조회를 일으키지 않는다**. hwmon이 이미 보여 주는 디스크(nvme0 ↔ nvme0n1, drivetemp의 블록 장치)는 뺀다. 확인 시각(`checkedAt`)을 함께 준다
- 위젯은 CPU·디스크 대표값(CPU 패키지, NVMe Composite, 없으면 가장 뜨거운 센서). 칩의 경고 온도(없으면 CPU 85, 디스크 60, 그 밖 80) 이상 위험, 10도 아래부터 주의

#### 컨테이너 (`containers`)

- `Compose.getContainers`(모든 사용자). 첫 행은 런타임 행이며 번역되지 않는 `name`으로 Docker·Podman만 판단하고, 번역된 `status`는 표시에만 쓴다
- 나머지 행의 상태 문구(docker CLI, 영어)에서 상태(running, paused, restarting, exited, created, dead, removing)와 헬스체크 결과를 읽는다. 원문도 함께 준다
- 위젯과 창 모두 문제 있는 컨테이너(dead, restarting, unhealthy, 헬스체크 대기)를 먼저 보여 준다. Podman이면 창에 미검증 안내를 띄운다
- 조작: `start`, `stop`(확인), `restart`(확인), 모두 관리자. `Compose.doContainerCommand`가 파라미터를 검사하지 않고 셸 명령을 만들므로(`data-sources.md`의 "조작에 쓴 출처") 명령은 고정값, 이름은 docker 형식(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)과 지금 `getContainers` 목록에 있는지를 확인한 뒤 보낸다. 시간 제한은 2분(`docker stop`의 종료 유예 시간)
  - 창: 실행 중·일시 정지·다시 시작 중이면 정지·재시작, 정지·생성됨·dead면 시작. 삭제 중·알 수 없음은 조작 없음(`containerActions`). 버튼 칸은 두 개 너비로 고정해 상태 열을 맞춘다
  - 위젯에는 조작이 없다

#### 디스크 건강 (`smart`)

- `Smart.getList`(관리자 전용)는 디스크마다 smartctl을 `--nocheck` 없이 실행하므로 대기(스핀다운) 중인 디스크를 깨운다(OMV `smarttrait.inc` → `SmartInformation`). 그래서 자동 주기 조회를 하지 않는다
- 백엔드 `smart.Cache`가 결과를 모든 세션이 함께 쓰도록 보관하고, 다음 경우에만 OMV에 묻는다
  - 보관한 결과가 없다
  - 결과가 가장 최근 기준 시각 이전이다. 기준 시각은 `-smart-refresh-at`(`MARU_SMART_REFRESH_AT`, 기본 `00:00`, 서버 로컬 시간). 매일 첫 접속 때 건강 상태를 한 번 확인하는 용도다
  - 새로고침(`?refresh=1`)이고 마지막으로 가져온 지 1분이 지났다. 응답의 `refreshInSeconds`로 남은 시간을 알려 주고, 창은 그동안 버튼을 막는다
- 동시 요청은 OMV 호출 하나로 합친다. 실패하면 1분 동안 다시 부르지 않고, 보관한 결과가 있으면 그것을 준다
- 파라미터는 OMV workbench의 SMART 대시보드 위젯과 같은 `{start: 0, limit: -1}`이다
- `overallstatus`는 workbench의 색 구분을 따라 good(GOOD), warning(BAD_SECTOR), failing(BAD_ATTRIBUTE_NOW, BAD_ATTRIBUTE_IN_THE_PAST, BAD_SECTOR_MANY), unknown(BAD_STATUS 등)으로 줄이고 원래 값도 준다. 시리얼 번호·WWN은 브라우저에 넘기지 않는다
- 위젯은 10분마다 백엔드에 묻지만 백엔드는 기준 시각을 넘긴 경우에만 OMV를 부른다

### 조작이 있는 내장 애드온

| id | 이름 | requires | provider | 조작 | 위젯 / 창 갱신 |
|---|---|---|---|---|---|
| `containers` | 컨테이너 | compose 플러그인 | `omv-compose` | `start`, `stop`(확인), `restart`(확인), 관리자 | 30초 / 15초 |
| `services` | 서비스 | admin | `omv-services` | `enable`, `disable`(확인) | 30초 / 10초 |
| `system-info` | 시스템 정보 | 없음 | `omv-power`(admin) | `shutdown`, `reboot`, `standby`(모두 확인), 관리자. 프로필 메뉴의 전원 모달이 부른다 | 5초 / 1초(숨기면 2초) |

#### 서비스 (`services`)

- 상태: `Services.getStatus`(관리자). `IServiceStatus`를 구현한 OMV 모듈마다 `{name, title, enabled, running}`이다. `name`은 대개 모듈 이름(`ssh`, `samba`, `nfs`, `rsyncd`)이지만 compose 플러그인처럼 다른 값(`Docker`)을 쓰는 모듈도 있다. `title`은 OMV가 번역한 문구라 표시에만 쓴다. 참거짓을 숫자·문자열로 주는 플러그인이 있을 수 있어 느슨하게 읽는다
- 상태 점과 문구: 켜짐+실행 ok "실행 중", 켜짐+정지 위험 "아직 정지 중", 꺼짐+실행 주의 "아직 실행 중"(설정과 실제가 다름), 꺼짐 idle "꺼짐". 위젯과 창 모두 문제 있는 것부터 보여 준다
- OMV는 설정을 먼저 바꾸고 실제 실행은 늦게 따라온다. 창은 켜기·끄기가 끝난 뒤에도 설정과 실제 실행이 모두 바뀔 때까지(`serviceReached`, 최대 15초) 진행 중으로 두고, 그동안 그 행은 조작 전 상태로 보여 준다(`holdServices`). 중간 상태가 문제로 보이거나 행이 정렬 순서를 따라 튀지 않게 하기 위해서다
- 켜기·끄기는 어댑터 표(`adapters`)에 있는 서비스만 한다: SSH(`SSH.get`/`set`), SMB/CIFS(`SMB.getSettings`/`setSettings`), NFS, Rsync 서버(`Rsyncd`). 응답의 `toggleable`로 알려 주고, 창은 나머지 서비스에 "OMV에서 설정" 안내를 둔다
- 흐름(`toggle`): `Config.isDirty`가 참이면 409 `config_dirty`로 거절(사용자가 workbench에서 적용하지 않은 변경을 대신 배포하지 않기 위해) → get, 이미 원하는 값이면 성공 → get 결과에서 set 스키마 속성만 골라 `enable`만 바꿔 set → `Config.applyChangesBg({modules: [], force: false})` → `WaitBg`. 시간 제한 3분
  - set 스키마(`datamodels/rpc.<svc>.json`)는 속성이 모두 필수다. get은 다른 속성도 줄 수 있으므로(NFS의 `shares`) 그대로 넘기지 않는다
  - 이 백엔드의 켜기·끄기는 하나씩 실행한다(`addons.ApplyChange`의 적용 잠금, 프로필 저장과 함께 쓴다). 적용이 끝나기 전에는 OMV가 적용 대기로 표시하므로, 겹치면 뒤의 요청이 앞 요청의 변경 때문에 `config_dirty`로 거절되기 때문이다. 뒤의 요청은 기다리며, 기다리는 시간도 시간 제한에 들어간다
  - isDirty와 set 사이에 다른 곳에서 만든 변경이 함께 배포될 수 있는 경쟁 조건은 받아들인다
  - set이 성공하고 적용이 실패하면 설정은 바뀐 채 적용 대기로 남는다(OMV workbench에 표시됨). `action_failed`로 알린다
- 끄기 확인 문구는 서비스마다 무엇이 끊기는지 알려 준다(SSH 세션, SMB·NFS 클라이언트, rsync 백업)

### 시스템 정보 (`system-info`)

- 시스템 정보 창이 첫 내장 애드온이다. 위젯은 `/api/system`(`Reader.System`, CPU는 백엔드가 직전 샘플과 비교), 창은 `GET /api/addons/system-info`(`Reader.Stats`)를 쓴다
- CPU 사용률·RAM·가동 시간 요약이 첫 위젯(`SystemInfoWidget`, 기본 3×2, 2×2~4×3)이다. 2칸 너비에서는 메모리 용량을 빼고 가동 시간을 두 줄로 나눈다
- 창 데이터는 누적 카운터와 단조 시각(`sampledAtMs`)이다: `/proc/stat`의 전체·코어별(`cpuN`, 꺼진 코어는 빠지므로 id로 맞춤) tick(전체, idle + iowait, 커널 = system + irq + softirq. steal은 하이퍼바이저 시간이라 커널에 넣지 않는다), `/proc/meminfo`, `/proc/uptime`, `/proc/loadavg`(부하 평균, 실행 중·전체 스레드 수), 물리 인터페이스 합계(네트워크 애드온의 reader, `virtual` 제외), 물리 디스크 합계(`/proc/diskstats` 중 `/sys/block/<이름>/device` 링크가 있는 장치. 파티션·loop·md·dm·zram은 빠진다). 네트워크·디스크·부하 평균을 읽지 못하면 그 값만 뺀다
- 메모리 구성(`memoryDetail`)은 htop 방식이다: 공유 = Shmem, 캐시 = Cached + SReclaimable − Shmem(`Cached`에 Shmem이 들어 있다), 버퍼 = Buffers, 여유 = MemFree, 압축 = zswap 풀(`Zswap`, 압축 뒤 크기) + zram 장치(`/sys/block/zram*/mm_stat`의 mem_used_total), 앱 = 나머지. 합이 늘 MemTotal이 되게 여유·버퍼·공유·캐시·압축 순으로 남은 양 안에서만 뗀다(`MemInfo.Detail`). `MemTotal`·`MemAvailable` 말고는 줄이 없으면 0이다(옛 커널의 `Zswap`). ZFS ARC는 `Cached`에 잡히지 않아 앱으로 보인다(사용 중 = MemTotal − MemAvailable도 같은 한계)
- 사용률·속도는 창이 직전 응답과의 차이로 구한다(`stats.ts`의 `appendSample`). 규칙은 네트워크의 `computeRates`와 같다: 간격이 30초를 넘거나 백엔드가 다시 시작하면 기록을 새로 시작하고, 카운터가 줄면 그 점만 비운다. 첫 값이 주기만큼 늦지 않게 공유 폴러의 `warmupMs`(250ms)로 두 번째 요청을 빨리 보낸다
- 갱신(`useSystemStats.ts`): 보이는 동안 1초, 탭을 숨기거나 창을 최소화하면 2초마다 가져온다. 기록은 창이 열려 있는 동안 계속 쌓아 돌아왔을 때 그사이 그래프가 이어지고, 창을 닫으면 버린다(백엔드 상태 없음)
  - 기록기(`addons/common/recorder.ts`)가 창이 열려 있는 동안 공유 폴러에 2초 주기로 붙어 `reduce`로 기록을 쌓고, 보기는 보이는 동안만 기록기를 구독해 그린다(`useSyncExternalStore`). 보이는 동안은 1초 주기로 한 번 더 붙어 공유 폴러가 짧은 주기를 쓴다
  - 타이머는 Worker 타이머(`workerTimer.ts`, `timer.worker.ts`)다. 숨긴 탭의 메인 스레드 타이머는 브라우저가 늦추지만 Worker 안의 타이머는 늦추지 않는다. Worker를 만들 수 없거나 불러오지 못하면(node 테스트, 업그레이드 뒤 옛 탭이 지워진 파일을 요청) `setTimeout`으로 대신한다
  - 한계: 사파리처럼 숨긴 페이지를 멈추는 브라우저, 크롬의 탭 정리(메모리 절약으로 탭을 버림)에서는 기록이 끊긴다. 컴퓨터가 잠들었다 깨어 간격이 30초를 넘으면 기록을 새로 시작한다. 창을 처음 열 때는 빈 그래프에서 시작한다
  - 프로세스 목록은 프로세스 페이지가 보일 때만 `?processes=1`로 받는다(`setProcessesWanted`). 그 페이지를 열면 주기를 기다리지 않고 바로 한 번 가져오고(`refresh`), CPU 값은 그다음 응답부터 나온다. 성능 페이지의 프로세스·스레드 수는 목록 없이 백엔드가 센 값(`tasks`, `/proc`의 PID 디렉터리 수와 `/proc/loadavg`)이다
- 창 모양은 Windows 11 작업 관리자를 따른다. 관리자에게는 사이드바([성능] [프로세스], 설정 앱과 같은 틀인 `shell/SidebarLayout`, 720px보다 좁으면 덮어 열기)를, 그 밖의 사용자에게는 사이드바 없이 성능 페이지만 보인다. 처음 페이지는 성능이다. 데이터 훅과 고른 자원은 페이지의 부모(`SystemInfoWindow`)에 있어 페이지를 바꿔도 기록과 프로세스 CPU 값이 이어진다
- 성능 페이지(`PerformancePanel.tsx`): 왼쪽 자원 목록(CPU, 메모리, 디스크, 네트워크, 부하 평균. 세로 tablist, 작은 그래프와 지금 값)에서 고른 자원을 오른쪽에 큰 그래프와 세부 값(CPU는 사용률·커널·코어 수·프로세스·스레드 수·가동 시간. 메모리는 사용 중·사용 가능·전체·스왑, 디스크·네트워크는 두 속도, 부하 평균은 1·5·15분·실행 중·코어 수)으로 보여 준다. 그래프는 최근 60초다
  - 메모리는 사용량 그래프 아래에 지금 메모리 구성(앱·공유·압축·버퍼·캐시·여유)을 가로 막대로 보인다(`MemoryComposition.tsx`, 조각 계산은 `memory.ts`). 조각 사이는 창 배경색 2px 틈이고 0인 항목은 빠진다. 항목별 값 목록은 두지 않고(막대 아래가 세부 값과 겹쳐 길어진다) 툴팁으로 이름·크기·비율을 보인다: 조각에 포인터를 올리거나, 막대에 포커스를 두고 좌우 화살표로 고른다(그래프의 십자선과 같은 방식, Esc로 닫음). 막대의 접근성 이름에 모든 항목을 담는다
  - 부하 평균은 1·5·15분 선 세 개다. 누적 값이 아니라 첫 응답부터 지금 값을 보인다. y축은 코어 수(모든 코어가 바쁜 부하)이고 부하가 넘치면 올림한 정수로 늘어난다(`loadAxisMax`)
  - 페이지가 560px보다 좁으면(컨테이너 쿼리) 자원 목록을 그래프 위 한 줄로 바꾸고 작은 그래프를 뺀다. 항목은 내용 너비보다 줄지 않고 남는 너비를 나눠 가지며 모자라면 가로로 스크롤한다(440px 창에서는 스크롤 없이 들어간다). 이름은 줄을 바꾸지 않고, 부하 평균은 1분 값만 보인다
  - 그래프는 의존성 없는 SVG다(`UsageChart.tsx`, 좌표는 `chart.ts`). 십자선과 모든 계열의 값을 담은 툴팁(포인터, 키보드 좌우 화살표)이 있다. 두 계열 그래프의 범례는 아래 세부 값의 계열 키가 맡는다. 한 그래프에 축은 하나다(네트워크와 디스크는 자원을 나눈다). %는 0~100 고정, 속도 축은 1·2·5 × 1024ⁿ(`niceMax`)이다. 자원 목록의 작은 그래프는 꾸밈이라(`decorative`) 툴팁·접근성 이름 없이 행이 포인터를 받는다
  - 계열 색은 dataviz 기준 팔레트 1~5번을 그 순서대로 창 배경(light·dark)에 맞춰 인접 쌍으로 검사한 값이다(`system-info.css`의 `--sysinfo-series-*`, 요소는 `.s1`~`.s5`로 `--sysinfo-color`를 받는다). 부하 평균의 세 선(1~3번)은 모든 쌍으로도 통과한다. 밝은 배경에서 3~5번은 대비가 3:1보다 낮다. 부하 평균은 계열 키가 붙은 세부 값이 라벨이고, 메모리 구성 막대는 툴팁과 접근성 이름이 이름을 알린다. 글자에는 계열색을 쓰지 않는다
  - CPU 그래프 영역의 우클릭 메뉴(Shift+F10, 길게 누르기)로 전체 사용률과 코어별 작은 그래프(작업 관리자의 논리 프로세서 보기)를 바꾼다. 코어별 격자는 그래프 영역을 채우고 정사각형에 가깝게(4코어 2×2) 놓이며, 격자가 포커스를 받아 키보드로도 메뉴를 연다
  - 같은 메뉴의 "커널 시간 표시"(기본 켜짐)를 켜면 전체·코어별 그래프에 커널 시간을 사용률과 같은 색의 진한 면으로 겹친다(작업 관리자 방식, `UsageChart`의 `kind: 'part'` 계열). 세부 값의 커널 %는 끄더라도 늘 보인다
- 프로세스(관리자): provider `proc-processes`(`Requires{Admin}`)가 `/proc/<pid>/stat`(comm은 마지막 `)`에서 자르고, 커널 스레드는 flags의 `PF_KTHREAD`로 판단. 좀비도 명령줄이 비므로 명령줄로 판단하지 않는다), `status`(실제 uid, VmRSS), `cmdline`(512바이트에서 글자 경계로 자름)을 읽는다. 사용자 이름은 `os/user`로 찾고 모르면 uid다. 읽는 사이 끝난 프로세스는 건너뛰고, 목록을 읽지 못하면 `failedSources`로 알린다
  - 일반 사용자 응답에는 넣지 않는다. 명령줄에 비밀이 있을 수 있고, OMV도 `System.getTopInfo`를 관리자 전용으로 둔다
  - CPU 사용률은 프로세스 tick 차이 / 모든 코어 tick 차이(전체 대비 0~100%)이고, 표의 우클릭 메뉴("CPU 사용률 기준 ▸")에서 코어당 100%(top 방식, × 코어 수)로 바꾼다. 성능 페이지의 CPU는 늘 0~100%다. PID 재사용은 시작 시각(`startTicks`)으로 구분한다
  - 검색(이름·명령줄·사용자·PID), 커널 스레드 표시(기본 숨김), 열 정렬(기본 CPU 내림차순, 값이 없는 행은 뒤)은 창 상태라 저장하지 않는다. 좁은 창에서는 상태·사용자 열을 숨긴다(컨테이너 쿼리)
- 두 우클릭 메뉴의 설정(`cpuGraph`, `cpuKernel`, `cpuScale`)은 사용자별로 저장한다. `cpuKernel`은 저장한 적 없으면(옛 설정 파일, 옛 탭의 저장) 켜짐이다(`PUT /api/preferences/system-info`, `sysinfoPrefsStore.ts`). 창이 열려 있는 동안 불러오고, 불러오기 전이나 실패하면 기본값을 보이고 메뉴 항목을 비활성으로 둔다
- 창 기본 크기는 사이드바·자원 목록·그래프가 나란히 넉넉하게 들어가는 960×600(실측), 최소 440×360이다(사이드바는 덮어 열고, 자원 목록은 한 줄, 세부 값은 스크롤)
- 전원 조작(관리자, `power.go`): 프로필 메뉴의 "전원" 모달이 쓴다(`.claude/rules/frontend-shell.md`의 "작업표시줄 트레이와 프로필 메뉴"). 조작 `shutdown`·`reboot`·`standby`는 대상이 없어 빈 대상만 받고, `System.shutdown`·`reboot`·`standby`를 `{delay: 2}`로 부른다. delay가 있으면 OMV가 fork해 바로 응답하므로 204가 브라우저에 닿은 뒤 시스템이 내려간다. 조작 키(`system-info` + 빈 대상)가 같아 전원 조작은 한 번에 하나다
  - 절전(`standby`)은 OMV 전원 관리의 대기 방식을 따르고, 지원하지 않는 방식이면 OMV가 전원을 끈다(`data-sources.md`의 "조작에 쓴 출처"). 그래서 모달을 열 때 `?power=1`(provider `omv-power`, `PowerMgmt.get`의 `standbymode`와 `PowerMgmt.enumerateStandbyModes`)로 받아 쓸 수 있는지 정한다. 읽지 못하면 `failedSources`로 알린다
- 프로세스 종료 같은 조작은 `.claude/docs/roadmap.md`에 있다

## 내장 애드온

### 위치와 등록

- 백엔드
  - `backend/internal/addons`: 계약(`Addon`, `Provider`, `Registry`, `Classify`, `WritesEnabled`), capability 감지·계산, TS 생성
  - `backend/internal/addons/<id>/`: manifest와 provider, 그리고 필요하면 API 핸들러. 계약 패키지를 import한다
  - `backend/internal/addons/builtin`: 정적 목록(`builtin.New`). 계약 패키지와 각 애드온이 서로 import하지 않게 따로 둔다
- 프론트엔드
  - `frontend/src/addons/<id>/`: 보기 컴포넌트(창, 위젯)와 데이터 훅
  - `frontend/src/addons/registry.ts`: 애드온 id별 보기 컴포넌트(`ADDON_VIEWS`). manifest가 선언한 보기마다 `Window`·`Widget` 컴포넌트가 있어야 한다(tsc가 검사)
    - 보기는 `lazyView`(`React.lazy`)로 등록해 보기마다 청크가 나뉜다. 바탕화면에 놓거나 연 애드온의 코드만 내려받고, 진입 청크는 애드온 수와 함께 커지지 않는다. 그리는 쪽(`Window`의 `WindowBody`, `WidgetFrame`)은 `shell/ViewBoundary`로 감싼다
  - `frontend/src/widgets/`: 위젯 그리드·배치 계산(`grid.ts`, `layout.ts`), 위젯 목록(`catalog.ts`), 저장 배치 저장소(`store.ts`). 창의 `src/windows/`와 서로 import하지 않는다
  - `frontend/src/addons/common/`: 애드온이 함께 쓰는 `poller.ts`, `format.ts`
  - `windows/apps.ts`는 창 보기가 있는 애드온을 manifest에서 만들고 준비 중 앱과 합친다. 준비 중 앱과 같은 id의 애드온을 만들면 준비 중 앱을 지운다
- 별도 패키지, 런타임 로딩, Go `plugin` 패키지는 쓰지 않는다
  - `lazyView`의 지연 로딩은 이 정적 목록 안의 일이다. 목록과 `AddonId`는 컴파일 타임에 정해지고, 빌드에 들어간 보기 코드를 내려받는 시점만 늦춘다

### manifest 원본과 id 일치 검사

- Go 구조체가 원본이다. `cmd/genaddons`가 `frontend/src/addons/manifests.gen.ts`(`MANIFESTS`, `AddonId`)를 `as const`로 생성하고, `make types`가 tygo와 함께 실행한다
  - JSON 파일 공유는 쓰지 않았다. `go:embed`는 패키지 밖 파일을 읽을 수 없고, Vite `server.fs.allow`에 backend 경로를 열어야 하며, JSON import는 리터럴 타입이 넓어져 `AddonId`를 얻으려면 결국 생성이 필요하다
- 백엔드·프론트 id 일치는 두 겹으로 검사한다
  1. Go 골든 테스트(`builtin`): 커밋된 `manifests.gen.ts`가 생성 결과와 같은지. 실패하면 `make types`를 실행한다. 테스트 캐시는 backend 모듈 밖 파일 변경을 추적하지 않으므로 `make test`는 Go 테스트를 `-count=1`로 실행한다
  2. tsc: `ADDON_VIEWS`는 애드온 id마다 manifest의 보기에 맞는 컴포넌트를 요구하므로, 보기가 빠지거나 없는 id가 있으면 오류가 난다. `title`·`icon`은 `AddonManifest` 타입으로 번역 키·아이콘 이름인지 검사한다

### 새 애드온 추가 체크리스트

1. 데이터 출처를 정하고 [data-sources.md](data-sources.md)에 추가한다. root가 필요한 정보는 OMV RPC로만 가져온다
2. RPC 메서드 이름과 역할은 추측하지 않는다. `/usr/share/openmediavault/engined/rpc/*.inc`에서 확인한다
3. manifest의 `requires`와 `rpc.read`를 채운다. `rpc.write`는 조작(`Addon.Actions`)이 부를 메서드만 채운다
   - 새 플러그인이 필요하면 `knownPlugins`에 패키지와 `.inc` 파일을 추가한다(`dpkg -S`로 확인)
4. provider를 만든다
   - 입력부(`fs.FS`, RPC 클라이언트)를 주입받게 한다
   - `testdata/`에 합성한 픽스처로 테스트한다. 식별 정보(시리얼, MAC, IP, 호스트 이름, 사용자 이름)는 가짜 값으로 바꾼다
5. `Addon.Fetch`로 응답을 만들고(OMV는 `Request.RPC`로만), 응답 구조체는 `api.AddonSources`를 품게 해 `internal/api`에 추가한다. 입력은 `builtin.Deps`·`HostDeps`에 두고 `builtin.New`에 등록한 뒤 `make types`를 실행한다(`types.gen.ts`, `manifests.gen.ts`). OMV RPC를 쓰면 `omvfake`에 합성 응답을 추가한다
6. 프론트 보기 컴포넌트(창, 위젯)를 만들어 `addons/registry.ts`에 `lazyView`로 등록한다(정적 import 금지, 진입 청크에 들어간다). 같은 id의 준비 중 앱이 있으면 지운다
   - capability가 없을 때의 안내는 창·위젯 틀(`AddonUnavailable`, 위젯은 `compact`)이 처리한다. 보기 컴포넌트는 "연결 끊김" 상태를 처리한다
   - 주기 요청은 `poller.ts` 방식을 따르고 보기의 `active`가 false면 멈춘다. 위젯은 로그인한 동안 계속 떠 있으므로 갱신 주기를 창보다 길게 잡는다
   - 위젯 보기는 `size`(칸)를 받는다. manifest의 최소 크기부터 최대 크기까지, 한국어·영어 모두 내용이 넘치지 않는지 브라우저에서 잰다
   - 목록형 창 보기의 최상위는 `.addon-window`(최대 폭 가운데 열)로 둔다. 창을 꽉 채우는 앱형 창만 쓰지 않는다(`.claude/rules/frontend-shell.md`의 "창 내용 폭")
   - 공통 클래스(`addons/common/common.css`)로 부족한 스타일만 `addons/<id>/<id>.css`에 두고 `src/index.css`의 애드온 import 목록에 알파벳순으로 넣는다(`.claude/rules/frontend.md`의 "CSS 배치")
7. 아이콘은 리졸버 매핑 테이블에 추가한다
8. `make check`, `make build`

## 향후 고려 (구현하지 않음)

공개 후 필요해지면 다시 검토한다. 지금 설계는 이 둘을 막지 않는 정도만 신경 쓴다.

### 선언형 애드온

- YAML 같은 데이터만으로 정의한다. "이 RPC를 호출해 표·게이지·키-값으로 보여 준다" 정도다. OMV workbench의 `dashboard.d/*.yaml` 위젯과 같은 방식이다
- 실행하는 코드가 없으므로 비교적 안전하다. RPC 허용 목록과 쓰기 제한도 manifest 검사로 그대로 강제할 수 있다
- tempmon, zfs, nut처럼 RPC 하나를 표로 보여 주면 되는 플러그인 대부분이 여기에 해당한다

### 코드 애드온

- 이 대시보드는 OMV와 같은 출처로 서빙된다. 그래서 서드파티 JS를 그냥 실행하면 OMV workbench와 같은 출처 권한을 갖는다
  - 브라우저에 OMV 로그인 쿠키가 있으면 `rpc.php`를 직접 호출할 수 있다. 백엔드 중계와 허용 목록을 모두 우회하게 된다
  - workbench의 sessionStorage도 읽을 수 있다
- 따라서 서드파티 프론트 코드는 반드시 `sandbox="allow-scripts"` iframe(`allow-same-origin` 없이, opaque origin)에 넣는다. 백엔드와는 postMessage SDK로만 통신하게 한다
- 백엔드 확장은 Go `plugin`을 쓰지 않는다. Go 버전과 의존성 버전이 정확히 같아야 하고 언로드도 안 된다. 대신 별도 프로세스(유닉스 소켓 HTTP)를 백엔드가 중계하는 방식으로 한다

### 공개 시 라이선스

- 아이콘 테마 Colloid는 GPL-3.0이다. 빌드 결과에 함께 들어가므로 프로젝트와 배포물의 라이선스를 정할 때 이 점을 고려한다
