// 보기와 따로 공유 폴러에 붙어 응답을 계속 접어 두는 기록기. React와 무관하게 동작해 node 환경에서 테스트한다.
//
// usePolledData는 보기가 보이는 동안(active)만 붙으므로 숨긴 탭·최소화된 창에서는 값이 쌓이지 않는다.
// 기록기는 retain한 동안(창이 열려 있는 동안) 계속 붙어 reduce로 값을 쌓고, 보기는 보일 때만 subscribe해
// 그린다(보이지 않는 표를 매번 다시 그리지 않게). 시스템 정보 창의 사용량 기록이 쓴다.
import type { ConnectionStatus, PollSnapshot, SharedPoll } from './sharedPoll'

export interface RecordedState<S> {
  data: S | null
  status: ConnectionStatus
}

export interface Recorder<S> {
  // retain은 intervalMs 주기로 기록을 시작하고(이미 했으면 수만 센다) 그만두는 함수를 돌려준다.
  // 모두 그만두면 폴러에서 떨어지고 쌓은 값을 버린다.
  retain: (intervalMs: number) => () => void
  // get은 지금 값이다. 바뀔 때마다 새 객체이므로 useSyncExternalStore의 getSnapshot으로 쓸 수 있다.
  get: () => RecordedState<S>
  // subscribe는 값이 바뀔 때마다 listener를 부른다.
  subscribe: (listener: () => void) => () => void
}

const EMPTY: RecordedState<never> = { data: null, status: 'loading' }

export function createRecorder<T, S>(poll: SharedPoll<T>, reduce: (prev: S | null, data: T) => S): Recorder<S> {
  let state: RecordedState<S> = EMPTY
  // reduce에 마지막으로 넘긴 응답. 같은 응답을 두 번 접지 않게 한다.
  let raw: T | null = null
  let holders = 0
  let detach: (() => void) | null = null
  const listeners = new Set<() => void>()

  const set = (next: RecordedState<S>) => {
    state = next
    for (const l of listeners) l()
  }

  const onSnapshot = (snap: PollSnapshot<T>) => {
    let data = state.data
    if (snap.data !== null && snap.data !== raw) {
      raw = snap.data
      data = reduce(state.data, snap.data)
    }
    if (data !== state.data || snap.status !== state.status) set({ data, status: snap.status })
  }

  return {
    retain(intervalMs) {
      holders++
      if (holders === 1) detach = poll.subscribe(intervalMs, onSnapshot)
      let released = false
      return () => {
        if (released) return
        released = true
        holders--
        if (holders > 0) return
        detach?.()
        detach = null
        raw = null
        set(EMPTY)
      }
    },
    get: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
  }
}
