import { describe, expect, it, vi } from 'vitest'
import type { Preferences } from '../../api/types.gen'
import { cpuGraphMenuItems, processMenuItems } from './menus'
import { type SysinfoPrefsApi, createSysinfoPrefsStore } from './sysinfoPrefsStore'

function setup(api: Partial<SysinfoPrefsApi> = {}) {
  const full: SysinfoPrefsApi = {
    fetchPreferences: vi.fn(
      async (): Promise<Preferences> => ({ systemInfo: { cpuScale: 'core', cpuGraph: 'cores' } }),
    ),
    saveSystemInfoPrefs: vi.fn(async () => {}),
    ...api,
  }
  return { store: createSysinfoPrefsStore(full), api: full }
}

const flushPromises = () => new Promise((r) => setTimeout(r, 0))

describe('sysinfoPrefsStore', () => {
  it('저장한 설정을 가져온다', async () => {
    const { store } = setup()
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', cpuScale: 'core', cpuGraph: 'cores' })
  })

  it('저장한 적 없거나 모르는 값이면 기본값', async () => {
    for (const prefs of [{}, { systemInfo: { cpuScale: 'irix', cpuGraph: 'heatmap' } }] satisfies Preferences[]) {
      const { store } = setup({ fetchPreferences: async () => prefs })
      await store.getState().load()
      expect(store.getState()).toMatchObject({ cpuScale: 'machine', cpuGraph: 'overall' })
    }
  })

  it('하나를 바꾸면 설정 전체를 저장한다', async () => {
    const { store, api } = setup({ fetchPreferences: async () => ({}) })
    await store.getState().load()
    store.getState().set({ cpuGraph: 'cores' })
    expect(store.getState().cpuGraph).toBe('cores')
    await flushPromises()
    expect(api.saveSystemInfoPrefs).toHaveBeenCalledExactlyOnceWith({ cpuScale: 'machine', cpuGraph: 'cores' })
  })

  it('같은 값이면 저장하지 않는다', async () => {
    const { store, api } = setup()
    await store.getState().load()
    store.getState().set({ cpuScale: 'core' })
    await flushPromises()
    expect(api.saveSystemInfoPrefs).not.toHaveBeenCalled()
  })

  it('불러오기 전이나 실패한 뒤에는 바꾸지 않는다', async () => {
    const { store, api } = setup({ fetchPreferences: async () => Promise.reject(new Error('down')) })
    store.getState().set({ cpuScale: 'core' })
    await store.getState().load()
    expect(store.getState().status).toBe('error')
    store.getState().set({ cpuScale: 'core' })
    await flushPromises()
    expect(store.getState().cpuScale).toBe('machine')
    expect(api.saveSystemInfoPrefs).not.toHaveBeenCalled()
  })

  it('저장이 실패하면 표시하고 값은 그대로 둔다', async () => {
    const { store } = setup({
      fetchPreferences: async () => ({}),
      saveSystemInfoPrefs: async () => Promise.reject(new Error('down')),
    })
    await store.getState().load()
    store.getState().set({ cpuScale: 'core' })
    await flushPromises()
    expect(store.getState()).toMatchObject({ cpuScale: 'core', saveFailed: true })
  })

  it('clear 뒤에 도착한 응답은 버린다', async () => {
    let resolve: (p: Preferences) => void = () => {}
    const { store } = setup({ fetchPreferences: () => new Promise((r) => (resolve = r)) })
    const loading = store.getState().load()
    store.getState().clear()
    resolve({ systemInfo: { cpuScale: 'core', cpuGraph: 'cores' } })
    await loading
    expect(store.getState()).toMatchObject({ status: 'idle', cpuScale: 'machine', cpuGraph: 'overall' })
  })
})

describe('menus', () => {
  it('CPU 그래프 메뉴는 지금 모양에 체크하고 고르면 알린다', () => {
    const select = vi.fn()
    const items = cpuGraphMenuItems('cores', select)
    expect(items.map((i) => i.kind === 'action' && [i.id, i.checked])).toEqual([
      ['overall', false],
      ['cores', true],
    ])
    const first = items[0]
    if (first.kind === 'action') first.onSelect()
    expect(select).toHaveBeenCalledWith('overall')
  })

  it('프로세스 메뉴는 CPU 기준 하위 메뉴다', () => {
    const select = vi.fn()
    const [sub] = processMenuItems('machine', select)
    expect(sub.kind).toBe('submenu')
    if (sub.kind !== 'submenu') return
    expect(sub.items.map((i) => i.kind === 'action' && [i.id, i.checked])).toEqual([
      ['machine', true],
      ['core', false],
    ])
    const core = sub.items[1]
    if (core.kind === 'action') core.onSelect()
    expect(select).toHaveBeenCalledWith('core')
  })
})
