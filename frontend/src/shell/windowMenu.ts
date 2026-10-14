// 창 우클릭 메뉴(작업표시줄 버튼, 제목 표시줄)의 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { WindowStore, WindowState } from '../windows/store'
import type { ContextMenuAction, ContextMenuItem } from './contextMenu'

// WindowMenuActions는 메뉴 항목이 부르는 창 저장소 동작이다.
// 최대화 애니메이션처럼 화면 처리가 더 필요하면 호출하는 쪽이 감싸서 넘긴다.
export type WindowMenuActions = Pick<WindowStore, 'closeMany' | 'minimize' | 'raiseGroup' | 'restore' | 'toggleMaximize'>

// stateItems는 창 상태를 바꾸는 항목(최소화·복원, 최대화·이전 크기로)이다.
function stateItems(win: WindowState, actions: WindowMenuActions): ContextMenuAction[] {
  const { id } = win
  return [
    // 최소화된 창이면 복원한다. 작업표시줄 버튼 클릭처럼 타일 그룹의 짝 창도 함께 앞으로 온다.
    win.minimized
      ? { kind: 'action', id: 'restore', labelKey: 'taskbar.restore', onSelect: () => actions.raiseGroup(id) }
      : {
          kind: 'action',
          id: 'minimize',
          labelKey: 'window.minimize',
          icon: 'window-minimize-symbolic',
          onSelect: () => actions.minimize(id),
        },
    {
      kind: 'action',
      id: 'maximize',
      labelKey: win.maximized ? 'window.restore' : 'window.maximize',
      icon: win.maximized ? 'window-restore-symbolic' : 'window-maximize-symbolic',
      // 최소화된 창이면 복원도 함께 해 바뀐 배치가 보이게 한다.
      onSelect: () => {
        actions.toggleMaximize(id)
        if (win.minimized) actions.restore(id)
      },
    },
  ]
}

const closeItem = (id: string, actions: WindowMenuActions): ContextMenuAction => ({
  kind: 'action',
  id: 'close',
  labelKey: 'window.close',
  icon: 'window-close-symbolic',
  onSelect: () => actions.closeMany([id]),
})

// titlebarMenuItems는 창 제목 표시줄의 메뉴 항목이다.
export function titlebarMenuItems(win: WindowState, actions: WindowMenuActions): ContextMenuItem[] {
  return [...stateItems(win, actions), { kind: 'separator', id: 'sep-close' }, closeItem(win.id, actions)]
}

// taskMenuItems는 창 id의 작업표시줄 버튼 메뉴 항목이다. "오른쪽"은 windows[](작업표시줄) 순서 기준이다.
// 닫을 창 목록은 메뉴를 연 시점의 windows로 정한다. 그 사이 닫힌 창은 closeMany가 무시한다.
// id가 windows에 없으면 빈 목록(메뉴를 열지 않음)이다.
export function taskMenuItems(windows: readonly WindowState[], id: string, actions: WindowMenuActions): ContextMenuItem[] {
  const index = windows.findIndex((w) => w.id === id)
  if (index < 0) return []
  const all = windows.map((w) => w.id)
  const others = all.filter((o) => o !== id)
  const right = all.slice(index + 1)
  const { closeMany } = actions
  return [
    ...stateItems(windows[index], actions),
    { kind: 'separator', id: 'sep-window' },
    {
      kind: 'action',
      id: 'close-others',
      labelKey: 'taskbar.closeOthers',
      disabled: others.length === 0,
      onSelect: () => closeMany(others),
    },
    {
      kind: 'action',
      id: 'close-right',
      labelKey: 'taskbar.closeRight',
      disabled: right.length === 0,
      onSelect: () => closeMany(right),
    },
    { kind: 'action', id: 'close-all', labelKey: 'taskbar.closeAll', onSelect: () => closeMany(all) },
    // 작업표시줄이 화면 아래에 있어 메뉴는 버튼 위로 열린다. 닫기를 맨 아래에 두어 누른 자리 가까이 오게 한다.
    { kind: 'separator', id: 'sep-close' },
    closeItem(id, actions),
  ]
}
