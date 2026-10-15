---
paths:
  - "frontend/src/shell/**"
  - "frontend/src/windows/**"
  - "frontend/src/widgets/**"
  - "frontend/src/shortcuts/**"
  - "frontend/src/App.tsx"
  - "frontend/src/index.css"
  - "frontend/src/base.css"
---

# 데스크톱 셸과 창 시스템

`src/windows/`(창 저장소·앱 목록·위치 제한·타일링), `src/widgets/`(위젯 그리드·배치·저장소), `src/shortcuts/`(바로가기 목록·저장소), `src/shell/`의 `*Anim.ts`, `taskbarDrag.ts`, `clock.ts`, `profileMenu.ts`, `contextMenu.ts`, `contextMenuStore.ts`, `windowMenu.ts`, `menuNav.ts`, `modalStack.ts`, `dialogs.ts`, `appDrawerStore.ts`, `appDrawerPrefsStore.ts`, `appSort.ts`, `gridNav.ts`, `appSearch.ts`, `hangul.ts`는 DOM에 의존하지 않는 계산 모듈이다. 컴포넌트는 이 결과를 그리기만 한다.

## 창 순서와 DOM

- `windows[]` 순서가 작업표시줄 순서다
- `Desktop`은 `stableOrder`로 창을 처음 연 순서대로 그린다. 작업표시줄 순서를 바꿔도 창 DOM이 옮겨지면 안 된다(옮겨지면 창 안의 스크롤 위치·iframe이 초기화된다)
- 위젯은 창 시스템과 분리한다. `windows[]`와 창 컴포넌트에 위젯용 분기를 넣지 않는다(`.claude/docs/addons.md`의 "보기: 창과 위젯")
- 창을 끄는 동안 위치는 저장소를 바꾸지 않고 요소 style(`left`·`top`)만 바꾸며, 놓거나 취소될 때 `moveTo`로 한 번 반영한다(style은 지우지 않는다). 떼어 내기(`detachAt`)만 끄는 중에 저장소를 바꾼다
- 창 내용은 `memo`한 `WindowBody`로 그린다. 창 틀이 다시 그려져도(포커스, 크기 조절, 타일 가이드) 내용은 다시 그리지 않는다

## 위젯 레이어

- 설계와 결정 사항은 `.claude/docs/addons.md`의 "위젯 시스템"에 있다
- `WidgetLayer`는 `.desktop-area` 안에서 아이콘 다음, 창 앞(DOM 순서)에 그린다. 평소에는 창 아래이고, 위젯을 끌거나 크기를 바꾸는 동안만 창 위로 올라온다(`.widget-layer.raised`)
- `src/widgets/`와 `src/windows/`는 서로 import하지 않는다. 위젯이 창을 여는 일은 `shell/WidgetFrame.tsx`가 창 저장소의 `open`을 불러 처리한다
- 저장 배치는 `widgets/store.ts`, 화면 배치는 `fitLayout`으로 렌더링 때 계산한다. 저장 배치는 사용자가 바꾼 위젯만 `commitPlacement`로 바꾼다(화면 크기 변화로 바꾸지 않는다)
- 끄는 동안 위치·크기는 렌더링 없이 요소 style을 직접 바꾼다. 끝나면 지우지 말고 지금 자리의 값으로 되돌린다(React는 값이 바뀌지 않은 style을 다시 넣지 않는다)
- 포인터 끌기 공통 처리는 `usePointerDrag`(임계 거리, pointer capture, 취소)를 쓴다. 창(`Window.tsx`)의 끌기 코드는 아직 이 훅으로 옮기지 않았다
- 가이드(`WidgetPreview`)는 타일링 가이드와 같은 모양(`.tile-preview`)을 쓰고, 칸이 바뀌면 임계 감쇠 스프링으로 따라간다(`widgetPreviewAnim.ts`). 끄는 동안 렌더 없이 핸들(`moveTo`)로 움직이고, 놓을 수 없는 자리에서는 숨기기만 한다

## 바로가기와 모든 앱

