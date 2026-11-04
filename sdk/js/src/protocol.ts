// SPDX-License-Identifier: MIT
// 애드온 패키지 프레임과 호스트(마루 NAS 대시보드) 사이의 postMessage 프로토콜. 호스트(frontend/src/addonpkg)와
// SDK 런타임(client.ts)이 이 파일 하나를 함께 쓴다. 의존성이 없고 DOM 타입만 쓴다.
//
// 프레임은 sandbox="allow-scripts"(불투명 출처)라 호스트의 쿠키·저장소에 닿지 못한다. 데이터는 이 프로토콜로
// 호스트에 요청하고 호스트가 자기 세션으로 대시보드 API를 부른다. 호스트는 애드온 id를 메시지가 아니라 프레임
// 바인딩에서 정한다.

// PROTOCOL_VERSION은 메시지의 maru 필드 값이다. 호환되지 않게 바꾸면 올린다.
export const PROTOCOL_VERSION = 1

// SDK_LEVEL은 이 프로토콜이 제공하는 SDK API 수준이다(manifest의 sdk). 백엔드 api.AddonSDKLevel과 같다.
export const SDK_LEVEL = 1

// MAX_MESSAGE_BYTES는 메시지 하나의 크기 상한(JSON 길이)이다.
export const MAX_MESSAGE_BYTES = 256 * 1024

// MAX_INFLIGHT는 프레임 하나가 동시에 기다릴 수 있는 요청 수다.
export const MAX_INFLIGHT = 16

export type ViewKind = 'window' | 'widget'

export type Theme = 'light' | 'dark'

// FrameContext는 호스트가 프레임에 주는 문맥이다. init에 전부, context에 바뀐 값만 담긴다.
export interface FrameContext {
  addonId: string
  view: ViewKind
  // active는 보기를 그려야 하는지다(창이 최소화되거나 탭이 숨겨지면 false). false면 주기 요청을 멈춘다.
  active: boolean
  // size는 프레임 크기(px)이고, 위젯이면 칸 수도 담는다.
  size: { width: number; height: number; cols?: number; rows?: number }
  theme: Theme
  animations: boolean
  language: string
  // messages는 패키지 번역 파일(i18n/<lang>.json) 위에 en을 깐 표다.
  messages: Record<string, string>
  admin: boolean
  sdkLevel: number
}

// 요청 이름. 창 보기는 모두, 위젯 보기는 WIDGET_METHODS만 쓸 수 있다.
export type RequestMethod =
  | 'data'
  | 'rpc'
  | 'action'
  | 'query'
  | 'storage.get'
  | 'storage.set'
  | 'openWorkbench'
  | 'confirm'
  | 'notify'

export const REQUEST_METHODS: readonly RequestMethod[] = [
  'data',
  'rpc',
  'action',
  'query',
  'storage.get',
  'storage.set',
  'openWorkbench',
  'confirm',
  'notify',
]

// WIDGET_METHODS는 위젯 보기가 쓸 수 있는 요청이다(위젯은 끌어 옮기는 요소라 조작·대화상자를 열지 않는다).
export const WIDGET_METHODS: readonly RequestMethod[] = ['data', 'rpc', 'storage.get']

// GESTURE_METHODS는 사용자가 그 프레임을 막 눌렀을 때만 받는 요청이다.
export const GESTURE_METHODS: readonly RequestMethod[] = ['action', 'openWorkbench']

export interface RequestParams {
  data: { query?: Record<string, string> }
  rpc: { service: string; method: string; params?: unknown }
  action: { name: string; target?: string; input?: unknown }
  query: { name: string; target?: string; input?: unknown }
  'storage.get': Record<string, never>
  'storage.set': { data: unknown }
  openWorkbench: { route: string }
  confirm: { title?: string; message: string; confirmLabel?: string; danger?: boolean }
  notify: { level: 'info' | 'success' | 'warning' | 'error'; key: string; params?: Record<string, string | number> }
}

export interface RequestResults {
  data: unknown
  rpc: unknown
  action: null
  query: unknown
  'storage.get': unknown
  'storage.set': null
  openWorkbench: null
  confirm: boolean
  notify: null
}

