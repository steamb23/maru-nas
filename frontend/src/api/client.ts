import type { SystemResponse } from './types.gen'

// BASE_URL은 Vite base 설정값('/desk/')이다.
const API_BASE = `${import.meta.env.BASE_URL}api`

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

async function getJSON(path: string, signal?: AbortSignal): Promise<unknown> {
  const res = await fetch(`${API_BASE}${path}`, { signal })
  if (!res.ok) {
    throw new ApiError(res.status, `${path}: HTTP ${res.status}`)
  }
  return res.json()
}

const isNum = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)
const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null

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

export async function fetchSystem(signal?: AbortSignal): Promise<SystemResponse> {
  const data = await getJSON('/system', signal)
  if (!isSystemResponse(data)) {
    throw new Error('/system: unexpected response shape')
  }
  return data
}
