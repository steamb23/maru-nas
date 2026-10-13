import { describe, expect, it } from 'vitest'
import { ACTIVITY_INTERVAL_MS, CONFIRM_GRACE_MS, type KeeperState, RENEW_MARGIN_MS, decide } from './sessionKeeper'

const MIN = 60_000
const state = (s: Partial<KeeperState>): KeeperState => ({
  expiresAt: null,
  lastInputAt: -Infinity,
  lastSentAt: 0,
  ...s,
})

describe('decide', () => {
  it('입력이 없으면 만료 시각 뒤에 확인한다', () => {
    const s = state({ expiresAt: 5 * MIN })
    expect(decide(s, 0)).toEqual({ kind: 'wait', at: 5 * MIN + CONFIRM_GRACE_MS })
    expect(decide(s, 5 * MIN + CONFIRM_GRACE_MS)).toEqual({ kind: 'confirm' })
  })

  it('묶음 간격이 지난 입력은 바로 보낸다', () => {
    const s = state({ expiresAt: 5 * MIN, lastInputAt: ACTIVITY_INTERVAL_MS })
    expect(decide(s, ACTIVITY_INTERVAL_MS)).toEqual({ kind: 'renew' })
  })

  it('묶음 간격 안의 입력은 간격이 끝날 때 보낸다', () => {
    const s = state({ expiresAt: 5 * MIN, lastInputAt: 10_000 })
    expect(decide(s, 10_000)).toEqual({ kind: 'wait', at: ACTIVITY_INTERVAL_MS })
    expect(decide(s, ACTIVITY_INTERVAL_MS)).toEqual({ kind: 'renew' })
  })

  it('timeout이 1분이어도 만료 전에 밀린 입력을 보낸다', () => {
    // 0초에 보냈고 50초에 입력이 있었다. 묶음 간격(60초)을 기다리면 이미 만료다.
    const s = state({ expiresAt: MIN, lastInputAt: 50_000 })
    expect(decide(s, 45_000)).toEqual({ kind: 'wait', at: MIN - RENEW_MARGIN_MS })
    expect(decide(s, 50_000)).toEqual({ kind: 'renew' })
  })

  it('만료 직전 여유 구간에 들어온 입력도 바로 보낸다', () => {
    const s = state({ expiresAt: MIN, lastInputAt: 55_000 })
    expect(decide(s, 55_000)).toEqual({ kind: 'renew' })
  })

  it('보낸 뒤의 입력이 없으면 다시 보내지 않는다', () => {
    const s = state({ expiresAt: 5 * MIN, lastInputAt: 1000, lastSentAt: 2000 })
    expect(decide(s, 3 * MIN).kind).toBe('wait')
  })

  it('자동 로그아웃이 꺼져 있으면 입력 묶음만 처리한다', () => {
    expect(decide(state({}), 10 * MIN)).toEqual({ kind: 'idle' })
    expect(decide(state({ lastInputAt: 1000 }), 1000)).toEqual({ kind: 'wait', at: ACTIVITY_INTERVAL_MS })
    expect(decide(state({ lastInputAt: 1000 }), ACTIVITY_INTERVAL_MS)).toEqual({ kind: 'renew' })
  })
})