// BridgeError는 요청이 실패한 이유다. status는 대시보드 API의 HTTP 상태(호스트가 거절하면 0), code는 대시보드의
// 오류 코드 또는 호스트의 거절 코드(BRIDGE_CODES), addonCode는 패키지 백엔드가 준 코드다.
export interface BridgeError {
  status: number
  code?: string
  addonCode?: string
  message: string
}

// 호스트가 요청을 대시보드에 보내지 않고 거절한 이유.
export const BRIDGE_CODES = {
  invalid: 'bridge_invalid',
  notAllowed: 'bridge_not_allowed',
  gesture: 'bridge_gesture_required',
  busy: 'bridge_busy',
  rate: 'bridge_rate_limited',
  timeout: 'bridge_timeout',
  closed: 'bridge_closed',
  // cancelled는 사용자가 호스트의 확인 대화상자에서 취소했다는 뜻이다.
  cancelled: 'bridge_cancelled',
} as const

// 프레임 → 호스트.
export type FrameMessage =
  | { maru: typeof PROTOCOL_VERSION; type: 'hello'; sdk: number }
  | { maru: typeof PROTOCOL_VERSION; type: 'request'; id: number; method: RequestMethod; params: unknown }
  // input은 프레임 안의 사용자 입력이다. 호스트가 창 올리기·바깥 클릭 닫기·세션 유지에 쓴다.
  | { maru: typeof PROTOCOL_VERSION; type: 'input'; kind: 'pointer' | 'key' }

// 호스트 → 프레임.
export type HostMessage =
  | { maru: typeof PROTOCOL_VERSION; type: 'init'; context: FrameContext }
  | { maru: typeof PROTOCOL_VERSION; type: 'context'; context: Partial<FrameContext> }
  | { maru: typeof PROTOCOL_VERSION; type: 'response'; id: number; ok: true; result: unknown }
  | { maru: typeof PROTOCOL_VERSION; type: 'response'; id: number; ok: false; error: BridgeError }

function isObj(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

// parseFrameMessage는 프레임이 보낸 값이 프로토콜 메시지인지 검사한다. 아니면 null이다.
export function parseFrameMessage(data: unknown): FrameMessage | null {
  if (!isObj(data) || data.maru !== PROTOCOL_VERSION) return null
  switch (data.type) {
    case 'hello':
      return typeof data.sdk === 'number' ? { maru: PROTOCOL_VERSION, type: 'hello', sdk: data.sdk } : null
    case 'request':
      if (
        typeof data.id !== 'number' ||
        !Number.isInteger(data.id) ||
        data.id < 0 ||
        !REQUEST_METHODS.includes(data.method as RequestMethod)
      )
        return null
      return {
        maru: PROTOCOL_VERSION,
        type: 'request',
        id: data.id,
        method: data.method as RequestMethod,
        params: data.params ?? {},
      }
    case 'input':
      return data.kind === 'pointer' || data.kind === 'key'
        ? { maru: PROTOCOL_VERSION, type: 'input', kind: data.kind }
        : null
  }
  return null
}

// parseHostMessage는 호스트가 보낸 값이 프로토콜 메시지인지 검사한다(SDK 쪽). 아니면 null이다.
export function parseHostMessage(data: unknown): HostMessage | null {
  if (!isObj(data) || data.maru !== PROTOCOL_VERSION) return null
  switch (data.type) {
    case 'init':
      return isObj(data.context) ? (data as HostMessage) : null
    case 'context':
      return isObj(data.context) ? (data as HostMessage) : null
    case 'response':
      if (typeof data.id !== 'number') return null
      if (data.ok === true) return data as HostMessage
      return data.ok === false && isObj(data.error) ? (data as HostMessage) : null
  }
  return null
}

// interpolate는 번역 문구의 {{name}}을 값으로 바꾼다(SDK의 t와 호스트의 알림이 함께 쓴다). 모르는 이름은 그대로 둔다.
export function interpolate(text: string, params?: Record<string, string | number>): string {
  if (!params) return text
  return text.replace(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g, (m, name: string) =>
    Object.hasOwn(params, name) ? String(params[name]) : m,
  )
}
