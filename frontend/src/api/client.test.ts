import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  copyFiles,
  deletePasskeyLogin,
  discardPasskeyLogin,
  enrollPasskeyLogin,
  fetchAddon,
  fetchCapabilities,
  fetchPreferences,
  fetchSession,
  fetchSystem,
  finishPasskeyLogin,
  hasAddonSources,
  isCapabilitiesResponse,
  isLoginResponse,
  isPreferences,
  isSecurityStatus,
  login,
  logout,
  parseCopyEvents,
  passkeyLoginOptions,
  passkeyLoginUnlock,
  runAddonAction,
  saveDrawer,
  saveShortcuts,
  saveWidgets,
  sendActivity,
  setUnauthorizedHandler,
} from './client'

const valid = {
  cpu: { usagePercent: 12.5 },
  memory: { totalBytes: 100, usedBytes: 40, availableBytes: 60 },
  uptimeSeconds: 99,
}

function stubFetch(res: Response | Error) {
  const fn = vi.fn(async (_url: string, _init?: RequestInit) => {
    if (res instanceof Error) throw res
    return res
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

const unauthorized = () => Response.json({ error: 'not logged in', code: 'unauthorized' }, { status: 401 })

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('fetchSystem', () => {
  it('정상 응답을 반환한다', async () => {
    stubFetch(Response.json(valid))
    await expect(fetchSystem()).resolves.toEqual(valid)
  })

  it('HTTP 오류(프록시 502 등)는 ApiError로 던진다', async () => {
    stubFetch(new Response('Bad Gateway', { status: 502 }))
    await expect(fetchSystem()).rejects.toBeInstanceOf(ApiError)
  })

  it('JSON이 아닌 응답은 오류로 던진다', async () => {
    stubFetch(new Response('<!doctype html>', { status: 200 }))
    await expect(fetchSystem()).rejects.toThrow()
  })

  it('형식이 다른 JSON은 오류로 던진다', async () => {
    stubFetch(Response.json({ cpu: {}, memory: valid.memory, uptimeSeconds: 1 }))
    await expect(fetchSystem()).rejects.toThrow(/unexpected response shape/)
  })

  it('네트워크 오류는 그대로 던진다', async () => {
    stubFetch(new TypeError('Failed to fetch'))
    await expect(fetchSystem()).rejects.toBeInstanceOf(TypeError)
  })
})

describe('401 처리', () => {
  it('일반 요청의 401은 등록한 처리 함수를 부른다', async () => {
    const handler = vi.fn()
    const off = setUnauthorizedHandler(handler)
    stubFetch(unauthorized())
    await expect(fetchSystem()).rejects.toMatchObject({ status: 401, code: 'unauthorized' })
    expect(handler).toHaveBeenCalledOnce()
    off()
  })

  it('활동 신호의 401도 처리 함수를 부른다', async () => {
    const handler = vi.fn()
    const off = setUnauthorizedHandler(handler)
    stubFetch(unauthorized())
    await expect(sendActivity()).rejects.toBeInstanceOf(ApiError)
    expect(handler).toHaveBeenCalledOnce()
    off()
  })

  it('로그인 관련 요청의 401은 처리 함수를 부르지 않는다', async () => {
    const handler = vi.fn()
    const off = setUnauthorizedHandler(handler)
    stubFetch(unauthorized())
    await expect(fetchSession()).rejects.toMatchObject({ status: 401 })
    stubFetch(unauthorized())
    await expect(login('a', 'b')).rejects.toMatchObject({ status: 401 })
    expect(handler).not.toHaveBeenCalled()
    off()
  })

  it('해제한 뒤에는 부르지 않는다', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)()
    stubFetch(unauthorized())
    await expect(fetchSystem()).rejects.toBeInstanceOf(ApiError)
    expect(handler).not.toHaveBeenCalled()
  })
})

describe('login', () => {
  it('JSON 본문과 Content-Type으로 POST한다', async () => {
    const fn = stubFetch(Response.json({ status: 'authenticated', session: { username: 'admin', admin: true, assurance: 'login' } }))
    await expect(login('admin', 'pw')).resolves.toEqual({
      status: 'authenticated',
      session: { username: 'admin', admin: true, assurance: 'login' },
    })
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/login$/)
    expect(init?.method).toBe('POST')
    expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json')
    expect(JSON.parse(String(init?.body))).toEqual({ username: 'admin', password: 'pw' })
  })

  it('오류 코드와 재시도 시간을 ApiError에 담는다', async () => {
    stubFetch(Response.json({ error: 'x', code: 'rate_limited', retryAfterSeconds: 30 }, { status: 429 }))
    await expect(login('admin', 'pw')).rejects.toMatchObject({ status: 429, code: 'rate_limited', retryAfterSeconds: 30 })
  })

  it('모르는 오류 코드는 버린다', async () => {
    stubFetch(Response.json({ error: 'x', code: 'weird' }, { status: 400 }))
    const err = await login('a', 'b').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).code).toBeUndefined()
  })

  it('로그아웃은 빈 204 응답을 받는다', async () => {
    stubFetch(new Response(null, { status: 204 }))
    await expect(logout()).resolves.toBeUndefined()
  })
})

