import { describe, expect, it } from 'vitest'
import type { NetInterface, NetworkResponse } from '../../api/types.gen'
import { computeRates, isNetworkResponse, MAX_SAMPLE_GAP_MS, widgetInterfaces } from './network'

const iface = (over: Partial<NetInterface>): NetInterface => ({
  name: 'eth0',
  state: 'up',
  rxBytes: 0,
  txBytes: 0,
  rxPackets: 0,
  txPackets: 0,
  rxErrors: 0,
  txErrors: 0,
  rxDropped: 0,
  txDropped: 0,
  ...over,
})

const sample = (ms: number, ...interfaces: NetInterface[]): NetworkResponse => ({
  sources: ['proc'],
  sampledAtMs: ms,
  interfaces,
})

describe('isNetworkResponse', () => {
  it('정상 응답을 받는다', () => {
    expect(isNetworkResponse(sample(1, iface({ speedMbps: 1000, virtual: true })))).toBe(true)
  })
  it('카운터가 숫자가 아니면 거부한다', () => {
    expect(isNetworkResponse(sample(1, { ...iface({}), rxBytes: '1' } as unknown as NetInterface))).toBe(false)
    expect(isNetworkResponse({ sources: [], interfaces: [] })).toBe(false)
  })
})

describe('computeRates', () => {
  it('두 샘플의 차이를 초당 바이트로 바꾼다', () => {
    const r = computeRates(
      sample(1000, iface({ rxBytes: 1000, txBytes: 0 })),
      sample(3000, iface({ rxBytes: 5000, txBytes: 2000 })),
    )
    expect(r.get('eth0')).toEqual({ rx: 2000, tx: 1000 })
  })
  it('직전 샘플이 없거나 간격이 이상하면 비운다', () => {
    const cur = sample(1000, iface({}))
    expect(computeRates(null, cur).size).toBe(0)
    expect(computeRates(sample(1000, iface({})), cur).size).toBe(0)
    expect(computeRates(sample(2000, iface({})), cur).size).toBe(0)
    expect(computeRates(sample(0, iface({})), sample(MAX_SAMPLE_GAP_MS + 1, iface({}))).size).toBe(0)
  })
  it('카운터가 줄었거나 새 인터페이스면 그 인터페이스만 뺀다', () => {
    const r = computeRates(
      sample(0, iface({ rxBytes: 100 }), iface({ name: 'eth1', rxBytes: 10 })),
      sample(1000, iface({ rxBytes: 50 }), iface({ name: 'eth1', rxBytes: 20 }), iface({ name: 'eth2' })),
    )
    expect([...r.keys()]).toEqual(['eth1'])
  })
})

it('widgetInterfaces는 물리 인터페이스만, 올라온 것을 먼저 둔다', () => {
  const list = [iface({ name: 'a', state: 'down' }), iface({ name: 'b' }), iface({ name: 'c', virtual: true })]
  expect(widgetInterfaces(list).map((i) => i.name)).toEqual(['b', 'a'])
})
