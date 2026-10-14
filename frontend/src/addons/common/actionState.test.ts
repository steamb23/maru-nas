import { describe, expect, it } from 'vitest'
import {
  INITIAL_ACTION_STATE,
  askConfirm,
  cancelConfirm,
  dismiss,
  finish,
  isBusy,
  settle,
  start,
} from './actionState'

describe('actionState', () => {
  it('runs an action through running → settling → done', () => {
    let s = start(INITIAL_ACTION_STATE, 'web', 'stop')
    expect(s.targets.web).toEqual({ kind: 'running', action: 'stop' })
    expect(isBusy(s, 'web')).toBe(true)
    expect(isBusy(s, 'db')).toBe(false)

    s = finish(s, 'web', null)
    expect(s.targets.web).toEqual({ kind: 'settling', action: 'stop' })
    expect(isBusy(s, 'web')).toBe(true)

    s = settle(s)
    expect(s.targets.web).toBeUndefined()
  })

  it('keeps errors until dismissed and clears them on the next start', () => {
    let s = finish(start(INITIAL_ACTION_STATE, 'web', 'start'), 'web', 'action_failed')
    expect(s.targets.web).toEqual({ kind: 'error', action: 'start', code: 'action_failed' })
    expect(isBusy(s, 'web')).toBe(false)
    expect(settle(s)).toBe(s)

    expect(dismiss(s, 'web').targets.web).toBeUndefined()
    s = start(s, 'web', 'start')
    expect(s.targets.web).toEqual({ kind: 'running', action: 'start' })
  })

  it('ignores a second start and stale results', () => {
    const s = start(INITIAL_ACTION_STATE, 'web', 'stop')
    expect(start(s, 'web', 'restart')).toBe(s)
    expect(finish(INITIAL_ACTION_STATE, 'web', null)).toBe(INITIAL_ACTION_STATE)
  })

  it('opens one confirmation at a time', () => {
    let s = askConfirm(INITIAL_ACTION_STATE, { target: 'web', action: 'stop', via: 'inline' })
    s = askConfirm(s, { target: 'db', action: 'restart', via: 'dialog' })
    expect(s.confirm).toEqual({ target: 'db', action: 'restart', via: 'dialog' })
    expect(cancelConfirm(s).confirm).toBeNull()

    // 확인한 대상을 시작하면 확인을 닫고, 다른 대상의 확인은 남긴다.
    expect(start(s, 'db', 'restart').confirm).toBeNull()
    expect(start(s, 'web', 'start').confirm).toEqual(s.confirm)
  })

  it('does not confirm a busy target', () => {
    const s = start(INITIAL_ACTION_STATE, 'web', 'stop')
    expect(askConfirm(s, { target: 'web', action: 'restart', via: 'inline' })).toBe(s)
  })
})