describe('isLoginResponse', () => {
  it('상태에 맞는 필드를 요구한다', () => {
    expect(isLoginResponse({ status: 'authenticated', session: { username: 'a', admin: false, assurance: 'login' } })).toBe(true)
    expect(
      isLoginResponse({
        status: 'authenticated',
        session: { username: 'a', admin: false, assurance: 'login', expiresInSeconds: 300 },
      }),
    ).toBe(true)
    expect(
      isLoginResponse({
        status: 'authenticated',
        session: { username: 'a', admin: false, assurance: 'login', expiresInSeconds: -1 },
      }),
    ).toBe(false)
    // 보증 수준이 없거나 모르는 값이면 받지 않는다.
    expect(isLoginResponse({ status: 'authenticated', session: { username: 'a', admin: false } })).toBe(false)
    expect(isLoginResponse({ status: 'authenticated', session: { username: 'a', admin: false, assurance: 'x' } })).toBe(false)
    // 로그인 직후의 2차 인증 권유는 아는 수단만 받는다.
    const session = { username: 'a', admin: false, assurance: 'login' }
    expect(isLoginResponse({ status: 'authenticated', session, stepUp: { methods: ['totp', 'recovery'] } })).toBe(true)
    expect(isLoginResponse({ status: 'authenticated', session, stepUp: { methods: ['sms'] } })).toBe(false)
    // 패스키로 로그인 권유는 기간(일)이 든 객체만 받는다.
    expect(isLoginResponse({ status: 'authenticated', session, passkeyLoginOffer: { days: 30 } })).toBe(true)
    expect(isLoginResponse({ status: 'authenticated', session, passkeyLoginOffer: true })).toBe(false)
    expect(isLoginResponse({ status: 'authenticated', session, passkeyLoginOffer: { days: '30' } })).toBe(false)
    expect(isLoginResponse({ status: 'authenticated' })).toBe(false)
    expect(isLoginResponse({ status: 'challengeRequired', challenge: { kind: 'totp' } })).toBe(true)
    expect(isLoginResponse({ status: 'challengeRequired', challenge: { kind: 1 } })).toBe(false)
    expect(isLoginResponse({ status: 'other' })).toBe(false)
    expect(isLoginResponse(null)).toBe(false)
  })
})