- 바탕화면에는 사용자가 고른 바로가기만 놓는다(`DesktopShortcuts`). 모든 앱은 작업표시줄 왼쪽 끝 버튼으로 여는 "모든 앱" 화면(`AppDrawer`)에 있다. 화면 용어는 "바로가기"(붙여 씀)와 "모든 앱", 코드 이름은 `shortcut`과 `AppDrawer`다
- 앱은 창 앱(`addon`, `placeholder`)과 링크 앱(`link`, 창 없이 브라우저 탭으로 연다)이다(`windows/apps.ts`). 창 저장소는 `WindowAppId`만 받으므로, 앱을 여는 곳(서랍 타일·바로가기·메뉴 "열기")은 `shell/launchApp.ts`의 `launchApp`을 부른다
  - 링크 앱(지금은 OMV 워크벤치)은 `openLink`가 `window.open(주소, 탭 이름)`으로 이름 붙은 탭 하나를 다시 쓴다. 이름 조회가 되도록 `noopener`를 주지 않으며, 팝업 차단을 피하려고 입력 처리기 안에서 동기로 부른다. 주소 계산은 `windows/links.ts`(끝에 OMV 첫 화면 `#/`를 붙여, 다시 열 때 새로 고침 없이 이동한다)
  - 열기 전에 "새 탭에서 열립니다" 데스크톱 대화상자를 띄운다(`linkNoticeDialog.tsx`의 `confirmLinkOpen`, 내용은 `LinkNotice.tsx`). 탭은 "열기" 클릭 처리 안에서 바로 연다(결과를 기다린 뒤 열면 팝업 차단). "다음에 알리지 않음"은 브라우저별 편의 설정이라 쿠키(`onmaru_link_notice_<id>=off`, 1년, Path 없음)에 두며 계산은 `linkNotice.ts`
  - 링크 앱은 주소(`/api/capabilities`의 `workbenchUrl`)가 있을 때만 보인다. capability를 받기 전에도 숨긴다(`appAccess`의 예외). 프로필 메뉴의 "OMV 워크벤치 열기"는 `requires: 'workbench'`로 같은 조건을 따른다
- 목록은 `shortcuts/store.ts`(사용자별 `/api/preferences`의 `shortcuts`, 불러오기·저장 방식은 위젯 저장소와 같다), 순서 계산은 `shortcuts/list.ts`, 칸 계산은 `shortcuts/layout.ts`가 한다
  - 저장 목록에는 지금 보이지 않는 앱(일반 사용자의 관리자 전용 앱)도 남긴다. 화면 순서와 놓을 자리는 보이는 목록 기준이며(`placeShortcut`), 보이지 않는 앱은 서로의 순서를 지킨다
  - 한 번도 저장하지 않았으면 기본 목록(시스템 정보·파일·저장소), 모두 지웠으면 빈 바탕화면이다. 모르는 앱 id는 불러올 때 뺀다
- 앱 이름(바탕화면 아이콘, 서랍 타일)은 띄어쓰기에서만 줄을 바꾸고(`word-break: keep-all`), 한 단어가 한 줄보다 길 때만 글자 사이에서 자른다(`overflow-wrap: anywhere`). 한글 이름이 단어 중간에서 잘리지 않게 하기 위해서다
- 배치는 왼쪽 위부터 위에서 아래로, 넘치면 다음 열이다(`grid-auto-flow: column`). `layout.ts`의 상수와 `rowsPerColumn`은 CSS(`.desktop-icons`, `.desktop-icon`)와 맞춘다. 끄는 동안 쓰는 한 열의 아이콘 수는 그려진 목록(`gridTemplateRows`)에서 잰다(브라우저 확대로 영역 높이가 소수점이면 반올림한 `bounds`로 계산한 값과 다를 수 있다)
- 위젯 레이어는 바로가기를 불러온 뒤(`iconsReady` = 저장소의 `loaded`)에 그린다. 먼저 그리면 아이콘 목록 너비가 0이라 위젯이 아이콘 자리에 놓였다가 밀려난다. `loaded`는 한 번 불러오면(실패 포함) 다시 시도하는 동안에도 남아, 재시도 때 위젯이 다시 마운트되지 않는다
- 메뉴: 바로가기 → "열기"·"바로가기 제거", 빈 바탕화면 → "바로가기 추가 ▸"(이미 있는 앱은 체크·비활성), 서랍 타일 → "열기"·"바탕화면에 바로가기 추가" 또는 "바로가기 제거"
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
  - 검색(`appSearch.ts`)은 지원 언어 모두의 앱 이름과 앱 id로 화면 언어와 무관하게 찾고, 한글은 자모로 풀어 초성과 조합 중인 글자도 맞춘다(`hangul.ts`). 후보는 `i18n.getFixedT(언어)`로 만들므로 모든 언어 번들이 로드되어 있어야 한다(`i18n.md`)
  - 열기·닫기 애니메이션 값은 `appDrawerAnim.ts`, 열림 상태는 `appDrawerStore.ts`다. 로그인·로그아웃 전환에서 데스크톱과 함께 움직인다(`screenTransition.ts`의 `DESKTOP_PARTS`)

## 컨텍스트 메뉴

