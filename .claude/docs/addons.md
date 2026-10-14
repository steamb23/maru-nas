# 애드온 구조

기능(시스템 정보, 저장소, 온도, 네트워크, 컨테이너 등)을 같은 계약으로 만들기 위한 설계다. 각 기능은 창과 바탕화면 위젯 중 하나 이상의 보기로 보여 준다. 3단계에서 도입했고(3-1 애드온 계약, 3-2 위젯 시스템, 3-3 위젯 세부 구현), 4단계에서 상태를 바꾸는 조작을 더했다(4-1 조작 계약, 4-2 컨테이너 조작, 4-3 OMV 서비스). 항목별 데이터 출처는 [data-sources.md](data-sources.md)를 본다.

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
- 세션 역할: 로그인 응답의 `permissions.role`에서 얻은 admin 여부
- `/api/capabilities`는 두 정보를 합쳐 애드온마다 다음 값을 돌려준다. 함께 `plugins`(감지한 알려진 플러그인 목록)도 준다
  - `available`: manifest의 `requires`를 만족하고, 쓸 수 있는 provider가 하나 이상 있다
  - `reason`: `admin_required`, `plugin_missing`. 관리자 조건을 먼저 본다(일반 사용자에게 플러그인 설치 안내는 쓸모없다)
  - `sources`: 쓸 수 있는 provider 이름 목록(우선순위 순서)
- 프론트(`src/addons/capabilities.ts`)는 로그인한 동안 5분마다 가져온다. 관리자 전용은 아이콘을 숨기고, 플러그인 없음은 아이콘을 둔 채 창 안에 안내한다(`AddonUnavailable`). 아직 받지 못했거나 요청이 실패하면 모두 그린다. 실제 차단은 백엔드가 한다
- 실제 호출이 OMV에서 `RPC service '...' not found.`(HTTP 404)로 실패하면 provider를 쓸 수 없는 상태로 바꾼다. 이 경로는 마지막 안전장치일 뿐, 감지의 기본 수단이 아니다

### RPC 허용 목록과 단계 규칙

- 백엔드의 OMV RPC 호출(`/api/rpc` 중계, provider의 내부 호출, 조작)은 등록된 manifest의 `rpc.read`·`rpc.write`에 있는 메서드만 통과시킨다(`Registry.Classify`, 대소문자까지 정확히 일치)
- 쓰기 RPC 허용 여부는 백엔드 한 곳(상수 `addons.WritesEnabled`)에서 정한다. 3단계에서는 꺼 두었고 4단계에서 켰다
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
- 3-3의 내장 애드온은 이 중계를 쓰지 않고 provider 내부 호출(아래 "애드온 데이터 API")로 OMV를 부른다. 중계는 나중의 선언형 애드온 등을 위해 남겨 둔다
- 통과 경로(허용된 메서드가 실제 OMV까지 가는 경우)는 Go 테스트(omvfake)로만 검증했다

### 애드온 데이터 API (3-3단계에서 정함)

내장 애드온의 현황 데이터는 브라우저가 OMV RPC를 직접 조합하지 않고 백엔드 provider가 만든다. 출처 합치기·중복 제거(온도), 세션 사이에 공유하는 캐시(SMART)가 백엔드에 있어야 하기 때문이다.

- `addons.Addon.Fetch(ctx, Request)`가 응답을 만든다. 없으면 `GET /api/addons/{id}`는 404다. 시스템 정보는 3-1 이전부터 있던 `/api/system`을 그대로 쓴다
- `Request`: `Admin`, `Sources`(capability로 계산한 쓸 수 있는 provider, 우선순위 순서. `Has(name)`), `Refresh`(`?refresh=1`, 캐시를 쓰는 애드온만 본다), `RPC`
- `RPC`(`addons.RPCCaller`)는 서버가 요청마다 사용자 세션으로 만든다
  - **호출한 애드온 자신이** manifest `rpc.read`에 선언한 메서드만 부른다(`Registry.Classify`로 확인, 다른 애드온이 선언한 메서드는 거부). 쓰기는 부를 수 없다(조작에서만)
  - 항상 `updatelastaccess=false`로 부른다. 위젯 주기 요청이 OMV 세션을 연장하면 안 된다
  - OMV 401이면 provider가 오류를 삼켜도 대시보드 세션을 지우고 401로 응답한다. 404(RPC 서비스 없음)면 플러그인 감지를 다시 하게 한다