describe('isCapabilitiesResponse', () => {
  const ok = {
    addons: [
      { id: 'system-info', available: true, sources: ['proc'] },
      { id: 'containers', available: false, reason: 'plugin_missing', sources: [] },
    ],
    plugins: ['openmediavault-compose'],
  }

  it('정상 응답을 받는다', () => {
    expect(isCapabilitiesResponse(ok)).toBe(true)
    expect(isCapabilitiesResponse({ addons: [], plugins: [] })).toBe(true)
  })

  it.each([
    ['null', null],
    ['addons 없음', { plugins: [] }],
    ['plugins 없음', { addons: [] }],
    ['plugins가 문자열 아님', { addons: [], plugins: [1] }],
    ['id 없음', { addons: [{ available: true, sources: [] }], plugins: [] }],
    ['available이 문자열', { addons: [{ id: 'a', available: 'yes', sources: [] }], plugins: [] }],
    ['모르는 reason', { addons: [{ id: 'a', available: false, reason: 'nope', sources: [] }], plugins: [] }],
    ['sources 없음', { addons: [{ id: 'a', available: true }], plugins: [] }],
  ])('%s는 거부한다', (_name, v) => {
    expect(isCapabilitiesResponse(v)).toBe(false)
  })

  it('fetchCapabilities는 형식이 다르면 던진다', async () => {
    stubFetch(Response.json({ addons: 'x' }))
    await expect(fetchCapabilities()).rejects.toThrow('unexpected response shape')
    stubFetch(Response.json(ok))
    await expect(fetchCapabilities()).resolves.toEqual(ok)
  })
})

describe('preferences', () => {
  const placement = { addon: 'system-info', right: 0, top: 0, cols: 3, rows: 2 }

  it.each([
    ['저장한 적 없음', {}],
    ['빈 배치', { widgets: [] }],
    ['위젯 하나', { widgets: [placement] }],
    ['빈 바로가기', { shortcuts: [] }],
    ['위젯과 바로가기', { widgets: [placement], shortcuts: [{ app: 'files' }] }],
    ['모든 앱 설정', { drawer: { sort: 'name' } }],
    ['모르는 정렬 방식(다른 버전)', { drawer: { sort: 'later' } }],
    ['시스템 정보 설정', { systemInfo: { cpuScale: 'core', cpuGraph: 'cores' } }],
    ['모르는 시스템 정보 값(다른 버전)', { systemInfo: { cpuScale: 'irix', cpuGraph: 'heatmap' } }],
  ])('%s를 받는다', (_name, v) => {
    expect(isPreferences(v)).toBe(true)
  })

  it.each([
    ['null', null],
    ['widgets가 배열 아님', { widgets: {} }],
    ['addon 없음', { widgets: [{ ...placement, addon: undefined }] }],
    ['음수 좌표', { widgets: [{ ...placement, right: -1 }] }],
    ['소수 좌표', { widgets: [{ ...placement, top: 0.5 }] }],
    ['크기가 문자열', { widgets: [{ ...placement, cols: '3' }] }],
    ['shortcuts가 배열 아님', { shortcuts: 'files' }],
    ['바로가기가 문자열', { shortcuts: ['files'] }],
    ['app이 숫자', { shortcuts: [{ app: 1 }] }],
    ['drawer가 문자열', { drawer: 'name' }],
    ['sort 없음', { drawer: {} }],
    ['systemInfo가 문자열', { systemInfo: 'core' }],
    ['cpuGraph 없음', { systemInfo: { cpuScale: 'core' } }],
  ])('%s는 거부한다', (_name, v) => {
    expect(isPreferences(v)).toBe(false)
  })

  it('fetchPreferences는 형식이 다르면 던진다', async () => {
    stubFetch(Response.json({ widgets: 'x' }))
    await expect(fetchPreferences()).rejects.toThrow('unexpected response shape')
  })

  it('fetchPreferences는 동시에 부르면 요청 하나를 함께 쓰고, 끝난 뒤에는 새로 묻는다', async () => {
    const fn = stubFetch(Response.json({ widgets: [] }))
    const [a, b] = await Promise.all([fetchPreferences(), fetchPreferences()])
    expect(a).toEqual({ widgets: [] })
    expect(b).toBe(a)
    expect(fn).toHaveBeenCalledTimes(1)
    fn.mockResolvedValueOnce(Response.json({ shortcuts: [] }))
    await expect(fetchPreferences()).resolves.toEqual({ shortcuts: [] })
    expect(fn).toHaveBeenCalledTimes(2)
  })

  it('saveWidgets는 JSON 본문으로 PUT하고 204를 받는다', async () => {
    const fn = stubFetch(new Response(null, { status: 204 }))
    await saveWidgets([placement])
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/preferences\/widgets$/)
    expect(init?.method).toBe('PUT')
    expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json')
    expect(JSON.parse(String(init?.body))).toEqual({ widgets: [placement] })
  })

  it('saveShortcuts는 JSON 본문으로 PUT한다', async () => {
    const fn = stubFetch(new Response(null, { status: 204 }))
    await saveShortcuts([{ app: 'files' }, { app: 'storage' }])
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/preferences\/shortcuts$/)
    expect(init?.method).toBe('PUT')
    expect(new Headers(init?.headers).get('Content-Type')).toBe('application/json')
    expect(JSON.parse(String(init?.body))).toEqual({ shortcuts: [{ app: 'files' }, { app: 'storage' }] })
  })

  it('saveDrawer는 JSON 본문으로 PUT한다', async () => {
    const fn = stubFetch(new Response(null, { status: 204 }))
    await saveDrawer({ sort: 'name' })
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/preferences\/drawer$/)
    expect(init?.method).toBe('PUT')
    expect(JSON.parse(String(init?.body))).toEqual({ drawer: { sort: 'name' } })
  })

  it('saveWidgets의 401은 로그인 화면으로 보낸다', async () => {
    const onUnauthorized = vi.fn()
    const off = setUnauthorizedHandler(onUnauthorized)
    stubFetch(unauthorized())
    await expect(saveWidgets([])).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).toHaveBeenCalledOnce()
    off()
  })
})