- 우클릭 메뉴는 모두 `contextMenu.ts`의 항목 모델(`action`, `separator`, `submenu`는 한 단계까지)로 만들고 `useContextMenuTrigger`로 연다. 화면에는 `Desktop`이 그리는 `ContextMenu` 하나만 있다(한 번에 메뉴 하나)
- `useContextMenuTrigger`가 우클릭, 키보드(메뉴 키, Shift+F10), 터치 길게 누르기를 함께 처리한다. 길게 눌러 메뉴를 연 누름은 끌기·클릭으로 이어 가지 않게 `openedByPress()`로 확인한다. 다른 포인터 처리기와는 `mergeHandlers`로 합친다
- 브라우저 기본 메뉴는 `nativeGuard`가 막는다(capture 단계 `preventDefault`라 React `onContextMenu`는 그대로 불린다)
- 바탕화면 메뉴는 빈 곳(아이콘 사이 포함)에서만 연다. 메뉴 항목은 기능을 가진 쪽이 만든다(바로가기 추가는 `DesktopShortcuts`, 위젯 추가는 `WidgetLayer`의 `desktopMenuItems`)
- 창 메뉴 항목은 `windowMenu.ts`가 만든다. 작업표시줄 버튼 메뉴(`taskMenuItems`)와 제목 표시줄 메뉴(`titlebarMenuItems`)가 상태 항목(최소화·복원, 최대화·이전 크기로)을 함께 쓴다. 여러 창은 저장소의 `closeMany`로 한 번에 닫는다
  - 작업표시줄: 상태 항목 / 다른 항목·오른쪽 항목·모두 닫기 / 닫기. 메뉴가 버튼 위로 열리므로 닫기는 누른 자리 가까이 오도록 맨 아래에 둔다. 최소화된 창에는 최소화 대신 복원(`raiseGroup`, 버튼 클릭과 같음)을 두고, 최소화된 창을 최대화하면 복원도 함께 한다. "오른쪽"은 `windows[]`(작업표시줄) 순서 기준이다. 메뉴는 `Taskbar`의 트리거 하나로 열고, 대상 창은 `items(el)`이 받는 버튼의 `data-window-id`로 찾는다. 끄는 중에는 열지 않는다
  - 제목 표시줄: 상태 항목 / 닫기. 제목 표시줄 버튼 위에서는 열지 않고(길게 누르면 버튼이 눌리지 않게), 창을 끄는 중에는 열지 않으며, 메뉴를 연 누름은 창 끌기로 이어 가지 않는다
- 비활성 항목은 `aria-disabled`로 두어 포커스는 받게 한다. 이유가 있으면 `detailKey`로 짧게 보여 준다
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
- 창 밖(작업표시줄 메뉴)에서 배치를 바꿀 때는 `captureWindowLayout(id)`로 그 창의 `captureFrom`을 부른다. 숨겨진(최소화된) 창은 재지 않고 복원 애니메이션이 새 배치로 재생된다
- 창 애니메이션 켜기/끄기는 `WINDOW_ANIMATIONS` 상수다. 설정 기능이 생기면 그쪽으로 옮긴다
- 닫기: 저장소의 `close`는 창을 바로 지워 작업표시줄·포커스가 즉시 바뀐다. `Desktop`은 `useClosingWindows`로 지워진 창의 마지막 상태와 타일 비율(`ClosingWindow`)을 받아 두고, 애니메이션이 끝나 `onClosed`가 불릴 때까지 그 상태로 그린다(`stableOrder`의 `keep`). 닫히는 창은 클릭을 받지 않고 창 내용의 주기 요청도 바로 멈춘다
- 작업표시줄 버튼: 등장은 `li`의 ref(`enterTask`), 퇴장은 `Taskbar`가 너비·투명도를 애니메이션한다. 닫힌 창의 버튼은 `useClosingWindows`로 받아 `.leaving` 버튼으로 남긴다(`withLeaving`)
- 작업표시줄 드래그 정렬: 드래그 중에는 transform으로만 움직이고 놓은 뒤 순서를 반영한다. 전환 효과는 부모가 아닌 각 `li` 클래스에 둔다. 배치 계산에서 `.leaving` 버튼은 뺀다
- 로그인 ↔ 데스크톱 전환(`screenAnim.ts`, 재생은 `screenTransition.ts`): 배경(`.desktop`)은 그대로 두고 내용(`.login-card`, `.desktop-area`·`.taskbar`)만 움직인다. 나가는 애니메이션은 auth 저장소가 화면을 바꾸기 전에 `beforeScreenChange` 훅(`leaveScreen`)으로, 들어오는 애니메이션은 각 화면이 처음 그려질 때 재생한다

## 작업표시줄 트레이와 프로필 메뉴

- 트레이는 `[시계][프로필]` 순서이며 프로필 버튼이 오른쪽 끝에 고정된다
- 프로필 메뉴 항목은 `profileMenu.ts`의 `PROFILE_MENU_ITEMS`에 데이터로 둔다(`requires`로 거름: `admin`은 관리자만, `workbench`는 OMV 웹 UI 주소가 있을 때만). 항목은 `action`과 `submenu` 두 종류다. `action` 항목을 추가하면 `ProfileMenu.tsx`의 `actions`에 동작도 넣는다
- 아바타 그리기는 `ProfileAvatar` 한 곳에 모은다
