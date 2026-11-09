import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import type { HelperStatus } from '../api/types.gen'
import { noticeTimeText } from '../notifications/messages'
import {
  DEFAULT_TIME,
  LAST_OFFSET,
  MAX_AHEAD_MS,
  MIN_AHEAD_MS,
  checkTarget,
  clockSkew,
  dayStart,
  initialForm,
  leftText,
  localTimeValue,
  nasWallTime,
  parseTimeValue,
  scheduleBlock,
  scheduleError,
  targetTime,
  utcOffsetMinutes,
} from './schedule'

// 시각은 테스트를 돌리는 시간대의 지역 시각으로 만든다(어느 시간대에서 돌려도 같은 결과).
const local = (y: number, mo: number, d: number, h: number, mi: number, s = 0) => new Date(y, mo - 1, d, h, mi, s).getTime()
const NOW = local(2026, 11, 9, 20, 48, 30)

describe('ranges', () => {
  it('matches the backend limits and lists today through the seventh day', () => {
    expect(MIN_AHEAD_MS).toBe(60_000)
    expect(MAX_AHEAD_MS).toBe(7 * 24 * 3600_000)
    expect(LAST_OFFSET).toBe(7)
  })
})

describe('clockSkew', () => {
  it('is the server clock minus the browser clock', () => {
    expect(clockSkew('2026-11-09T12:00:05Z', Date.parse('2026-11-09T12:00:00Z'))).toBe(5000)
    expect(clockSkew('2026-11-09T21:00:00+09:00', Date.parse('2026-11-09T12:00:30Z'))).toBe(-30_000)
  })

  it('is zero when the server time is missing', () => {
    expect(clockSkew('', NOW)).toBe(0)
  })
})

describe('parseTimeValue', () => {
  it('reads hours and minutes, ignoring seconds', () => {
    expect(parseTimeValue('03:00')).toEqual({ hour: 3, minute: 0 })
    expect(parseTimeValue('23:59:30')).toEqual({ hour: 23, minute: 59 })
    expect(parseTimeValue('23:59:30.5')).toEqual({ hour: 23, minute: 59 })
  })

  it('rejects anything else', () => {
    for (const v of ['', '3:00', '24:00', '12:60', '12:00 AM', 'ab:cd']) expect(parseTimeValue(v)).toBeNull()
  })
})

describe('targetTime', () => {
  it('is the chosen local time on the chosen day', () => {
    expect(targetTime(NOW, 0, '22:15')).toBe(local(2026, 11, 9, 22, 15))
    expect(targetTime(NOW, 1, '03:00')).toBe(local(2026, 11, 10, 3, 0))
    // 달이 바뀐다.
    expect(targetTime(NOW, 7, '00:00')).toBe(local(2026, 11, 16, 0, 0))
    expect(targetTime(local(2026, 11, 30, 10, 0), 2, '08:00')).toBe(local(2026, 12, 2, 8, 0))
  })

  it('is null without a time', () => {
    expect(targetTime(NOW, 0, '')).toBeNull()
  })

  it('goes with dayStart and localTimeValue', () => {
    expect(dayStart(NOW, 1)).toBe(local(2026, 11, 10, 0, 0))
    expect(localTimeValue(local(2026, 11, 10, 3, 5, 59))).toBe('03:05')
  })
})

describe('checkTarget', () => {
  it('accepts a time between one minute and seven days ahead', () => {
    expect(checkTarget(NOW + MIN_AHEAD_MS, NOW)).toBe('ok')
    expect(checkTarget(NOW + MAX_AHEAD_MS, NOW)).toBe('ok')
  })

  it('names what is wrong', () => {
    expect(checkTarget(null, NOW)).toBe('empty')
    expect(checkTarget(NOW, NOW)).toBe('past')
    expect(checkTarget(NOW - 60_000, NOW)).toBe('past')
    expect(checkTarget(NOW + MIN_AHEAD_MS - 1, NOW)).toBe('tooSoon')
    expect(checkTarget(NOW + MAX_AHEAD_MS + 1, NOW)).toBe('tooFar')
  })
})

describe('initialForm', () => {
  it('suggests the next 03:00', () => {
    expect(initialForm(NOW)).toEqual({ offset: 1, time: DEFAULT_TIME })
    expect(initialForm(local(2026, 11, 9, 1, 30))).toEqual({ offset: 0, time: DEFAULT_TIME })
    // 03:00까지 1분이 남지 않았으면 내일이다.
    expect(initialForm(local(2026, 11, 9, 2, 59, 30))).toEqual({ offset: 1, time: DEFAULT_TIME })
  })

  it('starts from the scheduled restart', () => {
    const at = new Date(local(2026, 11, 11, 4, 30)).toISOString()
    expect(initialForm(NOW, { at, mode: 'reboot' })).toEqual({ offset: 2, time: '04:30' })
  })

  it('ignores a shutdown, an unreadable time and a day outside the list', () => {
    const at = new Date(local(2026, 11, 11, 4, 30)).toISOString()
    expect(initialForm(NOW, { at, mode: 'poweroff' })).toEqual({ offset: 1, time: DEFAULT_TIME })
    expect(initialForm(NOW, { at: 'soon', mode: 'reboot' })).toEqual({ offset: 1, time: DEFAULT_TIME })
    const far = new Date(local(2026, 11, 20, 3, 0)).toISOString()
    expect(initialForm(NOW, { at: far, mode: 'reboot' })).toEqual({ offset: 1, time: DEFAULT_TIME })
  })
})

