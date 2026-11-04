// 애드온 프레임 브리지의 판정(DOM 없는 순수 함수). 프레임이 보낸 요청을 받을지, 파라미터가 맞는지, 한도를 넘는지 정한다.
// 실제 처리는 hostServices.ts, 메시지 주고받기는 AddonFrame.tsx가 한다.
import {
  BRIDGE_CODES,
  GESTURE_METHODS,
  MAX_INFLIGHT,
  MAX_MESSAGE_BYTES,
  type RequestMethod,
  type RequestParams,
  type ViewKind,
  WIDGET_METHODS,
} from '@maru-sdk/protocol'

export type RejectCode = (typeof BRIDGE_CODES)[keyof typeof BRIDGE_CODES]

// 요청마다 받는 빈도. [창 길이(ms), 그 안에 받는 수]. 없으면 RATE_DEFAULT다.
const RATES: Partial<Record<RequestMethod, [number, number]>> = {
  action: [1000, 1],
  openWorkbench: [1000, 1],
  'storage.set': [1000, 1],
  confirm: [1000, 1],
  notify: [60_000, 3],
}
const RATE_DEFAULT: [number, number] = [1000, 50]

// GESTURE_WINDOW_MS는 프레임 안을 누른 뒤 사용자 조작이 필요한 요청을 받는 시간이다(브라우저의 일시적 활성화와 같은 길이).
export const GESTURE_WINDOW_MS = 5000

// BridgeLimits는 프레임 하나의 진행 중 요청 수와 빈도를 센다.
export class BridgeLimits {
  private inflight = 0
  private recent = new Map<RequestMethod, number[]>()

  // start는 요청을 받을 수 있으면 null, 아니면 거절 코드를 돌려준다. 받으면 done을 꼭 부른다.
  start(method: RequestMethod, now: number): RejectCode | null {
    if (this.inflight >= MAX_INFLIGHT) return BRIDGE_CODES.busy
    const [span, limit] = RATES[method] ?? RATE_DEFAULT
    const times = (this.recent.get(method) ?? []).filter((t) => now - t < span)
    if (times.length >= limit) {
      this.recent.set(method, times)
      return BRIDGE_CODES.rate
    }
    times.push(now)
    this.recent.set(method, times)
    this.inflight++
    return null
  }

  done() {
    this.inflight = Math.max(0, this.inflight - 1)
  }

  get pending(): number {
    return this.inflight
  }
}

// GestureState는 사용자 조작 판정에 필요한 호스트의 지금 상태다.
export interface GestureState {
  // 호스트 문서에 일시적 사용자 활성화가 있는지(navigator.userActivation.isActive). 모르면 true로 본다.
  activation: boolean
  // 이 프레임이 지금 포커스를 가졌는지(document.activeElement === iframe).
  focused: boolean
  // 이 프레임이 든 창이 맨 앞 창이고 창 모달로 막히지 않았는지. 위젯은 false다.
  front: boolean
  // 이 프레임 안의 마지막 사용자 입력(포인터·키) 시각. 없으면 null.
  lastInput: number | null
  now: number
}

// gestureAllowed는 사용자 조작이 필요한 요청(조작 실행, 워크벤치 열기)을 받을지다. 일시적 활성화는 문서 전체에
// 퍼지므로 그것만 보면 다른 곳을 누른 직후 뒤쪽 프레임의 요청까지 받게 된다. 그래서 그 프레임에 포커스가 있고,
// 그 프레임 안에서 방금 입력이 있었고, 그 창이 맨 앞일 때만 받는다.
export function gestureAllowed(s: GestureState): boolean {
  return (
    s.activation && s.focused && s.front && s.lastInput !== null && s.now - s.lastInput >= 0 && s.now - s.lastInput < GESTURE_WINDOW_MS
  )
}

// admit은 요청을 받을지 판정한다. 받으면 null이다. 한도(BridgeLimits)는 따로 센다.
export function admit(method: RequestMethod, view: ViewKind, size: number, gesture: () => boolean): RejectCode | null {
  if (size > MAX_MESSAGE_BYTES) return BRIDGE_CODES.invalid
  if (view === 'widget' && !WIDGET_METHODS.includes(method)) return BRIDGE_CODES.notAllowed
  if (GESTURE_METHODS.includes(method) && !gesture()) return BRIDGE_CODES.gesture
  return null
}

