import { describe, expect, it } from 'vitest'
import type { Service } from '../../api/types.gen'
import { needsConfirm } from '../common/actionSpec'
import {
  countServices,
  holdServices,
  isServicesResponse,
  serviceActions,
  serviceReached,
  serviceState,
  serviceTone,
  sortByAttention,
} from './services'

const s = (name: string, enabled: boolean, running: boolean, title = name.toUpperCase()): Service => ({
  name,
  title,
  enabled,
  running,
  toggleable: false,
})

describe('isServicesResponse', () => {
  it('정상 응답을 받는다', () => {
    expect(isServicesResponse({ sources: ['omv-services'], services: [s('ssh', true, true)] })).toBe(true)
  })
  it('형식이 다르면 거부한다', () => {
    expect(isServicesResponse({ sources: [], services: [{ ...s('ssh', true, true), enabled: 1 }] })).toBe(false)
    expect(isServicesResponse({ sources: [] })).toBe(false)
  })
})

describe('serviceTone, serviceState', () => {
  it.each([
    [true, true, 'ok', 'running'],
    [true, false, 'crit', 'stopped'],
    [false, true, 'warn', 'disabledRunning'],
    [false, false, 'idle', 'disabled'],
  ] as const)('enabled=%s running=%s → %s/%s', (enabled, running, tone, state) => {
    expect(serviceTone(s('x', enabled, running))).toBe(tone)
    expect(serviceState(s('x', enabled, running))).toBe(state)
  })
})

describe('sortByAttention, countServices', () => {
  it('문제 → 켜짐 → 꺼짐 순서이고 같은 순위는 제목 순서다', () => {
    const list = [s('nfs', false, false), s('ssh', true, true), s('fail2ban', true, false), s('smb', true, true)]
    expect(sortByAttention(list).map((x) => x.name)).toEqual(['fail2ban', 'smb', 'ssh', 'nfs'])
    expect(countServices(list)).toEqual({ running: 2, attention: 1 })
  })
})

describe('serviceActions', () => {
  it('켜고 끌 수 있는 서비스만 조작이 있다', () => {
    expect(serviceActions(s('fail2ban', true, false))).toEqual([])
    const on = { ...s('ssh', true, true), toggleable: true }
    expect(serviceActions(on).map((a) => [a.action, a.confirmKey])).toEqual([['disable', 'services.action.confirmDisableSsh']])
    // 켜져 있는데 멈춘 서비스도 끄기다.
    expect(serviceActions({ ...on, running: false }).map((a) => a.action)).toEqual(['disable'])
    expect(serviceActions({ ...s('nfs', false, false), toggleable: true }).map((a) => a.action)).toEqual(['enable'])
  })

  it('끄기만 확인을 거친다(manifest)', () => {
    expect(needsConfirm('services', 'enable')).toBe(false)
    expect(needsConfirm('services', 'disable')).toBe(true)
  })
})

describe('serviceReached', () => {
  it('설정과 실제 실행이 모두 바뀌어야 끝난 것으로 본다', () => {
    expect(serviceReached(s('ssh', true, false), 'enable')).toBe(false)
    expect(serviceReached(s('ssh', true, true), 'enable')).toBe(true)
    expect(serviceReached(s('ssh', false, true), 'disable')).toBe(false)
    expect(serviceReached(s('ssh', false, false), 'disable')).toBe(true)
  })
})

describe('holdServices', () => {
  it('조작 중인 서비스는 처음 본 상태로 붙잡고, 끝나면 놓는다', () => {
    const before = [s('ssh', false, false), s('nfs', true, true)]
    let busy = new Set(['ssh'])
    const held = holdServices({}, before, (n) => busy.has(n))
    expect(held).toEqual({ ssh: before[0] })

    // 설정만 바뀐 응답이 와도 조작 전 상태를 그대로 둔다. 바뀐 것이 없으면 같은 객체다.
    const middle = [s('ssh', true, false), s('nfs', true, true)]
    expect(holdServices(held, middle, (n) => busy.has(n))).toBe(held)

    busy = new Set()
    expect(holdServices(held, middle, (n) => busy.has(n))).toEqual({})
    expect(holdServices({}, middle, () => false)).toEqual({})
  })
})
