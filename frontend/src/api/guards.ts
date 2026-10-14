// 응답 형태 검사용 작은 타입 가드. 프록시가 HTML 오류 페이지 등을 200으로 돌려줘도 화면이 깨지지 않게
// 화면에서 쓰는 필드를 확인하는 데 쓴다.

export const isNum = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v)
export const isStr = (v: unknown): v is string => typeof v === 'string'
export const isBool = (v: unknown): v is boolean => typeof v === 'boolean'
export const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null

// isArrayOf는 배열이고 모든 원소가 guard를 통과하는지다.
export function isArrayOf<T>(v: unknown, guard: (x: unknown) => x is T): v is T[] {
  return Array.isArray(v) && v.every(guard)
}

// isOptional은 값이 없거나(undefined) guard를 통과하는지다.
export function isOptional<T>(v: unknown, guard: (x: unknown) => x is T): v is T | undefined {
  return v === undefined || guard(v)
}

// isOneOf는 값이 values 중 하나인지다.
export function isOneOf<T extends string>(v: unknown, values: readonly T[]): v is T {
  return typeof v === 'string' && (values as readonly string[]).includes(v)
}
