import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '../api/client'
import type { LoginResponse, SessionInfo } from '../api/types.gen'
import { type AuthApi, type ScreenChange, createAuthStore } from './store'

const admin: SessionInfo = { username: 'admin', admin: true, assurance: 'login' }
const user: SessionInfo = { username: 'user', admin: false, assurance: 'login' }
const authed = (s: SessionInfo): LoginResponse => ({ status: 'authenticated', session: s })
const challenge: LoginResponse = { status: 'challengeRequired', challenge: { kind: 'totp' } }

function setup(
  api: Partial<AuthApi> = {},
  now = () => 1_000_000,
  beforeScreenChange?: (change: ScreenChange) => Promise<void>,
  shouldOfferPasskeyLogin?: (username: string) => boolean,
) {
  const resetWindows = vi.fn()
  const full: AuthApi = {
    fetchSession: vi.fn(async () => admin),
    login: vi.fn(async () => authed(admin)),
    verifyChallenge: vi.fn(async () => authed(admin)),
    finishPasskeyLogin: vi.fn(async () => authed({ ...admin, assurance: 'twoFactor' })),
    logout: vi.fn(async () => {}),
    ...api,
  }
  const store = createAuthStore({ api: full, resetWindows, now, beforeScreenChange, shouldOfferPasskeyLogin })
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

  it('보안 강화 모드의 2차 인증을 권하면 로그인 화면에 머물고, 마치거나 건너뛰면 데스크톱으로 간다', async () => {
    const changes: ScreenChange[] = []
    const stepped: LoginResponse = { status: 'authenticated', session: user, stepUp: { methods: ['totp', 'recovery'] } }
    const { store } = setup({ login: async () => stepped }, undefined, async (c) => void changes.push(c))
    await store.getState().login('user', 'pw')
    expect(store.getState()).toMatchObject({ status: 'stepUp', user, stepUp: { methods: ['totp', 'recovery'] } })
    expect(changes).toEqual([])
    await store.getState().finishStepUp('twoFactor')
    expect(store.getState()).toMatchObject({ status: 'loggedIn', stepUp: null, user: { ...user, assurance: 'twoFactor' } })
    expect(changes).toEqual(['signIn'])

    const skip = setup({ login: async () => stepped }).store
    await skip.getState().login('user', 'pw')
    await skip.getState().finishStepUp()
    expect(skip.getState()).toMatchObject({ status: 'loggedIn', user })
    // 데스크톱에서는 아무 일도 하지 않는다.
    await skip.getState().finishStepUp('twoFactor')
    expect(skip.getState().user?.assurance).toBe('login')

    // 권하는 동안 세션이 끝나면 로그인 화면으로 돌아간다.
    const late = setup({ login: async () => stepped }).store
    await late.getState().login('user', 'pw')
    late.getState().expire()
    expect(late.getState()).toMatchObject({ status: 'loggedOut', notice: 'expired', stepUp: null, user: null })
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

describe('패스키로 로그인', () => {
  const offered: LoginResponse = { status: 'authenticated', session: user, passkeyLoginOffer: { days: 30 } }

  it('권유를 받고 이 브라우저에서 권할 때만 로그인 화면에 머문다', async () => {
    const changes: ScreenChange[] = []
    const should = vi.fn((u: string) => u === 'user')
    const { store } = setup({ login: async () => offered }, undefined, async (c) => void changes.push(c), should)
    await store.getState().login('user', 'pw')
    expect(should).toHaveBeenCalledWith('user')
    expect(store.getState()).toMatchObject({ status: 'stepUp', user, stepUp: null, passkeyOffer: { days: 30 } })
    expect(changes).toEqual([])
    // 켜기는 세션을 twoFactor로 올린다.
    await store.getState().finishStepUp('twoFactor')
    expect(store.getState()).toMatchObject({ status: 'loggedIn', passkeyOffer: null, user: { ...user, assurance: 'twoFactor' } })
    expect(changes).toEqual(['signIn'])

    // 건너뛰면 그대로 들어간다.
    const skip = setup({ login: async () => offered }, undefined, undefined, () => true).store
    await skip.getState().login('user', 'pw')
    await skip.getState().finishStepUp()
    expect(skip.getState()).toMatchObject({ status: 'loggedIn', passkeyOffer: null, user })
  })

  it('이 브라우저에서 권하지 않으면(판단 함수가 false이거나 없음) 바로 데스크톱으로 간다', async () => {
    const no = setup({ login: async () => offered }, undefined, undefined, () => false).store
    await no.getState().login('user', 'pw')
    expect(no.getState()).toMatchObject({ status: 'loggedIn', passkeyOffer: null })
    const none = setup({ login: async () => offered }).store
    await none.getState().login('user', 'pw')
    expect(none.getState()).toMatchObject({ status: 'loggedIn', passkeyOffer: null })
  })

  it('2차 인증 권유와 함께 올 수 있다', async () => {
    const both: LoginResponse = { ...offered, stepUp: { methods: ['passkey'] } }
    const { store } = setup({ login: async () => both }, undefined, undefined, () => true)
    await store.getState().login('user', 'pw')
    expect(store.getState()).toMatchObject({ status: 'stepUp', stepUp: { methods: ['passkey'] }, passkeyOffer: { days: 30 } })
    // 2차 인증 수단이 없는 권유는 권유만 남긴다.
    const empty = setup({ login: async () => ({ ...offered, stepUp: { methods: [] } }) }, undefined, undefined, () => true).store
    await empty.getState().login('user', 'pw')
    expect(empty.getState()).toMatchObject({ status: 'stepUp', stepUp: null, passkeyOffer: { days: 30 } })
  })

  it('OMV 2차 인증 코드를 확인한 뒤에도 권한다', async () => {
    const { store } = setup({ login: async () => challenge, verifyChallenge: async () => offered }, undefined, undefined, () => true)
    await store.getState().login('user', 'pw')
    expect(store.getState().status).toBe('challenge')
    await store.getState().verify('123456')
    expect(store.getState()).toMatchObject({ status: 'stepUp', challenge: null, passkeyOffer: { days: 30 } })
  })

  it('권하는 동안 세션이 끝나거나 로그아웃하면 권유를 지운다', async () => {
    const late = setup({ login: async () => offered }, undefined, undefined, () => true).store
    await late.getState().login('user', 'pw')
    late.getState().expire()
    expect(late.getState()).toMatchObject({ status: 'loggedOut', notice: 'expired', passkeyOffer: null, user: null })

    const out = setup({ login: async () => offered }, undefined, undefined, () => true).store
    await out.getState().login('user', 'pw')
    await out.getState().logout()
    expect(out.getState()).toMatchObject({ status: 'loggedOut', passkeyOffer: null, user: null })
  })

  it('풀어 낸 비밀번호로 로그인을 마친다', async () => {
    const { store, api } = setup()
    store.setState({ status: 'loggedOut' })
    await store.getState().completePasskeyLogin('pw')
    expect(api.finishPasskeyLogin).toHaveBeenCalledWith('pw')
    expect(store.getState()).toMatchObject({ status: 'loggedIn', user: { ...admin, assurance: 'twoFactor' } })
  })

  it('OMV 2차 인증 계정은 코드 입력 단계를 거친다', async () => {
    const { store, api } = setup({ finishPasskeyLogin: async () => challenge })
    store.setState({ status: 'loggedOut' })
    await store.getState().completePasskeyLogin('pw')
    expect(store.getState()).toMatchObject({ status: 'challenge', challenge: { kind: 'totp' } })
    await store.getState().verify('123456')
    expect(api.verifyChallenge).toHaveBeenCalledWith('123456')
    expect(store.getState().status).toBe('loggedIn')
  })

  it('실패는 그대로 던지고 상태는 바꾸지 않는다', async () => {
    const err = new ApiError(400, 'x', 'passkey_login_stale')
    const { store } = setup({ finishPasskeyLogin: async () => Promise.reject(err) })
    store.setState({ status: 'loggedOut' })
    await expect(store.getState().completePasskeyLogin('pw')).rejects.toBe(err)
    expect(store.getState().status).toBe('loggedOut')
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

// transition은 끝을 직접 정하는 화면 전환 훅이다. 불린 방향은 calls에 남는다.
function transition() {
  const calls: ScreenChange[] = []
  let finish = () => {}
  const hook = vi.fn((change: ScreenChange) => {
    calls.push(change)
    return new Promise<void>((resolve) => (finish = resolve))
  })
  return { hook, calls, finish: () => finish() }
}

// flush는 대기 중인 Promise 후속 처리를 모두 돌린다.
const flush = () => new Promise((r) => setTimeout(r, 0))

describe('화면 전환', () => {
  it('로그인은 나가는 애니메이션이 끝난 뒤 데스크톱으로 바꾼다', async () => {
    const t = transition()
    const { store } = setup({}, undefined, t.hook)
    const pending = store.getState().login('admin', 'pw')
    await flush()
    expect(t.calls).toEqual(['signIn'])
    expect(store.getState().status).not.toBe('loggedIn')
    t.finish()
    await pending
    expect(store.getState().status).toBe('loggedIn')
  })

  it('2차 인증을 마칠 때도 같은 전환을 거치고, 2차 인증 요구로는 전환하지 않는다', async () => {
    const t = transition()
    const { store } = setup({ login: async () => challenge }, undefined, t.hook)
    await store.getState().login('totp', 'pw')
    expect(t.calls).toEqual([])
    const pending = store.getState().verify('123456')
    await flush()
    expect(t.calls).toEqual(['signIn'])
    t.finish()
    await pending
    expect(store.getState().status).toBe('loggedIn')
  })

  it('세션 복원은 전환을 거치지 않는다', async () => {
    const t = transition()
    const { store } = setup({}, undefined, t.hook)
    await store.getState().restore()
    expect(t.hook).not.toHaveBeenCalled()
    expect(store.getState().status).toBe('loggedIn')
  })

  it('로그아웃은 요청과 나가는 애니메이션이 모두 끝난 뒤 로그인 화면으로 바꾼다', async () => {
    const t = transition()
    const { store, api, resetWindows } = setup({}, undefined, t.hook)
    await store.getState().restore()
    const pending = store.getState().logout()
    await flush()
    expect(t.calls).toEqual(['signOut'])
    expect(api.logout).toHaveBeenCalled()
    expect(store.getState().status).toBe('loggedIn')
    expect(resetWindows).not.toHaveBeenCalled()
    t.finish()
    await pending
    expect(resetWindows).toHaveBeenCalledOnce()
    expect(store.getState().status).toBe('loggedOut')
  })

  it('세션 만료도 같은 전환을 거치고, 그동안 다시 들어온 만료·로그아웃은 무시한다', async () => {
    const t = transition()
    const { store, api } = setup({}, undefined, t.hook)
    await store.getState().restore()
    store.getState().expire()
    store.getState().expire()
    await store.getState().logout()
    expect(t.calls).toEqual(['signOut'])
    expect(api.logout).not.toHaveBeenCalled()
    expect(store.getState().status).toBe('loggedIn')
    t.finish()
    await flush()
    expect(store.getState()).toMatchObject({ status: 'loggedOut', notice: 'expired' })
  })

  it('로그아웃 중에 다른 요청이 받은 401(만료)은 무시한다', async () => {
    const t = transition()
    const { store } = setup({}, undefined, t.hook)
    await store.getState().restore()
    const pending = store.getState().logout()
    store.getState().expire()
    t.finish()
    await pending
    expect(t.calls).toEqual(['signOut'])
    expect(store.getState()).toMatchObject({ status: 'loggedOut', notice: null })
  })

  it('전환 훅이 실패해도 화면은 바꾼다', async () => {
    const { store } = setup({}, undefined, async () => Promise.reject(new Error('x')))
    await store.getState().login('admin', 'pw')
    expect(store.getState().status).toBe('loggedIn')
    store.getState().expire()
    await flush()
    expect(store.getState().status).toBe('loggedOut')
  })
})
