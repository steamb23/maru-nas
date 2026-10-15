import { describe, expect, it, vi } from 'vitest'
import type { Preferences } from '../api/types.gen'
import type { AppId } from '../windows/apps'
import { defaultShortcuts } from './list'
import { type ShortcutApi, createShortcutStore } from './store'

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

function setup(api: Partial<ShortcutApi> = {}) {
  const full: ShortcutApi = {
    fetchPreferences: vi.fn(async (): Promise<Preferences> => ({ shortcuts: [{ app: 'network' }] })),
    saveShortcuts: vi.fn(async () => {}),
    ...api,
  }
  return { store: createShortcutStore(full), api: full }
}

const flushPromises = () => new Promise((r) => setTimeout(r, 0))

describe('load', () => {
  it('저장한 목록을 가져온다', async () => {
    const { store } = setup()
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', apps: ['network'] })
  })

  it('저장한 적이 없으면 기본 목록(위젯 배치만 있어도)', async () => {
    const { store, api } = setup({ fetchPreferences: async () => ({ widgets: [] }) })
    await store.getState().load()
    expect(store.getState().apps).toEqual(defaultShortcuts())
    // 기본 목록을 불러왔다고 저장하지는 않는다.
    await flushPromises()
    expect(api.saveShortcuts).not.toHaveBeenCalled()
  })

  it('모두 지운 목록은 빈 목록으로 둔다', async () => {
    const { store } = setup({ fetchPreferences: async () => ({ shortcuts: [] }) })
    await store.getState().load()
    expect(store.getState().apps).toEqual([])
  })

  it('지금 빌드에 없는 앱과 중복은 뺀다', async () => {
    const { store } = setup({
      fetchPreferences: async () => ({ shortcuts: [{ app: 'files' }, { app: 'removed-app' }, { app: 'files' }] }),
    })
    await store.getState().load()
    expect(store.getState().apps).toEqual(['files'])
  })

  it('실패하면 error', async () => {
    const { store } = setup({ fetchPreferences: async () => Promise.reject(new Error('down')) })
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'error', loaded: true })
  })

  it('loaded는 다시 시도하는 동안에도 남고 clear에서만 지워진다', async () => {
    let fail = true
    const d = deferred<Preferences>()
    const { store } = setup({
      fetchPreferences: () => (fail ? Promise.reject(new Error('down')) : d.promise),
    })
    expect(store.getState().loaded).toBe(false)
    await store.getState().load()
    fail = false
    const retry = store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'loading', loaded: true })
    d.resolve({ shortcuts: [{ app: 'files' }] })
    await retry
    expect(store.getState()).toMatchObject({ status: 'ready', loaded: true, apps: ['files'] })
    store.getState().clear()
    expect(store.getState().loaded).toBe(false)
  })

  it('clear 뒤에 도착한 응답은 버린다', async () => {
    const d = deferred<Preferences>()
    const { store } = setup({ fetchPreferences: () => d.promise })
    const loading = store.getState().load()
    store.getState().clear()
    d.resolve({ shortcuts: [{ app: 'files' }] })
    await loading
    expect(store.getState()).toMatchObject({ status: 'idle', apps: [] })
  })
})

describe('set', () => {
  it('반영하고 {app} 목록으로 저장한다', async () => {
    const { store, api } = setup()
    await store.getState().load()
    const next: AppId[] = ['files', 'network']
    store.getState().set(next)
    expect(store.getState().apps).toEqual(next)
    await flushPromises()
    expect(api.saveShortcuts).toHaveBeenCalledExactlyOnceWith([{ app: 'files' }, { app: 'network' }])
  })

  it('불러오기 전에는 무시한다(빈 목록으로 덮어쓰지 않게)', () => {
    const { store, api } = setup()
    store.getState().set([])
    expect(api.saveShortcuts).not.toHaveBeenCalled()
    expect(store.getState().status).toBe('idle')
  })

  it('저장은 하나씩 보내고, 보내는 동안 바뀐 목록은 마지막 것만 보낸다', async () => {
    const first = deferred()
    let calls = 0
    const save = vi.fn<ShortcutApi['saveShortcuts']>(() => (++calls === 1 ? first.promise : Promise.resolve()))
    const { store } = setup({ saveShortcuts: save })
    await store.getState().load()
    store.getState().set(['files'])
    store.getState().set(['storage'])
    store.getState().set(['network'])
    expect(save).toHaveBeenCalledTimes(1)
    first.resolve()
    await flushPromises()
    expect(save.mock.calls.map(([v]) => v)).toEqual([[{ app: 'files' }], [{ app: 'network' }]])
  })

  it('실패하면 표시하고 다음 변경 때 다시 보낸다', async () => {
    const save = vi.fn<ShortcutApi['saveShortcuts']>(async () => {
      if (save.mock.calls.length === 1) throw new Error('down')
    })
    const { store } = setup({ saveShortcuts: save })
    await store.getState().load()
    store.getState().set(['files'])
    await flushPromises()
    expect(store.getState()).toMatchObject({ saveFailed: true, apps: ['files'] })
    store.getState().set(['files', 'storage'])
    await flushPromises()
    expect(store.getState().saveFailed).toBe(false)
    expect(save).toHaveBeenCalledTimes(2)
  })

  it('clear하면 기다리던 저장을 버린다', async () => {
    const first = deferred()
    const save = vi.fn<ShortcutApi['saveShortcuts']>(() => first.promise)
    const { store } = setup({ saveShortcuts: save })
    await store.getState().load()
    store.getState().set(['files'])
    store.getState().set(['storage'])
    store.getState().clear()
    first.resolve()
    await flushPromises()
    expect(save).toHaveBeenCalledTimes(1)
  })
})
