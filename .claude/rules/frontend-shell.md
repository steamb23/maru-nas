---
paths:
  - "frontend/src/shell/**"
  - "frontend/src/windows/**"
  - "frontend/src/widgets/**"
  - "frontend/src/App.tsx"
  - "frontend/src/index.css"
---

# 데스크톱 셸과 창 시스템

`src/windows/`(창 저장소·앱 목록·위치 제한·타일링), `src/widgets/`(위젯 그리드·배치·저장소), `src/shell/`의 `*Anim.ts`, `taskbarDrag.ts`, `clock.ts`, `profileMenu.ts`, `contextMenu.ts`, `contextMenuStore.ts`, `menuNav.ts`는 DOM에 의존하지 않는 계산 모듈이다. 컴포넌트는 이 결과를 그리기만 한다.

## 창 순서와 DOM

- `windows[]` 순서가 작업표시줄 순서다
- `Desktop`은 `stableOrder`로 창을 처음 연 순서대로 그린다. 작업표시줄 순서를 바꿔도 창 DOM이 옮겨지면 안 된다(옮겨지면 창 안의 스크롤 위치·iframe이 초기화된다)
- 위젯은 창 시스템과 분리한다. `windows[]`와 창 컴포넌트에 위젯용 분기를 넣지 않는다(`docs/addons.md`의 "보기: 창과 위젯")

## 위젯 레이어

- 설계와 결정 사항은 `docs/addons.md`의 "위젯 시스템"에 있다
- `WidgetLayer`는 `.desktop-area` 안에서 아이콘 다음, 창 앞(DOM 순서)에 그린다. 평소에는 창 아래이고, 위젯을 끌거나 크기를 바꾸는 동안만 창 위로 올라온다(`.widget-layer.raised`)
- `src/widgets/`와 `src/windows/`는 서로 import하지 않는다. 위젯이 창을 여는 일은 `shell/WidgetFrame.tsx`가 창 저장소의 `open`을 불러 처리한다
- 저장 배치는 `widgets/store.ts`, 화면 배치는 `fitLayout`으로 렌더링 때 계산한다. 저장 배치는 사용자가 바꾼 위젯만 `commitPlacement`로 바꾼다(화면 크기 변화로 바꾸지 않는다)
- 끄는 동안 위치·크기는 렌더링 없이 요소 style을 직접 바꾼다. 끝나면 지우지 말고 지금 자리의 값으로 되돌린다(React는 값이 바뀌지 않은 style을 다시 넣지 않는다)
- 포인터 끌기 공통 처리는 `usePointerDrag`(임계 거리, pointer capture, 취소)를 쓴다. 창(`Window.tsx`)의 끌기 코드는 아직 이 훅으로 옮기지 않았다
- 가이드(`WidgetPreview`)는 타일링 가이드와 같은 모양(`.tile-preview`)을 쓰고, 칸이 바뀌면 FLIP으로 미끄러진다(`widgetPreviewAnim.ts`)

## 컨텍스트 메뉴

- 우클릭 메뉴는 모두 `contextMenu.ts`의 항목 모델(`action`, `separator`, `submenu`는 한 단계까지)로 만들고 `useContextMenuTrigger`로 연다. 화면에는 `Desktop`이 그리는 `ContextMenu` 하나만 있다(한 번에 메뉴 하나)
- `useContextMenuTrigger`가 우클릭, 키보드(메뉴 키, Shift+F10), 터치 길게 누르기를 함께 처리한다. 길게 눌러 메뉴를 연 누름은 끌기·클릭으로 이어 가지 않게 `openedByPress()`로 확인한다. 다른 포인터 처리기와는 `mergeHandlers`로 합친다
- 브라우저 기본 메뉴는 `nativeGuard`가 막는다(capture 단계 `preventDefault`라 React `onContextMenu`는 그대로 불린다)
- 바탕화면 메뉴는 빈 곳(아이콘 사이 포함)에서만 연다. 메뉴 항목은 기능을 가진 쪽이 만든다(위젯 추가는 `WidgetLayer`의 `desktopMenuItems`)
- 비활성 항목은 `aria-disabled`로 두어 포커스는 받게 한다. 이유가 있으면 `detailKey`로 짧게 보여 준다
- 키보드 이동(`menuNav.ts`), 포커스 이동(`menuFocus.ts`), 열기·닫기 애니메이션 값(`menuAnim.ts`)은 프로필 메뉴와 함께 쓴다