- 서버(`internal/server/addondata.go`)는 capability를 먼저 확인한다(403 `admin_required`/`plugin_missing`, OMV에 보내지 않음). `Fetch`가 실패하면 OMV 호출 실패는 502 `omv_error`, 그 밖은 500
- 응답은 모두 `api.AddonSources`(`sources`: 값을 보탠 provider, `failedSources`: 쓸 수 있었지만 실패한 provider)를 품는다. 일부 출처만 실패하면 나머지 값으로 응답하고, 모두 실패했을 때만 오류로 본다
- 프론트: `fetchAddon(id, guard, {refresh})`(`api/client.ts`)와 응답 형태 검사(`api/guards.ts`), 공통 폴링 훅 `usePolledData`(`refresh()` 포함)

### 조작 (4-1단계에서 정함)

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

### 위젯 시스템 (3-2단계에서 정함)

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

- 아이콘 목록(`.desktop-icons`)의 크기를 ResizeObserver로 재서, 조금이라도 걸치는 칸을 예약 칸으로 뺀다(`reservedCells`). 위젯은 예약 칸에 놓이지 않는다
- 아이콘 수가 바뀌거나(capability로 숨김) 화면 높이가 바뀌어 아이콘 열 수가 바뀌어도 다시 잰다

#### 편집 방식

- 편집 모드 없이 평소에 바로 다룬다. 잠금(실수로 옮기지 않게)은 나중에 프로필 설정에 추가한다. 설정 저장 구조(`/api/preferences`)는 그 자리를 남겨 두었다
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
- `GET /api/preferences`: 한 번도 저장하지 않았으면 `widgets`가 없고, 프론트는 기본 배치(시스템 정보 위젯을 오른쪽 위에)를 쓴다. 모두 지웠으면 빈 목록이다
- `PUT /api/preferences/widgets`: 위젯 배치 전체를 바꾼다. 다른 설정은 그대로 둔다
  - 검사(`Registry.ValidateWidgets`): 위젯 보기가 있는 등록된 애드온, 애드온마다 하나, 크기는 manifest 범위 안, 좌표는 0~200칸, 서로 겹치지 않음, 64개 이하. 본문 64KB 이하, `application/json`만
  - capability(역할·플러그인)는 검사하지 않는다. 역할이나 플러그인이 바뀌어도 배치를 잃지 않게 하고, 표시 여부는 프론트가 정한다(관리자 전용은 그리지 않고, 플러그인 없음은 위젯 안에 짧은 안내)
- 프론트 저장소(`widgets/store.ts`)는 저장 요청을 하나씩 보내고, 보내는 동안 바뀐 배치는 마지막 것만 이어서 보낸다. 실패하면 로컬 배치를 유지하고 다음 변경 때 다시 보낸다

#### 주기 요청

- 탭이 보이지 않으면(`document.visibilityState`) 위젯과 창 모두 주기 요청을 멈추고, 다시 보이면 바로 한 번 가져온다(`addons/common/usePageVisible.ts`). 창·위젯 틀이 보기의 `active`에 넣으므로 애드온은 따로 처리하지 않는다
- 위젯은 늘 떠 있으므로 창보다 길게 갱신한다(시스템 정보: 창 2초, 위젯 5초)
- 알려진 한계: 시스템 정보의 CPU 사용률은 백엔드가 직전 샘플과 비교해 계산하므로, 창과 위젯이 함께 열려 있으면 위젯 값은 5초 평균이 아니라 직전 요청(창 포함) 이후의 평균이다

### 3-3단계 내장 애드온

