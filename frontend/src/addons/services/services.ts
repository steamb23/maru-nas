// OMV 서비스 응답 검사와 표시 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { hasAddonSources } from '../../api/client'
import { isArrayOf, isObj, isStr } from '../../api/guards'
import type { Service, ServicesResponse } from '../../api/types.gen'

function isService(v: unknown): v is Service {
  return (
    isObj(v) &&
    isStr(v.name) &&
    isStr(v.title) &&
    typeof v.enabled === 'boolean' &&
    typeof v.running === 'boolean' &&
    typeof v.toggleable === 'boolean'
  )
}

export function isServicesResponse(v: unknown): v is ServicesResponse {
  return isObj(v) && hasAddonSources(v) && isArrayOf(v.services, isService)
}

// Tone은 상태 점의 색이다. ok: 켜져 있고 실행 중, crit: 켜져 있는데 멈춤, warn: 꺼져 있는데 실행 중
// (설정과 실제가 다름), idle: 꺼짐.
export type Tone = 'ok' | 'warn' | 'crit' | 'idle'

export function serviceTone(s: Service): Tone {
  if (s.enabled) return s.running ? 'ok' : 'crit'
  return s.running ? 'warn' : 'idle'
}

// ServiceState는 상태 문구의 종류다(services.state.*).
export type ServiceState = 'running' | 'stopped' | 'disabled' | 'disabledRunning'

export function serviceState(s: Service): ServiceState {
  if (s.enabled) return s.running ? 'running' : 'stopped'
  return s.running ? 'disabledRunning' : 'disabled'
}

const TONE_ORDER: Record<Tone, number> = { crit: 0, warn: 1, ok: 2, idle: 3 }

// sortByAttention은 문제가 있는 서비스를 먼저, 그다음 켜진 것, 꺼진 것 순서로 둔다. 같은 순위는 이름 순서다.
export function sortByAttention(list: readonly Service[], locale?: string): Service[] {
  return [...list].sort(
    (a, b) => TONE_ORDER[serviceTone(a)] - TONE_ORDER[serviceTone(b)] || a.title.localeCompare(b.title, locale),
  )
}

export interface ServiceCounts {
  // 켜져 있고 실행 중인 서비스 수
  running: number
  // 설정과 실제 상태가 다른 서비스 수
  attention: number
}

export function countServices(list: readonly Service[]): ServiceCounts {
  let running = 0
  let attention = 0
  for (const s of list) {
    const tone = serviceTone(s)
    if (tone === 'ok') running++
    if (tone === 'warn' || tone === 'crit') attention++
  }
  return { running, attention }
}
