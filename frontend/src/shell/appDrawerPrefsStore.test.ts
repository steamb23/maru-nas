import { describe, expect, it, vi } from 'vitest'
import type { Preferences } from '../api/types.gen'
import { type AppDrawerPrefsApi, createAppDrawerPrefsStore } from './appDrawerPrefsStore'

function setup(api: Partial<AppDrawerPrefsApi> = {}) {
  const full: AppDrawerPrefsApi = {
    fetchPreferences: vi.fn(async (): Promise<Preferences> => ({ drawer: { sort: 'name' } })),
    saveDrawer: vi.fn(async () => {}),
    ...api,
  }
  return { store: createAppDrawerPrefsStore(full), api: full }
}

const flushPromises = () => new Promise((r) => setTimeout(r, 0))

describe('appDrawerPrefsStore', () => {
  it('저장한 정렬 방식을 가져온다', async () => {
    const { store } = setup()
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', sort: 'name' })
  })

  it('저장한 적 없거나 모르는 값이면 추천순', async () => {
    for (const prefs of [{}, { drawer: { sort: 'random' } }] satisfies Preferences[]) {
      const { store } = setup({ fetchPreferences: async () => prefs })
      await store.getState().load()
      expect(store.getState().sort).toBe('recommended')
    }
  })

  it('바꾸면 설정 전체를 저장한다', async () => {
    const { store, api } = setup({ fetchPreferences: async () => ({}) })
    await store.getState().load()
    store.getState().setSort('name')
    expect(store.getState().sort).toBe('name')
    await flushPromises()
    expect(api.saveDrawer).toHaveBeenCalledExactlyOnceWith({ sort: 'name' })
  })

  it('같은 값이면 저장하지 않는다', async () => {
    const { store, api } = setup()
    await store.getState().load()
    store.getState().setSort('name')
    await flushPromises()
    expect(api.saveDrawer).not.toHaveBeenCalled()
  })

  it('불러오기 전이나 실패한 뒤에는 바꾸지 않는다', async () => {
    const { store, api } = setup({ fetchPreferences: async () => Promise.reject(new Error('down')) })
    store.getState().setSort('name')
    await store.getState().load()
    store.getState().setSort('name')
    await flushPromises()
    expect(store.getState()).toMatchObject({ status: 'error', sort: 'recommended' })
    expect(api.saveDrawer).not.toHaveBeenCalled()
  })

  it('저장에 실패하면 표시하고, clear는 처음 상태로 돌린다', async () => {
    const { store } = setup({ fetchPreferences: async () => ({}), saveDrawer: async () => Promise.reject(new Error('down')) })
    await store.getState().load()
    store.getState().setSort('name')
    await flushPromises()
    expect(store.getState()).toMatchObject({ sort: 'name', saveFailed: true })
    store.getState().clear()
    expect(store.getState()).toMatchObject({ status: 'idle', sort: 'recommended', saveFailed: false })
  })
})