| id | 이름 | requires | provider (우선순위) | 위젯 / 창 갱신 |
|---|---|---|---|---|
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
- 속도는 **프론트가 계산**한다. 백엔드는 누적 카운터와 단조 시각(`sampledAtMs`, 백엔드 프로세스 시작 기준)을 주고, 보기마다 직전 응답과의 차이로 구한다(`computeRates`). 그래서 창과 위젯이 서로의 샘플에 영향을 주지 않는다(시스템 정보 CPU 사용률의 한계를 피함). 첫 값은 두 번째 응답부터 나오고, 간격이 30초를 넘거나 카운터가 줄면 다음 샘플을 기다린다

#### 온도 (`temperature`)

- `hwmon`: `/sys/class/hwmon/*`의 `name`, `tempN_input`·`_label`·`_max`·`_crit`. 칩 이름으로 CPU(coretemp, k10temp 등)·디스크(nvme, drivetemp)·그 밖으로 나눈다. 경고·위험 온도는 0 초과 200 미만만 쓴다(NVMe 센서가 "없음"을 큰 값으로 준다)
- `omv-cputemp`: hwmon에 CPU 센서가 없을 때만 `CpuTemp.get`을 부른다(같은 센서 중복 방지). 스크립트를 설정하지 않으면 0을 주므로 버린다
- `omv-smart`(관리자): SMART 캐시에 값이 있을 때만 디스크 온도를 더한다. **SMART 조회를 일으키지 않는다**. hwmon이 이미 보여 주는 디스크(nvme0 ↔ nvme0n1, drivetemp의 블록 장치)는 뺀다. 확인 시각(`checkedAt`)을 함께 준다
- 위젯은 CPU·디스크 대표값(CPU 패키지, NVMe Composite, 없으면 가장 뜨거운 센서). 칩의 경고 온도(없으면 CPU 85, 디스크 60, 그 밖 80) 이상 위험, 10도 아래부터 주의

#### 컨테이너 (`containers`)

- `Compose.getContainers`(모든 사용자). 첫 행은 런타임 행이며 번역되지 않는 `name`으로 Docker·Podman만 판단하고, 번역된 `status`는 표시에만 쓴다
- 나머지 행의 상태 문구(docker CLI, 영어)에서 상태(running, paused, restarting, exited, created, dead, removing)와 헬스체크 결과를 읽는다. 원문도 함께 준다
- 위젯과 창 모두 문제 있는 컨테이너(dead, restarting, unhealthy, 헬스체크 대기)를 먼저 보여 준다. Podman이면 창에 미검증 안내를 띄운다
- 조작(4-2단계): `start`, `stop`(확인), `restart`(확인), 모두 관리자. `Compose.doContainerCommand`가 파라미터를 검사하지 않고 셸 명령을 만들므로(`data-sources.md`의 "4단계 조작에서 쓴 출처") 명령은 고정값, 이름은 docker 형식(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)과 지금 `getContainers` 목록에 있는지를 확인한 뒤 보낸다. 시간 제한은 2분(`docker stop`의 종료 유예 시간)
  - 창: 실행 중·일시 정지·다시 시작 중이면 정지·재시작, 정지·생성됨·dead면 시작. 삭제 중·알 수 없음은 조작 없음(`containerActions`). 버튼 칸은 두 개 너비로 고정해 상태 열을 맞춘다
  - 위젯에는 조작이 없다

#### 디스크 건강 (`smart`)

- `Smart.getList`(관리자 전용)는 디스크마다 smartctl을 `--nocheck` 없이 실행하므로 대기(스핀다운) 중인 디스크를 깨운다(OMV `smarttrait.inc` → `SmartInformation`). 그래서 자동 주기 조회를 하지 않는다
- 백엔드 `smart.Cache`가 결과를 모든 세션이 함께 쓰도록 보관하고, 다음 경우에만 OMV에 묻는다
  - 보관한 결과가 없다
  - 결과가 가장 최근 기준 시각 이전이다. 기준 시각은 `-smart-refresh-at`(`ONMARU_SMART_REFRESH_AT`, 기본 `00:00`, 서버 로컬 시간). 매일 첫 접속 때 건강 상태를 한 번 확인하는 용도다
  - 새로고침(`?refresh=1`)이고 마지막으로 가져온 지 1분이 지났다. 응답의 `refreshInSeconds`로 남은 시간을 알려 주고, 창은 그동안 버튼을 막는다
