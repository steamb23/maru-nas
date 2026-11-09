import { describe, expect, it } from 'vitest'
import type { Notice } from '../api/types.gen'
import {
  IMMINENT_MS,
  MAX_WAIT_MS,
  SOON_MS,
  countdownText,
  minutesLeft,
  nextCheck,
  progress,
  scheduledShutdown,
  warningPhase,
} from './countdown'

const notice = (over: Partial<Notice>): Notice => ({
  id: 'power-schedule',
  code: 'rebootScheduled',
  level: 'warning',
  revision: 'r1',
  at: '2026-11-10T03:00:00Z',
  action: { kind: 'power', page: 'schedule' },
  ...over,
})

describe('scheduledShutdown', () => {
  it('reads the scheduled restart from the notices', () => {
    const other: Notice = { id: 'apt-updates', code: 'aptUpdates', level: 'info', revision: 'x' }
    expect(scheduledShutdown([other, notice({ items: ['admin'] })])).toEqual({
      at: Date.parse('2026-11-10T03:00:00Z'),
      reboot: true,
      by: 'admin',
      revision: 'r1',
    })
  })

  it('tells a shutdown scheduled outside the dashboard', () => {
    expect(scheduledShutdown([notice({ code: 'shutdownScheduled', revision: 'r2' })])).toEqual({
      at: Date.parse('2026-11-10T03:00:00Z'),
      reboot: false,
      by: undefined,
      revision: 'r2',
    })
  })

  it('is null without a schedule or a readable time', () => {
    expect(scheduledShutdown([])).toBeNull()
    expect(scheduledShutdown([notice({ at: undefined })])).toBeNull()
    expect(scheduledShutdown([notice({ at: 'soon' })])).toBeNull()
  })
})

describe('warningPhase', () => {
  it('steps through the warnings', () => {
    expect(warningPhase(SOON_MS + 1)).toBe('later')
    expect(warningPhase(SOON_MS)).toBe('soon')
    expect(warningPhase(IMMINENT_MS + 1)).toBe('soon')
    expect(warningPhase(IMMINENT_MS)).toBe('imminent')
    expect(warningPhase(1)).toBe('imminent')
    expect(warningPhase(0)).toBe('due')
    expect(warningPhase(-59_999)).toBe('due')
    // 오래 지난 알림(보관된 낡은 결과)으로는 전원 화면에 가지 않는다.
    expect(warningPhase(-60_000)).toBe('past')
  })
})

describe('nextCheck', () => {
  it('waits for the next phase, at most MAX_WAIT_MS', () => {
    expect(nextCheck(SOON_MS + 5000)).toBe(5000)
    expect(nextCheck(SOON_MS + 3_600_000)).toBe(MAX_WAIT_MS)
    expect(nextCheck(IMMINENT_MS + 2000)).toBe(2000)
  })

  it('ticks with the shown second in the last minute', () => {
    expect(nextCheck(59_400)).toBe(400)
    expect(nextCheck(59_000)).toBe(1000)
    expect(nextCheck(10)).toBe(50)
    expect(nextCheck(-500)).toBe(1000)
  })
})

describe('countdown text', () => {
  it('rounds the seconds up', () => {
    expect(countdownText(60_000)).toBe('1:00')
    expect(countdownText(59_001)).toBe('1:00')
    expect(countdownText(59_000)).toBe('0:59')
    expect(countdownText(1)).toBe('0:01')
    expect(countdownText(-5)).toBe('0:00')
  })

  it('counts the minutes and the bar', () => {
    expect(minutesLeft(SOON_MS)).toBe(10)
    expect(minutesLeft(9 * 60_000 + 1)).toBe(10)
    expect(minutesLeft(61_000)).toBe(2)
    expect(progress(IMMINENT_MS * 2)).toBe(1)
    expect(progress(IMMINENT_MS / 2)).toBe(0.5)
    expect(progress(-1)).toBe(0)
  })
})
