import { describe, expect, it } from 'vitest'
import type { Preferences } from '../api/types.gen'
import { createAvatarStore } from './store'

describe('avatar store', () => {
  it('저장한 사진 정보를 가져온다', async () => {
    const store = createAvatarStore(async () => ({ avatarImage: { rev: 'r1' } }))
    await store.getState().load()
    expect(store.getState()).toMatchObject({ status: 'ready', image: { rev: 'r1' } })
  })

  it('사진이 없으면 null이고, 실패하면 error다', async () => {
    const empty = createAvatarStore(async () => ({}))
    await empty.getState().load()
    expect(empty.getState()).toMatchObject({ status: 'ready', image: null })
    const failing = createAvatarStore(async () => Promise.reject(new Error('down')))
    await failing.getState().load()
    expect(failing.getState().status).toBe('error')
  })

  it('올리거나 지우면 바로 바뀌고, 그 전에 시작한 불러오기 결과는 버린다', async () => {
    let resolve: (p: Preferences) => void = () => {}
    const store = createAvatarStore(() => new Promise((r) => (resolve = r)))
    const loading = store.getState().load()
    store.getState().uploaded({ rev: 'new' })
    resolve({ avatarImage: { rev: 'old' } })
    await loading
    expect(store.getState().image).toEqual({ rev: 'new' })
    store.getState().deleted()
    expect(store.getState().image).toBeNull()
  })

  it('clear는 처음 상태로 돌린다', async () => {
    const store = createAvatarStore(async () => ({ avatarImage: { rev: 'r1' } }))
    await store.getState().load()
    store.getState().clear()
    expect(store.getState()).toMatchObject({ status: 'idle', image: null })
  })
})