- 동시 요청은 OMV 호출 하나로 합친다. 실패하면 1분 동안 다시 부르지 않고, 보관한 결과가 있으면 그것을 준다
- 파라미터는 OMV workbench의 SMART 대시보드 위젯과 같은 `{start: 0, limit: -1}`이다
- `overallstatus`는 workbench의 색 구분을 따라 good(GOOD), warning(BAD_SECTOR), failing(BAD_ATTRIBUTE_NOW, BAD_ATTRIBUTE_IN_THE_PAST, BAD_SECTOR_MANY), unknown(BAD_STATUS 등)으로 줄이고 원래 값도 준다. 시리얼 번호·WWN은 브라우저에 넘기지 않는다
- 위젯은 10분마다 백엔드에 묻지만 백엔드는 기준 시각을 넘긴 경우에만 OMV를 부른다

### 4단계 내장 애드온

| id | 이름 | requires | provider | 조작 | 위젯 / 창 갱신 |
|---|---|---|---|---|---|
| `containers` | 컨테이너 | compose 플러그인 | `omv-compose` | `start`, `stop`(확인), `restart`(확인), 관리자 | 30초 / 15초 |
| `services` | 서비스 | admin | `omv-services` | `enable`, `disable`(확인) | 30초 / 10초 |

#### 서비스 (`services`, 4-3단계)

- 상태: `Services.getStatus`(관리자). `IServiceStatus`를 구현한 OMV 모듈마다 `{name, title, enabled, running}`이다. `name`은 대개 모듈 이름(`ssh`, `samba`, `nfs`, `rsyncd`)이지만 compose 플러그인처럼 다른 값(`Docker`)을 쓰는 모듈도 있다. `title`은 OMV가 번역한 문구라 표시에만 쓴다. 참거짓을 숫자·문자열로 주는 플러그인이 있을 수 있어 느슨하게 읽는다
- 상태 점과 문구: 켜짐+실행 ok "실행 중", 켜짐+정지 위험 "아직 정지 중", 꺼짐+실행 주의 "아직 실행 중"(설정과 실제가 다름), 꺼짐 idle "꺼짐". 위젯과 창 모두 문제 있는 것부터 보여 준다
- OMV는 설정을 먼저 바꾸고 실제 실행은 늦게 따라온다. 창은 켜기·끄기가 끝난 뒤에도 설정과 실제 실행이 모두 바뀔 때까지(`serviceReached`, 최대 15초) 진행 중으로 두고, 그동안 그 행은 조작 전 상태로 보여 준다(`holdServices`). 중간 상태가 문제로 보이거나 행이 정렬 순서를 따라 튀지 않게 하기 위해서다
- 켜기·끄기는 어댑터 표(`adapters`)에 있는 서비스만 한다: SSH(`SSH.get`/`set`), SMB/CIFS(`SMB.getSettings`/`setSettings`), NFS, Rsync 서버(`Rsyncd`). 응답의 `toggleable`로 알려 주고, 창은 나머지 서비스에 "OMV에서 설정" 안내를 둔다
- 흐름(`toggle`): `Config.isDirty`가 참이면 409 `config_dirty`로 거절(사용자가 workbench에서 적용하지 않은 변경을 대신 배포하지 않기 위해) → get, 이미 원하는 값이면 성공 → get 결과에서 set 스키마 속성만 골라 `enable`만 바꿔 set → `Config.applyChangesBg({modules: [], force: false})` → `WaitBg`. 시간 제한 3분
  - set 스키마(`datamodels/rpc.<svc>.json`)는 속성이 모두 필수다. get은 다른 속성도 줄 수 있으므로(NFS의 `shares`) 그대로 넘기지 않는다
  - 이 백엔드의 켜기·끄기는 하나씩 실행한다(`toggleLock`). 적용이 끝나기 전에는 OMV가 적용 대기로 표시하므로, 겹치면 뒤의 요청이 앞 요청의 변경 때문에 `config_dirty`로 거절되기 때문이다. 뒤의 요청은 기다리며, 기다리는 시간도 시간 제한에 들어간다
  - isDirty와 set 사이에 다른 곳에서 만든 변경이 함께 배포될 수 있는 경쟁 조건은 받아들인다
  - set이 성공하고 적용이 실패하면 설정은 바뀐 채 적용 대기로 남는다(OMV workbench에 표시됨). `action_failed`로 알린다