describe('fetchAddon', () => {
  const isThing = (v: unknown): v is { n: number } =>
    typeof v === 'object' && v !== null && typeof (v as { n?: unknown }).n === 'number'

  it('애드온 경로로 GET하고 refresh면 쿼리를 붙인다', async () => {
    const fn = stubFetch(Response.json({ n: 1 }))
    await expect(fetchAddon('smart', isThing)).resolves.toEqual({ n: 1 })
    expect(fn.mock.calls[0][0]).toMatch(/\/api\/addons\/smart$/)
    const fn2 = stubFetch(Response.json({ n: 2 }))
    await fetchAddon('smart', isThing, { refresh: true })
    expect(fn2.mock.calls[0][0]).toMatch(/\/api\/addons\/smart\?refresh=1$/)
    const fn3 = stubFetch(Response.json({ n: 3 }))
    await fetchAddon('system-info', isThing, { query: { processes: '1' }, refresh: true })
    expect(fn3.mock.calls[0][0]).toMatch(/\/api\/addons\/system-info\?processes=1&refresh=1$/)
  })

  it('형식이 다르면 던진다', async () => {
    stubFetch(Response.json({ n: 'x' }))
    await expect(fetchAddon('smart', isThing)).rejects.toThrow(/unexpected response shape/)
  })

  it('capability 오류 코드를 ApiError에 담는다', async () => {
    stubFetch(Response.json({ error: 'addon is not available', code: 'admin_required' }, { status: 403 }))
    await expect(fetchAddon('smart', isThing)).rejects.toMatchObject({ status: 403, code: 'admin_required' })
  })

  it('hasAddonSources는 출처 목록을 검사한다', () => {
    expect(hasAddonSources({ sources: ['proc'] })).toBe(true)
    expect(hasAddonSources({ sources: ['proc'], failedSources: ['omv-smart'] })).toBe(true)
    expect(hasAddonSources({ sources: 'proc' })).toBe(false)
    expect(hasAddonSources({ sources: [], failedSources: [1] })).toBe(false)
  })
})

