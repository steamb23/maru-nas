import { describe, expect, it } from 'vitest'
import {
  DOWN_STUCK_MS,
  type PowerWatchEvent,
  type PowerWatchState,
  SENT_STUCK_MS,
  createPowerWatch,
  idlePowerWatch,
  screenView,
  step,
} from './watch'

const sent = (action: PowerWatchState['action']): PowerWatchState => ({ phase: 'sent', action, modeKey: null, since: 0 })

function run(events: PowerWatchEvent[], start: PowerWatchState) {
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
  it('waits for the NAS to go down, then reloads when it answers again', () => {
    const { state, effects } = run(
      [{ type: 'healthy' }, { type: 'unreachable' }, { type: 'unreachable' }, { type: 'healthy' }],
      sent('reboot'),
    )
    expect(effects).toEqual(['none', 'none', 'none', 'reload'])
    expect(state.phase).toBe('down')
    expect(state.since).toBe(2000)
  })

  it('does nothing while idle', () => {
    expect(step(idlePowerWatch, { type: 'unreachable' }, 1)).toEqual({ state: idlePowerWatch, effect: 'none' })
  })
})

describe('screenView', () => {
  it('shows a spinner while restarting, and a hint when it does not come back', () => {
    expect(screenView(sent('reboot'), 1000)).toMatchObject({ titleKey: 'power.screen.rebootTitle', icon: 'spinner', back: false })
    const down: PowerWatchState = { ...sent('reboot'), phase: 'down' }
    expect(screenView(down, DOWN_STUCK_MS - 1).hintKey).toBeUndefined()
    expect(screenView(down, DOWN_STUCK_MS + 1)).toMatchObject({ hintKey: 'power.screen.rebootStuck', back: false })
  })

  it('switches shutdown and sleep to their final state once the NAS is down', () => {
    expect(screenView(sent('shutdown'), 0)).toMatchObject({ titleKey: 'power.screen.shuttingDownTitle', icon: 'spinner' })
    expect(screenView({ ...sent('shutdown'), phase: 'down' }, 0)).toMatchObject({
      titleKey: 'power.screen.shutdownTitle',
      icon: 'system-shutdown-symbolic',
    })
    expect(screenView(sent('standby'), 0)).toMatchObject({ titleKey: 'power.screen.enteringStandbyTitle', icon: 'spinner' })
    expect(screenView({ ...sent('standby'), phase: 'down' }, 0)).toMatchObject({
      titleKey: 'power.screen.standbyTitle',
      icon: 'system-suspend-symbolic',
    })
  })

  it('offers a way back when the NAS keeps answering', () => {
    for (const action of ['reboot', 'shutdown', 'standby'] as const) {
      expect(screenView(sent(action), SENT_STUCK_MS - 1).back).toBe(false)
      expect(screenView(sent(action), SENT_STUCK_MS + 1)).toMatchObject({ hintKey: 'power.screen.notDown', back: true })
    }
  })
})

describe('createPowerWatch', () => {
  it('polls health until the NAS goes down and comes back, then reloads', async () => {
    const timers: (() => void)[] = []
    const health = [true, false, false, true]
    let calls = 0
    let reloaded = 0
    const store = createPowerWatch({
      fetchHealth: async () => {
        calls++
        if (!health.shift()) throw new Error('down')
      },
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
    store.getState().start('reboot')
    expect(store.getState()).toMatchObject({ phase: 'sent', action: 'reboot', modeKey: null })
    // 이미 감시 중이면 다시 시작하지 않는다.
    store.getState().start('shutdown')
    expect(store.getState().action).toBe('reboot')
    await flush()
    expect(store.getState().phase).toBe('sent')
    await flush()
    expect(store.getState().phase).toBe('down')
    await flush()
    await flush()
    expect(calls).toBe(4)
    expect(reloaded).toBe(1)
    expect(timers).toHaveLength(0)
  })

  it('keeps the sleep mode name for the screen and stops cleanly', () => {
    const store = createPowerWatch({ fetchHealth: async () => {}, reload: () => {}, schedule: () => () => {} })
    store.getState().start('standby', 'power.mode.suspend')
    expect(store.getState().modeKey).toBe('power.mode.suspend')
    store.getState().stop()
    expect(store.getState().phase).toBe('idle')
  })
})
