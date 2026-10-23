import { describe, expect, it } from 'vitest'
import type { UpgradeInfo } from '../api/types.gen'
import type { UpgradeStatus } from '../api/upgrade.gen'
import { type WatchEvent, type WatchState, createUpgradeWatch, idleWatch, showsRestartScreen, startWatch, step } from './watch'

const TARGET = 'b'.repeat(40)

function info(status: Partial<UpgradeStatus> | null, interrupted = false): UpgradeInfo {
  return {
    enabled: true,
    pending: false,
    pendingStalled: false,
    interrupted,
    version: '0.3.1',
    status: status ? ({ format: 1, updatedAt: '', state: 'checked', ...status } as UpgradeStatus) : undefined,
    channel: '',
  }
}

const applying = (stage: string, target = TARGET) =>
  info({ state: 'applying', stage, apply: { target, user: 'totp', requestedAt: '', startedAt: '' } })

function run(events: WatchEvent[], start: WatchState = startWatch(TARGET, null, 0)) {
  let state = start
  const effects: string[] = []
  events.forEach((ev, i) => {
    const r = step(state, ev, (i + 1) * 1000)
    state = r.state
    effects.push(r.effect)
  })
  return { state, effects }
}

describe('step', () => {
  it('waits until the service picks up the request, then shows the restart screen at install', () => {
    const { state, effects } = run([
      { type: 'status', info: info({ state: 'checked' }) },
      { type: 'status', info: applying('build') },
      { type: 'status', info: applying('install') },
    ])
    expect(effects).toEqual(['none', 'none', 'none'])
    expect(state.phase).toBe('restarting')
    expect(state.since).toBe(3000)
    expect(showsRestartScreen(state.phase)).toBe(true)
  })

  it('reloads when the session is gone (maru restarted)', () => {
    const { effects } = run([{ type: 'status', info: applying('install') }, { type: 'unauthorized' }])
    expect(effects).toEqual(['none', 'reload'])
  })

  it('waits for health after the connection drops', () => {
    const { state, effects } = run([
      { type: 'status', info: applying('install') },
      { type: 'unreachable' },
      { type: 'unreachable' },
      { type: 'healthy' },
    ])
    expect(effects).toEqual(['none', 'none', 'none', 'reload'])
    // 끊긴 시각부터 잰다.
    expect(state.phase).toBe('down')
    expect(state.since).toBe(2000)
  })

  it('ends when the upgrade failed or finished without a restart', () => {
    const failed = run([
      { type: 'status', info: info({ state: 'apply-failed', stage: 'build', apply: { target: TARGET, user: '', requestedAt: '', startedAt: '' } }) },
    ])
    expect(failed.effects).toEqual(['done'])
    expect(failed.state.phase).toBe('idle')
    expect(failed.state.info?.status?.state).toBe('apply-failed')
    const applied = run([{ type: 'status', info: info({ state: 'applied', apply: { target: TARGET, user: '', requestedAt: '', startedAt: '' } }) }])
    expect(applied.effects).toEqual(['done'])
  })

  it('ends when the service was interrupted', () => {
    const { effects } = run([{ type: 'status', info: { ...applying('build'), interrupted: true } }])
    expect(effects).toEqual(['done'])
  })

  it('ignores an older upgrade result', () => {
    const old = info({ state: 'applied', apply: { target: 'a'.repeat(40), user: '', requestedAt: '', startedAt: '' } })
    const { state, effects } = run([{ type: 'status', info: old }])
    expect(effects).toEqual(['none'])
    expect(state.phase).toBe('watching')
  })

  it('does nothing while idle', () => {
    expect(step(idleWatch, { type: 'unauthorized' }, 1)).toEqual({ state: idleWatch, effect: 'none' })
  })
})

describe('createUpgradeWatch', () => {
  it('polls the status, then health, then reloads', async () => {
    const timers: (() => void)[] = []
    const responses: (() => Promise<UpgradeInfo>)[] = [
      async () => applying('install'),
      async () => {
        throw new Error('network')
      },
    ]
    let healthCalls = 0
    let reloaded = 0
    const store = createUpgradeWatch({
      fetchStatus: () => (responses.shift() ?? (async () => applying('install')))(),
      fetchHealth: async () => {
        healthCalls++
        if (healthCalls < 2) throw new Error('down')
      },
      classify: () => 'unreachable',
      reload: () => reloaded++,
      now: () => 0,
      schedule: (fn, ms) => {
        if (ms === 2000) timers.push(fn)
        return () => {}
      },
    })
    const flush = async () => {
      const fn = timers.shift()
      if (!fn) throw new Error('no timer')
      fn()
      await new Promise((r) => setTimeout(r, 0))
    }
    store.getState().start(TARGET, null)
    expect(store.getState().phase).toBe('watching')
    await flush()
    expect(store.getState().phase).toBe('restarting')
    await flush()
    expect(store.getState().phase).toBe('down')
    await flush()
    expect(healthCalls).toBe(1)
    await flush()
    expect(healthCalls).toBe(2)
    expect(reloaded).toBe(1)
    expect(timers).toHaveLength(0)
  })
})
