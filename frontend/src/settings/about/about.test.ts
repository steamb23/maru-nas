import { describe, expect, it } from 'vitest'
import { formatRevisionTime, shortRevision } from './about'

describe('shortRevision', () => {
  it('앞 7자로 줄인다', () => {
    expect(shortRevision('0123456789abcdef')).toBe('0123456')
  })
})

describe('formatRevisionTime', () => {
  const base = { version: '0.2', goVersion: 'go1.0' }

  it('화면 언어로 날짜와 시각을 보여 준다', () => {
    expect(formatRevisionTime({ ...base, revisionTime: '2026-01-02T03:04:05Z' }, 'en', 'UTC')).toBe(
      'Jan 2, 2026, 3:04 AM',
    )
  })

  it('시각이 없거나 읽을 수 없으면 null이다', () => {
    expect(formatRevisionTime(base, 'en')).toBeNull()
    expect(formatRevisionTime({ ...base, revisionTime: 'nope' }, 'en')).toBeNull()
  })
})
