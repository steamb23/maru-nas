import type {
  AddonCapability,
  CapabilitiesResponse,
  ChallengeInfo,
  ErrorResponse,
  LoginResponse,
  SessionInfo,
  SystemResponse,
} from './types.gen'

// BASE_URL은 Vite base 설정값('/desk/')이다.
const API_BASE = `${import.meta.env.BASE_URL}api`

export type ErrorCode = NonNullable<ErrorResponse['code']>

const ERROR_CODES: readonly ErrorCode[] = [
  'invalid_credentials',
  'rate_limited',
  'challenge_failed',
  'challenge_expired',
  'omv_unavailable',
  'unauthorized',
  'bad_request',
  'rpc_forbidden',
  'admin_required',
  'plugin_missing',
  'omv_error',
]

export class ApiError extends Error {
  readonly status: number
  // code, retryAfterSeconds는 백엔드가 ErrorResponse로 보낸 값이다. 프록시 오류 페이지 등에서는 없다.
  readonly code?: ErrorCode
  readonly retryAfterSeconds?: number

  constructor(status: number, message: string, code?: ErrorCode, retryAfterSeconds?: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.retryAfterSeconds = retryAfterSeconds
  }
}

// 로그인되지 않음·세션 만료(401)를 받으면 부르는 함수. 로그인 화면으로 보내는 데 쓴다.
let onUnauthorized: (() => void) | null = null

// setUnauthorizedHandler는 401 처리 함수를 등록하고 해제 함수를 반환한다.
// 로그인 관련 요청(fetchSession, login, verify, logout)의 401에는 부르지 않는다.
// 그 요청들은 401을 "로그인 안 됨"이라는 정상 결과로 직접 처리한다.
export function setUnauthorizedHandler(fn: () => void): () => void {
  onUnauthorized = fn
  return () => {
    if (onUnauthorized === fn) onUnauthorized = null
  }
}

const isNum = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)
const isStr = (v: unknown): v is string => typeof v === 'string'
const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null

async function toApiError(path: string, res: Response): Promise<ApiError> {
  let code: ErrorCode | undefined
  let retryAfter: number | undefined
  try {
    const body: unknown = await res.json()
    if (isObj(body)) {
      if (ERROR_CODES.includes(body.code as ErrorCode)) code = body.code as ErrorCode
      if (isNum(body.retryAfterSeconds) && body.retryAfterSeconds > 0) retryAfter = body.retryAfterSeconds
    }
  } catch {
    // JSON이 아닌 오류 응답(프록시 502 페이지 등)
  }
  return new ApiError(res.status, `${path}: HTTP ${res.status}`, code, retryAfter)
}

interface RequestOptions {
  signal?: AbortSignal
  body?: unknown
  // true면 401을 받아도 onUnauthorized를 부르지 않는다.
  authRequest?: boolean
}

async function request(method: 'GET' | 'POST', path: string, opts: RequestOptions = {}): Promise<Response> {
  const init: RequestInit = { method, signal: opts.signal, credentials: 'same-origin' }
  if (method === 'POST') {
    // 백엔드는 POST에 application/json만 받는다(CSRF 보조 장치).
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify(opts.body ?? {})
  }
  const res = await fetch(`${API_BASE}${path}`, init)
  if (!res.ok) {
    const err = await toApiError(path, res)
    if (res.status === 401 && !opts.authRequest) onUnauthorized?.()
    throw err
  }
  return res
}

async function getJSON(path: string, opts?: RequestOptions): Promise<unknown> {
  return (await request('GET', path, opts)).json()
}

async function postJSON(path: string, opts?: RequestOptions): Promise<unknown> {
  return (await request('POST', path, opts)).json()
}

// isSystemResponse는 화면에서 쓰는 필드가 모두 숫자인지 확인한다.
// 프록시가 HTML 오류 페이지 등을 200으로 돌려줘도 창이 깨지지 않게 한다.
export function isSystemResponse(v: unknown): v is SystemResponse {
  return (
    isObj(v) &&
    isObj(v.cpu) &&
    isNum(v.cpu.usagePercent) &&
    isObj(v.memory) &&
    isNum(v.memory.totalBytes) &&
    isNum(v.memory.usedBytes) &&
    isNum(v.memory.availableBytes) &&
    isNum(v.uptimeSeconds)
  )
}

