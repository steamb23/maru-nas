import { describe, expect, it } from 'vitest'
import { formatOMVVersion, formatRevisionTime, shortRevision } from './about'

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

describe('formatOMVVersion', () => {
  const base = { version: '0.2', goVersion: 'go1.0' }

  it('버전과 코드명을 OMV 대시보드처럼 보여 준다', () => {
    expect(formatOMVVersion({ ...base, omvVersion: '8.0.0-1', omvVersionName: 'Synthetic' })).toBe(
      '8.0.0-1 (Synthetic)',
    )
  })

  it('코드명이 없으면 버전만 보여 준다', () => {
    expect(formatOMVVersion({ ...base, omvVersion: '8.0.0-1' })).toBe('8.0.0-1')
  })

  it('버전이 없으면 코드명이 있어도 null이다', () => {
    expect(formatOMVVersion(base)).toBeNull()
    expect(formatOMVVersion({ ...base, omvVersionName: 'Synthetic' })).toBeNull()
  })
})
