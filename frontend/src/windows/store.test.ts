import { beforeEach, describe, expect, it } from 'vitest'
import { MIN_VISIBLE, TITLEBAR_HEIGHT } from './geometry'
import {
  type WindowState,
  initialWindowData,
  moveItem,
  nextInstance,
  selectActiveId,
  stableOrder,
  useWindowStore,
} from './store'

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

describe('openNew', () => {
  it('창을 여러 개 여는 앱은 열 때마다 새 창이 맨 앞에 열린다', () => {
    const a = store().open('terminal', bounds)
    const b = store().openNew('terminal', bounds)
    const c = store().openNew('terminal', bounds)
    expect(new Set([a, b, c]).size).toBe(3)
    expect(store().windows.map((w) => w.appId)).toEqual(['terminal', 'terminal', 'terminal'])
    expect(activeId()).toBe(c)
  })

  it('창이 하나뿐인 앱은 open처럼 기존 창을 앞으로 가져온다', () => {
    const a = store().open('system-info', bounds)
    store().open('files', bounds)
    expect(store().openNew('system-info', bounds)).toBe(a)
    expect(store().windows).toHaveLength(2)
    expect(activeId()).toBe(a)
  })

  it('여러 창이 열린 앱을 open하면 그중 가장 앞에 있던 창을 복원해 가져온다', () => {
    const a = store().open('terminal', bounds)
    const b = store().openNew('terminal', bounds)
    store().open('files', bounds)
    store().focus(a)
    store().minimize(a)
    // b보다 a가 나중에 앞에 있었으므로 최소화돼 있어도 a다.
    expect(store().open('terminal', bounds)).toBe(a)
    expect(win(a).minimized).toBe(false)
    expect(activeId()).toBe(a)
    expect(store().windows.filter((w) => w.id === b)).toHaveLength(1)
  })
})

