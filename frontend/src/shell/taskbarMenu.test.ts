import { describe, expect, it } from 'vitest'
import type { WindowState } from '../windows/store'
import type { ContextMenuAction } from './contextMenu'
import { type TaskMenuActions, taskMenuItems } from './taskbarMenu'

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

// selectItem은 메뉴 항목 하나를 누른 결과로 닫히는 id 목록, 불린 동작 순서, 그 항목을 돌려준다.
function selectItem(ws: readonly WindowState[], id: string, item: string) {
  let closed: readonly string[] | null = null
  const calls: string[] = []
  const actions: TaskMenuActions = {
    closeMany: (ids) => {
      closed = ids
    },
    minimize: (w) => calls.push(`minimize ${w}`),
    raiseGroup: (w) => calls.push(`raiseGroup ${w}`),
    restore: (w) => calls.push(`restore ${w}`),
    toggleMaximize: (w) => calls.push(`toggleMaximize ${w}`),
  }
  const items = taskMenuItems(ws, id, actions)
  const action = items.find((i): i is ContextMenuAction => i.kind === 'action' && i.id === item)
  if (!action) throw new Error(`item ${item} not found`)
  action.onSelect()
  return { closed, calls, action }
}

describe('taskMenuItems', () => {
  it('최소화는 그 창을 최소화한다', () => {
    expect(selectItem(windows, 'b', 'minimize').calls).toEqual(['minimize b'])
  })

  it('최소화된 창이면 최소화 대신 복원(짝 창과 함께)이 있다', () => {
    const minimized = [{ ...win('a'), minimized: true }]
    expect(selectItem(minimized, 'a', 'restore').calls).toEqual(['raiseGroup a'])
    expect(() => selectItem(minimized, 'a', 'minimize')).toThrow()
  })

  it('최대화는 최대화를 바꾸고, 최대화된 창이면 이전 크기로 표시한다', () => {
    const r = selectItem(windows, 'b', 'maximize')
    expect(r.calls).toEqual(['toggleMaximize b'])
    expect(r.action.labelKey).toBe('window.maximize')
    expect(selectItem([{ ...win('a'), maximized: true }], 'a', 'maximize').action.labelKey).toBe('window.restore')
  })

  it('최소화된 창을 최대화하면 복원도 함께 한다', () => {
    expect(selectItem([{ ...win('a'), minimized: true }], 'a', 'maximize').calls).toEqual([
      'toggleMaximize a',
      'restore a',
    ])
  })

  it('닫기는 메뉴 맨 아래에 있다(버튼 위로 열리는 메뉴에서 누른 자리 가까이)', () => {
    const items = taskMenuItems(windows, 'b', { closeMany() {}, minimize() {}, raiseGroup() {}, restore() {}, toggleMaximize() {} })
    expect(items.at(-1)?.id).toBe('close')
  })

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
    const noop = () => {}
    const actions = { closeMany: noop, minimize: noop, raiseGroup: noop, restore: noop, toggleMaximize: noop }
    expect(taskMenuItems(windows, 'x', actions)).toEqual([])
  })
})
