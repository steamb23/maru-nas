import { describe, expect, it } from 'vitest'
import { formatClockDate, formatClockTime, msUntilNextMinute } from './clock'

describe('msUntilNextMinute', () => {
  it('다음 분 경계까지 남은 시간', () => {
    const base = Date.UTC(2026, 0, 1, 10, 42, 0)
    expect(msUntilNextMinute(base)).toBe(60_000)
    expect(msUntilNextMinute(base + 1)).toBe(59_999)
    expect(msUntilNextMinute(base + 59_500)).toBe(500)
  })
})

describe('formatClockTime', () => {
  const d = new Date(Date.UTC(2026, 0, 1, 14, 5, 0))

  it('로케일에 맞는 시:분', () => {
    expect(formatClockTime(d, 'en-GB', 'UTC')).toBe('14:05')
    expect(formatClockTime(d, 'en-US', 'UTC')).toMatch(/^2:05\sPM$/)
    expect(formatClockTime(d, 'ko-KR', 'UTC')).toMatch(/오후\s?2:05/)
  })

  it('날짜 툴팁', () => {
    expect(formatClockDate(d, 'ko-KR', 'UTC')).toMatch(/2026년 1월 1일/)
  })
})
