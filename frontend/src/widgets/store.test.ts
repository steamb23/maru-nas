import { describe, expect, it, vi } from 'vitest'
import type { Preferences, WidgetPlacement } from '../api/types.gen'
import { defaultLayout } from './catalog'
import { type WidgetApi, createWidgetStore } from './store'

const placement: WidgetPlacement = { addon: 'system-info', right: 2, top: 1, cols: 3, rows: 2 }

// deferred는 밖에서 끝낼 수 있는 Promise다.
function deferred<T = void>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

function setup(api: Partial<WidgetApi> = {}) {
  const full: WidgetApi = {
    fetchPreferences: vi.fn(async (): Promise<Preferences> => ({ widgets: [placement] })),
    saveWidgets: vi.fn(async () => {}),
    ...api,
  }
  return { store: createWidgetStore(full), api: full }
}

const flushPromises = () => new Promise((r) => setTimeout(r, 0))

describe('load', () => {
  it('저장한 배치를 가져온다', async () => {
    const { store } = setup()
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', placements: [placement] })
  })

  it('저장한 적이 없으면 기본 배치', async () => {
    const { store } = setup({ fetchPreferences: async () => ({}) })
    await store.getState().load()
    expect(store.getState().placements).toEqual(defaultLayout())
    expect(defaultLayout()).toEqual([{ addon: 'system-info', right: 0, top: 0, cols: 3, rows: 2 }])
  })

  it('모두 지운 배치는 빈 배치로 둔다', async () => {
    const { store } = setup({ fetchPreferences: async () => ({ widgets: [] }) })
    await store.getState().load()
    expect(store.getState().placements).toEqual([])
  })

  it('지금 빌드에 없는 위젯은 뺀다', async () => {
    const gone = { ...placement, addon: 'removed-addon', right: 6 }
    const { store } = setup({ fetchPreferences: async () => ({ widgets: [placement, gone] }) })
    await store.getState().load()
    expect(store.getState().placements).toEqual([placement])
  })

  it('실패하면 error', async () => {
    const { store } = setup({ fetchPreferences: async () => Promise.reject(new Error('down')) })
    await store.getState().load()
    expect(store.getState().status).toBe('error')
  })

  it('clear 뒤에 도착한 응답은 버린다', async () => {
    const d = deferred<Preferences>()
    const { store } = setup({ fetchPreferences: () => d.promise })
    const loading = store.getState().load()
    store.getState().clear()
    d.resolve({ widgets: [placement] })
    await loading
    expect(store.getState()).toMatchObject({ status: 'idle', placements: [] })
  })
})

describe('set', () => {
  it('반영하고 저장한다', async () => {
    const { store, api } = setup()
    await store.getState().load()
    const next = [{ ...placement, top: 3 }]
    store.getState().set(next)
    expect(store.getState().placements).toEqual(next)
    await flushPromises()
    expect(api.saveWidgets).toHaveBeenCalledExactlyOnceWith(next)
  })

  it('불러오기 전에는 무시한다(빈 배치로 덮어쓰지 않게)', () => {
    const { store, api } = setup()
    store.getState().set([])
    expect(api.saveWidgets).not.toHaveBeenCalled()
    expect(store.getState().status).toBe('idle')
  })

  it('저장은 하나씩 보내고, 보내는 동안 바뀐 배치는 마지막 것만 보낸다', async () => {
    const first = deferred()
    let calls = 0
    const save = vi.fn<WidgetApi['saveWidgets']>(() => (++calls === 1 ? first.promise : Promise.resolve()))
    const { store } = setup({ saveWidgets: save })
    await store.getState().load()
    const a = [{ ...placement, top: 1 }]
    const b = [{ ...placement, top: 2 }]
    const c = [{ ...placement, top: 3 }]
    store.getState().set(a)
    store.getState().set(b)
    store.getState().set(c)
    expect(save).toHaveBeenCalledTimes(1)
    first.resolve()
    await flushPromises()
    expect(save.mock.calls.map(([v]) => v)).toEqual([a, c])
  })

  it('실패하면 표시하고 다음 변경 때 다시 보낸다', async () => {
    const save = vi.fn<WidgetApi['saveWidgets']>(async () => {
      if (save.mock.calls.length === 1) throw new Error('down')
    })
    const { store } = setup({ saveWidgets: save })
    await store.getState().load()
    store.getState().set([{ ...placement, top: 4 }])
    await flushPromises()
    expect(store.getState()).toMatchObject({ saveFailed: true, placements: [{ ...placement, top: 4 }] })
    store.getState().set([{ ...placement, top: 5 }])
    await flushPromises()
    expect(store.getState().saveFailed).toBe(false)
    expect(save).toHaveBeenCalledTimes(2)
  })

  it('clear하면 기다리던 저장을 버린다', async () => {
    const first = deferred()
    const save = vi.fn<WidgetApi['saveWidgets']>(() => first.promise)
    const { store } = setup({ saveWidgets: save })
    await store.getState().load()
    store.getState().set([{ ...placement, top: 1 }])
    store.getState().set([{ ...placement, top: 2 }])
    store.getState().clear()
    first.resolve()
    await flushPromises()
    expect(save).toHaveBeenCalledTimes(1)
  })
})
