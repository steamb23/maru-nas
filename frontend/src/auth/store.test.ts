import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../api/client'
import type { LoginResponse, SessionInfo } from '../api/types.gen'
import { type AuthApi, createAuthStore } from './store'

const admin: SessionInfo = { username: 'admin', admin: true }
const user: SessionInfo = { username: 'user', admin: false }
const authed = (s: SessionInfo): LoginResponse => ({ status: 'authenticated', session: s })
const challenge: LoginResponse = { status: 'challengeRequired', challenge: { kind: 'totp' } }

function setup(api: Partial<AuthApi> = {}, now = () => 1_000_000) {
  const resetWindows = vi.fn()
  const full: AuthApi = {
    fetchSession: vi.fn(async () => admin),
    login: vi.fn(async () => authed(admin)),
    verifyChallenge: vi.fn(async () => authed(admin)),
    logout: vi.fn(async () => {}),
    ...api,
  }
  const store = createAuthStore({ api: full, resetWindows, now })
  return { store, api: full, resetWindows }
}

describe('restore', () => {
  it('세션이 있으면 로그인 상태로 복원한다', async () => {
    const { store } = setup()
    expect(store.getState().status).toBe('checking')
    await store.getState().restore()
    expect(store.getState()).toMatchObject({ status: 'loggedIn', user: admin })
  })

  it('401이면 로그인 화면', async () => {
    const { store } = setup({ fetchSession: async () => Promise.reject(new ApiError(401, 'x', 'unauthorized')) })
    await store.getState().restore()
    expect(store.getState().status).toBe('loggedOut')
  })

  it('네트워크 오류·프록시 오류면 offline', async () => {
    const { store } = setup({ fetchSession: async () => Promise.reject(new TypeError('Failed to fetch')) })
    await store.getState().restore()
    expect(store.getState().status).toBe('offline')
    const s2 = setup({ fetchSession: async () => Promise.reject(new ApiError(502, 'x')) }).store
    await s2.getState().restore()
    expect(s2.getState().status).toBe('offline')
  })

  it('연결 끊김 화면에서 다시 시도하는 동안 화면을 바꾸지 않고, 연결되면 복원한다', async () => {
    const fetchSession = vi.fn<AuthApi['fetchSession']>().mockRejectedValueOnce(new TypeError('x'))
    let resolve: (s: SessionInfo) => void = () => {}
    fetchSession.mockImplementationOnce(() => new Promise((r) => (resolve = r)))
    const { store } = setup({ fetchSession })
    await store.getState().restore()
    const pending = store.getState().restore()
    expect(store.getState().status).toBe('offline')
    resolve(admin)
    await pending
    expect(store.getState().status).toBe('loggedIn')
  })
})

describe('login', () => {
  it('바로 인증되면 로그인 상태', async () => {
    const { store, api } = setup()
    await store.getState().login('admin', 'pw')
    expect(api.login).toHaveBeenCalledWith('admin', 'pw')
    expect(store.getState()).toMatchObject({ status: 'loggedIn', user: admin, notice: null })
  })

  it('2차 인증이 필요하면 challenge를 거쳐 로그인한다', async () => {
    const { store, api } = setup({ login: async () => challenge })
    await store.getState().login('totp', 'pw')
    expect(store.getState()).toMatchObject({ status: 'challenge', challenge: { kind: 'totp' } })
    await store.getState().verify('123456')
    expect(api.verifyChallenge).toHaveBeenCalledWith('123456')
    expect(store.getState()).toMatchObject({ status: 'loggedIn', challenge: null })
  })

  it('실패는 그대로 던지고 상태는 바꾸지 않는다', async () => {
    const err = new ApiError(400, 'x', 'invalid_credentials')
    const { store } = setup({ login: async () => Promise.reject(err) })
    store.setState({ status: 'loggedOut' })
    await expect(store.getState().login('admin', 'bad')).rejects.toBe(err)
    expect(store.getState().status).toBe('loggedOut')
  })

  it('2차 인증 실패는 challenge에 머물고, 대기 만료면 로그인 화면으로 돌아간다', async () => {
    const failed = new ApiError(400, 'x', 'challenge_failed')
    const expired = new ApiError(401, 'x', 'challenge_expired')
    const verify = vi.fn<AuthApi['verifyChallenge']>().mockRejectedValueOnce(failed).mockRejectedValueOnce(expired)
    const { store } = setup({ login: async () => challenge, verifyChallenge: verify })
    await store.getState().login('totp', 'pw')
    await expect(store.getState().verify('0')).rejects.toBe(failed)
    expect(store.getState().status).toBe('challenge')
    await expect(store.getState().verify('0')).rejects.toBe(expired)
    expect(store.getState()).toMatchObject({ status: 'loggedOut', challenge: null })
  })

  it('cancelChallenge는 로그인 화면으로 돌아간다', async () => {
    const { store } = setup({ login: async () => challenge })
    await store.getState().login('totp', 'pw')
    store.getState().cancelChallenge()
    expect(store.getState()).toMatchObject({ status: 'loggedOut', challenge: null })
  })
})

