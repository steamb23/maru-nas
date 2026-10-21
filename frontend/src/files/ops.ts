// 여러 항목에 차례로 하는 작업(삭제, 옮기기)의 진행과 실패 정리, 창 사이의 변경 알림. DOM에 의존하지 않아 node 환경에서
// 테스트한다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'
import { within } from './paths'

export interface OpProgress {
  done: number
  total: number
}

export interface OpFailure<T> {
  item: T
  err: unknown
}

export interface OpResult<T> {
  // done은 끝낸(성공하거나 실패한) 항목 수다. stopped면 중지해서 남은 항목을 하지 않았다.
  done: number
  failures: OpFailure<T>[]
  stopped: boolean
}

// runEach는 items에 run을 하나씩 차례로 부른다(도우미의 사용자당 동시 연산 4개 안에서 목록에 자리를 남긴다).
// signal이 끊기면 하던 항목까지 하고 멈춘다. 실패해도 다음 항목을 계속하고, 실패를 모아 돌려준다.
export async function runEach<T>(
  items: readonly T[],
  run: (item: T) => Promise<void>,
  onProgress: (p: OpProgress) => void,
  signal: AbortSignal,
): Promise<OpResult<T>> {
  const failures: OpFailure<T>[] = []
  let done = 0
  for (const item of items) {
    if (signal.aborted) return { done, failures, stopped: true }
    try {
      await run(item)
    } catch (err) {
      failures.push({ item, err })
    }
    done++
    onProgress({ done, total: items.length })
  }
  return { done, failures, stopped: false }
}

const OP_ERROR_KEYS: Partial<Record<string, ParseKeys>> = {
  not_found: 'files.ops.errors.not_found',
  permission_denied: 'files.ops.errors.permission_denied',
  file_exists: 'files.ops.errors.file_exists',
  cross_device: 'files.ops.errors.cross_device',
  root_folder: 'files.ops.errors.root_folder',
  not_empty: 'files.ops.errors.not_empty',
  read_only: 'files.ops.errors.read_only',
  no_space: 'files.ops.errors.no_space',
  helper_busy: 'files.ops.errors.helper_busy',
  transfer_timeout: 'files.ops.errors.timeout',
  outside_roots: 'files.ops.errors.outside_roots',
  bad_request: 'files.ops.errors.bad_request',
}

// opErrorKey는 만들기·이름 바꾸기·옮기기·삭제 실패의 문구 키다.
export function opErrorKey(err: unknown): ParseKeys {
  if (err instanceof ApiError && err.code) return OP_ERROR_KEYS[err.code] ?? 'files.ops.errors.generic'
  if (err instanceof TypeError) return 'files.ops.errors.network'
  return 'files.ops.errors.generic'
}

export interface FailureGroup {
  key: ParseKeys
  names: string[]
}

// groupFailures는 실패를 이유별로 묶는다(이유가 처음 나온 순서). 이름은 묶음마다 앞의 limit개만 두고 more에 나머지 수를 둔다.
export function groupFailures<T>(
  failures: readonly OpFailure<T>[],
  nameOf: (item: T) => string,
  limit = 5,
): (FailureGroup & { more: number })[] {
  const groups = new Map<ParseKeys, FailureGroup & { more: number }>()
  for (const f of failures) {
    const key = opErrorKey(f.err)
    let g = groups.get(key)
    if (!g) {
      g = { key, names: [], more: 0 }
      groups.set(key, g)
    }
    if (g.names.length < limit) g.names.push(nameOf(f.item))
    else g.more++
  }
  return [...groups.values()]
}

// FilesChange는 한 창이 바꾼 폴더다. dirs는 안의 항목이 바뀐 폴더, gone은 없어지거나 다른 이름이 된 경로다.
export interface FilesChange {
  dirs: readonly string[]
  gone: readonly string[]
  // origin은 바꾼 창, refreshed는 그 창이 바로 새로 읽는 자기 폴더다. 같은 세션의 목록은 경로마다 하나라 여러 창이
  // 같은 폴더를 함께 새로 읽으면 서로 밀어내므로, 다른 창은 잠시 뒤 그 목록을 함께 쓴다.
  origin?: unknown
  refreshed?: string
}

// affects는 path를 보는 창이 목록을 다시 열어야 하는지다(그 폴더가 바뀌었거나, 그 폴더나 그 위가 없어졌다).
export function affects(change: FilesChange, path: string): boolean {
  return change.dirs.includes(path) || change.gone.some((g) => within(path, g))
}

type Listener = (change: FilesChange) => void
const listeners = new Set<Listener>()

// notifyFilesChanged는 열린 파일 탐색기 창 모두에 알린다(바꾼 창 자신 포함). 창마다 상태를 따로 두므로 모듈 저장소 대신
// 알림만 나눈다.
export function notifyFilesChanged(change: FilesChange) {
  for (const l of [...listeners]) l(change)
}

// onFilesChanged는 알림을 받는다. 해제 함수를 돌려준다.
export function onFilesChanged(l: Listener): () => void {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}