- 끄기 확인 문구는 서비스마다 무엇이 끊기는지 알려 준다(SSH 세션, SMB·NFS 클라이언트, rsync 백업)

### 시스템 정보

- 3-1단계: 시스템 정보 창을 첫 내장 애드온(창 보기, provider `proc`)으로 옮겼다
- 3-2단계: CPU 사용률·RAM·가동 시간 요약을 첫 위젯(`SystemInfoWidget`, 기본 3×2, 2×2~4×3)으로 옮겼다. 2칸 너비에서는 메모리 용량을 빼고 가동 시간을 두 줄로 나눈다
- 이후(시점 미정): 창 보기를 프로세스 모니터로 확장한다. 프로세스 목록 읽기는 읽기 전용이지만, 프로세스 종료 같은 조작은 4단계 이후다

## 내장 애드온

### 위치와 등록

- 백엔드
  - `backend/internal/addons`: 계약(`Addon`, `Provider`, `Registry`, `Classify`, `WritesEnabled`), capability 감지·계산, TS 생성
  - `backend/internal/addons/<id>/`: manifest와 provider, 그리고 필요하면 API 핸들러. 계약 패키지를 import한다
  - `backend/internal/addons/builtin`: 정적 목록(`builtin.New`). 계약 패키지와 각 애드온이 서로 import하지 않게 따로 둔다
- 프론트엔드
  - `frontend/src/addons/<id>/`: 보기 컴포넌트(창, 위젯)와 데이터 훅
  - `frontend/src/addons/registry.ts`: 애드온 id별 보기 컴포넌트(`ADDON_VIEWS`). manifest가 선언한 보기마다 `Window`·`Widget` 컴포넌트가 있어야 한다(tsc가 검사)
  - `frontend/src/widgets/`: 위젯 그리드·배치 계산(`grid.ts`, `layout.ts`), 위젯 목록(`catalog.ts`), 저장 배치 저장소(`store.ts`). 창의 `src/windows/`와 서로 import하지 않는다
  - `frontend/src/addons/common/`: 애드온이 함께 쓰는 `poller.ts`, `format.ts`
  - `windows/apps.ts`는 창 보기가 있는 애드온을 manifest에서 만들고 준비 중 앱과 합친다. 준비 중 앱과 같은 id의 애드온을 만들면 준비 중 앱을 지운다
- 별도 패키지, 런타임 로딩, Go `plugin` 패키지는 쓰지 않는다

### manifest 원본과 id 일치 검사 (3-1단계에서 정함)

- Go 구조체가 원본이다. `cmd/genaddons`가 `frontend/src/addons/manifests.gen.ts`(`MANIFESTS`, `AddonId`)를 `as const`로 생성하고, `make types`가 tygo와 함께 실행한다
  - JSON 파일 공유는 쓰지 않았다. `go:embed`는 패키지 밖 파일을 읽을 수 없고, Vite `server.fs.allow`에 backend 경로를 열어야 하며, JSON import는 리터럴 타입이 넓어져 `AddonId`를 얻으려면 결국 생성이 필요하다
- 백엔드·프론트 id 일치는 두 겹으로 검사한다
  1. Go 골든 테스트(`builtin`): 커밋된 `manifests.gen.ts`가 생성 결과와 같은지. 실패하면 `make types`를 실행한다. 테스트 캐시는 backend 모듈 밖 파일 변경을 추적하지 않으므로 `make test`는 Go 테스트를 `-count=1`로 실행한다
  2. tsc: `ADDON_VIEWS`는 애드온 id마다 manifest의 보기에 맞는 컴포넌트를 요구하므로, 보기가 빠지거나 없는 id가 있으면 오류가 난다. `title`·`icon`은 `AddonManifest` 타입으로 번역 키·아이콘 이름인지 검사한다

### 새 애드온 추가 체크리스트

