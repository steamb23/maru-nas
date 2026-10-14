import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPoller } from './poller'

// loading: 아직 응답이 없음. ok: 마지막 요청 성공. disconnected: 마지막 요청 실패(재시도 중).
export type ConnectionStatus = 'loading' | 'ok' | 'disconnected'

export interface PolledState<T> {
  // 연결이 끊겨도 마지막으로 받은 값을 유지한다.
  data: T | null
  status: ConnectionStatus
}

export interface PolledData<T> extends PolledState<T> {
  // refresh는 백엔드 캐시를 건너뛰고 바로 한 번 가져온다(fetch의 refresh 인자 true).
  refresh: () => void
  // refresh 요청이 진행 중인지다.
  refreshing: boolean
}

export type PolledFetch<T> = (signal: AbortSignal, refresh: boolean) => Promise<T>

const DEFAULT_TIMEOUT_MS = 10_000

// usePolledData는 active인 동안 intervalMs마다 fetch로 데이터를 가져온다(poller.ts).
// active가 false가 되거나 언마운트되면 멈춘다. fetch는 렌더마다 바뀌어도 되며 마지막 것을 쓴다.
export function usePolledData<T>(
  fetch: PolledFetch<T>,
  active: boolean,
  intervalMs: number,
  timeoutMs = DEFAULT_TIMEOUT_MS,
): PolledData<T> {
  const [state, setState] = useState<PolledState<T>>({ data: null, status: 'loading' })
  const [refreshing, setRefreshing] = useState(false)
  const fetchRef = useRef(fetch)
  useLayoutEffect(() => {
    fetchRef.current = fetch
  })
  const refreshCtrl = useRef<AbortController | null>(null)

  const onData = useCallback((data: T) => setState({ data, status: 'ok' }), [])
  const onError = useCallback(
    () => setState((s) => (s.status === 'disconnected' ? s : { ...s, status: 'disconnected' })),
    [],
  )

  useEffect(() => {
    if (!active) return
    const poller = createPoller({
      fetch: (signal) => fetchRef.current(signal, false),
      intervalMs,
      timeoutMs,
      onData,
      onError,
    })
    poller.start()
    return poller.stop
  }, [active, intervalMs, timeoutMs, onData, onError])

  // 언마운트하면 진행 중인 새로고침을 취소한다.
  useEffect(() => () => refreshCtrl.current?.abort(), [])

  const refresh = useCallback(() => {
    if (refreshCtrl.current) return
    const ctrl = new AbortController()
    refreshCtrl.current = ctrl
    setRefreshing(true)
    // 새로고침은 백엔드가 OMV를 다시 부르므로(SMART 등) 주기 요청보다 오래 기다린다.
    const timer = setTimeout(() => ctrl.abort(new Error('request timed out')), timeoutMs * 6)
    fetchRef
      .current(ctrl.signal, true)
      .then((data) => !ctrl.signal.aborted && onData(data))
      .catch(() => !ctrl.signal.aborted && onError())
      .finally(() => {
        clearTimeout(timer)
        if (refreshCtrl.current === ctrl) {
          refreshCtrl.current = null
          setRefreshing(false)
        }
      })
  }, [timeoutMs, onData, onError])

  return { ...state, refresh, refreshing }
}