describe('leftText', () => {
  it('rounds up to the minute', () => {
    expect(leftText((6 * 60 + 11) * 60_000 + 30_000, 'en')).toBe('6 hours 12 minutes')
    expect(leftText(60_000, 'en')).toBe('1 minute')
    expect(leftText(-5000, 'en')).toBe('0 minutes')
  })
})

describe('utcOffsetMinutes', () => {
  it('reads the offset at the end of an RFC 3339 time', () => {
    expect(utcOffsetMinutes('2026-11-09T20:48:30+09:00')).toBe(540)
    expect(utcOffsetMinutes('2026-11-09T07:18:30-05:30')).toBe(-330)
    expect(utcOffsetMinutes('2026-11-09T11:48:30Z')).toBe(0)
    expect(utcOffsetMinutes('')).toBeNull()
  })
})

describe('nasWallTime', () => {
  const at = local(2026, 11, 10, 3, 0)
  const here = Intl.DateTimeFormat().resolvedOptions().timeZone
  const other = here === 'Asia/Seoul' ? 'America/New_York' : 'Asia/Seoul'
  // 이 시간대에서 at의 UTC 오프셋(서버 now의 오프셋으로 쓴다)
  const offsetText = (minutes: number) => {
    const sign = minutes < 0 ? '-' : '+'
    const m = Math.abs(minutes)
    return `${sign}${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`
  }
  const hereOffset = -new Date(at).getTimezoneOffset()

  it('is null when the NAS shows the same wall time', () => {
    expect(nasWallTime(at, 'en', here, '')).toBeNull()
    expect(nasWallTime(at, 'en', undefined, `2026-11-09T20:48:30${offsetText(hereOffset)}`)).toBeNull()
  })

  it('shows the NAS wall time in another zone', () => {
    expect(nasWallTime(at, 'en', other, '')).toBe(noticeTimeText(at, 'en', other))
  })

  it('falls back to the server offset for a missing or unknown zone', () => {
    const shifted = hereOffset + 120
    const want = noticeTimeText(at + shifted * 60_000, 'en', 'UTC')
    expect(nasWallTime(at, 'en', undefined, `2026-11-09T20:48:30${offsetText(shifted)}`)).toBe(want)
    expect(nasWallTime(at, 'en', 'Nowhere/Unknown', `2026-11-09T20:48:30${offsetText(shifted)}`)).toBe(want)
  })

  it('is null when nothing tells the NAS zone', () => {
    expect(nasWallTime(at, 'en', undefined, '')).toBeNull()
  })
})

describe('scheduleBlock', () => {
  const status = (state: HelperStatus['state'], canElevate = true): HelperStatus => ({ state, canElevate, elevatedSeconds: 0 })

  it('lets a session that can elevate schedule', () => {
    expect(scheduleBlock(status('active'))).toBeNull()
    // 아직 모르면 막지 않는다.
    expect(scheduleBlock(null)).toBeNull()
  })

  it('says why the session cannot schedule', () => {
    expect(scheduleBlock(status('active', false))).toBe('power.schedule.blocked.elevate')
    expect(scheduleBlock(status('disabled'))).toBe('power.schedule.blocked.helper')
    expect(scheduleBlock(status('unavailable'))).toBe('power.schedule.blocked.helper')
    expect(scheduleBlock(status('none'))).toBe('power.schedule.blocked.relogin')
    expect(scheduleBlock(status('reauth'))).toBe('power.schedule.blocked.relogin')
  })
})

describe('scheduleError', () => {
  it('maps backend codes', () => {
    const fallback = 'power.schedule.error.failed'
    expect(scheduleError(new ApiError(400, 'x', 'power_time_invalid'), fallback)).toBe('power.schedule.error.time')
    expect(scheduleError(new ApiError(403, 'x', 'elevation_not_allowed'), fallback)).toBe('power.schedule.blocked.elevate')
    expect(scheduleError(new ApiError(401, 'x', 'helper_reauth'), fallback)).toBe('power.schedule.blocked.relogin')
    expect(scheduleError(new ApiError(503, 'x', 'helper_unavailable'), fallback)).toBe('power.schedule.blocked.helper')
    expect(scheduleError(new ApiError(403, 'x', 'admin_required'), fallback)).toBe('actions.error.admin_required')
  })

  it('falls back for anything else', () => {
    expect(scheduleError(new ApiError(502, 'x', 'omv_error'), 'power.schedule.error.cancelFailed')).toBe(
      'power.schedule.error.cancelFailed',
    )
    expect(scheduleError(new ApiError(502, 'Bad Gateway'), 'power.schedule.error.failed')).toBe('power.schedule.error.failed')
    expect(scheduleError(new TypeError('fetch'), 'power.schedule.error.failed')).toBe('power.schedule.error.failed')
  })
})
