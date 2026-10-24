---
paths:
  - "frontend/src/shell/**"
  - "frontend/src/windows/**"
  - "frontend/src/widgets/**"
  - "frontend/src/shortcuts/**"
  - "frontend/src/notifications/**"
  - "frontend/src/settings/**"
  - "frontend/src/appearance/**"
  - "frontend/src/App.tsx"
  - "frontend/src/index.css"
  - "frontend/src/base.css"
---

# 데스크톱 셸과 창 시스템

`src/windows/`(창 저장소·앱 목록·위치 제한·타일링), `src/widgets/`(위젯 그리드·배치·저장소), `src/shortcuts/`(바로가기 목록·저장소), `src/shell/`의 `*Anim.ts`, `taskbarDrag.ts`, `taskbarFit.ts`, `clock.ts`, `profileMenu.ts`, `contextMenu.ts`, `contextMenuStore.ts`, `windowMenu.ts`, `menuNav.ts`, `modalStack.ts`, `dialogs.ts`, `appDrawerStore.ts`, `appDrawerPrefsStore.ts`, `appSort.ts`, `gridNav.ts`, `appSearch.ts`, `hangul.ts`, `src/notifications/`의 `model.ts`, `items.ts`, `messages.ts`, `store.ts`는 DOM에 의존하지 않는 계산 모듈이다. 컴포넌트는 이 결과를 그리기만 한다.

## 창 순서와 DOM

- `windows[]` 순서가 작업표시줄 순서다
- `Desktop`은 `stableOrder`로 창을 처음 연 순서대로 그린다. 작업표시줄 순서를 바꿔도 창 DOM이 옮겨지면 안 된다(옮겨지면 창 안의 스크롤 위치·iframe이 초기화된다)
- 위젯은 창 시스템과 분리한다. `windows[]`와 창 컴포넌트에 위젯용 분기를 넣지 않는다(`.claude/docs/addons.md`의 "보기: 창과 위젯")
- 창을 끄는 동안 위치는 저장소를 바꾸지 않고 요소 style(`left`·`top`)만 바꾸며, 놓거나 취소될 때 `moveTo`로 한 번 반영한다(style은 지우지 않는다). 떼어 내기(`detachAt`)만 끄는 중에 저장소를 바꾼다
- 창 내용은 `memo`한 `WindowBody`로 그린다. 창 틀이 다시 그려져도(포커스, 크기 조절, 타일 가이드) 내용은 다시 그리지 않는다
- 창·위젯 보기는 처음 그릴 때 내려받으므로(`addons/registry.ts`·`shellViews.ts`의 `lazyView`) `ViewBoundary`(Suspense + 오류 경계)로 감싼다. 받는 동안은 내용이 비고, 받지 못하거나 렌더 오류가 나면 그 창·위젯만 안내로 바뀐다(창은 페이지 새로 고침 단추, 위젯은 짧은 안내만. 위젯 틀이 포인터를 잡아 안의 단추가 눌리지 않는다). `lazy`는 실패를 기억하므로 다시 시도 대신 새로 고침이다. 창 크기는 저장소, 위젯 크기는 칸에서 나오므로 내용이 비어도 애니메이션·배치에는 영향이 없다

## 창 내용 폭

- 설정 창을 기준으로, 창 내용은 넓은 창에서 최대 폭(`base.css`의 `--window-content-width`, 720px) 열로 가운데 정렬한다. 내용 컨테이너의 좌우 padding을 `max(여백, calc((100% - var(--window-content-width)) / 2))`로 계산한다(설정은 `.settings .sidebar-header`·`.settings-page`, 목록형 애드온 창은 `.addon-window`). `.window-body`에는 걸지 않는다(사이드바 틀은 창 너비를 다 써야 한다)
- 예외는 창을 꽉 채우는 앱형 창이다. 위아래 고정 영역(도구 막대·상태 표시줄)이나 사이드바가 있고, 목록·표·그래프가 남은 높이를 채워 자기 안에서 스크롤하며, 넓을수록 한 줄에 더 많이 보이는 창이다. 시스템 정보 창과 파일 탐색기가 그렇다(`.addon-window`를 쓰지 않는다). 이런 창의 안내 문구는 가운데 정렬 안내(`AddonUnavailable` 등)로 두고, 대화상자는 모달이라 영향이 없다

## 위젯 레이어

- 설계와 결정 사항은 `.claude/docs/addons.md`의 "위젯 시스템"에 있다
- `WidgetLayer`는 `.desktop-area` 안에서 아이콘 다음, 창 앞(DOM 순서)에 그린다. 평소에는 창 아래이고, 위젯을 끌거나 크기를 바꾸는 동안만 창 위로 올라온다(`.widget-layer.raised`)
- `src/widgets/`와 `src/windows/`는 서로 import하지 않는다. 위젯이 창을 여는 일은 `shell/WidgetFrame.tsx`가 창 저장소의 `open`을 불러 처리한다
- 저장 배치는 `widgets/store.ts`, 화면 배치는 `fitLayout`으로 렌더링 때 계산한다. 저장 배치는 사용자가 바꾼 위젯만 `commitPlacement`로 바꾼다(화면 크기 변화로 바꾸지 않는다)
- 끄는 동안 위치·크기는 렌더링 없이 요소 style을 직접 바꾼다. 끝나면 지우지 말고 지금 자리의 값으로 되돌린다(React는 값이 바뀌지 않은 style을 다시 넣지 않는다)
- 위젯 잠금은 `WidgetLayer`가 화면 설정 저장소(`appearance/store.ts`의 `widgetsLocked`)에서 읽어 `WidgetFrame`의 `locked`로 넘긴다. 잠기면 끌기를 시작하지 않고 크기 조절 핸들을 그리지 않으며, "위젯 추가 ▸"(`desktopMenuItems`)와 "위젯 제거"를 비활성("잠김")으로 둔다. 끌기를 시작했는지는 `mode`가 아니라 ref(`moving`)로 판단한다(놓는 순간 `mode`가 아직 렌더되지 않았을 수 있다). 끄는 도중 잠겨도 시작한 끌기는 끝낸다
- 포인터 끌기 공통 처리는 `usePointerDrag`(임계 거리, pointer capture, 취소)를 쓴다. 창(`Window.tsx`)의 끌기 코드는 아직 이 훅으로 옮기지 않았다
- 가이드(`WidgetPreview`)는 타일링 가이드와 같은 모양(`.tile-preview`)을 쓰고, 칸이 바뀌면 임계 감쇠 스프링으로 따라간다(`widgetPreviewAnim.ts`). 끄는 동안 렌더 없이 핸들(`moveTo`)로 움직이고, 놓을 수 없는 자리에서는 숨기기만 한다

## 바로가기와 모든 앱

