// 컨테이너 응답 검사와 표시 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { hasAddonSources } from '../../api/client'
import { isArrayOf, isObj, isOneOf, isOptional, isStr } from '../../api/guards'
import type { Container, ContainersResponse } from '../../api/types.gen'

const STATES = ['running', 'paused', 'restarting', 'exited', 'created', 'dead', 'removing', 'unknown'] as const
const HEALTHS = ['healthy', 'unhealthy', 'starting'] as const

function isContainer(v: unknown): v is Container {
  return (
    isObj(v) &&
    isStr(v.name) &&
    isStr(v.image) &&
    isOneOf(v.state, STATES) &&
    isOptional(v.health, (h): h is Container['health'] => isOneOf(h, HEALTHS)) &&
    isStr(v.status)
  )
}

export function isContainersResponse(v: unknown): v is ContainersResponse {
  return (
    isObj(v) &&
    hasAddonSources(v) &&
    isOneOf(v.runtime, ['docker', 'podman', ''] as const) &&
    isStr(v.runtimeStatus) &&
    isArrayOf(v.containers, isContainer)
  )
}

// Tone은 상태 점의 색이다. ok: 실행 중, warn: 주의(다시 시작 중, 헬스체크 실패·대기), crit: dead,
// idle: 정지·생성됨·일시 정지.
export type Tone = 'ok' | 'warn' | 'crit' | 'idle'

export function containerTone(c: Container): Tone {
  if (c.state === 'dead') return 'crit'
  if (c.state === 'restarting' || c.health === 'unhealthy') return 'warn'
  if (c.state === 'running') return c.health === 'starting' ? 'warn' : 'ok'
  return 'idle'
}

const TONE_ORDER: Record<Tone, number> = { crit: 0, warn: 1, ok: 2, idle: 3 }

// sortByAttention은 문제가 있는 컨테이너를 먼저, 그다음 실행 중, 정지 순서로 둔다. 같은 순위는 이름 순서다.
export function sortByAttention(list: readonly Container[]): Container[] {
  return [...list].sort(
    (a, b) => TONE_ORDER[containerTone(a)] - TONE_ORDER[containerTone(b)] || a.name.localeCompare(b.name),
  )
}

export interface ContainerCounts {
  running: number
  stopped: number
  // 주의·위험 상태 수
  attention: number
}

// countContainers는 실행 중(일시 정지 제외)·그 밖·주의가 필요한 컨테이너 수다.
export function countContainers(list: readonly Container[]): ContainerCounts {
  let running = 0
  let attention = 0
  for (const c of list) {
    if (c.state === 'running') running++
    const tone = containerTone(c)
    if (tone === 'warn' || tone === 'crit') attention++
  }
  return { running, stopped: list.length - running, attention }
}