describe('runAddonAction', () => {
  it('대상을 본문에 담아 조작 경로로 보낸다', async () => {
    const fn = stubFetch(new Response(null, { status: 204 }))
    await expect(runAddonAction('containers', 'stop', 'web')).resolves.toBeUndefined()
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/addons\/containers\/actions\/stop$/)
    expect(init?.method).toBe('POST')
    expect(JSON.parse(String(init?.body))).toEqual({ target: 'web' })
  })

  it('실패하면 오류 코드를 담은 ApiError를 던진다', async () => {
    stubFetch(Response.json({ error: 'pending changes', code: 'config_dirty' }, { status: 409 }))
    await expect(runAddonAction('services', 'disable', 'ssh')).rejects.toMatchObject({ status: 409, code: 'config_dirty' })
  })
})

// 모두 합성한 값이다.
const securityStatus = {
  assurance: 'login',
  manageSeconds: 0,
  passkeysAvailable: true,
  passkeys: [{ id: 'cred1', name: 'Laptop', created: '2026-01-01T00:00:00Z' }],
  recoveryCodes: 10,
  methods: ['passkey', 'recovery'],
  policy: { terminal: 'twoFactor', passkeyLoginDays: 30 },
  passkeyLogins: [{ id: 'login1', passkey: 'cred1', created: '2026-01-01T00:00:00Z', expiresAt: '2026-01-31T00:00:00Z' }],
}

describe('isSecurityStatus', () => {
  it('패스키로 로그인 등록과 만료 기간을 받는다', () => {
    expect(isSecurityStatus(securityStatus)).toBe(true)
    expect(isSecurityStatus({ ...securityStatus, passkeyLogins: [] })).toBe(true)
    expect(isSecurityStatus({ ...securityStatus, policy: {} })).toBe(true)
    const used = { ...securityStatus.passkeyLogins[0], lastUsed: '2026-01-02T00:00:00Z' }
    expect(isSecurityStatus({ ...securityStatus, passkeyLogins: [used] })).toBe(true)
  })

  it.each([
    ['passkeyLogins 없음', { ...securityStatus, passkeyLogins: undefined }],
    ['passkeyLogins가 null', { ...securityStatus, passkeyLogins: null }],
    ['등록의 패스키 없음', { ...securityStatus, passkeyLogins: [{ id: 'l', created: 'x', expiresAt: 'y' }] }],
    ['등록의 만료가 숫자', { ...securityStatus, passkeyLogins: [{ id: 'l', passkey: 'c', created: 'x', expiresAt: 1 }] }],
    ['lastUsed가 숫자', { ...securityStatus, passkeyLogins: [{ ...securityStatus.passkeyLogins[0], lastUsed: 1 }] }],
    ['만료 기간이 문자열', { ...securityStatus, policy: { passkeyLoginDays: '30' } }],
  ])('%s는 거부한다', (_name, v) => {
    expect(isSecurityStatus(v)).toBe(false)
  })
})