- 바탕화면에는 사용자가 고른 바로가기만 놓는다(`DesktopShortcuts`). 모든 앱은 작업표시줄 왼쪽 끝 버튼으로 여는 "모든 앱" 화면(`AppDrawer`)에 있다. 화면 용어는 "바로가기"(붙여 씀)와 "모든 앱", 코드 이름은 `shortcut`과 `AppDrawer`다
- 앱은 창 앱(`addon`, `placeholder`, 셸이 직접 그리는 `shell`)과 링크 앱(`link`, 창 없이 브라우저 탭으로 연다)이다(`windows/apps.ts`). 셸 앱의 보기 컴포넌트는 `shell/shellViews.ts`에 두어 node 테스트 대상인 `apps.ts`가 컴포넌트를 import하지 않게 한다. 창 저장소는 `WindowAppId`만 받으므로, 앱을 여는 곳(서랍 타일·바로가기·메뉴 "열기")은 `shell/launchApp.ts`의 `launchApp`을 부른다
  - 창 앱은 앱마다 창 하나이고, 다시 열면 그 창을 복원해 앞으로 가져온다(저장소의 `open`). `multiInstance` 앱(지금은 파일 탐색기와 웹 터미널)만 열 때마다(바로가기·서랍 클릭, "열기") 새 창을 연다. 열린 창을 앞으로 가져오는 것은 작업표시줄 버튼이 한다
  - 바로가기는 한 번 눌러 연다. 습관대로 두 번 누르면 `multiInstance` 앱이 창 두 개를 열므로 연속 누르기의 둘째 click부터(`detail` 2 이상)는 무시한다(`DesktopShortcuts`의 `onClick`)
  - 창 id는 늘 `win-<n>`이고 앱 id와 같지 않다. 앱 id로 창 하나를 찾는 코드는 `multiInstance` 앱에서 틀리므로, 새로 만들면 여러 창을 전제로 한다
  - 창마다 같은 앱 안의 번호(`WindowState.instance`)를 열 때 받는다(그 앱의 열린 창이 쓰지 않는 가장 작은 수, 닫으면 다시 씀). 둘째 창부터 제목에 붙이며(`windows/title.ts`의 `windowTitle`, 문구 `window.numberedTitle`), 제목을 그리는 곳(제목 표시줄, 작업표시줄 버튼·툴팁·퇴장 버튼, "더 보기" 메뉴의 `windowTitleLabel`)은 모두 이 함수를 쓴다. `windows[]` 순서로 번호를 매기지 않는다(작업표시줄에서 순서를 바꾸면 제목이 바뀐다)
  - 링크 앱(지금은 OMV 워크벤치)은 `openLink`가 `window.open(주소, 탭 이름)`으로 이름 붙은 탭 하나를 다시 쓴다. 이름 조회가 되도록 `noopener`를 주지 않으며, 팝업 차단을 피하려고 입력 처리기 안에서 동기로 부른다. 주소 계산은 `windows/links.ts`(끝에 OMV 첫 화면 `#/`를 붙여, 다시 열 때 새로 고침 없이 이동한다)
  - 열기 전에 "새 탭에서 열립니다" 데스크톱 대화상자를 띄운다(`linkNoticeDialog.tsx`의 `confirmLinkOpen`, 내용은 `LinkNotice.tsx`). 탭은 "열기" 클릭 처리 안에서 바로 연다(결과를 기다린 뒤 열면 팝업 차단). "다음에 알리지 않음"은 브라우저별 편의 설정이라 쿠키(`maru_link_notice_<id>=off`, 1년, Path 없음)에 두며 계산은 `linkNotice.ts`
  - 링크 앱은 주소(`/api/capabilities`의 `workbenchUrl`)가 있을 때만 보인다. capability를 받기 전에도 숨긴다(`appAccess`의 예외). 셸 앱인 웹 터미널(`terminal`)도 같은 예외로 `capabilities.terminal`(관리자가 허용)일 때만 보인다. 프로필 메뉴의 "OMV 워크벤치 열기"는 `requires: 'workbench'`로 같은 조건을 따른다. 파일 탐색기(`files`)는 권한 도우미가 없어도(`capabilities.helper`가 false) 숨기지 않고, 창 안에 보기 대신 못 쓰는 이유를 안내한다(`notice: 'helper_missing'`, 프런트만 쓰는 사유). 파일 작업 창(`transfers`)은 탐색기가 여는 창이라 늘 숨긴다(창은 작업표시줄에만 보인다, `.claude/rules/files.md`의 "파일 작업")
- 목록은 `shortcuts/store.ts`(사용자별 `/api/preferences`의 `shortcuts`, 불러오기·저장 방식은 위젯 저장소와 같다), 순서 계산은 `shortcuts/list.ts`, 칸 계산은 `shortcuts/layout.ts`가 한다
  - 저장 목록에는 지금 보이지 않는 앱(일반 사용자의 관리자 전용 앱)도 남긴다. 화면 순서와 놓을 자리는 보이는 목록 기준이며(`placeShortcut`), 보이지 않는 앱은 서로의 순서를 지킨다
  - 한 번도 저장하지 않았으면 기본 목록(시스템 정보·파일·저장소), 모두 지웠으면 빈 바탕화면이다. 모르는 앱 id는 불러올 때 뺀다
