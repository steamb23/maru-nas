// 시스템 정보 창 응답 검사와 사용량 기록 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 백엔드는 누적 카운터와 단조 시각(sampledAtMs)만 준다. 사용률·속도는 보기마다 직전 응답과의 차이로 구하므로
// 창과 다른 세션이 서로의 측정 구간에 영향을 주지 않는다(네트워크 애드온의 computeRates와 같은 방식).
import { hasAddonSources } from '../../api/client'
import { isArrayOf, isBool, isNum, isObj, isOptional, isStr } from '../../api/guards'
import type {
  CPUCounters,
  CoreTicks,
  DiskTotals,
  LoadAverage,
  MemoryDetail,
  NetTotals,
  PowerInfo,
  Process,
  SystemStatsResponse,
  TaskCounts,
} from '../../api/types.gen'
import { percentOf } from '../common/format'

// 그래프가 보여 주는 시간(최근 60초)
export const HISTORY_MS = 60_000
// 두 샘플 간격이 이보다 길면(탭을 오래 숨겼다 돌아옴 등) 긴 구간의 평균 대신 기록을 새로 시작한다.
export const MAX_SAMPLE_GAP_MS = 30_000

const isCore = (v: unknown): v is CoreTicks =>
  isObj(v) && isNum(v.id) && isNum(v.totalTicks) && isNum(v.idleTicks) && isNum(v.kernelTicks)

const isCPUCounters = (v: unknown): v is CPUCounters =>
  isObj(v) && isNum(v.totalTicks) && isNum(v.idleTicks) && isNum(v.kernelTicks) && isArrayOf(v.cores, isCore)

const MEMORY_DETAIL_KEYS = [
  'appsBytes',
  'sharedBytes',
  'compressedBytes',
  'buffersBytes',
  'cacheBytes',
  'freeBytes',
  'swapTotalBytes',
  'swapUsedBytes',
] as const satisfies readonly (keyof MemoryDetail)[]

const isMemoryDetail = (v: unknown): v is MemoryDetail => isObj(v) && MEMORY_DETAIL_KEYS.every((k) => isNum(v[k]))

const isLoadAverage = (v: unknown): v is LoadAverage =>
  isObj(v) && isNum(v.one) && isNum(v.five) && isNum(v.fifteen) && isNum(v.running)

const isNetTotals = (v: unknown): v is NetTotals => isObj(v) && isNum(v.rxBytes) && isNum(v.txBytes)

const isDiskTotals = (v: unknown): v is DiskTotals => isObj(v) && isNum(v.readBytes) && isNum(v.writeBytes)

const isProcessList = (v: unknown): v is Process[] => isArrayOf(v, isProcess)

const isTaskCounts = (v: unknown): v is TaskCounts => isObj(v) && isNum(v.processes) && isNum(v.threads)

function isProcess(v: unknown): v is Process {
  return (
    isObj(v) &&
    isNum(v.pid) &&
    isNum(v.startTicks) &&
    isStr(v.name) &&
    isOptional(v.command, isStr) &&
    isStr(v.user) &&
    isStr(v.state) &&
    isNum(v.cpuTicks) &&
    isNum(v.rssBytes) &&
    isNum(v.threads) &&
    isOptional(v.kernel, isBool)
  )
}

export function isSystemStatsResponse(v: unknown): v is SystemStatsResponse {
  return (
    isObj(v) &&
    hasAddonSources(v) &&
    isNum(v.sampledAtMs) &&
    isCPUCounters(v.cpu) &&
    isObj(v.memory) &&
    isNum(v.memory.totalBytes) &&
    isNum(v.memory.usedBytes) &&
    isNum(v.memory.availableBytes) &&
    isMemoryDetail(v.memoryDetail) &&
    isNum(v.uptimeSeconds) &&
    isOptional(v.load, isLoadAverage) &&
    isOptional(v.network, isNetTotals) &&
    isOptional(v.disk, isDiskTotals) &&
    isOptional(v.tasks, isTaskCounts) &&
    isOptional(v.processes, isProcessList) &&
    isOptional(v.power, isPowerInfo)
  )
}

// isPowerInfo는 OMV 전원 관리의 대기 방식(?power=1, 전원 모달)을 검사한다.
export function isPowerInfo(v: unknown): v is PowerInfo {
  return isObj(v) && isStr(v.standbyMode) && isArrayOf(v.standbyModes, isStr)
}

// StatsPoint는 그래프의 한 점이다. t 시각까지 직전 샘플 이후의 평균이며, 구할 수 없는 값은 null이다.
export interface StatsPoint {
  // 백엔드 단조 시각(밀리초)
  t: number
  // 전체 CPU 사용률과 그중 커널 시간(0~100)
  cpu: number | null
  cpuKernel: number | null
  // 코어 id → 사용률, 커널 시간(0~100)
  cores: ReadonlyMap<number, number>
  coresKernel: ReadonlyMap<number, number>
  // 메모리 사용률(0~100)과 사용량(바이트)
  memory: number
  memoryUsed: number
  // 1·5·15분 부하 평균(응답 값 그대로). 응답에 없으면 null이다.
  load1: number | null
  load5: number | null
  load15: number | null
  // 초당 바이트
  rx: number | null
  tx: number | null
  read: number | null
  write: number | null
}

