import { useCallback, useEffect, useRef, useState } from 'react'
import { runAddonQuery } from '../../api/client'
import type { SmartDetail, SmartRaw } from '../../api/types.gen'
import { detailProblem, isSmartDetail, isSmartRaw } from './detail'

// Load는 한 번 받는 데이터의 상태다. key(요청)가 지금 것과 다르면 받는 중으로 본다(시스템 업데이트와 같다).
export type Load<T> = { key: string } & ({ status: 'loading' } | { status: 'ready'; data: T } | { status: 'error'; err: unknown })

// 같은 디스크를 다른 요청(다른 창, 다른 관리자)이 읽는 중이면 이만큼 뒤에 다시 묻는다.
const PENDING_RETRY_MS = 2_000

export function fetchSmartDetail(device: string, signal?: AbortSignal): Promise<SmartDetail> {
  return runAddonQuery('smart', 'detail', device, isSmartDetail, { signal })
}

export function fetchSmartRaw(device: string, signal?: AbortSignal): Promise<SmartRaw> {
  return runAddonQuery('smart', 'raw', device, isSmartRaw, { signal })
}

export interface SmartDetails {
  // state는 고른 디스크의 상세다. 고르지 않았으면 null이다.
  state: Load<SmartDetail> | null
  // reload는 고른 디스크를 다시 읽는다(잠든 디스크를 깨운다).
  reload: () => void
}

// useSmartDetails는 창이 열려 있는 동안 디스크별 상세를 기억한다. 디스크를 처음 고를 때만 읽고, 다시 고르면
// 기억한 결과를 보인다(읽을 때마다 smartctl이 디스크를 깨우므로). 다른 디스크로 옮겨도 읽던 요청은 끝까지 받는다
// (백엔드는 브라우저가 끊어도 끝까지 읽으므로, 끊으면 다시 고를 때 한 번 더 읽게 된다). 창을 닫으면 버린다.
export function useSmartDetails(device: string | null): SmartDetails {
  const [attempts, setAttempts] = useState<Record<string, number>>({})
  const [results, setResults] = useState<Record<string, Load<SmartDetail>>>({})
  const key = device === null ? '' : `${device}\0${attempts[device] ?? 0}`
  // started는 시작한 요청의 key다(끝난 것 포함). 같은 key로 다시 묻지 않는다.
  const started = useRef(new Set<string>())
  const ctl = useRef<AbortController | null>(null)
  const timers = useRef(new Set<number>())

  // 창이 닫히면 진행 중인 요청과 다시 묻기를 멈춘다. StrictMode가 정리했다가 다시 붙여도 새 컨트롤러로 이어 간다.
  useEffect(() => {
    const c = new AbortController()
    const pending = timers.current
    const seen = started.current
    ctl.current = c
    return () => {
      c.abort()
      for (const id of pending) window.clearTimeout(id)
      pending.clear()
      seen.clear()
    }
  }, [])

  useEffect(() => {
    const c = ctl.current
    if (device === null || c === null || started.current.has(key)) return
    started.current.add(key)
    const run = () => {
      fetchSmartDetail(device, c.signal).then(
        (data) => setResults((r) => ({ ...r, [device]: { key, status: 'ready', data } })),
        (err: unknown) => {
          if (c.signal.aborted) return
          if (detailProblem(err) === 'pending') {
            const id = window.setTimeout(() => {
              timers.current.delete(id)
              run()
            }, PENDING_RETRY_MS)
            timers.current.add(id)
            return
          }
          setResults((r) => ({ ...r, [device]: { key, status: 'error', err } }))
        },
      )
    }
    run()
  }, [device, key])

  const reload = useCallback(() => {
    if (device !== null) setAttempts((a) => ({ ...a, [device]: (a[device] ?? 0) + 1 }))
  }, [device])

  if (device === null) return { state: null, reload }
  const r = results[device]
  return { state: r && r.key === key ? r : { key, status: 'loading' }, reload }
}

// useSmartRaw는 원문 탭을 처음 열 때 원문을 받는다. 상세에 원문이 있으면(NVMe) 묻지 않는다. readAt(상세를 읽은
// 시각)이 바뀌면(다시 읽기) 다시 받는다.
export function useSmartRaw(device: string, enabled: boolean, detail: SmartDetail | null): { state: Load<SmartRaw>; retry: () => void } {
  const [attempt, setAttempt] = useState(0)
  const key = `${device}\0${detail?.readAt ?? ''}\0${attempt}`
  const [result, setResult] = useState<Load<SmartRaw>>({ key: '', status: 'loading' })
  const inline = detail?.raw
  useEffect(() => {
    if (!enabled || detail === null || inline !== undefined) return
    const c = new AbortController()
    let timer = 0
    const run = () => {
      fetchSmartRaw(device, c.signal).then(
        (data) => setResult({ key, status: 'ready', data }),
        (err: unknown) => {
          if (c.signal.aborted) return
          if (detailProblem(err) === 'pending') {
            timer = window.setTimeout(run, PENDING_RETRY_MS)
            return
          }
          setResult({ key, status: 'error', err })
        },
      )
    }
    run()
    return () => {
      c.abort()
      window.clearTimeout(timer)
    }
  }, [device, enabled, detail, inline, key])
  const retry = useCallback(() => setAttempt((n) => n + 1), [])
  if (detail !== null && inline !== undefined) {
    return {
      state: { key, status: 'ready', data: { device, readAt: detail.readAt, text: inline, truncated: detail.rawTruncated } },
      retry,
    }
  }
  return { state: result.key === key ? result : { key, status: 'loading' }, retry }
}