describe('창 배치', () => {
  it('로그아웃하면 창을 닫는다', async () => {
    const { store, api, resetWindows } = setup()
    await store.getState().login('admin', 'pw')
    await store.getState().logout()
    expect(api.logout).toHaveBeenCalled()
    expect(resetWindows).toHaveBeenCalledOnce()
    expect(store.getState()).toMatchObject({ status: 'loggedOut', user: null, windowsOwner: null })
  })

  it('백엔드에 닿지 않아도 로그아웃 화면으로 간다', async () => {
    const { store } = setup({ logout: async () => Promise.reject(new TypeError('Failed to fetch')) })
    await store.getState().login('admin', 'pw')
    await store.getState().logout()
    expect(store.getState().status).toBe('loggedOut')
  })

  it('세션 만료 후 같은 사용자가 다시 로그인하면 창을 남기고, 다른 사용자면 닫는다', async () => {
    const login = vi.fn<AuthApi['login']>(async (name) => authed(name === 'admin' ? admin : user))
    const { store, resetWindows } = setup({ login })
    await store.getState().login('admin', 'pw')

    store.getState().expire()
    expect(store.getState()).toMatchObject({ status: 'loggedOut', notice: 'expired' })
    await store.getState().login('admin', 'pw')
    expect(resetWindows).not.toHaveBeenCalled()
    expect(store.getState().notice).toBeNull()

    store.getState().expire()
    await store.getState().login('user', 'pw')
    expect(resetWindows).toHaveBeenCalledOnce()
    expect(store.getState().windowsOwner).toBe('user')
  })

  it('로그인 상태가 아닐 때의 expire는 무시한다', () => {
    const { store } = setup()
    store.setState({ status: 'loggedOut' })
    store.getState().expire()
    expect(store.getState().notice).toBeNull()
  })
})

describe('만료 시각', () => {
  it('남은 시간을 이 브라우저 시각 기준 만료 시각으로 바꾼다', async () => {
    let t = 1_000_000
    const { store } = setup({ login: async () => authed({ ...admin, expiresInSeconds: 300 }) }, () => t)
    await store.getState().login('admin', 'pw')
    expect(store.getState().expiresAt).toBe(1_000_000 + 300_000)
    t += 60_000
    store.getState().sessionUpdated({ ...admin, expiresInSeconds: 300 })
    expect(store.getState().expiresAt).toBe(1_060_000 + 300_000)
    store.getState().sessionUpdated({ ...admin })
    expect(store.getState().expiresAt).toBeNull()
  })

  it('로그인 상태가 아니거나 다른 사용자의 정보는 무시한다', async () => {
    const { store } = setup()
    store.getState().sessionUpdated({ ...admin, expiresInSeconds: 10 })
    expect(store.getState().expiresAt).toBeNull()
    await store.getState().login('admin', 'pw')
    store.getState().sessionUpdated({ ...user, expiresInSeconds: 10 })
    expect(store.getState().user).toEqual(admin)
  })

  it('만료·로그아웃하면 지운다', async () => {
    const { store } = setup({ login: async () => authed({ ...admin, expiresInSeconds: 300 }) })
    await store.getState().login('admin', 'pw')
    store.getState().expire()
    expect(store.getState().expiresAt).toBeNull()
  })
})
