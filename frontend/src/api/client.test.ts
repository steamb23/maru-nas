import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  fetchAddon,
  fetchCapabilities,
  fetchPreferences,
  fetchSession,
  fetchSystem,
  hasAddonSources,
  isCapabilitiesResponse,
  isLoginResponse,
  isPreferences,
  login,
  logout,
  runAddonAction,
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
    const fn = stubFetch(Response.json({ status: 'authenticated', session: { username: 'admin', admin: true } }))
    await expect(login('admin', 'pw')).resolves.toEqual({
      status: 'authenticated',
      session: { username: 'admin', admin: true },
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
    expect(isLoginResponse({ status: 'authenticated', session: { username: 'a', admin: false } })).toBe(true)
    expect(
      isLoginResponse({ status: 'authenticated', session: { username: 'a', admin: false, expiresInSeconds: 300 } }),
    ).toBe(true)
    expect(
      isLoginResponse({ status: 'authenticated', session: { username: 'a', admin: false, expiresInSeconds: -1 } }),
    ).toBe(false)
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
  ])('%s는 거부한다', (_name, v) => {
    expect(isPreferences(v)).toBe(false)
  })

  it('fetchPreferences는 형식이 다르면 던진다', async () => {
    stubFetch(Response.json({ widgets: 'x' }))
    await expect(fetchPreferences()).rejects.toThrow('unexpected response shape')
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
