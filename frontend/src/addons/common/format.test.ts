import { describe, expect, it } from 'vitest'
import { formatBytes, formatLinkSpeed, formatPercent, formatRate, formatUnit, formatUptime, percentOf } from './format'

describe('formatBytes', () => {
  it.each([
    [0, '0 B'],
    [512, '512 B'],
    [1536, '1.5 KiB'],
    [300 * 1024 ** 2, '300.0 MiB'],
    [16 * 1024 ** 3, '16.0 GiB'],
    [5.25 * 1024 ** 4, '5.3 TiB'],
  ])('%d → %s', (bytes, want) => {
    expect(formatBytes(bytes, 'ko')).toBe(want)
    expect(formatBytes(bytes, 'en')).toBe(want)
  })

  it('소수 구분자는 locale을 따른다', () => {
    expect(formatBytes(1536, 'de')).toBe('1,5 KiB')
  })
})

describe('formatUptime', () => {
  it.each([
    [0, '0분', '0 minutes'],
    [59, '0분', '0 minutes'],
    [60, '1분', '1 minute'],
    [60 * 12, '12분', '12 minutes'],
    [3600 + 60, '1시간 1분', '1 hour 1 minute'],
    [3600 * 4 + 60 * 12, '4시간 12분', '4 hours 12 minutes'],
    [86400 * 3 + 60 * 12, '3일 0시간 12분', '3 days 0 hours 12 minutes'],
    [86400 * 3 + 3600 * 4 + 60 * 12 + 30.5, '3일 4시간 12분', '3 days 4 hours 12 minutes'],
  ])('%d초 → %s / %s', (sec, ko, en) => {
    expect(formatUptime(sec, 'ko')).toBe(ko)
    expect(formatUptime(sec, 'en')).toBe(en)
  })

  it('narrow는 단위를 짧게 쓴다', () => {
    const sec = 86400 * 14 + 3600 * 3 + 60 * 31
    expect(formatUptime(sec, 'en', 'narrow')).toBe('14d 3h 31m')
    expect(formatUptime(sec, 'ko', 'narrow')).toBe('14일 3시간 31분')
  })
})

describe('formatUnit', () => {
  it('복수형을 locale에 맞춘다', () => {
    expect(formatUnit(30, 'second', 'ko')).toBe('30초')
    expect(formatUnit(1, 'second', 'en')).toBe('1 second')
    expect(formatUnit(30, 'second', 'en')).toBe('30 seconds')
  })
})

describe('formatPercent / percentOf', () => {
  it('소수점 한 자리로 표시한다', () => {
    expect(formatPercent(21, 'ko')).toBe('21.0%')
    expect(formatPercent(3.14159, 'en')).toBe('3.1%')
  })

  it('비율을 0~100으로 계산하고 total이 0이면 0', () => {
    expect(percentOf(25, 100)).toBe(25)
    expect(percentOf(5, 0)).toBe(0)
    expect(percentOf(200, 100)).toBe(100)
  })
})

describe('formatRate, formatLinkSpeed', () => {
  it('초당 바이트에 /s를 붙인다', () => {
    expect(formatRate(1536, 'en')).toBe('1.5 KiB/s')
  })
  it('링크 속도는 1000 이상이면 Gbps', () => {
    expect(formatLinkSpeed(2500, 'en')).toBe('2.5 Gbps')
    expect(formatLinkSpeed(1000, 'en')).toBe('1 Gbps')
    expect(formatLinkSpeed(100, 'en')).toBe('100 Mbps')
  })
})
