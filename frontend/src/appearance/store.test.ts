import { describe, expect, it, vi } from 'vitest'
import type { Preferences } from '../api/types.gen'
import { type AppearanceApi, allowsAnimations, createAppearanceStore, widgetsLocked } from './store'

function setup(prefs: Preferences = {}, api: Partial<AppearanceApi> = {}) {
  const full: AppearanceApi = {
    fetchPreferences: vi.fn(async () => prefs),
    saveAppearance: vi.fn(async () => {}),
    ...api,
  }
  return { store: createAppearanceStore(full), api: full }
}

const flushPromises = () => new Promise((r) => setTimeout(r, 0))

describe('appearance store', () => {
  it('저장한 화면 설정과 올린 이미지를 가져온다', async () => {
    const { store } = setup({ appearance: { theme: 'dark' }, wallpaperImage: { rev: 'r1' } })
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', prefs: { theme: 'dark' }, image: { rev: 'r1' } })
  })

  it('저장한 적 없으면 빈 설정이다', async () => {
    const { store } = setup()
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', prefs: {}, image: null })
  })

  it('일부를 바꾸면 설정 전체를 저장하고, undefined는 값을 비운다', async () => {
    const { store, api } = setup({ appearance: { theme: 'dark', animations: false } })
    await store.getState().load()
    store.getState().update({ wallpaper: { kind: 'preset', preset: 'ocean' } })
    store.getState().update({ theme: undefined })
    await flushPromises()
    await flushPromises()
    expect(store.getState().prefs).toEqual({ animations: false, wallpaper: { kind: 'preset', preset: 'ocean' } })
    expect(api.saveAppearance).toHaveBeenLastCalledWith({ animations: false, wallpaper: { kind: 'preset', preset: 'ocean' } })
  })

  it('불러오기 전에는 바꾸지 않는다', async () => {
    const { store, api } = setup()
    store.getState().update({ theme: 'light' })
    await flushPromises()
    expect(store.getState().prefs).toEqual({})
    expect(api.saveAppearance).not.toHaveBeenCalled()
  })

  it('이미지를 올리면 바탕화면을 그 이미지로, 지우면 기본값으로 돌린다', async () => {
    const { store } = setup({ appearance: { wallpaper: { kind: 'preset', preset: 'night' } } })
    await store.getState().load()
    store.getState().imageUploaded({ rev: 'r2' })
    expect(store.getState()).toMatchObject({ image: { rev: 'r2' }, prefs: { wallpaper: { kind: 'image' } } })
    store.getState().imageDeleted()
    expect(store.getState().image).toBeNull()
    expect(store.getState().prefs.wallpaper).toBeUndefined()
  })

  it('프리셋을 고른 뒤 이미지를 지우면 바탕화면은 그대로다', async () => {
    const { store, api } = setup({ appearance: { wallpaper: { kind: 'preset', preset: 'night' } }, wallpaperImage: { rev: 'r' } })
    await store.getState().load()
    store.getState().imageDeleted()
    await flushPromises()
    expect(store.getState().prefs.wallpaper).toEqual({ kind: 'preset', preset: 'night' })
    expect(api.saveAppearance).not.toHaveBeenCalled()
  })

  it('저장에 실패하면 표시하고, clear는 처음 상태로 돌린다', async () => {
    const { store } = setup({}, { saveAppearance: async () => Promise.reject(new Error('down')) })
    await store.getState().load()
    store.getState().update({ theme: 'light' })
    await flushPromises()
    expect(store.getState().saveFailed).toBe(true)
    store.getState().clear()
    expect(store.getState()).toMatchObject({ status: 'idle', prefs: {}, image: null, saveFailed: false })
  })

  it('애니메이션은 꺼 둔 경우만 끄고, 값이 없으면 켜진 것으로 본다', async () => {
    expect(allowsAnimations({})).toBe(true)
    expect(allowsAnimations({ animations: true })).toBe(true)
    expect(allowsAnimations({ animations: false })).toBe(false)
    const { store } = setup({ appearance: { animations: false } })
    expect(allowsAnimations(store.getState().prefs)).toBe(true)
    await store.getState().load()
    expect(allowsAnimations(store.getState().prefs)).toBe(false)
    store.getState().clear()
    expect(allowsAnimations(store.getState().prefs)).toBe(true)
  })

  it('위젯은 잠근 경우만 잠그고, 값을 비우면 저장 본문에서도 빠진다', async () => {
    expect(widgetsLocked({})).toBe(false)
    expect(widgetsLocked({ widgetsLocked: false })).toBe(false)
    const { store, api } = setup({ appearance: { widgetsLocked: true } })
    expect(widgetsLocked(store.getState().prefs)).toBe(false)
    await store.getState().load()
    expect(widgetsLocked(store.getState().prefs)).toBe(true)
    store.getState().update({ widgetsLocked: undefined })
    expect(widgetsLocked(store.getState().prefs)).toBe(false)
    expect(api.saveAppearance).toHaveBeenLastCalledWith({})
  })
})
