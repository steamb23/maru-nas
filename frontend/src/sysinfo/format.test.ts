import { describe, expect, it } from 'vitest'
import { formatBytes, formatPercent, formatUptime, percentOf } from './format'

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [512, '512 B'],
    [1536, '1.5 KiB'],
    [300 * 1024 ** 2, '300.0 MiB'],
    [16 * 1024 ** 3, '16.0 GiB'],
    [5.25 * 1024 ** 4, '5.3 TiB'],
  ])('%d → %s', (bytes, want) => {
    expect(formatBytes(bytes)).toBe(want)
  })
})

describe('formatUptime', () => {
  it.each([
    [0, '0분'],
    [59, '0분'],
    [60 * 12, '12분'],
    [3600 * 4 + 60 * 12, '4시간 12분'],
    [86400 * 3 + 60 * 12, '3일 0시간 12분'],
    [86400 * 3 + 3600 * 4 + 60 * 12 + 30.5, '3일 4시간 12분'],
  ])('%d초 → %s', (sec, want) => {
    expect(formatUptime(sec)).toBe(want)
  })
})

describe('formatPercent / percentOf', () => {
  it('소수점 한 자리로 표시한다', () => {
    expect(formatPercent(21)).toBe('21.0%')
    expect(formatPercent(3.14159)).toBe('3.1%')
  })

  it('비율을 0~100으로 계산하고 total이 0이면 0', () => {
    expect(percentOf(25, 100)).toBe(25)
    expect(percentOf(5, 0)).toBe(0)
    expect(percentOf(200, 100)).toBe(100)
  })
})