- 앱 이름(바탕화면 아이콘, 서랍 타일)은 띄어쓰기에서만 줄을 바꾸고(`word-break: keep-all`), 한 단어가 한 줄보다 길 때만 글자 사이에서 자른다(`overflow-wrap: anywhere`). 한글 이름이 단어 중간에서 잘리지 않게 하기 위해서다
- 배치는 왼쪽 위부터 위에서 아래로, 넘치면 다음 열이다(`grid-auto-flow: column`). `layout.ts`의 상수와 `rowsPerColumn`은 CSS(`.desktop-icons`, `.desktop-icon`)와 맞춘다. 끄는 동안 쓰는 한 열의 아이콘 수는 그려진 목록(`gridTemplateRows`)에서 잰다(브라우저 확대로 영역 높이가 소수점이면 반올림한 `bounds`로 계산한 값과 다를 수 있다)
- 위젯 레이어는 바로가기를 불러온 뒤(`iconsReady` = 저장소의 `loaded`)에 그린다. 먼저 그리면 아이콘 목록 너비가 0이라 위젯이 아이콘 자리에 놓였다가 밀려난다. `loaded`는 한 번 불러오면(실패 포함) 다시 시도하는 동안에도 남아, 재시도 때 위젯이 다시 마운트되지 않는다
- 메뉴: 바로가기 → "열기"·"바로가기 제거", 빈 바탕화면 → "바로가기 추가 ▸"(이미 있는 앱은 체크·비활성), 서랍 타일 → "열기"·"바탕화면에 바로가기 추가" 또는 "바로가기 제거". `multiInstance` 앱도 같다("열기"가 새 창을 연다)
- 끌기(바탕화면 순서 바꾸기, "모든 앱" → 바탕화면)
  - pointer capture는 누른 요소(바탕화면 아이콘 버튼, 서랍 타일)가 갖는다. 끄는 동안 그 요소를 지우거나 DOM에서 옮기거나 `inert`로 만들지 않는다(capture가 풀려 끌기가 취소된다). `li`의 순서는 놓거나 취소한 뒤 `settle`에서만 바뀐다
  - 끄는 버튼과 고스트는 렌더 없이 style로 움직이고, 다른 아이콘은 놓을 칸이 바뀔 때만 렌더해 transform으로 비켜 준다(`previewOffsets`, `li.shifting`). 끄는 동안 아이콘 목록은 창 위로 올라온다(`.desktop-icons.raised`)
  - 놓으면 먼저 목록을 바꾸고 아이콘마다 보이던 자리에서 새 자리로 미끄러지게 한다(FLIP, `slideKeyframes`). 서랍에서 놓은 앱은 고스트 자리에서 들어온다. 착지가 끝나도 그사이 새 끌기가 시작됐으면 목록을 내리지 않는다
  - 끌기·길게 누르기 뒤의 click은 무시하되 포인터 click에만 적용하고 표시는 늘 지운다(키보드 click은 `detail`이 0). 끈 요소가 놓는 순간 DOM에서 옮겨지거나 지워지면 브라우저가 click을 보내지 않아 표시가 남기 때문이다. 끌기가 시작되면 길게 누르기 대기를 멈춘다(`cancelLongPress`)
  - `usePointerDrag`는 누른 요소가 끄는 도중 지워져 끝 이벤트(pointerup, lostpointercapture)가 React까지 오지 않으면, 다음 누름에서 그 끌기를 취소로 정리한다
  - 데스크톱 영역 밖(작업표시줄)에 놓거나 Esc, 브라우저 창이 포커스를 잃거나, 데스크톱 범위 모달이 열리면(`cancelDrag`) 취소한다. effect·언마운트에서 부르는 취소는 flushSync를 쓰는 `settle`을 microtask로 미룬다
  - 서랍 타일은 마우스·펜으로만 끈다. 터치는 격자 스크롤과 겹치므로 끌지 않고(`touch-action: manipulation`) 타일 메뉴의 "바탕화면에 바로가기 추가"를 쓴다
  - 서랍 타일을 끌면 서랍을 언마운트하지 않고 투명하게만 만들고, 고스트(`DesktopIconFace`)를 서랍 옆에 그린다. 놓을 곳은 좌표로만 판단한다(투명한 서랍이 위에 있어 `elementFromPoint`를 쓸 수 없다). 이미 바탕화면에 있는 앱이면 원래 아이콘은 숨기고(`li.picked`) 칸만 차지한다. 끝나면 서랍을 애니메이션 없이 닫는다(`close(true)`)
- "모든 앱" 화면
  - DOM에서 `.desktop-area` 다음, 작업표시줄 앞에 `.desktop` 수준으로 그린다. `nav.taskbar`의 `backdrop-filter`가 위치 기준 상자가 되므로 작업표시줄 안에 둘 수 없다. 창·위젯 위, 작업표시줄·모달·컨텍스트 메뉴 아래다
  - 열린 동안 `.desktop-area`는 `inert`이고 작업표시줄은 그대로 쓴다. 바깥(배경, 격자의 빈 곳, 시계·프로필 등 작업표시줄의 다른 요소) 누르기, Esc(검색어가 있으면 먼저 지운다), 앱 열기, 작업 버튼 누르기, 데스크톱 범위 모달, 로그아웃이면 닫힌다. 작업 버튼은 바깥으로 보지 않고 `onTaskClick`이 닫은 뒤 그 창을 보인다(서랍에 가려 있던 활성 창을 최소화하지 않게). 타일 메뉴(`.context-menu`)를 누르는 것은 바깥으로 보지 않고, 메뉴의 Esc는 메뉴만 닫는다(`defaultPrevented`). 가상 키보드 때문에 창 크기 변화로는 닫지 않는다
  - 열면 검색 상자에 포커스한다(터치로 열었으면 하지 않는다). 격자는 로빙 tabindex이고 화살표 이동은 `gridNav.ts`(첫 줄에서 위 → 검색 상자)다
  - 정렬은 검색 상자 옆 버튼의 메뉴(컨텍스트 메뉴 모델)로 고른다: "추천순"(대시보드가 정한 순서, `APP_IDS`)과 "이름순"(지금 화면 언어, `Intl.Collator`). 고른 방식은 사용자별로 저장하고(`appDrawerPrefsStore.ts`, `/api/preferences`의 `drawer`), 불러오기 전이나 실패하면 추천순으로 보이며 바꿀 수 없다. 검색 결과는 맞는 정도가 같으면 정렬 순서를 따른다
  - 검색(`appSearch.ts`)은 지원 언어 모두의 앱 이름과 앱 id로 화면 언어와 무관하게 찾고, 한글은 자모로 풀어 초성과 조합 중인 글자도 맞춘다(`hangul.ts`). 후보는 카탈로그의 앱 이름만 담은 `i18n/appNames.ts`로 만든다(다른 언어 카탈로그는 받지 않았을 수 있다, `i18n.md`)
  - 열기·닫기 애니메이션 값은 `appDrawerAnim.ts`, 열림 상태는 `appDrawerStore.ts`다. 로그인·로그아웃 전환에서 데스크톱과 함께 움직인다(`screenTransition.ts`의 `DESKTOP_PARTS`)

## 컨텍스트 메뉴

- 우클릭 메뉴는 모두 `contextMenu.ts`의 항목 모델(`action`, `separator`, `submenu`는 한 단계까지)로 만들고 `useContextMenuTrigger`로 연다. 문구는 키(`labelKey`)와 치환값(`labelParams`)으로 둔다. 화면에는 `Desktop`이 그리는 `ContextMenu` 하나만 있다(한 번에 메뉴 하나)
- `useContextMenuTrigger`가 우클릭, 키보드(메뉴 키, Shift+F10), 터치 길게 누르기를 함께 처리한다. 길게 눌러 메뉴를 연 누름은 끌기·클릭으로 이어 가지 않게 `openedByPress()`로 확인한다. 다른 포인터 처리기와는 `mergeHandlers`로 합친다
- 브라우저 기본 메뉴는 `nativeGuard`가 막는다(capture 단계 `preventDefault`라 React `onContextMenu`는 그대로 불린다)
- 바탕화면 메뉴는 빈 곳(아이콘 사이 포함)에서만 연다. 메뉴 항목은 기능을 가진 쪽이 만든다(바로가기 추가는 `DesktopShortcuts`, 위젯 추가는 `WidgetLayer`의 `desktopMenuItems`). 맨 아래에는 구분선 뒤에 "화면 설정 열기"(설정의 화면 페이지, `Desktop`의 `openSettings`)를 둔다
- 창 메뉴 항목은 `windowMenu.ts`가 만든다. 작업표시줄 버튼 메뉴(`taskMenuItems`)와 제목 표시줄 메뉴(`titlebarMenuItems`)가 상태 항목(최소화·복원, 최대화·이전 크기로)을 함께 쓴다. 여러 창은 저장소의 `closeMany`로 한 번에 닫는다
  - 작업표시줄: (`multiInstance` 앱이면 "새 창" /) 상태 항목 / 다른 항목·오른쪽 항목·모두 닫기 / 닫기. "새 창"은 `launchApp`과 같고, 데스크톱 크기가 필요하므로 `Desktop`이 `Taskbar`의 `onNewWindow`로 넘긴다. 메뉴가 버튼 위로 열리므로 닫기는 누른 자리 가까이 오도록 맨 아래에 둔다. 최소화된 창에는 최소화 대신 복원(`raiseGroup`, 버튼 클릭과 같음)을 두고, 최소화된 창을 최대화하면 복원도 함께 한다. "오른쪽"은 `windows[]`(작업표시줄) 순서 기준이다. 메뉴는 `Taskbar`의 트리거 하나로 열고, 대상 창은 `items(el)`이 받는 버튼의 `data-window-id`로 찾는다. 끄는 중에는 열지 않는다
  - 제목 표시줄: 상태 항목 / 닫기. 제목 표시줄 버튼 위에서는 열지 않고(길게 누르면 버튼이 눌리지 않게), 창을 끄는 중에는 열지 않으며, 메뉴를 연 누름은 창 끌기로 이어 가지 않는다