export interface StatsHistory {
  // 다음 응답과 차이를 구할 직전 응답
  last: SystemStatsResponse
  // 오래된 것부터. 60초 창의 왼쪽 끝까지 선이 닿도록 창보다 오래된 점을 하나 남긴다.
  points: readonly StatsPoint[]
}

// busyPercent는 두 누적 값 사이의 사용 시간 비율(0~100)이다. 차이가 없거나 줄었으면 null이다.
export function busyPercent(
  prev: { totalTicks: number; idleTicks: number },
  cur: { totalTicks: number; idleTicks: number },
): number | null {
  const total = cur.totalTicks - prev.totalTicks
  const idle = cur.idleTicks - prev.idleTicks
  if (total <= 0 || idle < 0) return null
  return Math.min(100, Math.max(0, ((total - idle) / total) * 100))
}

// kernelPercent는 두 누적 값 사이의 커널 시간 비율(0~100)이다. 차이가 없거나 줄었으면 null이다.
export function kernelPercent(
  prev: { totalTicks: number; kernelTicks: number },
  cur: { totalTicks: number; kernelTicks: number },
): number | null {
  const total = cur.totalTicks - prev.totalTicks
  const kernel = cur.kernelTicks - prev.kernelTicks
  if (total <= 0 || kernel < 0) return null
  return Math.min(100, Math.max(0, (kernel / total) * 100))
}

// rate는 누적 바이트 두 값의 초당 차이다. 값이 없거나 카운터가 줄었으면(재설정) null이다.
function rate(prev: number | undefined, cur: number | undefined, dtMs: number): number | null {
  if (prev === undefined || cur === undefined || cur < prev) return null
  return ((cur - prev) * 1000) / dtMs
}

// sampleGap은 같은 백엔드의 연속된 두 응답 사이 간격이다. 쓸 수 없으면(백엔드 재시작, 너무 긴 간격) null이다.
export function sampleGap(prev: SystemStatsResponse, cur: SystemStatsResponse): number | null {
  const dt = cur.sampledAtMs - prev.sampledAtMs
  return dt > 0 && dt <= MAX_SAMPLE_GAP_MS ? dt : null
}

// appendSample은 cur를 기록에 더한다. 첫 응답이거나 간격을 쓸 수 없으면 점 없이 새로 시작한다.
export function appendSample(prev: StatsHistory | null, cur: SystemStatsResponse): StatsHistory {
  if (!prev) return { last: cur, points: [] }
  const p = prev.last
  const dt = sampleGap(p, cur)
  if (dt === null) return { last: cur, points: [] }

  const before = new Map(p.cpu.cores.map((c) => [c.id, c]))
  const cores = new Map<number, number>()
  const coresKernel = new Map<number, number>()
  for (const c of cur.cpu.cores) {
    const b = before.get(c.id)
    if (!b) continue
    const v = busyPercent(b, c)
    if (v !== null) cores.set(c.id, v)
    const k = kernelPercent(b, c)
    if (k !== null) coresKernel.set(c.id, k)
  }
  const point: StatsPoint = {
    t: cur.sampledAtMs,
    cpu: busyPercent(p.cpu, cur.cpu),
    cpuKernel: kernelPercent(p.cpu, cur.cpu),
    cores,
    coresKernel,
    memory: percentOf(cur.memory.usedBytes, cur.memory.totalBytes),
    memoryUsed: cur.memory.usedBytes,
    load1: cur.load?.one ?? null,
    load5: cur.load?.five ?? null,
    load15: cur.load?.fifteen ?? null,
    rx: rate(p.network?.rxBytes, cur.network?.rxBytes, dt),
    tx: rate(p.network?.txBytes, cur.network?.txBytes, dt),
    read: rate(p.disk?.readBytes, cur.disk?.readBytes, dt),
    write: rate(p.disk?.writeBytes, cur.disk?.writeBytes, dt),
  }
  const points = [...prev.points, point]
  // 창 왼쪽 끝(cur - HISTORY_MS)보다 오래된 점은 마지막 하나만 남긴다.
  const from = cur.sampledAtMs - HISTORY_MS
  let start = 0
  while (start + 1 < points.length && points[start + 1].t <= from) start++
  return { last: cur, points: points.slice(start) }
}

// latest는 가장 최근 점이다.
export function latest(h: StatsHistory | null): StatsPoint | null {
  return h && h.points.length > 0 ? h.points[h.points.length - 1] : null
}
