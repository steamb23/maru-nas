// 작업표시줄 버튼의 우클릭 메뉴 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { WindowState } from '../windows/store'
import type { ContextMenuItem } from './contextMenu'

// taskMenuItems는 창 id의 작업표시줄 버튼 메뉴 항목이다. "오른쪽"은 windows[](작업표시줄) 순서 기준이다.
// 닫을 창 목록은 메뉴를 연 시점의 windows로 정한다. 그 사이 닫힌 창은 closeMany가 무시한다.
// id가 windows에 없으면 빈 목록(메뉴를 열지 않음)이다.
export function taskMenuItems(
  windows: readonly WindowState[],
  id: string,
  closeMany: (ids: readonly string[]) => void,
): ContextMenuItem[] {
  const index = windows.findIndex((w) => w.id === id)
  if (index < 0) return []
  const all = windows.map((w) => w.id)
  const others = all.filter((o) => o !== id)
  const right = all.slice(index + 1)
  return [
    {
      kind: 'action',
      id: 'close',
      labelKey: 'window.close',
      icon: 'window-close-symbolic',
      onSelect: () => closeMany([id]),
    },
    { kind: 'separator', id: 'sep' },
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
  ]
}