- 비활성 항목은 `aria-disabled`로 두어 포커스는 받게 한다(비활성 하위 메뉴는 열리지 않는다). 이유가 있으면 `detailKey`로 짧게 보여 준다(하위 메뉴는 화살표 앞)
- 키보드 이동(`menuNav.ts`), 포커스 이동(`menuFocus.ts`), 열기·닫기 애니메이션 값(`menuAnim.ts`)은 프로필 메뉴와 함께 쓴다

## 모달

- 범위는 창(`window`)과 데스크톱(`desktop`) 두 가지다. `Window`와 `Desktop`이 각각 호스트 요소(`.modal-host-window`, `.modal-host-desktop`)와 컨텍스트(`WindowModalContext`, `DesktopModalContext`, `modalScope.ts`)를 제공하고, `Modal`은 자기 범위의 호스트에 포털로 그린다. 창 보기 안에서는 두 컨텍스트가 모두 보이므로 `scope`를 생략하면 그 창, `'desktop'`이면 데스크톱이다
- 차단은 포커스 가두기가 아니라 `inert`로 한다. 범위의 스택(`modalStack.ts`)이 비어 있지 않으면 창은 `.window-body`만, 데스크톱은 `.desktop-area`와 작업표시줄(`Taskbar`의 `inert`)을 막는다. 데스크톱 범위 모달이 열리면 컨텍스트 메뉴를 닫는다
  - 창 범위는 제목 표시줄(끌기·최소화·닫기)과 크기 조절 핸들을 막지 않는다. 호스트는 `.resize-handle`과 같은 z-index에 DOM에서 앞이라 핸들이 위에 온다
  - 창 범위에서는 다른 창이 막히지 않으므로 `Modal`이 Tab을 상자 안에서 돌린다
- 같은 범위에서 겹치면 연 순서대로 쌓고 맨 위만 입력을 받는다(아래는 `inert`, 배경은 맨 위만 어둡게). 닫기 시작하면 바로 스택에서 빠져 차단이 풀린다
- `Modal`은 틀(범위·차단·포커스·애니메이션·쌓임)만 다루고 내용·단추·모양은 쓰는 쪽이 정한다(`title`, `footer`, `size`, `backdrop`, `className` 등). 닫는 애니메이션 동안에는 상자(`ModalBox`)를 다시 그리지 않아 마지막 내용이 남는다. 포커스는 열 때 `initialFocus`(없으면 첫 포커스 가능한 요소)로, 닫힐 때 갈 곳을 잃었으면 열기 전 요소로 돌려준다. 내용에서 `autoFocus`는 쓰지 않는다(열기 전 포커스를 기억하기 전에 옮겨 버린다)
- 포털 안의 React 이벤트가 모달을 그린 쪽(행 우클릭 메뉴, 끌기)으로 올라가지 않게 `Modal`이 멈춘다. 문서의 capture 처리기는 영향이 없다
- 명령형: `dialogs.ts`의 `openDialog(scope, { render, ...옵션 })`이 `close(값)`의 값 또는 null로 끝나는 Promise를 돌려준다. 범위마다 `DialogOutlet`이 그리며, 그릴 곳이 없는 범위로 열면 바로 null이고 범위가 사라지면(창 닫힘, 로그아웃) 남은 대화상자도 null로 끝난다. 기본 형태(`messageDialog`, `confirmDialog`, `alertDialog`)와 컴포넌트용 훅 `useDialogs`는 `dialogPresets.tsx`에 있다
- 클래스 이름은 기존 셸 클래스(`.window`, `.desktop` 등)와 겹치지 않게 `modal-` 접두사를 붙인다

## 타일링

- 코드에서는 "타일링(tile)"이라는 용어만 쓴다
- 창을 끄는 포인터가 위 가장자리에 닿으면 최대화, 좌우 가장자리에 닿으면 반쪽 배치(모서리에서는 좌우 우선)
- 최대화·타일 창의 위치는 `tileRect` 하나로 계산한다. 가이드(`TilePreview`)도 같은 함수를 쓴다
- 좌우 타일 창은 `tileSplit` 하나를 공유하는 타일 그룹이다. 경계(`TileResizeHandle`, `TileDivider`, 공용 훅 `useTileSplitDrag`)를 끌면 함께 크기가 바뀌고, 작업표시줄에서 누르면 짝 창도 함께 앞으로 온다(`raiseGroup`)

## 애니메이션

- 재생은 Web Animations API로 하며 transform·opacity만 움직인다. 설정값과 FLIP 계산은 DOM 없는 `*Anim.ts`에 둔다
  - 예외: 위젯 가이드처럼 목표가 계속 바뀌는 추적은 애니메이션을 재시작하면 속도가 끊기므로 requestAnimationFrame 스프링으로 transform을 바꾼다(`WidgetPreview`)
- 누름 피드백: 클릭 가능한 요소에는 `:hover` 옆에 `:active`를 둔다. 토큰은 `base.css`의 `--press-bg`, `--press-scale`, `--press-in`·`--press-out`이다
  - 아이콘·타일·버튼류는 진한 배경과 줄어듦(`scale: var(--press-scale)`, 평소 `transition: scale var(--press-out)`, 누를 때 `transition-duration: var(--press-in)`), 누르면 닫히거나 이동하는 메뉴·사이드바 행은 배경만 바꾼다. 선택 상태(활성 창 버튼, 현재 페이지)의 배경이 hover와 같으면 hover·누름을 한 단계 진하게 한다
  - 줄어듦은 `transform: scale()`이 아니라 개별 속성 `scale`로 한다(끌기가 요소 style의 `transform`을 직접 쓴다). 끌 수 있는 요소는 끄는 동안 `:active`가 남으므로 주인의 끌기 클래스(`li.dragging` 등)에서 `scale: 1`로 되돌린다
  - `button`이 아닌 클릭 요소(label, 위젯 틀)에는 `-webkit-tap-highlight-color: transparent`를 함께 둔다(`button`은 `base.css`가 끈다)
