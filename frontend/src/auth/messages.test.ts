import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { PasskeyLoginBrokenError, PrfUnsupportedError } from '../security/passkeyLoginFlow'
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

  it('패스키로 로그인의 실패', () => {
    expect(loginErrorMessage(new ApiError(400, 'x', 'passkey_login_unknown')).key).toBe('login.error.passkeyUnknown')
    expect(loginErrorMessage(new ApiError(400, 'x', 'passkey_login_expired')).key).toBe('login.error.passkeyExpired')
    expect(loginErrorMessage(new ApiError(400, 'x', 'passkey_login_stale')).key).toBe('login.error.passkeyStale')
    expect(loginErrorMessage(new ApiError(400, 'x', 'factor_failed')).key).toBe('login.error.passkeyFailed')
    expect(loginErrorMessage(new ApiError(409, 'x', 'passkey_unavailable')).key).toBe('login.error.passkeyUnavailable')
    expect(loginErrorMessage(new PrfUnsupportedError()).key).toBe('login.error.passkeyPrf')
    expect(loginErrorMessage(new PasskeyLoginBrokenError()).key).toBe('login.error.passkeyBroken')
  })

  it('패스키 창을 취소했는지 구분한다', () => {
    expect(loginErrorMessage(new DOMException('x', 'NotAllowedError')).key).toBe('login.error.passkeyCanceled')
    expect(loginErrorMessage(new DOMException('x', 'AbortError')).key).toBe('login.error.passkeyCanceled')
    expect(loginErrorMessage(new DOMException('x', 'SecurityError')).key).toBe('login.error.passkeyFailed')
  })
})