describe('패스키로 로그인', () => {
  const assertion = { id: 'cred1', clientDataJSON: 'Ag', authenticatorData: 'Aw', signature: 'BA' }

  it('enrollPasskeyLogin은 응답의 상태와 만료 시각을 검사한다', async () => {
    const ok = { id: 'login1', share: 'c2hhcmU', expiresAt: '2026-01-31T00:00:00Z', status: securityStatus }
    const fn = stubFetch(Response.json(ok))
    await expect(enrollPasskeyLogin({ password: 'pw', passkey: assertion, replace: 'old' })).resolves.toEqual(ok)
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/security\/passkey-logins$/)
    expect(init?.method).toBe('POST')
    expect(JSON.parse(String(init?.body))).toEqual({ password: 'pw', passkey: assertion, replace: 'old' })
    stubFetch(Response.json({ ...ok, expiresAt: 'later' }))
    await expect(enrollPasskeyLogin({ password: 'pw', passkey: assertion })).rejects.toThrow(/unexpected response shape/)
    stubFetch(Response.json({ ...ok, status: { ...securityStatus, passkeyLogins: undefined } }))
    await expect(enrollPasskeyLogin({ password: 'pw', passkey: assertion })).rejects.toThrow(/unexpected response shape/)
  })

  it('deletePasskeyLogin은 등록 ID를 경로에 넣고 보안 상태를 받는다', async () => {
    const fn = stubFetch(Response.json(securityStatus))
    await expect(deletePasskeyLogin('a/b')).resolves.toEqual(securityStatus)
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/security\/passkey-logins\/a%2Fb$/)
    expect(init?.method).toBe('DELETE')
  })

  it('로그인 단계 요청은 401에 처리 함수를 부르지 않고 오류 코드를 담는다', async () => {
    const handler = vi.fn()
    const off = setUnauthorizedHandler(handler)
    stubFetch(Response.json({ error: 'x', code: 'passkey_login_unknown' }, { status: 400 }))
    await expect(passkeyLoginOptions('alice', 'login1')).rejects.toMatchObject({ status: 400, code: 'passkey_login_unknown' })
    stubFetch(Response.json({ error: 'x', code: 'passkey_login_expired' }, { status: 400 }))
    await expect(passkeyLoginOptions('alice', 'login1')).rejects.toMatchObject({ code: 'passkey_login_expired' })
    stubFetch(Response.json({ error: 'x', code: 'passkey_login_stale' }, { status: 400 }))
    await expect(finishPasskeyLogin('pw', true)).rejects.toMatchObject({ code: 'passkey_login_stale' })
    stubFetch(unauthorized())
    await expect(passkeyLoginUnlock(assertion)).rejects.toMatchObject({ status: 401 })
    expect(handler).not.toHaveBeenCalled()
    off()
  })

  it('passkeyLoginOptions는 사용자 이름과 등록 ID를 보내고 옵션을 받는다', async () => {
    const opts = { challenge: 'AAEC', rpId: 'desk.example.lan', timeoutMs: 120000, credentials: [{ id: 'cred1' }] }
    const fn = stubFetch(Response.json(opts))
    await expect(passkeyLoginOptions('alice', 'login1')).resolves.toEqual(opts)
    const [url, init] = fn.mock.calls[0]
    expect(url).toMatch(/\/api\/login\/passkey\/options$/)
    expect(JSON.parse(String(init?.body))).toEqual({ username: 'alice', enrollment: 'login1' })
  })

  it('passkeyLoginUnlock은 share와 nextShare만 꺼낸다', async () => {
    const fn = stubFetch(Response.json({ share: 'c2hhcmU', nextShare: 'bmV4dA', other: 1 }))
    await expect(passkeyLoginUnlock(assertion)).resolves.toEqual({ share: 'c2hhcmU', nextShare: 'bmV4dA' })
    expect(fn.mock.calls[0][0]).toMatch(/\/api\/login\/passkey$/)
    expect(JSON.parse(String(fn.mock.calls[0][1]?.body))).toEqual({ passkey: assertion })
    for (const bad of [{ share: 1, nextShare: 'bmV4dA' }, { share: 'c2hhcmU' }, { share: 'c2hhcmU', nextShare: null }]) {
      stubFetch(Response.json(bad))
      await expect(passkeyLoginUnlock(assertion)).rejects.toThrow(/unexpected response shape/)
    }
  })

  it('finishPasskeyLogin은 비밀번호와 회전 여부를 보내고 로그인 응답을 받는다', async () => {
    const res = { status: 'authenticated', session: { username: 'alice', admin: false, assurance: 'twoFactor' } }
    const fn = stubFetch(Response.json(res))
    await expect(finishPasskeyLogin('pw', true)).resolves.toEqual(res)
    expect(fn.mock.calls[0][0]).toMatch(/\/api\/login\/passkey\/finish$/)
    expect(JSON.parse(String(fn.mock.calls[0][1]?.body))).toEqual({ password: 'pw', rotated: true })
    const again = stubFetch(Response.json(res))
    await finishPasskeyLogin('pw', false)
    expect(JSON.parse(String(again.mock.calls[0][1]?.body))).toEqual({ password: 'pw', rotated: false })
  })

  it('discardPasskeyLogin은 discard를 보내고 204를 받는다', async () => {
    const fn = stubFetch(new Response(null, { status: 204 }))
    await expect(discardPasskeyLogin()).resolves.toBeUndefined()
    expect(fn.mock.calls[0][0]).toMatch(/\/api\/login\/passkey\/finish$/)
    expect(JSON.parse(String(fn.mock.calls[0][1]?.body))).toEqual({ discard: true })
  })
})

