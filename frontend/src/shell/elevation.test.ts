import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import { elevationError, formatClock } from './elevation'

describe('elevationError', () => {
  it('오류 코드마다 문구가 다르다', () => {
    expect(elevationError(new ApiError(400, 'x', 'elevation_failed'))).toEqual({ key: 'elevation.wrongPassword' })
    expect(elevationError(new ApiError(429, 'x', 'rate_limited', 30))).toEqual({
      key: 'elevation.rateLimited',
      retryAfter: 30,
    })
    expect(elevationError(new ApiError(403, 'x', 'elevation_not_allowed')).key).toBe('elevation.notAllowed')
    expect(elevationError(new ApiError(403, 'x', 'admin_required')).key).toBe('elevation.notAllowed')
    expect(elevationError(new ApiError(409, 'x', 'helper_reauth')).key).toBe('elevation.reauth')
    expect(elevationError(new ApiError(503, 'x', 'helper_unavailable')).key).toBe('elevation.unavailable')
  })

  it('모르는 오류는 일반 실패', () => {
    expect(elevationError(new ApiError(502, 'x')).key).toBe('elevation.failed')
    expect(elevationError(new Error('network')).key).toBe('elevation.failed')
  })
})

describe('formatClock', () => {
  it('m:ss로 쓴다', () => {
    expect(formatClock(300)).toBe('5:00')
    expect(formatClock(61)).toBe('1:01')
    expect(formatClock(0.2)).toBe('0:01')
    expect(formatClock(-5)).toBe('0:00')
  })
})