## 타일링

- 코드에서는 "타일링(tile)"이라는 용어만 쓴다
- 창을 끄는 포인터가 위 가장자리에 닿으면 최대화, 좌우 가장자리에 닿으면 반쪽 배치(모서리에서는 좌우 우선)
- 최대화·타일 창의 위치는 `tileRect` 하나로 계산한다. 가이드(`TilePreview`)도 같은 함수를 쓴다
- 좌우 타일 창은 `tileSplit` 하나를 공유하는 타일 그룹이다. 경계(`TileResizeHandle`, `TileDivider`, 공용 훅 `useTileSplitDrag`)를 끌면 함께 크기가 바뀌고, 작업표시줄에서 누르면 짝 창도 함께 앞으로 온다(`raiseGroup`)

## 애니메이션

- 재생은 Web Animations API로 하며 transform·opacity만 움직인다. 설정값과 FLIP 계산은 DOM 없는 `*Anim.ts`에 둔다
- 창 애니메이션 켜기/끄기는 `WINDOW_ANIMATIONS` 상수다. 설정 기능이 생기면 그쪽으로 옮긴다
- 닫기: 저장소의 `close`는 창을 바로 지워 작업표시줄·포커스가 즉시 바뀐다. `Desktop`은 `useClosingWindows`로 지워진 창의 마지막 상태와 타일 비율(`ClosingWindow`)을 받아 두고, 애니메이션이 끝나 `onClosed`가 불릴 때까지 그 상태로 그린다(`stableOrder`의 `keep`). 닫히는 창은 클릭을 받지 않고 창 내용의 주기 요청도 바로 멈춘다
- 작업표시줄 버튼: 등장은 `li`의 ref(`enterTask`), 퇴장은 `Taskbar`가 너비·투명도를 애니메이션한다. 닫힌 창의 버튼은 `useClosingWindows`로 받아 `.leaving` 버튼으로 남긴다(`withLeaving`)
- 작업표시줄 드래그 정렬: 드래그 중에는 transform으로만 움직이고 놓은 뒤 순서를 반영한다. 전환 효과는 부모가 아닌 각 `li` 클래스에 둔다. 배치 계산에서 `.leaving` 버튼은 뺀다
- 로그인 ↔ 데스크톱 전환(`screenAnim.ts`, 재생은 `screenTransition.ts`): 배경(`.desktop`)은 그대로 두고 내용(`.login-card`, `.desktop-area`·`.taskbar`)만 움직인다. 나가는 애니메이션은 auth 저장소가 화면을 바꾸기 전에 `beforeScreenChange` 훅(`leaveScreen`)으로, 들어오는 애니메이션은 각 화면이 처음 그려질 때 재생한다

## 작업표시줄 트레이와 프로필 메뉴

- 트레이는 `[시계][프로필]` 순서이며 프로필 버튼이 오른쪽 끝에 고정된다
- 프로필 메뉴 항목은 `profileMenu.ts`의 `PROFILE_MENU_ITEMS`에 데이터로 둔다(`requires: 'admin'`으로 권한별로 거름). 항목은 `action`과 `submenu` 두 종류다. `action` 항목을 추가하면 `ProfileMenu.tsx`의 `actions`에 동작도 넣는다
- 아바타 그리기는 `ProfileAvatar` 한 곳에 모은다
