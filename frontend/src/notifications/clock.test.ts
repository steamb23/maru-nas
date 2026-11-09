import { describe, expect, it } from 'vitest'
import { clockSkew } from './clock'

describe('clockSkew', () => {
  it('is the server clock minus the browser clock', () => {
    expect(clockSkew('2026-11-09T12:00:05Z', Date.parse('2026-11-09T12:00:00Z'))).toBe(5000)
    expect(clockSkew('2026-11-09T21:00:00+09:00', Date.parse('2026-11-09T12:00:30Z'))).toBe(-30_000)
  })

  it('is zero when the server time is missing', () => {
    expect(clockSkew('', Date.parse('2026-11-09T12:00:00Z'))).toBe(0)
  })
})