- 창 밖(작업표시줄 메뉴)에서 배치를 바꿀 때는 `captureWindowLayout(id)`로 그 창의 `captureFrom`을 부른다. 숨겨진(최소화된) 창은 재지 않고 복원 애니메이션이 새 배치로 재생된다
- 애니메이션 켜기/끄기는 사용자 화면 설정(`appearance/store.ts`의 `animations`, 불러오기 전에는 켜짐)이고, 시스템의 '동작 줄이기'가 켜져 있으면 늘 끈다(`animationsEnabled`). 창·메뉴·화면 전환·배경 페이드가 함께 따른다
  - CSS 전환도 같은 기준이다. 사용자가 끄면 `<html data-animations="off">`(`appearance/dom.ts`의 `applyAnimations`)를 두고, `base.css`가 이 속성과 `prefers-reduced-motion` 미디어 쿼리에서 모든 `transition`을 끈다. 파일마다 전환 끄기 규칙을 따로 두지 않는다. 키프레임 애니메이션(진행 표시 등)은 이 규칙에 들지 않으므로 각 파일에서 다룬다
- 닫기: 저장소의 `close`는 창을 바로 지워 작업표시줄·포커스가 즉시 바뀐다. `Desktop`은 `useClosingWindows`로 지워진 창의 마지막 상태와 타일 비율(`ClosingWindow`)을 받아 두고, 애니메이션이 끝나 `onClosed`가 불릴 때까지 그 상태로 그린다(`stableOrder`의 `keep`). 닫히는 창은 클릭을 받지 않고 창 내용의 주기 요청도 바로 멈춘다
- 작업표시줄 버튼: 등장은 `li`의 ref(`enterTask`), 퇴장은 `Taskbar`가 너비·투명도를 애니메이션한다. 닫힌 창의 버튼은 `useClosingWindows`로 받아 `.leaving` 버튼으로 남긴다(`withLeaving`)
- 작업표시줄 드래그 정렬: 드래그 중에는 transform으로만 움직이고 놓은 뒤 순서를 반영한다. 전환 효과는 부모가 아닌 각 `li` 클래스에 둔다. 배치 계산에서 `.leaving` 버튼과 접힌 버튼(`li.overflowed`)은 뺀다
- 로그인 ↔ 데스크톱 전환(`screenAnim.ts`, 재생은 `screenTransition.ts`): 배경은 움직이지 않고 내용(`.login-card`, `.desktop-area`·`.taskbar`)만 움직인다. 로그인 배경과 바탕화면이 다르면 배경 레이어가 따로 페이드한다(아래 "화면 설정"). 나가는 애니메이션은 auth 저장소가 화면을 바꾸기 전에 `beforeScreenChange` 훅(`leaveScreen`)으로, 들어오는 애니메이션은 각 화면이 처음 그려질 때 재생한다

## 작업표시줄 트레이와 프로필 메뉴

- 트레이는 `[시계][알림][프로필]` 순서이며 프로필 버튼이 오른쪽 끝에 고정된다. 알림 버튼은 아래 "알림" 절에 있다
- "모든 앱" 버튼과 창 버튼 목록은 함께 정렬된다. 정렬은 사용자 화면 설정(`appearance.taskbarAlign`, 계산은 `appearance/taskbar.ts`)이고 기본은 가운데(관리자 기본값 없음)다. 가운데는 `.taskbar.centered`에서 앞쪽 빈 칸(`::before`)과 트레이를 같은 비율로 늘려 전체 너비의 가운데에 두며, 창이 없으면 빈 목록 앞의 gap을 지운다. 드래그 정렬이 `ul`의 자식을 재므로 "모든 앱" 버튼은 목록 밖에 둔다
- 창 버튼이 다 들어가지 않으면 단계적으로 줄인다(계산은 `taskbarFit.ts`, 측정은 `Taskbar`의 `measureFit`). 스크롤과 두 줄 감싸기는 쓰지 않는다(드래그 정렬의 슬롯, 터치 끌기, 최소화 목표, 고정된 `--taskbar-reserve`와 맞지 않는다)
  - 제목 모드: 모든 버튼에 같은 최대 너비(`--task-cap`)를 걸어 긴 제목부터 말줄임으로 줄인다. 최대 너비가 `TASK_LABEL_MIN`(한글 세 글자와 말줄임표)보다 작아지면 아이콘 모드(`ul.icons`)로 바꾼다
  - 아이콘만으로도 넘치면 뒤쪽 버튼을 `li.overflowed`로 숨기고 "더 보기" 버튼(`.taskbar-more`, ⋯)의 메뉴(컨텍스트 메뉴 모델)로 보인다. 순서(`windows[]`)는 바꾸지 않는다. 접힌 창이 활성 창이면 "더 보기" 버튼에 밑줄, 메뉴에는 체크를 표시하고, 항목을 고르면 그 창 버튼을 누른 것과 같다(`onTaskClick`). "더 보기" 버튼도 드래그 정렬 때문에 목록 밖, 목록과 트레이 사이에 둔다
  - 측정 입력은 결과(최대 너비, 아이콘 모드, 접기)에 따라 바뀌지 않아야 한다. 버튼 너비는 제목 span의 `scrollWidth`로 재므로, 아이콘 모드는 제목을 지우지 않고 너비 0으로 접고, 접힌 버튼도 `display: none`이 아니라 `position: absolute; visibility: hidden`으로 둔다(`display: none`이면 너비가 0이 되어 넓어진 화면에서 다시 펼치지 못한다). 트레이는 늘어난 너비가 아니라 자식 너비의 합으로 잰다
  - 제목이 잘린 버튼과 아이콘 버튼에만 `title` 툴팁을 붙인다
  - 접힌 창을 최소화하면 "더 보기" 버튼으로 줄어들고(`useWindowAnimation.ts`의 `minimizeTarget`), 닫으면 퇴장 애니메이션 없이 뺀다(닫히는 순간 접혀 있었는지를 `useClosingWindows`의 snapshot에 남긴다)
