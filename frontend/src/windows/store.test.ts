import { beforeEach, describe, expect, it } from 'vitest'
import { MIN_VISIBLE, TITLEBAR_HEIGHT } from './geometry'
import { type WindowState, initialWindowData, selectActiveId, useWindowStore } from './store'

const bounds = { width: 1280, height: 720 }

const store = () => useWindowStore.getState()
const win = (id: string): WindowState => {
  const w = store().windows.find((w) => w.id === id)
  if (!w) throw new Error(`window ${id} not found`)
  return w
}
const activeId = () => selectActiveId(store())

beforeEach(() => {
  useWindowStore.setState(initialWindowData)
})

describe('open', () => {
  it('새 창을 열면 맨 앞이자 활성 창이 된다', () => {
    const a = store().open('system-info', bounds)
    const b = store().open('files', bounds)
    expect(store().windows).toHaveLength(2)
    expect(win(b).z).toBeGreaterThan(win(a).z)
    expect(activeId()).toBe(b)
  })

  it('이미 열린 앱을 다시 열면 창을 늘리지 않고 복원해 맨 앞으로 가져온다', () => {
    const a = store().open('system-info', bounds)
    const b = store().open('files', bounds)
    store().minimize(a)
    expect(store().open('system-info', bounds)).toBe(a)
    expect(store().windows).toHaveLength(2)
    expect(win(a).minimized).toBe(false)
    expect(win(a).z).toBeGreaterThan(win(b).z)
    expect(activeId()).toBe(a)
  })
})

describe('focus', () => {
  it('포커스한 창은 다른 모든 창보다 앞에 온다', () => {
    const ids = [
      store().open('system-info', bounds),
      store().open('files', bounds),
      store().open('storage', bounds),
    ]
    for (const id of [ids[0], ids[2], ids[1], ids[0]]) {
      store().focus(id)
      const others = store().windows.filter((w) => w.id !== id)
      expect(others.every((o) => o.z < win(id).z)).toBe(true)
      expect(activeId()).toBe(id)
    }
  })

  it('이미 맨 앞인 창을 포커스해도 상태가 바뀌지 않는다', () => {
    const a = store().open('system-info', bounds)
    const before = store().windows
    store().focus(a)
    expect(store().windows).toBe(before)
  })
})

describe('minimize / restore', () => {
  it('최소화 후 복원하면 위치·크기·최대화 상태가 그대로다', () => {
    const a = store().open('system-info', bounds)
    store().moveTo(a, 300, 200, bounds)
    store().toggleMaximize(a)
    const before = win(a)

    store().minimize(a)
    expect(win(a).minimized).toBe(true)
    store().restore(a)

    const after = win(a)
    expect(after.minimized).toBe(false)
    expect({ ...after, z: 0 }).toEqual({ ...before, z: 0 })
  })

  it('활성 창을 최소화하면 그다음 앞 창이 활성 창이 된다', () => {
    const a = store().open('system-info', bounds)
    const b = store().open('files', bounds)
    store().minimize(b)
    expect(activeId()).toBe(a)
    store().minimize(a)
    expect(activeId()).toBeNull()
  })
})

describe('toggleMaximize / moveTo', () => {
  it('최대화를 풀면 원래 위치·크기로 돌아온다', () => {
    const a = store().open('files', bounds)
    store().moveTo(a, 250, 120, bounds)
    const before = win(a)
    store().toggleMaximize(a)
    expect(win(a).maximized).toBe(true)
    store().toggleMaximize(a)
    expect(win(a)).toMatchObject({ x: before.x, y: before.y, width: before.width, height: before.height, maximized: false })
  })

  it('최대화된 창은 이동하지 않는다', () => {
    const a = store().open('files', bounds)
    store().toggleMaximize(a)
    const { x, y } = win(a)
    store().moveTo(a, x + 100, y + 100, bounds)
    expect(win(a)).toMatchObject({ x, y })
  })

  it('화면 밖으로 끌어도 제목 표시줄이 남는다', () => {
    const a = store().open('files', bounds)
    store().moveTo(a, -9999, -9999, bounds)
    expect(win(a).y).toBe(0)
    expect(win(a).x + win(a).width).toBe(MIN_VISIBLE)
    store().moveTo(a, 9999, 9999, bounds)
    expect(win(a).y).toBe(bounds.height - TITLEBAR_HEIGHT)
    expect(bounds.width - win(a).x).toBe(MIN_VISIBLE)
  })
})

describe('clampAll', () => {
  it('데스크톱이 줄어들면 화면 밖 창을 다시 끌어온다', () => {
    const a = store().open('files', bounds)
    store().moveTo(a, 1100, 600, bounds)
    const small = { width: 800, height: 500 }
    store().clampAll(small)
    expect(win(a).x).toBe(small.width - MIN_VISIBLE)
    expect(win(a).y).toBe(small.height - TITLEBAR_HEIGHT)
  })

  it('바뀔 창이 없으면 상태를 그대로 둔다', () => {
    store().open('files', bounds)
    const before = store().windows
    store().clampAll(bounds)
    expect(store().windows).toBe(before)
  })
})

describe('close', () => {
  it('닫은 창은 목록에서 사라진다', () => {
    const a = store().open('system-info', bounds)
    const b = store().open('files', bounds)
    store().close(b)
    expect(store().windows.map((w) => w.id)).toEqual([a])
    expect(activeId()).toBe(a)
  })
})

describe('resize', () => {
  it('위치와 크기를 함께 바꾼다', () => {
    const a = store().open('system-info', bounds)
    store().resize(a, { x: 10, y: 20, width: 500, height: 400 })
    expect(win(a)).toMatchObject({ x: 10, y: 20, width: 500, height: 400 })
  })

  it('최대화된 창은 크기를 바꾸지 않는다', () => {
    const a = store().open('system-info', bounds)
    store().toggleMaximize(a)
    const before = win(a)
    store().resize(a, { x: 10, y: 20, width: 500, height: 400 })
    expect(win(a)).toBe(before)
  })

  it('같은 값이면 상태가 바뀌지 않는다', () => {
    const a = store().open('system-info', bounds)
    const { x, y, width, height } = win(a)
    const before = store().windows
    store().resize(a, { x, y, width, height })
    expect(store().windows).toBe(before)
  })
})
