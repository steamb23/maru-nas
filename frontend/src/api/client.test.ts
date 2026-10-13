import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, fetchSystem } from './client'

const valid = {
  cpu: { usagePercent: 12.5 },
  memory: { totalBytes: 100, usedBytes: 40, availableBytes: 60 },
  uptimeSeconds: 99,
}

function stubFetch(res: Response | Error) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => {
      if (res instanceof Error) throw res
      return res
    }),
  )
}

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
