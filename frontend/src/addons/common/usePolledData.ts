import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import type { ConnectionStatus, PollSnapshot, SharedPoll } from './sharedPoll'

export type { ConnectionStatus, PolledFetch } from './sharedPoll'

export interface PolledState<T> {
  // 연결이 끊겨도 마지막으로 받은 값을 유지한다.
  data: T | null
  status: ConnectionStatus
}

export interface PolledData<T> extends PolledState<T> {
  // refresh는 백엔드 캐시를 건너뛰고 바로 한 번 가져온다(fetch의 refresh 인자 true).
  // 같은 엔드포인트를 보는 다른 보기도 결과를 받는다.
  refresh: () => void
  // refresh 요청이 진행 중인지다.
  refreshing: boolean
}

interface LocalState<T, S> extends PolledState<S> {
  refreshing: boolean
  // reduce에 마지막으로 넘긴 응답. 같은 응답을 두 번 접지 않게 한다.
  raw: T | null
}

// usePolledData는 active인 동안 공유 폴러(sharedPoll.ts)에 intervalMs 주기로 붙는다.
// 같은 폴러를 보는 보기가 여럿이면 요청은 가장 짧은 주기로 하나만 나간다.
//
// active가 false가 되거나 언마운트되면 떨어진다. 마지막 값은 이 보기의 상태에 남아 다시 붙어도 유지된다.
// reduce를 주면 응답을 받을 때마다 직전 값과 함께 접어 보기별 값을 만든다(네트워크 속도 등).
// reduce는 렌더마다 바뀌어도 되며 마지막 것을 쓴다.
export function usePolledData<T>(poll: SharedPoll<T>, active: boolean, intervalMs: number): PolledData<T>
export function usePolledData<T, S>(
  poll: SharedPoll<T>,
  active: boolean,
  intervalMs: number,
  reduce: (prev: S | null, data: T) => S,
): PolledData<S>
export function usePolledData<T, S>(
  poll: SharedPoll<T>,
  active: boolean,
  intervalMs: number,
  reduce?: (prev: S | null, data: T) => S,
): PolledData<S> {
  const [state, setState] = useState<LocalState<T, S>>({ data: null, status: 'loading', refreshing: false, raw: null })
  const reduceRef = useRef(reduce)
  useLayoutEffect(() => {
    reduceRef.current = reduce
  })

  useEffect(() => {
    if (!active) return
    const onSnapshot = (snap: PollSnapshot<T>) =>
      setState((s) => {
        let { data, raw } = s
        if (snap.data !== null && snap.data !== raw) {
          raw = snap.data
          const fold = reduceRef.current
          data = fold ? fold(s.data, snap.data) : (snap.data as unknown as S)
        }
        // 다시 붙은 직후(공유 폴러가 아직 loading)에는 이 보기의 마지막 상태를 유지한다.
        const status = snap.status === 'loading' ? s.status : snap.status
        return { data, raw, status, refreshing: snap.refreshing }
      })
    const unsubscribe = poll.subscribe(intervalMs, onSnapshot)
    return () => {
      unsubscribe()
      // 떨어진 뒤에는 공유 폴러가 refresh 끝을 알려 주지 않는다.
      setState((s) => (s.refreshing ? { ...s, refreshing: false } : s))
    }
  }, [poll, active, intervalMs])

  const refresh = useCallback(() => poll.refresh(), [poll])

  return { data: state.data, status: state.status, refresh, refreshing: state.refreshing }
}
