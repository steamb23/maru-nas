import { describe, expect, it } from 'vitest'
import {
  INITIAL_ACTION_STATE,
  SETTLE_TIMEOUT_MS,
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

    s = finish(s, 'web', null, 1000)
    expect(s.targets.web).toEqual({ kind: 'settling', action: 'stop', since: 1000 })
    expect(isBusy(s, 'web')).toBe(true)

    s = settle(s, 1000)
    expect(s.targets.web).toBeUndefined()
  })

  it('settles only targets that reached the result or timed out', () => {
    let s = finish(start(INITIAL_ACTION_STATE, 'ssh', 'enable'), 'ssh', null, 1000)
    s = finish(start(s, 'nfs', 'disable'), 'nfs', null, 1000)
    const reached = new Set(['nfs'])
    const done = (target: string) => reached.has(target)

    // nfs만 결과 상태에 이르렀다.
    s = settle(s, 2000, done)
    expect(Object.keys(s.targets)).toEqual(['ssh'])
    expect(isBusy(s, 'ssh')).toBe(true)

    // 아직 이르지 않았으면 그대로 둔다.
    expect(settle(s, 1000 + SETTLE_TIMEOUT_MS - 1, done)).toBe(s)
    // 시간이 지나면 이르지 않아도 푼다.
    expect(settle(s, 1000 + SETTLE_TIMEOUT_MS, done).targets.ssh).toBeUndefined()
  })

  it('keeps errors until dismissed and clears them on the next start', () => {
    let s = finish(start(INITIAL_ACTION_STATE, 'web', 'start'), 'web', 'action_failed', 0)
    expect(s.targets.web).toEqual({ kind: 'error', action: 'start', code: 'action_failed' })
    expect(isBusy(s, 'web')).toBe(false)
    expect(settle(s, 0)).toBe(s)

    expect(dismiss(s, 'web').targets.web).toBeUndefined()
    s = start(s, 'web', 'start')
    expect(s.targets.web).toEqual({ kind: 'running', action: 'start' })
  })

  it('ignores a second start and stale results', () => {
    const s = start(INITIAL_ACTION_STATE, 'web', 'stop')
    expect(start(s, 'web', 'restart')).toBe(s)
    expect(finish(INITIAL_ACTION_STATE, 'web', null, 0)).toBe(INITIAL_ACTION_STATE)
  })

  it('opens one confirmation at a time', () => {
    let s = askConfirm(INITIAL_ACTION_STATE, { target: 'web', action: 'stop' })
    s = askConfirm(s, { target: 'db', action: 'restart' })
    expect(s.confirm).toEqual({ target: 'db', action: 'restart' })
    expect(cancelConfirm(s).confirm).toBeNull()

    // 확인한 대상을 시작하면 확인을 닫고, 다른 대상의 확인은 남긴다.
    expect(start(s, 'db', 'restart').confirm).toBeNull()
    expect(start(s, 'web', 'start').confirm).toEqual(s.confirm)
  })

  it('does not confirm a busy target', () => {
    const s = start(INITIAL_ACTION_STATE, 'web', 'stop')
    expect(askConfirm(s, { target: 'web', action: 'restart' })).toBe(s)
  })
})