- 프로필 메뉴 항목은 `profileMenu.ts`의 `PROFILE_MENU_ITEMS`에 데이터로 둔다(`requires`로 거름: `admin`은 관리자만, `workbench`는 OMV 웹 UI 주소가 있을 때만). 항목은 `action`과 `submenu` 두 종류다. `action` 항목을 추가하면 `ProfileMenu.tsx`의 `actions`에 동작도 넣는다
- "전원"(관리자, 로그아웃 바로 위 같은 묶음)은 데스크톱 대화상자를 연다(`power/powerDialog.tsx`, 내용 `PowerOptions.tsx`, 계산 `power.ts`). 타일 세 개(시스템 종료·다시 시작·절전) 중 하나를 고르면 같은 대화상자가 확인 단계(뒤로·실행, 포커스는 뒤로)로 바뀐다. 제목과 본문이 단계를 함께 보므로 대화상자마다 작은 저장소(`dialogStore.ts`)를 둔다. 조작은 시스템 정보 애드온의 `shutdown`·`reboot`·`standby`이고(`.claude/docs/addons.md`의 "시스템 정보"), 절전은 열 때 받은 OMV 대기 방식(`?power=1`)이 전원 끄기이거나 지원되지 않거나 받지 못했으면 이유와 함께 막는다(`sleepState`)
  - 조작을 보내면(백엔드가 코드로 거절하지 않았으면. 연결 끊김·프록시 5xx도 보낸 것으로 본다, `sendOutcome`) 전원 감시(`power/watch.ts`의 순수 `step`·`screenView`, 저장소는 `watchStore.ts`)가 2초마다 `/api/health`를 받고 App이 `Screen` 대신 전원 화면(`PowerScreen.tsx`, 재시작 화면과 같은 틀)을 그린다. 끊겼다가 다시 답하면 새로 고친다(업그레이드 결과 힌트는 남기지 않는다). 2분이 지나도 끊기지 않으면 "대시보드로 돌아가기"(새로 고침)를, 다시 시작이 5분 넘게 돌아오지 않으면 확인할 곳을 안내한다
- 프로필 메뉴 머리글(아바타·이름·역할)은 메뉴 항목 버튼이며 누르면 설정 창의 프로필 페이지를 연다. 메뉴에 따로 "설정" 항목은 두지 않는다. `ProfileMenu`는 데스크톱 크기를 모르므로 `Desktop`이 `onOpenSettings`를 `Taskbar`를 거쳐 넘긴다
- 아바타 그리기는 `ProfileAvatar` 한 곳에 모은다. 프로필 사진(`profile/store.ts`, 로그인한 동안 불러옴)이 있으면 원형 이미지로, 없거나 받지 못하면 기본 아이콘으로 그린다

## 알림

시안은 Design 캔버스 "알림 시스템 시안"의 B(오른쪽 패널), T(토스트), S(상태)와 "시스템 업데이트 앱 시안"의 N(백그라운드 작업)·T(작업 결과 토스트)다. 서버 쪽은 `.claude/rules/backend.md`의 "알림"·"서버 백그라운드 작업".

- 알림은 두 가지다. 서버 알림(`api.Notice`)은 백엔드가 알리는 상태라 조건이 풀리면 사라지고 사용자가 지울 수 없으며, 읽음 상태(id → 리비전)를 사용자별로 저장한다. 예외로 작업 결과(`dismissible`)는 지울 수 있다(`dismissNotice`: 바로 숨기고 `DELETE /api/notifications/{id}`, 실패하면 되돌리며 그사이 받은 응답이 되살리지 않게 거른다). 클라이언트 알림(`ClientEvent`)은 이 탭에서 생긴 일이라 탭의 메모리에만 두고(최대 50개, 새로 고치면 사라짐) 지울 수 있다. 문구는 저장하지 않고 코드와 치환값만 두며 `notifications/messages.ts`가 키를 고른다
- `src/notifications/`는 다른 기능을 import하지 않는 잎 모듈이다(`files/`·`shell/`·`windows/` 금지). 다른 기능은 `notify.ts`의 `notify()`만 부른다(지금은 `files/transferNotices.ts`). 동작 실행(`shell/noticeAction.ts`의 `runNoticeAction`: 앱·설정 페이지, 워크벤치 라우트, 전원 대화상자)과 화면은 `shell/`에 둔다
- 저장소(`notifications/store.ts`)는 서버 알림, 읽음 상태, 클라이언트 알림, 토스트, 센터 열림(`centerOpen`)을 함께 둔다. `SignedIn`의 `useNotifications`가 로그인한 동안 60초마다(숨긴 탭에서 멈추고 다시 보이면 바로) `/api/notifications`를 받고, 언마운트하면 `clear`로 비운다. 로그인하지 않은 동안의 `notify()`(로그아웃 뒤 끝난 파일 작업 등)와 응답은 버린다
  - 읽음 저장은 저장 큐로 보내고 지금 목록에 있는 id만 담는다. 저장 전에 받은 응답이 읽음 표시를 되돌리지 않게 이 탭에서 읽은 값(`overrides`)을 서버가 같은 값을 줄 때까지 얹는다(`mergeSeen`·`settleOverrides`)
- 읽음: 알림 센터를 열면 모두 읽음이 되어 배지가 사라진다. 열기 전에 읽지 않았던 항목(`fresh`)은 열린 동안 점과 굵은 제목을 남긴다. 토스트를 닫거나 동작을 누르면 그 알림을 읽고, 시간이 지나 내려간 토스트는 읽지 않은 채 둔다
- 토스트: 클라이언트 알림은 늘 띄운다. 서버 알림은 이 탭의 첫 응답 뒤에 새로 생기거나 리비전이 바뀐 읽지 않은 것만 띄운다(로그인할 때마다 같은 알림이 뜨지 않게, `noticesToToast`). 예외로 읽지 않은 작업 결과는 첫 응답에서도 띄운다(로그아웃한 동안 끝난 작업을 다음 로그인 때 알린다). 센터가 열려 있으면 띄우지 않고 읽음으로 둔다
- 알림 버튼(`NotificationButton`): 트레이 자식이라 작업표시줄의 `measureFit`이 너비를 잰다. 처음부터 늘 그리고 프로필 버튼과 같은 정사각형이며, 배지(읽지 않은 수, 9 초과 `9+`, 읽지 않은 오류가 있으면 빨강)는 위에 겹쳐 너비를 바꾸지 않는다
- 알림 센터(`NotificationCenter`): 작업표시줄의 `backdrop-filter` 안에 둘 수 없으므로 `Desktop`이 "모든 앱" 화면 다음, 작업표시줄 앞에 그린다. 화면 오른쪽 위에서 작업표시줄 위까지 400px 패널이고 바탕화면을 막지 않는다(`role="dialog"`, 열면 패널에 포커스). 패널·알림 버튼 밖 누르기, Esc(포커스를 알림 버튼으로), 알림 버튼, 데스크톱 범위 모달(`blocked`)로 닫는다. "오늘"·"이전" 묶음과 "3분 전" 같은 시각은 `items.ts`(`buildCenter`, `ageText`)가 계산하고, 시각을 주지 않은 서버 알림은 오늘의 맨 앞에 둔다. "모두 지우기"는 클라이언트 알림만 지운다. 열기·닫기 값은 `notificationAnim.ts`
- 백그라운드 작업(시안 N): 알림 센터 맨 위의 "백그라운드 작업" 구역이 진행 중인 서버 작업(`tasks`)을 보인다. 시스템 업데이트 창이 열려 있으면 창이 진행을 보이므로 숨긴다. 카드는 머리말, 시작한 사람과 경과, 오가는 막대, 마지막 출력 한 줄(`lastLine`), "열기"이고 감시 대기(`waiting`)면 점선 카드와 안내다. `tasks`가 있으면 알림을 10초마다 받고(`pollInterval`, 바뀌면 `reschedule`), 시스템 업데이트 창이 작업을 시작하면 `refreshNotifications`로 바로 받는다
  - 작업 결과의 동작은 "출력 보기"(`actionLabel`의 `job`)이고, 실행하면 `usePackagesNav.showJob(id)`를 남긴 뒤 창을 연다(`runNoticeAction`의 `job`). 창은 그 작업을 작업 보기로 보인다(진행 중인 작업이 있으면 그것이 먼저)