1. 데이터 출처를 정하고 [data-sources.md](data-sources.md)에 추가한다. root가 필요한 정보는 OMV RPC로만 가져온다
2. RPC 메서드 이름과 역할은 추측하지 않는다. `/usr/share/openmediavault/engined/rpc/*.inc`에서 확인한다
3. manifest의 `requires`와 `rpc.read`를 채운다. `rpc.write`는 현재 단계가 허용할 때만 채운다
   - 새 플러그인이 필요하면 `knownPlugins`에 패키지와 `.inc` 파일을 추가한다(`dpkg -S`로 확인)
4. provider를 만든다
   - 입력부(`fs.FS`, RPC 클라이언트)를 주입받게 한다
   - `testdata/`에 합성한 픽스처로 테스트한다. 식별 정보(시리얼, MAC, IP, 호스트 이름, 사용자 이름)는 가짜 값으로 바꾼다
5. `Addon.Fetch`로 응답을 만들고(OMV는 `Request.RPC`로만), 응답 구조체는 `api.AddonSources`를 품게 해 `internal/api`에 추가한다. 입력은 `builtin.Deps`·`HostDeps`에 두고 `builtin.New`에 등록한 뒤 `make types`를 실행한다(`types.gen.ts`, `manifests.gen.ts`). OMV RPC를 쓰면 `omvfake`에 합성 응답을 추가한다
6. 프론트 보기 컴포넌트(창, 위젯)를 만들어 `addons/registry.ts`에 등록한다. 같은 id의 준비 중 앱이 있으면 지운다
   - capability가 없을 때의 안내는 창·위젯 틀(`AddonUnavailable`, 위젯은 `compact`)이 처리한다. 보기 컴포넌트는 "연결 끊김" 상태를 처리한다
   - 주기 요청은 `poller.ts` 방식을 따르고 보기의 `active`가 false면 멈춘다. 위젯은 로그인한 동안 계속 떠 있으므로 갱신 주기를 창보다 길게 잡는다
   - 위젯 보기는 `size`(칸)를 받는다. manifest의 최소 크기부터 최대 크기까지, 한국어·영어 모두 내용이 넘치지 않는지 브라우저에서 잰다
7. 아이콘은 리졸버 매핑 테이블에 추가한다
8. `make check`, `make build`

## 향후 고려 (구현하지 않음)

공개 후 필요해지면 다시 검토한다. 지금 설계는 이 둘을 막지 않는 정도만 신경 쓴다.

### 선언형 애드온

- YAML 같은 데이터만으로 정의한다. "이 RPC를 호출해 표·게이지·키-값으로 보여 준다" 정도다. OMV workbench의 `dashboard.d/*.yaml` 위젯과 같은 방식이다
- 실행하는 코드가 없으므로 비교적 안전하다. RPC 허용 목록과 단계 규칙도 manifest 검사로 그대로 강제할 수 있다
- tempmon, zfs, nut처럼 RPC 하나를 표로 보여 주면 되는 플러그인 대부분이 여기에 해당한다

### 코드 애드온

- 이 대시보드는 OMV와 같은 출처로 서빙된다. 그래서 서드파티 JS를 그냥 실행하면 OMV workbench와 같은 출처 권한을 갖는다
  - 브라우저에 OMV 로그인 쿠키가 있으면 `rpc.php`를 직접 호출할 수 있다. 백엔드 중계와 허용 목록을 모두 우회하게 된다
  - workbench의 sessionStorage도 읽을 수 있다
- 따라서 서드파티 프론트 코드는 반드시 `sandbox="allow-scripts"` iframe(`allow-same-origin` 없이, opaque origin)에 넣는다. 백엔드와는 postMessage SDK로만 통신하게 한다
- 백엔드 확장은 Go `plugin`을 쓰지 않는다. Go 버전과 의존성 버전이 정확히 같아야 하고 언로드도 안 된다. 대신 별도 프로세스(유닉스 소켓 HTTP)를 백엔드가 중계하는 방식으로 한다

### 공개 시 라이선스

- 아이콘 테마 Colloid는 GPL-3.0이다. 빌드 결과에 함께 들어가므로 프로젝트와 배포물의 라이선스를 정할 때 이 점을 고려한다