export function isSessionInfo(v: unknown): v is SessionInfo {
  return (
    isObj(v) &&
    isStr(v.username) &&
    v.username !== '' &&
    typeof v.admin === 'boolean' &&
    (v.expiresInSeconds === undefined || (isNum(v.expiresInSeconds) && v.expiresInSeconds >= 0))
  )
}

function isChallengeInfo(v: unknown): v is ChallengeInfo {
  return isObj(v) && isStr(v.kind) && (v.redirectUrl === undefined || isStr(v.redirectUrl))
}

// isLoginResponse는 상태에 맞는 필드가 들어 있는지 확인한다.
export function isLoginResponse(v: unknown): v is LoginResponse {
  if (!isObj(v)) return false
  if (v.status === 'authenticated') return isSessionInfo(v.session)
  if (v.status === 'challengeRequired') return isChallengeInfo(v.challenge)
  return false
}

const CAPABILITY_REASONS: readonly NonNullable<AddonCapability['reason']>[] = ['plugin_missing', 'admin_required']

function isAddonCapability(v: unknown): v is AddonCapability {
  return (
    isObj(v) &&
    isStr(v.id) &&
    typeof v.available === 'boolean' &&
    (v.reason === undefined || CAPABILITY_REASONS.includes(v.reason as NonNullable<AddonCapability['reason']>)) &&
    Array.isArray(v.sources) &&
    v.sources.every(isStr)
  )
}

export function isCapabilitiesResponse(v: unknown): v is CapabilitiesResponse {
  return (
    isObj(v) &&
    Array.isArray(v.addons) &&
    v.addons.every(isAddonCapability) &&
    Array.isArray(v.plugins) &&
    v.plugins.every(isStr)
  )
}

// fetchCapabilities는 플러그인 설치 상태와 사용자 역할로 계산한 애드온별 사용 가능 여부를 가져온다.
export async function fetchCapabilities(signal?: AbortSignal): Promise<CapabilitiesResponse> {
  const data = await getJSON('/capabilities', { signal })
  if (!isCapabilitiesResponse(data)) {
    throw new Error('/capabilities: unexpected response shape')
  }
  return data
}

export async function fetchSystem(signal?: AbortSignal): Promise<SystemResponse> {
  const data = await getJSON('/system', { signal })
  if (!isSystemResponse(data)) {
    throw new Error('/system: unexpected response shape')
  }
  return data
}

// fetchSession은 현재 브라우저의 대시보드 세션을 묻는다. 로그인되어 있지 않으면 401 ApiError를 던진다.
export async function fetchSession(signal?: AbortSignal): Promise<SessionInfo> {
  const data = await getJSON('/session', { signal, authRequest: true })
  if (!isSessionInfo(data)) {
    throw new Error('/session: unexpected response shape')
  }
  return data
}

async function loginRequest(path: string, body: unknown): Promise<LoginResponse> {
  const data = await postJSON(path, { body, authRequest: true })
  if (!isLoginResponse(data)) {
    throw new Error(`${path}: unexpected response shape`)
  }
  return data
}

export function login(username: string, password: string): Promise<LoginResponse> {
  return loginRequest('/login', { username, password })
}

export function verifyChallenge(response: string): Promise<LoginResponse> {
  return loginRequest('/login/verify', { response })
}

export async function logout(): Promise<void> {
  await request('POST', '/logout', { authRequest: true })
}

// sendActivity는 사용자가 화면을 조작했음을 알려 OMV 세션을 연장하고, 새 만료 시간을 담은
// 세션 정보를 반환한다. 세션이 이미 만료됐으면 401을 받아 onUnauthorized가 불린다.
export async function sendActivity(): Promise<SessionInfo> {
  const data = await postJSON('/session/activity')
  if (!isSessionInfo(data)) {
    throw new Error('/session/activity: unexpected response shape')
  }
  return data
}
