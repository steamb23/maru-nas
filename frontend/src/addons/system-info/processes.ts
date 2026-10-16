// 프로세스 표의 계산(CPU 사용률, 거르기, 정렬, 상태 이름). DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import type { Process, SystemStatsResponse } from '../../api/types.gen'
import { sampleGap } from './stats'

// processKey는 프로세스를 구분하는 값이다. PID는 다시 쓰일 수 있으므로 시작 시각을 함께 쓴다.
export function processKey(p: Process): string {
  return `${p.pid}:${p.startTicks}`
}

// computeProcessCpu는 두 응답 사이 프로세스별 CPU 사용률(전체 대비 0~100)이다. 키는 processKey다.
// 직전 응답이 없거나 간격을 쓸 수 없거나, 그 사이에 시작한 프로세스는 값이 없다.
export function computeProcessCpu(prev: SystemStatsResponse | null, cur: SystemStatsResponse): Map<string, number> {
  const out = new Map<string, number>()
  if (!prev?.processes || !cur.processes || sampleGap(prev, cur) === null) return out
  // 두 단위가 같으므로(clock tick) 프로세스 시간 / 모든 코어의 시간이 전체 대비 비율이다.
  const total = cur.cpu.totalTicks - prev.cpu.totalTicks
  if (total <= 0) return out
  const before = new Map(prev.processes.map((p) => [processKey(p), p.cpuTicks]))
  for (const p of cur.processes) {
    const b = before.get(processKey(p))
    if (b === undefined || p.cpuTicks < b) continue
    out.set(processKey(p), Math.min(100, ((p.cpuTicks - b) / total) * 100))
  }
  return out
}

export type CpuScale = 'machine' | 'core'

// scaleCpu는 전체 대비 사용률을 보여 줄 기준으로 바꾼다. 코어 기준이면 코어 하나가 100%다(top 방식).
export function scaleCpu(percent: number, scale: CpuScale, cores: number): number {
  return scale === 'core' ? percent * Math.max(1, cores) : percent
}

// filterProcesses는 검색어(이름, 명령줄, 사용자, PID에서 대소문자 무시)와 커널 스레드 표시로 거른다.
export function filterProcesses(list: readonly Process[], query: string, showKernel: boolean): Process[] {
  const q = query.trim().toLocaleLowerCase()
  return list.filter((p) => {
    if (p.kernel && !showKernel) return false
    if (q === '') return true
    return (
      p.name.toLocaleLowerCase().includes(q) ||
      (p.command ?? '').toLocaleLowerCase().includes(q) ||
      p.user.toLocaleLowerCase().includes(q) ||
      String(p.pid).includes(q)
    )
  })
}

export type ProcessSortKey = 'name' | 'pid' | 'user' | 'state' | 'cpu' | 'memory'
export type SortDir = 'asc' | 'desc'

export interface ProcessSort {
  key: ProcessSortKey
  dir: SortDir
}

export const DEFAULT_PROCESS_SORT: ProcessSort = { key: 'cpu', dir: 'desc' }

// nextSort는 열 머리글을 눌렀을 때의 정렬이다. 같은 열이면 방향을 바꾸고, 다른 열이면 그 열의 기본 방향이다
// (수치는 큰 것부터, 글자는 가나다순).
export function nextSort(cur: ProcessSort, key: ProcessSortKey): ProcessSort {
  if (cur.key === key) return { key, dir: cur.dir === 'asc' ? 'desc' : 'asc' }
  return { key, dir: key === 'cpu' || key === 'memory' ? 'desc' : 'asc' }
}

// sortProcesses는 sort로 정렬한 새 목록이다. CPU 값이 아직 없는 행은 방향과 무관하게 뒤에 두고,
// 같으면 PID 오름차순이다. compare는 글자 비교(화면 언어의 Intl.Collator)다.
export function sortProcesses(
  list: readonly Process[],
  sort: ProcessSort,
  cpu: ReadonlyMap<string, number>,
  compare: (a: string, b: string) => number,
): Process[] {
  const sign = sort.dir === 'asc' ? 1 : -1
  const byKey = (a: Process, b: Process): number => {
    switch (sort.key) {
      case 'name':
        return compare(a.name, b.name)
      case 'pid':
        return a.pid - b.pid
      case 'user':
        return compare(a.user, b.user)
      case 'state':
        return compare(a.state, b.state)
      case 'memory':
        return a.rssBytes - b.rssBytes
      case 'cpu':
        return (cpu.get(processKey(a)) ?? 0) - (cpu.get(processKey(b)) ?? 0)
    }
  }
  return [...list].sort((a, b) => {
    if (sort.key === 'cpu') {
      const ha = cpu.has(processKey(a))
      const hb = cpu.has(processKey(b))
      if (ha !== hb) return ha ? -1 : 1
    }
    return sign * byKey(a, b) || a.pid - b.pid
  })
}

const STATE_KEYS: Record<string, ParseKeys> = {
  R: 'sysinfo.states.running',
  S: 'sysinfo.states.sleeping',
  D: 'sysinfo.states.diskSleep',
  Z: 'sysinfo.states.zombie',
  T: 'sysinfo.states.stopped',
  I: 'sysinfo.states.idle',
}

// stateKey는 커널 상태 글자의 번역 키다. 드문 글자(t, X, P, W 등)는 null이며 글자를 그대로 보여 준다.
export function stateKey(state: string): ParseKeys | null {
  return STATE_KEYS[state] ?? null
}