describe('instance', () => {
  it('같은 앱의 창은 1부터 번호를 받고, 다른 앱은 따로 센다', () => {
    const a = store().open('terminal', bounds)
    const f = store().open('files', bounds)
    const b = store().openNew('terminal', bounds)
    expect([win(a).instance, win(b).instance, win(f).instance]).toEqual([1, 2, 1])
  })

  it('닫힌 창의 번호는 다음 창이 다시 쓴다(가장 작은 빈 번호)', () => {
    const a = store().open('terminal', bounds)
    const b = store().openNew('terminal', bounds)
    const c = store().openNew('terminal', bounds)
    store().close(b)
    expect(win(store().openNew('terminal', bounds)).instance).toBe(2)
    store().close(a)
    expect(win(store().openNew('terminal', bounds)).instance).toBe(1)
    expect(win(c).instance).toBe(3)
  })

  it('작업표시줄 순서를 바꿔도 번호는 그대로다', () => {
    const a = store().open('terminal', bounds)
    const b = store().openNew('terminal', bounds)
    store().reorder(b, 0)
    expect([win(a).instance, win(b).instance]).toEqual([1, 2])
  })

  it('nextInstance는 그 앱이 쓰지 않는 가장 작은 번호다', () => {
    const base: WindowState = {
      id: 'x',
      appId: 'terminal',
      instance: 1,
      x: 0,
      y: 0,
      width: 100,
      height: 100,
      z: 1,
      minimized: false,
      maximized: false,
      tile: null,
    }
    const w = (appId: 'terminal' | 'files', instance: number): WindowState => ({ ...base, appId, instance })
    expect(nextInstance([], 'terminal')).toBe(1)
    expect(nextInstance([w('terminal', 1), w('terminal', 3), w('files', 2)], 'terminal')).toBe(2)
    expect(nextInstance([w('terminal', 1), w('terminal', 3), w('files', 2)], 'files')).toBe(1)
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

describe('closeMany', () => {
  it('여러 창을 한 번에 닫고 남은 창의 순서는 유지한다', () => {
    const a = store().open('system-info', bounds)
    const b = store().open('files', bounds)
    const c = store().open('storage', bounds)
    store().closeMany([a, c])
    expect(store().windows.map((w) => w.id)).toEqual([b])
  })

  it('없는 id만 넘기면 상태를 바꾸지 않는다', () => {
    store().open('system-info', bounds)
    const before = store().windows
    store().closeMany(['nope'])
    expect(store().windows).toBe(before)
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

describe('reorder', () => {
  const order = () => store().windows.map((w) => w.id)

  it('창을 지정한 자리로 옮긴다', () => {
    const [a, b, c] = [
      store().open('system-info', bounds),
      store().open('files', bounds),
      store().open('storage', bounds),
    ]
    store().reorder(a, 2)
    expect(order()).toEqual([b, c, a])
    store().reorder(a, 0)
    expect(order()).toEqual([a, b, c])
    store().reorder(c, 1)
    expect(order()).toEqual([a, c, b])
  })

  it('범위를 벗어난 자리는 양 끝으로 제한한다', () => {
    const a = store().open('system-info', bounds)
    const b = store().open('files', bounds)
    store().reorder(a, 99)
    expect(order()).toEqual([b, a])
    store().reorder(a, -5)
    expect(order()).toEqual([a, b])
  })

  it('z와 활성 창은 바뀌지 않는다', () => {
    const a = store().open('system-info', bounds)
    const b = store().open('files', bounds)
    store().reorder(b, 0)
    expect(win(b).z).toBeGreaterThan(win(a).z)
    expect(activeId()).toBe(b)
  })

  it('같은 자리이거나 없는 창이면 상태가 바뀌지 않는다', () => {
    const a = store().open('system-info', bounds)
    store().open('files', bounds)
    const before = store().windows
    store().reorder(a, 0)
    store().reorder('nope', 1)
    expect(store().windows).toBe(before)
  })
})

describe('moveItem', () => {
  it('원본을 바꾸지 않고 새 배열을 반환한다', () => {
    const arr = ['a', 'b', 'c', 'd']
    expect(moveItem(arr, 0, 2)).toEqual(['b', 'c', 'a', 'd'])
    expect(moveItem(arr, 3, 1)).toEqual(['a', 'd', 'b', 'c'])
    expect(arr).toEqual(['a', 'b', 'c', 'd'])
  })
})

describe('stableOrder', () => {
  it('next의 순서가 바뀌어도 prev의 순서를 유지한다', () => {
    expect(stableOrder(['a', 'b', 'c'], ['c', 'a', 'b'])).toEqual(['a', 'b', 'c'])
  })

  it('닫힌 id는 빼고 새 id는 next의 순서대로 뒤에 붙인다', () => {
    expect(stableOrder(['a', 'b', 'c'], ['e', 'c', 'a', 'd'])).toEqual(['a', 'c', 'e', 'd'])
  })

  it('처음에는 next 순서를 그대로 쓴다', () => {
    expect(stableOrder([], ['b', 'a'])).toEqual(['b', 'a'])
  })

  it('keep에 든 id는 닫혔어도 제자리에 남긴다', () => {
    expect(stableOrder(['a', 'b', 'c', 'e'], ['c', 'a', 'd'], new Set(['b']))).toEqual(['a', 'b', 'c', 'd'])
  })
})

describe('detachAt', () => {
  it('최대화를 풀고 원래 크기로 지정한 위치에 놓는다', () => {
    const a = store().open('system-info', bounds)
    const { width, height } = win(a)
    store().toggleMaximize(a)
    store().detachAt(a, 300, 200, bounds)
    expect(win(a)).toMatchObject({ maximized: false, x: 300, y: 200, width, height })
  })

  it('타일링을 풀고 원래 크기로 지정한 위치에 놓는다', () => {
    const a = store().open('system-info', bounds)
    const { width, height } = win(a)
    store().tile(a, 'left')
    store().detachAt(a, 300, 200, bounds)
    expect(win(a)).toMatchObject({ tile: null, x: 300, y: 200, width, height })
  })

  it('위치는 clampPosition으로 제한한다', () => {
    const a = store().open('system-info', bounds)
    store().toggleMaximize(a)
    store().detachAt(a, 100, -40, bounds)
    expect(win(a).y).toBe(0)
  })

  it('최대화·타일링되지 않은 창은 바꾸지 않는다', () => {
    const a = store().open('system-info', bounds)
    const before = store().windows
    store().detachAt(a, 300, 200, bounds)
    expect(store().windows).toBe(before)
  })
})

describe('tile', () => {
  it('좌우에 타일링해도 원래 위치·크기는 보존된다', () => {
    const a = store().open('files', bounds)
    store().moveTo(a, 250, 120, bounds)
    const { x, y, width, height } = win(a)
    store().tile(a, 'right')
    expect(win(a)).toMatchObject({ tile: 'right', maximized: false, x, y, width, height })
  })

  it('max에 놓으면 최대화하고 타일링은 끈다', () => {
    const a = store().open('files', bounds)
    store().tile(a, 'left')
    store().tile(a, 'max')
    expect(win(a)).toMatchObject({ maximized: true, tile: null })
  })

  it('최대화된 창을 좌우에 놓으면 최대화를 끈다', () => {
    const a = store().open('files', bounds)
    store().toggleMaximize(a)
    store().tile(a, 'left')
    expect(win(a)).toMatchObject({ maximized: false, tile: 'left' })
  })

  it('타일링한 창은 맨 앞으로 온다', () => {
    const a = store().open('files', bounds)
    store().open('storage', bounds)
    store().tile(a, 'left')
    expect(activeId()).toBe(a)
  })

  it('타일링된 창을 최대화했다 복원하면 타일링 전 위치·크기로 돌아온다', () => {
    const a = store().open('files', bounds)
    const { x, y, width, height } = win(a)
    store().tile(a, 'left')
    store().toggleMaximize(a)
    expect(win(a)).toMatchObject({ maximized: true, tile: null })
    store().toggleMaximize(a)
    expect(win(a)).toMatchObject({ maximized: false, tile: null, x, y, width, height })
  })

  it('타일링된 창은 이동·크기 조절하지 않는다', () => {
    const a = store().open('files', bounds)
    store().tile(a, 'left')
    const before = store().windows
    store().moveTo(a, 10, 10, bounds)
    store().resize(a, { x: 10, y: 10, width: 300, height: 300 })
    expect(store().windows).toBe(before)
  })

  it('반대쪽에 타일 창이 있으면 그룹의 비율을 유지한다', () => {
    const a = store().open('files', bounds)
    const b = store().open('storage', bounds)
    store().tile(a, 'left')
    store().setTileSplit(0.6)
    store().tile(b, 'right')
    expect(store().tileSplit).toBe(0.6)
  })

  it('반대쪽이 비어 있으면 새 그룹이므로 비율을 반반으로 되돌린다', () => {
    const a = store().open('files', bounds)
    store().tile(a, 'left')
    store().setTileSplit(0.6)
    store().tile(a, 'right')
    expect(store().tileSplit).toBe(0.5)
  })
})

describe('raiseGroup', () => {
  it('짝 창을 복원해 바로 뒤에 두고 대상 창을 맨 앞에 둔다', () => {
    const a = store().open('files', bounds)
    const b = store().open('storage', bounds)
    const c = store().open('system-info', bounds)
    store().tile(a, 'left')
    store().tile(b, 'right')
    store().minimize(b)
    store().focus(c)
    store().raiseGroup(a)
    expect(win(b).minimized).toBe(false)
    expect(activeId()).toBe(a)
    expect(win(b).z).toBeGreaterThan(win(c).z)
    expect(win(a).z).toBeGreaterThan(win(b).z)
  })

  it('짝 창이 없으면 restore와 같다', () => {
    const a = store().open('files', bounds)
    const b = store().open('storage', bounds)
    store().tile(a, 'left')
    store().minimize(a)
    const bz = win(b).z
    store().raiseGroup(a)
    expect(win(a).minimized).toBe(false)
    expect(activeId()).toBe(a)
    expect(win(b)).toMatchObject({ z: bz, minimized: false })
  })

  it('같은 쪽에 타일링된 창은 짝이 아니다', () => {
    const a = store().open('files', bounds)
    const b = store().open('storage', bounds)
    store().tile(a, 'left')
    store().tile(b, 'left')
    store().minimize(b)
    store().raiseGroup(a)
    expect(win(b).minimized).toBe(true)
  })
})
