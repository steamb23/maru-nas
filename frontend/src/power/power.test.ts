import { describe, expect, it } from 'vitest'
import { ApiError } from '../api/client'
import type { SystemStatsResponse } from '../api/types.gen'
import { POWER_ACTIONS, POWER_SOURCE, powerInfoOf, sendOutcome, sleepState } from './power'

describe('POWER_ACTIONS', () => {
  it('lists shutdown, restart and sleep in tile order, with only shutdown in the danger color', () => {
    expect(POWER_ACTIONS.map((a) => a.id)).toEqual(['shutdown', 'reboot', 'standby'])
    expect(POWER_ACTIONS.filter((a) => a.danger).map((a) => a.id)).toEqual(['shutdown'])
  })
})

describe('sleepState', () => {
  it('waits for the standby mode', () => {
    expect(sleepState(undefined, false)).toEqual({ kind: 'loading' })
  })

  it('allows sleep when the configured mode is supported, with its name', () => {
    expect(sleepState({ standbyMode: 'suspend', standbyModes: ['poweroff', 'suspend'] }, false)).toEqual({
      kind: 'available',
      modeKey: 'power.mode.suspend',
    })
    expect(sleepState({ standbyMode: 'hibernate', standbyModes: ['poweroff', 'hibernate'] }, false)).toEqual({
      kind: 'available',
      modeKey: 'power.mode.hibernate',
    })
  })

  it('blocks sleep when OMV would power off instead', () => {
    // 대기 방식이 전원 끄기
    expect(sleepState({ standbyMode: 'poweroff', standbyModes: ['poweroff', 'suspend'] }, false)).toEqual({
      kind: 'unavailable',
      reasonKey: 'power.sleepPoweroff',
    })
    // 지원하지 않는 방식(OMV는 suspendhybrid를 지원한다고 알리지 않는다)
    for (const mode of ['suspend', 'suspendhybrid', '']) {
      expect(sleepState({ standbyMode: mode, standbyModes: ['poweroff'] }, false)).toEqual({
        kind: 'unavailable',
        reasonKey: 'power.sleepUnsupported',
      })
    }
  })

  it('blocks sleep when the mode could not be read', () => {
    expect(sleepState(undefined, true)).toEqual({ kind: 'unavailable', reasonKey: 'power.sleepUnknown' })
  })
})

describe('powerInfoOf', () => {
  const base = { sources: ['proc'] } as unknown as SystemStatsResponse
  const info = { standbyMode: 'suspend', standbyModes: ['poweroff', 'suspend'] }

  it('takes the power info only when its source answered', () => {
    expect(powerInfoOf({ ...base, sources: ['proc', POWER_SOURCE], power: info })).toEqual({ info, failed: false })
    expect(powerInfoOf({ ...base, failedSources: [POWER_SOURCE] })).toEqual({ failed: true })
    expect(powerInfoOf(base)).toEqual({ failed: true })
  })
})

describe('sendOutcome', () => {
  it('treats a dropped connection or a proxy error page as sent', () => {
    expect(sendOutcome(new TypeError('Failed to fetch'))).toBe('sent')
    expect(sendOutcome(new ApiError(502, 'Bad Gateway'))).toBe('sent')
  })

  it('shows the reason when the backend refused', () => {
    expect(sendOutcome(new ApiError(502, 'rpc', 'omv_error'))).toBe('power.error.omv')
    expect(sendOutcome(new ApiError(409, 'busy', 'action_busy'))).toBe('power.error.busy')
    expect(sendOutcome(new ApiError(403, 'admin', 'admin_required'))).toBe('actions.error.admin_required')
    expect(sendOutcome(new ApiError(400, 'bad', 'bad_request'))).toBe('actions.error.unknown')
    expect(sendOutcome(new ApiError(404, 'not found'))).toBe('actions.error.unknown')
    // 백엔드 자신의 500(코드 없음)은 보낸 것으로 보지 않는다.
    expect(sendOutcome(new ApiError(500, 'action failed'))).toBe('actions.error.unknown')
  })
})
