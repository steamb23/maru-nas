// 웹 터미널 WebSocket(GET /api/terminal)의 제어 메시지. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 바이너리 프레임은 터미널 입출력 바이트이고, 텍스트 프레임은 JSON 제어 메시지다(backend api.Terminal*Message).
import { isArrayOf, isBool, isNum, isObj, isOneOf, isOptional, isStr } from '../api/guards'
import type { TerminalClientMessage, TerminalPrompt, TerminalServerMessage } from '../api/types.gen'

// MAX_INPUT_CHUNK는 입력 바이너리 프레임 하나의 최대 크기다. 백엔드의 읽기 한도(64KiB)보다 작게 나눠 보낸다.
export const MAX_INPUT_CHUNK = 16 * 1024

export type CloseReason = NonNullable<TerminalServerMessage['reason']>

const STATES: readonly NonNullable<TerminalServerMessage['state']>[] = ['connecting', 'connected', 'closed']

const REASONS: readonly CloseReason[] = [
  'exit',
  'session_ended',
  'disabled',
  'connection_lost',
  'security_required',
  'ssh_unavailable',
  'ssh_host_key',
  'ssh_auth_failed',
  'ssh_failed',
  'canceled',
  'too_many_windows',
  'busy',
  'rate_limited',
]

const isPrompt = (v: unknown): v is TerminalPrompt => isObj(v) && isStr(v.text) && isBool(v.echo)

// parseServerMessage는 텍스트 프레임을 읽는다. 형식이 맞지 않으면 null이다. 모르는 reason은 ssh_failed로 본다
// (백엔드가 새 이유를 더해도 창이 멈추지 않게).
export function parseServerMessage(text: string): TerminalServerMessage | null {
  let v: unknown
  try {
    v = JSON.parse(text)
  } catch {
    return null
  }
  if (!isObj(v)) return null
  if (v.type === 'status' && isOneOf(v.state, STATES)) {
    const msg: TerminalServerMessage = { type: 'status', state: v.state }
    if (v.state === 'closed') msg.reason = isOneOf(v.reason, REASONS) ? v.reason : 'ssh_failed'
    if (isNum(v.exitCode)) msg.exitCode = v.exitCode
    if (isStr(v.keyCode) && v.keyCode !== '') msg.keyCode = v.keyCode
    if (isArrayOf(v.notAllowedGroups, isStr) && v.notAllowedGroups.length > 0) msg.notAllowedGroups = v.notAllowedGroups
    if (isNum(v.retryAfterSeconds)) msg.retryAfterSeconds = v.retryAfterSeconds
    if (isOneOf(v.required, ['password', 'twoFactor'] as const)) msg.required = v.required
    return msg
  }
  if (
    v.type === 'prompt' &&
    isOneOf(v.kind, ['password', 'keyboard-interactive'] as const) &&
    isOptional(v.instruction, isStr) &&
    isArrayOf(v.prompts, isPrompt) &&
    v.prompts.length > 0
  ) {
    return { type: 'prompt', kind: v.kind, instruction: v.instruction ?? '', prompts: v.prompts }
  }
  return null
}

export function resizeMessage(cols: number, rows: number): string {
  return JSON.stringify({ type: 'resize', cols, rows } satisfies TerminalClientMessage)
}

export function answerMessage(answers: string[]): string {
  return JSON.stringify({ type: 'answer', answers } satisfies TerminalClientMessage)
}

export function cancelMessage(): string {
  return JSON.stringify({ type: 'cancel' } satisfies TerminalClientMessage)
}

// inputChunks는 입력을 MAX_INPUT_CHUNK 이하 조각으로 나눈다(큰 붙여 넣기).
export function inputChunks(data: Uint8Array<ArrayBuffer>, size = MAX_INPUT_CHUNK): Uint8Array<ArrayBuffer>[] {
  const out: Uint8Array<ArrayBuffer>[] = []
  for (let i = 0; i < data.length; i += size) out.push(data.subarray(i, i + size))
  return out
}

// terminalUrl은 문서 주소(baseURI) 기준의 WebSocket 주소다. apiBase는 API 경로 접두사(Vite BASE_URL + 'api')다.
// 대시보드가 경로 접두사 아래에 있어도 상대 경로로 찾는다.
export function terminalUrl(baseURI: string, apiBase: string, cols: number, rows: number): string {
  const url = new URL(`${apiBase}/terminal`, baseURI)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  url.search = new URLSearchParams({ cols: String(cols), rows: String(rows) }).toString()
  url.hash = ''
  return url.toString()
}
