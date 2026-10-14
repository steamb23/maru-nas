import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { loginErrorMessage } from './messages'

describe('loginErrorMessage', () => {
  it('오류 코드별 문구 키', () => {
    expect(loginErrorMessage(new ApiError(400, 'x', 'invalid_credentials')).key).toBe('login.error.invalidCredentials')
    expect(loginErrorMessage(new ApiError(400, 'x', 'challenge_failed')).key).toBe('login.error.challengeFailed')
    expect(loginErrorMessage(new ApiError(502, 'x', 'omv_unavailable')).key).toBe('login.error.omvUnavailable')
    expect(loginErrorMessage(new ApiError(401, 'x', 'unauthorized')).key).toBe('login.error.unauthorized')
  })

  it('재시도 시간을 함께 넘긴다', () => {
    expect(loginErrorMessage(new ApiError(429, 'x', 'rate_limited', 30))).toMatchObject({
      key: 'login.error.rateLimited',
      retryAfterSeconds: 30,
    })
  })

  it('네트워크 오류와 코드 없는 HTTP 오류', () => {
    expect(loginErrorMessage(new TypeError('Failed to fetch')).key).toBe('login.error.network')
    expect(loginErrorMessage(new ApiError(502, 'x'))).toEqual({ key: 'login.error.httpStatus', params: { status: 502 } })
  })
})
