import { describe, expect, it } from 'vitest'
import type { Container } from '../../api/types.gen'
import { containerActions, containerTone, countContainers, isContainersResponse, sortByAttention } from './containers'
import { needsConfirm } from '../common/actionSpec'

const c = (name: string, state: Container['state'], health?: Container['health']): Container => ({
  name,
  image: 'example/x',
  state,
  health,
  status: 'raw',
})

describe('isContainersResponse', () => {
  it('정상 응답을 받는다', () => {
    expect(
      isContainersResponse({
        sources: ['omv-compose'],
        runtime: 'docker',
        runtimeStatus: 'running',
        containers: [c('a', 'running', 'healthy')],
      }),
    ).toBe(true)
  })
  it('모르는 상태는 거부한다', () => {
    expect(
      isContainersResponse({ sources: [], runtime: 'docker', runtimeStatus: '', containers: [{ ...c('a', 'running'), state: 'zombie' }] }),
    ).toBe(false)
  })
})

describe('containerTone', () => {
  it.each([
    [c('a', 'running'), 'ok'],
    [c('a', 'running', 'healthy'), 'ok'],
    [c('a', 'running', 'starting'), 'warn'],
    [c('a', 'running', 'unhealthy'), 'warn'],
    [c('a', 'restarting'), 'warn'],
    [c('a', 'dead'), 'crit'],
    [c('a', 'exited'), 'idle'],
    [c('a', 'paused'), 'idle'],
  ] as const)('%o → %s', (ctr, tone) => {
    expect(containerTone(ctr)).toBe(tone)
  })
})

it('sortByAttention은 문제 → 실행 → 정지 순서다', () => {
  const list = [c('b', 'exited'), c('a', 'running'), c('z', 'restarting'), c('c', 'running')]
  expect(sortByAttention(list).map((x) => x.name)).toEqual(['z', 'a', 'c', 'b'])
})

it('countContainers', () => {
  const list = [c('a', 'running'), c('b', 'running', 'unhealthy'), c('c', 'exited'), c('d', 'paused')]
  expect(countContainers(list)).toEqual({ running: 2, stopped: 2, attention: 1 })
})

describe('containerActions', () => {
  it.each([
    ['running', ['stop', 'restart']],
    ['paused', ['stop', 'restart']],
    ['restarting', ['stop', 'restart']],
    ['exited', ['start']],
    ['created', ['start']],
    ['dead', ['start']],
    ['removing', []],
    ['unknown', []],
  ] as const)('%s → %j', (state, actions) => {
    expect(containerActions(c('a', state))).toEqual(actions)
  })

  it('정지·재시작만 확인을 거친다(manifest)', () => {
    expect(needsConfirm('containers', 'start')).toBe(false)
    expect(needsConfirm('containers', 'stop')).toBe(true)
    expect(needsConfirm('containers', 'restart')).toBe(true)
    expect(needsConfirm('system-info', 'stop')).toBe(false)
  })
})
