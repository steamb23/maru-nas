import { describe, expect, it } from 'vitest'
import type { WindowState } from '../windows/store'
import type { ContextMenuAction } from './contextMenu'
import { taskMenuItems } from './taskbarMenu'

const win = (id: string): WindowState => ({
  id,
  appId: 'files',
  x: 0,
  y: 0,
  width: 100,
  height: 100,
  z: 1,
  minimized: false,
  maximized: false,
  tile: null,
})

const windows = ['a', 'b', 'c', 'd'].map(win)

// selectItem은 메뉴 항목 하나를 누른 결과로 닫히는 id 목록과 그 항목을 돌려준다.
function selectItem(ws: readonly WindowState[], id: string, item: string) {
  let closed: readonly string[] | null = null
  const items = taskMenuItems(ws, id, (ids) => {
    closed = ids
  })
  const action = items.find((i): i is ContextMenuAction => i.kind === 'action' && i.id === item)
  if (!action) throw new Error(`item ${item} not found`)
  action.onSelect()
  return { closed, action }
}

describe('taskMenuItems', () => {
  it('닫기는 그 창만 닫는다', () => {
    expect(selectItem(windows, 'b', 'close').closed).toEqual(['b'])
  })

  it('다른 항목 닫기는 그 창 외에 모두 닫는다', () => {
    expect(selectItem(windows, 'b', 'close-others').closed).toEqual(['a', 'c', 'd'])
  })

  it('오른쪽 항목 닫기는 작업표시줄 순서에서 뒤쪽 창을 닫는다', () => {
    expect(selectItem(windows, 'b', 'close-right').closed).toEqual(['c', 'd'])
  })

  it('모두 닫기는 모든 창을 닫는다', () => {
    expect(selectItem(windows, 'b', 'close-all').closed).toEqual(['a', 'b', 'c', 'd'])
  })

  it('맨 끝 창이면 오른쪽 항목 닫기가 비활성이다', () => {
    expect(selectItem(windows, 'd', 'close-right').action.disabled).toBe(true)
    expect(selectItem(windows, 'a', 'close-right').action.disabled).toBe(false)
  })

  it('창이 하나면 다른 항목 닫기가 비활성이다', () => {
    expect(selectItem([win('a')], 'a', 'close-others').action.disabled).toBe(true)
  })

  it('없는 창이면 항목이 없다', () => {
    expect(taskMenuItems(windows, 'x', () => {})).toEqual([])
  })
})
