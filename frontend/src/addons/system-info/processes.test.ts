import { describe, expect, it } from 'vitest'
import type { Process, SystemStatsResponse } from '../../api/types.gen'
import {
  DEFAULT_PROCESS_SORT,
  computeProcessCpu,
  filterProcesses,
  nextSort,
  processKey,
  scaleCpu,
  sortProcesses,
  stateKey,
} from './processes'

function proc(over: Partial<Process>): Process {
  return {
    pid: 1,
    startTicks: 1,
    name: 'demo',
    user: 'demo',
    state: 'S',
    cpuTicks: 0,
    rssBytes: 0,
    threads: 1,
    ...over,
  }
}

function stats(sampledAtMs: number, totalTicks: number, processes?: Process[]): SystemStatsResponse {
  return {
    sources: ['proc'],
    sampledAtMs,
    cpu: { totalTicks, idleTicks: 0, kernelTicks: 0, cores: [] },
    memory: { totalBytes: 1, usedBytes: 0, availableBytes: 1 },
    memoryDetail: {
      appsBytes: 0,
      sharedBytes: 0,
      compressedBytes: 0,
      buffersBytes: 0,
      cacheBytes: 0,
      freeBytes: 1,
      swapTotalBytes: 0,
      swapUsedBytes: 0,
    },
    uptimeSeconds: 1,
    processes,
  }
}

describe('computeProcessCpu', () => {
  it('프로세스 시간 / 모든 코어의 시간이다', () => {
    const prev = stats(0, 1000, [proc({ pid: 10, cpuTicks: 50 }), proc({ pid: 20, cpuTicks: 5 })])
    const cur = stats(2000, 1800, [
      proc({ pid: 10, cpuTicks: 250 }), // 200 / 800 = 25%
      proc({ pid: 20, cpuTicks: 5 }),
      proc({ pid: 30, cpuTicks: 99 }), // 새 프로세스
    ])
    const cpu = computeProcessCpu(prev, cur)
    expect(cpu.get('10:1')).toBe(25)
    expect(cpu.get('20:1')).toBe(0)
    expect(cpu.has('30:1')).toBe(false)
  })

  it('PID가 다시 쓰이면(시작 시각이 다름) 새 프로세스로 본다', () => {
    const prev = stats(0, 1000, [proc({ pid: 10, startTicks: 5, cpuTicks: 50 })])
    const cur = stats(2000, 1800, [proc({ pid: 10, startTicks: 9, cpuTicks: 60 })])
    expect(computeProcessCpu(prev, cur).size).toBe(0)
  })

  it('목록이 없거나 간격을 쓸 수 없으면 비어 있다', () => {
    const list = [proc({ pid: 10, cpuTicks: 50 })]
    expect(computeProcessCpu(null, stats(0, 1000, list)).size).toBe(0)
    expect(computeProcessCpu(stats(0, 1000), stats(2000, 1800, list)).size).toBe(0)
    expect(computeProcessCpu(stats(0, 1000, list), stats(40_000, 1800, list)).size).toBe(0)
    expect(computeProcessCpu(stats(0, 1000, list), stats(2000, 1000, list)).size).toBe(0)
  })
})

describe('scaleCpu', () => {
  it('코어 기준이면 코어 수를 곱한다', () => {
    expect(scaleCpu(25, 'machine', 4)).toBe(25)
    expect(scaleCpu(25, 'core', 4)).toBe(100)
    expect(scaleCpu(25, 'core', 0)).toBe(25)
  })
})

describe('filterProcesses', () => {
  const list = [
    proc({ pid: 1, name: 'systemd', user: 'root', command: '/sbin/init' }),
    proc({ pid: 2, name: 'kthreadd', user: 'root', kernel: true }),
    proc({ pid: 812, name: 'demo-daemon', user: 'Demo', command: '/opt/demo --config /etc/demo.conf' }),
  ]
  const pids = (l: Process[]) => l.map((p) => p.pid)

  it('커널 스레드는 켤 때만 보인다', () => {
    expect(pids(filterProcesses(list, '', false))).toEqual([1, 812])
    expect(pids(filterProcesses(list, '', true))).toEqual([1, 2, 812])
  })

  it('이름·명령줄·사용자·PID에서 대소문자 없이 찾는다', () => {
    expect(pids(filterProcesses(list, 'INIT', false))).toEqual([1])
    expect(pids(filterProcesses(list, 'demo.conf', false))).toEqual([812])
    expect(pids(filterProcesses(list, 'demo', false))).toEqual([812])
    expect(pids(filterProcesses(list, ' 81 ', false))).toEqual([812])
    expect(pids(filterProcesses(list, 'kthread', false))).toEqual([])
  })
})

describe('sortProcesses', () => {
  const compare = (a: string, b: string) => a.localeCompare(b)
  const list = [
    proc({ pid: 3, name: 'b', rssBytes: 10, startTicks: 3 }),
    proc({ pid: 1, name: 'a', rssBytes: 30, startTicks: 1 }),
    proc({ pid: 2, name: 'a', rssBytes: 20, startTicks: 2 }),
    proc({ pid: 4, name: 'c', rssBytes: 20, startTicks: 4 }),
  ]
  const cpu = new Map([
    ['3:3', 5],
    ['1:1', 1],
    ['2:2', 5],
  ])
  const pids = (l: Process[]) => l.map((p) => p.pid)

  it('기본은 CPU 내림차순이고, 값이 없는 행은 뒤, 같으면 PID 순이다', () => {
    expect(pids(sortProcesses(list, DEFAULT_PROCESS_SORT, cpu, compare))).toEqual([2, 3, 1, 4])
    expect(pids(sortProcesses(list, { key: 'cpu', dir: 'asc' }, cpu, compare))).toEqual([1, 2, 3, 4])
  })

  it('이름·메모리·PID로 정렬한다', () => {
    expect(pids(sortProcesses(list, { key: 'name', dir: 'asc' }, cpu, compare))).toEqual([1, 2, 3, 4])
    expect(pids(sortProcesses(list, { key: 'memory', dir: 'desc' }, cpu, compare))).toEqual([1, 2, 4, 3])
    expect(pids(sortProcesses(list, { key: 'pid', dir: 'desc' }, cpu, compare))).toEqual([4, 3, 2, 1])
  })

  it('원래 목록은 바꾸지 않는다', () => {
    const copy = [...list]
    sortProcesses(list, { key: 'pid', dir: 'asc' }, cpu, compare)
    expect(list).toEqual(copy)
  })
})

describe('nextSort', () => {
  it('같은 열은 방향을 바꾸고, 다른 열은 그 열의 기본 방향이다', () => {
    expect(nextSort(DEFAULT_PROCESS_SORT, 'cpu')).toEqual({ key: 'cpu', dir: 'asc' })
    expect(nextSort(DEFAULT_PROCESS_SORT, 'name')).toEqual({ key: 'name', dir: 'asc' })
    expect(nextSort({ key: 'name', dir: 'asc' }, 'memory')).toEqual({ key: 'memory', dir: 'desc' })
  })
})

describe('stateKey', () => {
  it('흔한 상태는 번역 키, 드문 글자는 null이다', () => {
    expect(stateKey('R')).toBe('sysinfo.states.running')
    expect(stateKey('D')).toBe('sysinfo.states.diskSleep')
    expect(stateKey('t')).toBeNull()
    expect(stateKey('X')).toBeNull()
  })
})

describe('processKey', () => {
  it('PID와 시작 시각이다', () => {
    expect(processKey(proc({ pid: 42, startTicks: 7 }))).toBe('42:7')
  })
})
