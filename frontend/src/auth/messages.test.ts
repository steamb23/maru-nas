import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { loginErrorMessage } from './messages'

describe('loginErrorMessage', () => {
  it('오류 코드별 문구', () => {
    expect(loginErrorMessage(new ApiError(400, 'x', 'invalid_credentials')).message).toMatch(/비밀번호/)
    expect(loginErrorMessage(new ApiError(400, 'x', 'challenge_failed')).message).toMatch(/인증 코드/)
    expect(loginErrorMessage(new ApiError(502, 'x', 'omv_unavailable')).message).toMatch(/OpenMediaVault/)
  })

  it('재시도 시간을 함께 넘긴다', () => {
    expect(loginErrorMessage(new ApiError(429, 'x', 'rate_limited', 30))).toMatchObject({ retryAfterSeconds: 30 })
  })

  it('네트워크 오류와 코드 없는 HTTP 오류', () => {
    expect(loginErrorMessage(new TypeError('Failed to fetch')).message).toMatch(/서버에 연결/)
    expect(loginErrorMessage(new ApiError(502, 'x')).message).toMatch(/HTTP 502/)
  })
})
