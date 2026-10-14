// 네트워크 응답 검사와 속도 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { hasAddonSources } from '../../api/client'
import { isBool, isNum, isObj, isOptional, isStr } from '../../api/guards'
import type { NetInterface, NetworkResponse } from '../../api/types.gen'

const COUNTERS = [
  'rxBytes',
  'txBytes',
  'rxPackets',
  'txPackets',
  'rxErrors',
  'txErrors',
  'rxDropped',
  'txDropped',
] as const

function isNetInterface(v: unknown): v is NetInterface {
  return (
    isObj(v) &&
    isStr(v.name) &&
    isOptional(v.virtual, isBool) &&
    isStr(v.state) &&
    isOptional(v.speedMbps, isNum) &&
    COUNTERS.every((k) => isNum(v[k]))
  )
}

export function isNetworkResponse(v: unknown): v is NetworkResponse {
  return (
    isObj(v) &&
    hasAddonSources(v) &&
    isNum(v.sampledAtMs) &&
    Array.isArray(v.interfaces) &&
    v.interfaces.every(isNetInterface)
  )
}

// Rate는 초당 바이트다.
export interface Rate {
  rx: number
  tx: number
}

// 두 샘플 간격이 이보다 길면(탭을 오래 숨겼다 돌아옴 등) 긴 구간의 평균 대신 다음 샘플을 기다린다.
export const MAX_SAMPLE_GAP_MS = 30_000

// computeRates는 같은 백엔드의 두 응답으로 인터페이스별 초당 바이트를 구한다.
// 직전 샘플이 없거나, 간격이 0 이하·너무 길거나(백엔드 재시작 포함), 카운터가 줄었으면(재설정) 값이 없다.
export function computeRates(prev: NetworkResponse | null, cur: NetworkResponse): Map<string, Rate> {
  const out = new Map<string, Rate>()
  if (!prev) return out
  const dt = cur.sampledAtMs - prev.sampledAtMs
  if (dt <= 0 || dt > MAX_SAMPLE_GAP_MS) return out
  const before = new Map(prev.interfaces.map((i) => [i.name, i]))
  for (const i of cur.interfaces) {
    const p = before.get(i.name)
    if (!p || i.rxBytes < p.rxBytes || i.txBytes < p.txBytes) continue
    out.set(i.name, { rx: ((i.rxBytes - p.rxBytes) * 1000) / dt, tx: ((i.txBytes - p.txBytes) * 1000) / dt })
  }
  return out
}

// isUp은 링크가 올라와 있는지다. VPN·브리지 등은 operstate가 unknown이어도 동작하므로 함께 본다.
export function isUp(i: NetInterface): boolean {
  return i.state === 'up' || i.state === 'unknown'
}

// widgetInterfaces는 위젯에 보여 줄 물리 인터페이스다. 링크가 올라온 것을 먼저 둔다.
export function widgetInterfaces(list: readonly NetInterface[]): NetInterface[] {
  const physical = list.filter((i) => !i.virtual)
  return [...physical.filter(isUp), ...physical.filter((i) => !isUp(i))]
}
