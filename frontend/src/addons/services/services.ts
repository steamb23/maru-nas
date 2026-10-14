// OMV 서비스 응답 검사와 표시 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { hasAddonSources } from '../../api/client'
import { isArrayOf, isObj, isStr } from '../../api/guards'
import type { ParseKeys } from 'i18next'
import type { Service, ServicesResponse } from '../../api/types.gen'
import type { ActionSpec, AddonActionName } from '../common/actionSpec'

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

export type ServiceAction = AddonActionName<'services'>

// serviceReached는 켜기·끄기의 결과 상태(설정과 실제 실행이 모두 바뀜)에 이르렀는지다. OMV는 설정을 먼저
// 바꾸고 실제 실행은 조금 늦게 따라오므로, 그때까지 조작을 끝난 것으로 보지 않는다.
export function serviceReached(s: Service, action: string): boolean {
  if (action === 'enable') return s.enabled && s.running
  if (action === 'disable') return !s.enabled && !s.running
  return true
}

// holdServices는 조작 중인 서비스의 표시를 조작 전 상태로 붙잡아 둔다. 처음 busy가 된 서비스는 지금 값을
// 붙잡고, busy가 아니게 되면 놓는다. 바뀐 것이 없으면 held를 그대로 돌려준다(렌더링 중 상태 갱신에 쓴다).
// 붙잡아 두면 설정만 바뀐 중간 상태(아직 정지 중 등)가 보이거나 행이 정렬 순서를 따라 튀지 않는다.
export function holdServices(
  held: Readonly<Record<string, Service>>,
  list: readonly Service[],
  busy: (name: string) => boolean,
): Readonly<Record<string, Service>> {
  const next: Record<string, Service> = {}
  let changed = false
  for (const s of list) {
    if (!busy(s.name)) continue
    next[s.name] = held[s.name] ?? s
    if (!held[s.name]) changed = true
  }
  if (!changed && Object.keys(held).length === Object.keys(next).length) return held
  return next
}

// 끌 때 무엇이 끊기는지 알려 주는 확인 문구. 표에 없는 서비스는 일반 문구를 쓴다.
const DISABLE_CONFIRM: Record<string, ParseKeys> = {
  ssh: 'services.action.confirmDisableSsh',
  samba: 'services.action.confirmDisableSamba',
  nfs: 'services.action.confirmDisableNfs',
  rsyncd: 'services.action.confirmDisableRsyncd',
}

// serviceActions는 서비스의 지금 설정에서 쓸 수 있는 조작이다. 켜고 끌 수 없는 서비스는 없다.
// 켜져 있으면(멈춰 있어도) 끄기, 꺼져 있으면 켜기다.
export function serviceActions(s: Service): ActionSpec<ServiceAction>[] {
  if (!s.toggleable) return []
  if (s.enabled) {
    return [
      {
        action: 'disable',
        icon: 'media-playback-stop-symbolic',
        labelKey: 'services.action.disable',
        runningKey: 'services.action.disabling',
        confirmKey: DISABLE_CONFIRM[s.name] ?? 'services.action.confirmDisable',
        danger: true,
      },
    ]
  }
  return [
    {
      action: 'enable',
      icon: 'media-playback-start-symbolic',
      labelKey: 'services.action.enable',
      runningKey: 'services.action.enabling',
      confirmKey: 'services.action.confirmEnable',
    },
  ]
}