- 토스트(`ToastLayer`): `Desktop`이 작업표시줄 다음, 컨텍스트 메뉴·모달 호스트 앞에 그리고 `blocked`면 `inert`다. 오른쪽 아래 360px에 최근 것이 아래로 세 개까지 보이고 나머지는 "+N개 더"로 접는다(`visibleToasts`). 정보·성공 5초, 경고 8초 뒤 내려가고 오류는 남는다(`toastDuration`). 남은 시간은 막대의 CSS 키프레임이 재고 끝(`animationend`)에 내리며, 포인터나 포커스가 레이어에 있으면 `animation-play-state`로 함께 멈춘다. 애니메이션을 끈 설정에서는 막대만 숨긴다. 목록은 `aria-live="polite"`이고 오류 토스트는 `role="alert"`이며 포커스를 가져가지 않는다. 빈 자리는 포인터를 받지 않는다
- "모든 앱" 화면은 토스트를 누르는 것을 바깥으로 보지 않는다(`KEEP_OPEN`). 로그인·로그아웃 전환에서 알림 센터와 토스트도 데스크톱과 함께 움직인다(`DESKTOP_PARTS`)

## 설정 앱

- `src/settings/`. 페이지 목록은 데이터(`pages.ts`: id, 문구 키, 아이콘, 그룹, `requires: 'admin'`)이고 화면 컴포넌트는 `pageViews.ts`의 `Record`에 둔다(빠지면 tsc가 잡는다). 관리자 전용 페이지(전역 설정)는 일반 사용자에게 숨긴다. OMV·시스템 설정을 넣게 되면 그룹을 더한다
- 어느 페이지를 보일지는 창 저장소가 아니라 `navStore.ts`에 둔다(창 밖에서 특정 페이지로 열 때 `show` 후 `launchApp`). 창이 닫힌 뒤 다시 열면 기본 페이지로 연다. 창 저장소를 구독해 창이 사라질 때 표시(`resetPending`)만 해 두고 다시 열릴 때 되돌린다(닫히는 창은 닫기 애니메이션 동안 이 저장소의 페이지로 그려지므로, 닫힐 때 바꾸면 기본 페이지가 잠깐 보인다). 그 사이 `show`를 부르면 표시가 지워진다. 창 컴포넌트의 정리 함수는 쓰지 않는다(StrictMode 개발 모드에서 마운트 직후에도 불린다)
- 업데이트 페이지(`update`, 관리자, `UpdatePage.tsx`, 계산은 `update.ts`)는 대시보드에서 업그레이드의 화면이다(`.claude/rules/backend.md`의 "대시보드에서 업그레이드"). 업데이트 채널은 카드(숨긴 라디오 버튼, `channelCards`)로 고르며 고르면 바로 저장한다(개발 채널은 확인 대화상자 뒤). 저장소가 `stable`·`main`이 아니거나 브랜치를 모르면 "지금 브랜치" 카드(빈 값)를 함께 보이고, 확인한 뒤 채널을 바꾸면 결과를 숨긴다(`CheckOutcome`의 `channel`). 설치기 문구(사유, 질문, 할 일, 오류)는 상태 파일의 영어·한국어 중 화면 언어에 맞는 쪽(`pickText`)을 보이고 번역 카탈로그에 넣지 않는다. 확인 결과는 상태가 `checked`일 때만 쓴다(업그레이드한 뒤·실패한 뒤에는 다시 확인). 버전은 `versionLabel`로 "0.4.0 (2222222)"처럼 짧은 커밋을 괄호 안에 붙인다. 업그레이드는 확인 대화상자 뒤 `elevation_required`면 `requestElevation`으로 상승하고 다시 요청한다
  - 업그레이드를 요청하면(또는 진행 중인 업그레이드를 보면) 앱 전체 감시(`upgrade/watch.ts`의 순수 `step`, 저장소는 `upgrade/watchStore.ts`)가 2초마다 상태를 받는다. 설치 단계·연결 끊김이면 App이 `Screen` 대신 재시작 화면(`upgrade/RestartScreen.tsx`)을 그려 데스크톱을 내리고(401로 세션이 끝나도 남는다), 상태 요청이 401이거나 끊긴 뒤 `/api/health`가 답하면 결과 힌트(`upgrade/resultHint.ts`, sessionStorage 키 하나. 로그인 상태가 아니다)를 남기고 새로 고친다(새 자산을 받기 위해). 로그인 화면은 힌트가 있으면 업그레이드 안내를, `Desktop`은 처음 그릴 때 힌트를 지우고 관리자면 업데이트 페이지를 연다
  - 명령을 보이는 곳은 `controls.tsx`의 `CommandBlock`(선택할 수 있는 `pre`와 복사 단추)을 쓴다. 서버 페이지와 함께 쓴다
