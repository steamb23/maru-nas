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

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, { signal })
  if (!res.ok) {
    throw new ApiError(res.status, `${path}: HTTP ${res.status}`)
  }
  return (await res.json()) as T
}

export function fetchSystem(signal?: AbortSignal): Promise<SystemResponse> {
  return getJSON<SystemResponse>('/system', signal)
}