describe('copyFiles', () => {
  // stream은 SSE 글자를 조각마다 보내는 응답이다(사건이 조각 사이에서 끊긴다).
  const stream = (chunks: string[]) =>
    new Response(
      new ReadableStream<Uint8Array>({
        start(c) {
          for (const s of chunks) c.enqueue(new TextEncoder().encode(s))
          c.close()
        },
      }),
      { headers: { 'Content-Type': 'text/event-stream' } },
    )
  const progress = { files: 1, bytes: 10, totalFiles: 2, totalBytes: 20 }

  it('사건을 조각에 걸쳐 읽는다', () => {
    const first = parseCopyEvents('data: {"a":1}\n\ndata: {"b"')
    expect(first.events).toEqual([{ a: 1 }])
    expect(first.rest).toBe('data: {"b"')
    const second = parseCopyEvents(first.rest + ':2}\r\n\r\n: comment\n\ndata: not json\n\n')
    expect(second.events).toEqual([{ b: 2 }])
    expect(second.rest).toBe('')
  })

  it('진행을 알리고 결과를 돌려준다', async () => {
    const fn = stubFetch(
      stream([
        `data: ${JSON.stringify({ progress: { ...progress, scanning: true } })}\n`,
        `\ndata: ${JSON.stringify({ progress })}\n\nda`,
        `ta: ${JSON.stringify({ result: { copied: ['a'], failed: [{ name: 'b', code: 'file_exists' }, { name: 'c', code: 'nope' }], skipped: 1 } })}\n\n`,
      ]),
    )
    const seen: unknown[] = []
    const res = await copyFiles({ dir: '/srv/d/a', names: ['a', 'b', 'c'], to: '/home/u' }, (p) => seen.push(p))
    expect(seen).toEqual([{ ...progress, scanning: true }, progress])
    expect(res).toEqual({
      copied: ['a'],
      failed: [
        { name: 'b', code: 'file_exists' },
        { name: 'c', code: 'helper_unavailable' },
      ],
      skipped: 1,
    })
    const init = fn.mock.calls[0][1]
    expect(init?.method).toBe('POST')
    expect(JSON.parse(String(init?.body))).toEqual({ dir: '/srv/d/a', names: ['a', 'b', 'c'], to: '/home/u' })
  })

  it('시작한 뒤의 오류와 끊긴 응답을 던진다', async () => {
    stubFetch(stream([`data: ${JSON.stringify({ progress })}\n\ndata: ${JSON.stringify({ error: { error: 'x', code: 'no_space' }, status: 507 })}\n\n`]))
    const err = await copyFiles({ dir: '/a', names: ['b'], to: '/c' }, () => {}).catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect(err).toMatchObject({ status: 507, code: 'no_space' })

    stubFetch(stream([`data: ${JSON.stringify({ progress })}\n\n`]))
    await expect(copyFiles({ dir: '/a', names: ['b'], to: '/c' }, () => {})).rejects.toBeInstanceOf(TypeError)

    stubFetch(Response.json({ error: 'x', code: 'security_required', required: 'password' }, { status: 403 }))
    await expect(copyFiles({ dir: '/a', names: ['b'], to: '/c' }, () => {})).rejects.toMatchObject({ code: 'security_required' })
  })
})