- 보안 페이지(`security`, 사용자)와 보안 정책 페이지(`security-policy`, 관리자)는 보안 강화 모드의 화면이다. 확인·관리 대화상자와 수단 등록 대화상자는 `src/security/`에 있다(`.claude/rules/security.md`)
- 프로필 페이지(`profile`)는 사이드바 목록에 넣지 않고(`nav: 'card'`) 사이드바 맨 위 사용자 카드 버튼으로 연다. 볼 수 없는 페이지를 요청하면 기본 페이지(화면)로 간다
- 프로필 사진 자르기 편집기(`AvatarEditor.tsx`, 계산은 `profile/avatarCrop.ts`)는 창 범위 대화상자다. 끌기는 `usePointerDrag`, 휠은 passive가 아닌 네이티브 리스너로 받고, 저장은 512px WebP(안 되면 PNG)다. OMV 사용자 편집 화면은 워크벤치 앱과 같은 이름 붙은 탭으로 연다(`launchApp.ts`의 `openWorkbenchRoute`, 라우트는 `windows/links.ts`)
- 사이드바 틀은 `shell/SidebarLayout`이다(시스템 정보 창도 쓴다. 아래 "사이드바 틀"). 설정의 좁은 기준은 `SETTINGS_NARROW_WIDTH`(640px)이고, 사용자 카드·그룹·페이지 영역(`.settings-body`)은 설정 것이다
- 페이지 제목과 내용은 창 틀(`SettingsWindow.css`)이 창 내용 최대 폭(`--window-content-width`, 720px, 위 "창 내용 폭") 열로 가운데 정렬한다. 제목 머리(`.settings .sidebar-header`)와 `.settings-page`의 좌우 padding을 같은 퍼센트 식으로 계산해 스크롤바가 생겨도 왼쪽 끝이 맞는다. 페이지마다 폭을 따로 정하지 않는다
- 페이지가 함께 쓰는 틀과 컨트롤은 `controls.tsx`(절, 묶음 카드, 행, 스위치, 안내), 테마·배경 고르기는 `pickers.tsx`(숨긴 기본 라디오 버튼이라 키보드·화면 읽기는 브라우저 기본 동작)다
- 사용자 값은 바로 적용·저장한다(저장 큐). 전역 설정은 고른 값을 먼저 보여 주고 저장한 뒤 서버 값을 다시 받으며, 실패하면 되돌린다(`siteSettings.ts`)
- 페이지 보기는 창의 `active`를 받는다(`pageViews.ts`). 페이지 위에 겹쳐 그리는 레이어는 `.settings-page`(위치 지정 없음) 안에서 absolute로 그려 기준 상자 `.settings-body`(제목 머리 아래 페이지 영역)에 붙인다. 스크롤 컨테이너에 잘리거나 함께 스크롤되지 않는다
- 정보 페이지 대기 이스터에그(`ClawdParade.tsx`): Claude Code에 대한 오마주로, 정보 페이지에서 30초 동안 누르지 않으면(`shell/useIdle.ts`) Clawd가 페이지 영역 좌우 가장자리에서 한 마리씩 기어 들어와 자기 줄에서 쉬거나 배회하고, 아무 곳이나 누르면 모두 점프한 뒤 가까운 가장자리로 도망친다. 그림은 코드 안 12×8 픽셀 데이터(`about/clawd.ts`, 외부 이미지 파일 없음), 움직임 계산은 `about/clawdParade.ts`, 프레임마다 요소 style만 바꾼다. 창을 최소화하거나 닫거나 다른 페이지로 가면, 또는 애니메이션이 꺼져 있으면 연출 없이 없앤다. 탭이 숨겨진 동안은 없애지 않고 멈춰 두었다가 이어서 움직이며, 대기 시간은 숨겨진 동안에도 잰다(창의 `active`는 탭 숨김에도 false라 `usePageVisible`로 구분한다). 문구가 없어 현지화 대상이 아니다. 문제가 되면 통째로 지운다

## 사이드바 틀

- `SidebarLayout`(`shell/SidebarLayout.tsx`·`.css`)은 왼쪽 사이드바와 오른쪽 내용(제목 머리 + 본문)으로 나눈 창 내용 틀이다. 설정 앱, 시스템 정보 창, 파일 탐색기가 쓴다. 머리에는 제목(`title`) 대신 다른 내용(`header`, 파일 탐색기의 도구 막대)을 둘 수 있다. 항목은 쓰는 쪽이 그리고(공통 모양 `.sidebar-nav-item`, 지금 페이지에 `aria-current="page"`), 너비는 `--sidebar-width`로 바꾼다
- 창 너비가 `narrowWidth`보다 좁으면(ResizeObserver로 재서 `.narrow`) 사이드바를 숨기고 메뉴 단추(☰, `hamburger-menu-symbolic`)로 내용 위에 덮어 연다. 열면 지금 페이지 항목에 포커스하고, 열린 동안 내용은 `inert`이며, Esc·바깥 누르기·항목(`navItems`) 누르기로 닫는다. 닫힌 뒤(내용의 `inert`가 풀린 뒤) 메뉴 단추로 포커스를 돌려준다. 열기·닫기 값은 `sidebarAnim.ts`

## 화면 설정(테마·배경)

- 계산은 `appearance/`의 DOM 없는 모듈(`theme.ts`, `taskbar.ts`, `wallpapers.ts`, `wallpaperLayers.ts`, `store.ts`, `siteStore.ts`), 문서·localStorage는 `appearance/dom.ts`, 반영은 `appearance/useAppearance.ts`의 훅을 App이 부른다
- 테마는 `<html data-theme>`로 `color-scheme`을 덮는다(`base.css`). 로그인한 동안은 사용자 값 → 관리자 기본 테마, 로그인 화면은 이 브라우저의 마지막 테마(localStorage 힌트)를 쓴다(로그아웃하며 테마가 바뀌지 않게). `main.tsx`가 그리기 전에 힌트를 먼저 적용한다
- 배경은 App이 화면과 관계없이 두는 배경 레이어(`shell/Wallpaper.tsx`, `.wallpaper`)가 그리고 `.desktop`은 배경이 없다. 로그인·연결 끊김 화면은 로그인 배경, 데스크톱은 사용자 바탕화면 → 관리자 기본 바탕화면 → 기본 배경(`dusk`)이다. 모르는 프리셋·없는 이미지는 다음 후보로 넘어간다
- 배경이 바뀌면 새 레이어를 위에 겹쳐 투명도만 올린다(`wallpaperAnim.ts`). 이미지는 `decode()`가 끝난 뒤 시작하고 실패하면 대신 그릴 배경(`useWallpaperView`의 `fallback`, 보통 기본 배경)으로 바꾼다. 페이드 중에 또 바뀌면 그 위에 겹쳐 이어 간다. 사용자 설정을 받기 전에는 배경을 바꾸지 않아(마지막 바탕화면 힌트로 그림) 로그인 뒤 한 번만 바뀐다
  - 세션 확인 중에는 마지막 바탕화면 힌트를 그리되, 그 세션이 끝났을 것 같으면 로그인 배경 힌트를 그린다. 바탕화면 힌트에는 세션의 예상 만료 시각(인증 저장소의 `expiresAt`, 모르면 null)을 함께 남기고, 로그인 화면으로 오면(로그아웃, 세션 만료) 0으로 표시한다(`dom.ts`, 판단은 `wallpapers.ts`의 `viewBeforeSessionCheck`). 끝난 세션으로 사용자 이미지를 요청해 401을 받지 않게 하기 위해서다
  - 세션 확인 중에 바탕화면 이미지를 받지 못하면(백엔드 재시작처럼 예상과 달리 끝난 세션) 기본 배경을 거치지 않고 로그인 배경으로 바로 바꾼다(`fallback`)
  - 받는 중인 이미지는 key로만 버린다(effect 정리 함수로 버리지 않는다). 힌트는 그릴 때마다 새 객체이고 전역 정보를 다시 받으면 배경도 새 객체가 되므로, 정리 함수로 버리면 그 배경을 잃고 같은 key라 다시 받지도 않는다
  - 첫 레이어는 힌트를 확인 없이 바로 그리므로, 이미지면 확인되지 않은 레이어다(세션이 끝난 채 새로 고침하면 사용자 바탕화면이 401로 비어 있다). 첫 배경도 받아 확인하고, 같은 배경이라도 확인되지 않은 레이어는 확인된 새 레이어로 덮는다(`wallpaperLayers.ts`의 `pushLayer`)
- 바탕화면 아이콘 글자가 흰색이므로 프리셋은 어둡거나 중간 밝기만 둔다
