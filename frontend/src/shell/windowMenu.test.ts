import { describe, expect, it } from 'vitest'
import type { WindowState } from '../windows/store'
import type { ContextMenuAction, ContextMenuItem } from './contextMenu'
import { type TaskMenuActions, taskMenuItems, titlebarMenuItems } from './windowMenu'

const win = (id: string): WindowState => ({
  id,
  appId: 'files',
  instance: 1,
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

// recorder는 불린 동작을 기록하는 동작 묶음이다. closeMany는 calls 대신 closed에 남긴다.
function recorder() {
  const log = { closed: null as readonly string[] | null, calls: [] as string[] }
  const actions: TaskMenuActions = {
    closeMany: (ids) => {
      log.closed = ids
    },
    newWindow: (app) => log.calls.push(`newWindow ${app}`),
    minimize: (w) => log.calls.push(`minimize ${w}`),
    raiseGroup: (w) => log.calls.push(`raiseGroup ${w}`),
    restore: (w) => log.calls.push(`restore ${w}`),
    toggleMaximize: (w) => log.calls.push(`toggleMaximize ${w}`),
  }
  return { log, actions }
}

// select는 메뉴 항목 하나를 누른 결과로 닫히는 id 목록, 불린 동작 순서, 그 항목을 돌려준다.
function select(build: (actions: TaskMenuActions) => ContextMenuItem[], item: string) {
  const { log, actions } = recorder()
  const action = build(actions).find((i): i is ContextMenuAction => i.kind === 'action' && i.id === item)
  if (!action) throw new Error(`item ${item} not found`)
  action.onSelect()
  return { ...log, action }
}

const selectTask = (ws: readonly WindowState[], id: string, item: string) =>
  select((actions) => taskMenuItems(ws, id, actions), item)

const ids = (items: ContextMenuItem[]) => items.map((i) => i.id)

describe('taskMenuItems', () => {
  it('최소화는 그 창을 최소화한다', () => {
    expect(selectTask(windows, 'b', 'minimize').calls).toEqual(['minimize b'])
  })

  it('최소화된 창이면 최소화 대신 복원(짝 창과 함께)이 있다', () => {
    const minimized = [{ ...win('a'), minimized: true }]
    expect(selectTask(minimized, 'a', 'restore').calls).toEqual(['raiseGroup a'])
    expect(() => selectTask(minimized, 'a', 'minimize')).toThrow()
  })

  it('최대화는 최대화를 바꾸고, 최대화된 창이면 이전 크기로 표시한다', () => {
    const r = selectTask(windows, 'b', 'maximize')
    expect(r.calls).toEqual(['toggleMaximize b'])
    expect(r.action.labelKey).toBe('window.maximize')
    expect(selectTask([{ ...win('a'), maximized: true }], 'a', 'maximize').action.labelKey).toBe('window.restore')
  })

  it('최소화된 창을 최대화하면 복원도 함께 한다', () => {
    expect(selectTask([{ ...win('a'), minimized: true }], 'a', 'maximize').calls).toEqual([
      'toggleMaximize a',
      'restore a',
    ])
  })

  it('닫기는 메뉴 맨 아래에 있다(버튼 위로 열리는 메뉴에서 누른 자리 가까이)', () => {
    expect(ids(taskMenuItems(windows, 'b', recorder().actions)).at(-1)).toBe('close')
  })

  it('닫기는 그 창만 닫는다', () => {
    expect(selectTask(windows, 'b', 'close').closed).toEqual(['b'])
  })

  it('다른 항목 닫기는 그 창 외에 모두 닫는다', () => {
    expect(selectTask(windows, 'b', 'close-others').closed).toEqual(['a', 'c', 'd'])
  })

  it('오른쪽 항목 닫기는 작업표시줄 순서에서 뒤쪽 창을 닫는다', () => {
    expect(selectTask(windows, 'b', 'close-right').closed).toEqual(['c', 'd'])
  })

  it('모두 닫기는 모든 창을 닫는다', () => {
    expect(selectTask(windows, 'b', 'close-all').closed).toEqual(['a', 'b', 'c', 'd'])
  })

  it('맨 끝 창이면 오른쪽 항목 닫기가 비활성이다', () => {
    expect(selectTask(windows, 'd', 'close-right').action.disabled).toBe(true)
    expect(selectTask(windows, 'a', 'close-right').action.disabled).toBe(false)
  })

  it('창이 하나면 다른 항목 닫기가 비활성이다', () => {
    expect(selectTask([win('a')], 'a', 'close-others').action.disabled).toBe(true)
  })

  it('없는 창이면 항목이 없다', () => {
    expect(taskMenuItems(windows, 'x', recorder().actions)).toEqual([])
  })

  it('창을 여러 개 여는 앱이면 맨 위의 새 창이 같은 앱의 새 창을 연다', () => {
    const terminals: WindowState[] = [{ ...win('a'), appId: 'terminal' }, win('b')]
    expect(ids(taskMenuItems(terminals, 'a', recorder().actions)).slice(0, 3)).toEqual([
      'new-window',
      'sep-new',
      'minimize',
    ])
    expect(selectTask(terminals, 'a', 'new-window').calls).toEqual(['newWindow terminal'])
  })

  it('창이 하나뿐인 앱에는 새 창이 없다', () => {
    expect(ids(taskMenuItems(windows, 'b', recorder().actions))).not.toContain('new-window')
  })
})

describe('titlebarMenuItems', () => {
  it('최소화, 최대화 / 닫기 순서다', () => {
    expect(ids(titlebarMenuItems(win('a'), recorder().actions))).toEqual(['minimize', 'maximize', 'sep-close', 'close'])
  })

  it('각 항목이 그 창에 동작한다', () => {
    const selectTitlebar = (w: WindowState, item: string) => select((actions) => titlebarMenuItems(w, actions), item)
    expect(selectTitlebar(win('a'), 'minimize').calls).toEqual(['minimize a'])
    expect(selectTitlebar(win('a'), 'maximize').calls).toEqual(['toggleMaximize a'])
    expect(selectTitlebar(win('a'), 'close').closed).toEqual(['a'])
    expect(selectTitlebar({ ...win('a'), maximized: true }, 'maximize').action.labelKey).toBe('window.restore')
  })
})