function isObj(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function isStrMap(v: unknown, maxLen: number): v is Record<string, string> {
  return isObj(v) && Object.values(v).every((x) => typeof x === 'string' && x.length <= maxLen)
}

const SERVICE_METHOD = /^[A-Za-z][A-Za-z0-9]*$/
const ACTION_NAME = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/
// WORKBENCH_ROUTE는 워크벤치 라우트(# 뒤의 경로)다. 다른 주소로 가지 않게 앞 슬래시·스킴·쿼리를 받지 않는다.
const WORKBENCH_ROUTE = /^[A-Za-z0-9][A-Za-z0-9/_.-]{0,255}$/
const I18N_KEY = /^[A-Za-z0-9_.-]{1,100}$/

// parseParams는 요청 파라미터를 검사한다. 맞지 않으면 null이다.
export function parseParams<M extends RequestMethod>(method: M, p: unknown): RequestParams[M] | null {
  if (!isObj(p)) return null
  const ok = (v: RequestParams[RequestMethod]) => v as RequestParams[M]
  switch (method) {
    case 'data': {
      if (p.query === undefined) return ok({})
      if (!isStrMap(p.query, 512) || Object.keys(p.query).length > 32) return null
      return ok({ query: p.query })
    }
    case 'rpc':
      if (typeof p.service !== 'string' || typeof p.method !== 'string') return null
      if (!SERVICE_METHOD.test(p.service) || !SERVICE_METHOD.test(p.method)) return null
      return ok({ service: p.service, method: p.method, params: p.params })
    case 'action':
    case 'query':
      if (typeof p.name !== 'string' || !ACTION_NAME.test(p.name)) return null
      if (p.target !== undefined && (typeof p.target !== 'string' || p.target.length > 1024)) return null
      if (p.input !== undefined && !isObj(p.input)) return null
      return ok({ name: p.name, target: p.target as string | undefined, input: p.input })
    case 'storage.get':
      return ok({})
    case 'storage.set':
      return 'data' in p && p.data !== undefined ? ok({ data: p.data }) : null
    case 'openWorkbench':
      return typeof p.route === 'string' && WORKBENCH_ROUTE.test(p.route) && !p.route.includes('..')
        ? ok({ route: p.route })
        : null
    case 'confirm':
      if (typeof p.message !== 'string' || p.message.length === 0 || p.message.length > 600) return null
      if (p.title !== undefined && (typeof p.title !== 'string' || p.title.length > 80)) return null
      if (p.confirmLabel !== undefined && (typeof p.confirmLabel !== 'string' || p.confirmLabel.length > 40)) return null
      return ok({
        message: p.message,
        title: p.title as string | undefined,
        confirmLabel: p.confirmLabel as string | undefined,
        danger: p.danger === true,
      })
    case 'notify':
      if (!['info', 'success', 'warning', 'error'].includes(p.level as string)) return null
      if (typeof p.key !== 'string' || !I18N_KEY.test(p.key)) return null
      if (
        p.params !== undefined &&
        !(isObj(p.params) && Object.values(p.params).every((x) => typeof x === 'number' || (typeof x === 'string' && x.length <= 200)))
      )
        return null
      return ok({ level: p.level as RequestParams['notify']['level'], key: p.key, params: p.params as Record<string, string | number> | undefined })
  }
  return null
}

// plainText는 패키지가 준 문구를 호스트 화면(확인 대화상자, 토스트)에 보일 평문으로 만든다. 제어·서식 문자
// (양방향 재정의 포함)를 지우고 공백을 하나로 줄인다.
export function plainText(s: string, max: number): string {
  // oxlint-disable-next-line no-control-regex
  const cleaned = s.replace(/[\u0000-\u001f\u007f-\u009f\u00ad\u061c\u180e\u200b-\u200f\u202a-\u202e\u2060-\u206f\ufeff]/g, ' ')
  const collapsed = cleaned.replace(/\s+/g, ' ').trim()
  return collapsed.length > max ? `${collapsed.slice(0, max - 1)}…` : collapsed
}
