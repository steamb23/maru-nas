// 같은 엔드포인트를 보는 여러 보기(창, 위젯)가 요청 하나를 함께 쓰는 폴러. React와 무관하게 동작해
// node 환경에서 테스트한다.
//
// - 엔드포인트마다 모듈 수준에서 하나 만든다(useContainers.ts 등). 보기는 subscribe로 붙는다.
// - 주기는 지금 붙어 있는 보기의 주기 중 가장 짧은 값이다. 더 짧은 보기가 붙으면 마지막 응답 시각
//   기준으로 다시 예약하고, 떨어지면 다음 예약부터 긴 주기로 돌아간다.
// - 결과는 붙어 있는 모든 보기에 보낸다. 늦게 붙은 보기는 받아 둔 값을 바로 받는다.
// - 보기가 모두 떨어지면 멈추고 받아 둔 값을 버린다(로그아웃 뒤 다음 사용자에게 남지 않게).
//   보기는 자기 상태에 마지막 값을 들고 있으므로 다시 붙을 때 빈 화면이 되지 않는다.
// - refresh는 백엔드 캐시를 건너뛰는 요청 하나를 보내고 결과를 모든 보기에 보낸다.

import { createPoller } from './poller'

// loading: 아직 응답이 없음. ok: 마지막 요청 성공. disconnected: 마지막 요청 실패(재시도 중).
export type ConnectionStatus = 'loading' | 'ok' | 'disconnected'

// PolledFetch는 데이터를 한 번 가져온다. refresh가 true면 백엔드 캐시를 건너뛴다(?refresh=1).
export type PolledFetch<T> = (signal: AbortSignal, refresh: boolean) => Promise<T>

export interface PollSnapshot<T> {
  // 연결이 끊겨도 마지막으로 받은 값을 유지한다.
  data: T | null
  status: ConnectionStatus
  // refresh 요청이 진행 중인지다.
  refreshing: boolean
}

export interface SharedPollOptions<T> {
  fetch: PolledFetch<T>
  // 주기 요청 하나의 제한 시간. refresh는 백엔드가 OMV를 다시 부르므로 이보다 길게(REFRESH_TIMEOUT_FACTOR배) 기다린다.
  timeoutMs: number
  // warmupMs가 있으면 폴러를 (다시) 시작한 뒤 첫 응답을 받고 나서 두 번째 요청만 이만큼 뒤에 보낸다.
  // 누적 카운터의 차이로 값을 구하는 보기(시스템 정보 창)가 첫 값을 주기만큼 기다리지 않게 한다.
  warmupMs?: number
}

export interface SharedPoll<T> {
  // subscribe는 intervalMs 주기로 붙고, 떨어지는 함수를 돌려준다. 받아 둔 값이 있으면 listener를 바로 부른다.
  subscribe: (intervalMs: number, listener: (s: PollSnapshot<T>) => void) => () => void
  // refresh는 붙어 있는 보기가 없으면 아무것도 하지 않는다.
  refresh: () => void
}

export const DEFAULT_TIMEOUT_MS = 10_000
export const REFRESH_TIMEOUT_FACTOR = 6

const EMPTY = { data: null, status: 'loading', refreshing: false } as const

export function createSharedPoll<T>(opts: SharedPollOptions<T>): SharedPoll<T> {
  interface Sub {
    intervalMs: number
    listener: (s: PollSnapshot<T>) => void
  }
  const subs = new Set<Sub>()
  let snap: PollSnapshot<T> = EMPTY
  let refreshCtrl: AbortController | null = null
  // 시작한 뒤 받은 주기 응답 수. warmupMs를 첫 응답 뒤에만 쓰는 데 쓴다.
  let received = 0

  const minInterval = () => Math.min(...Array.from(subs, (s) => s.intervalMs))
  const nextDelay = () => {
    const interval = minInterval()
    return opts.warmupMs !== undefined && received === 1 ? Math.min(opts.warmupMs, interval) : interval
  }
  const emit = (next: PollSnapshot<T>) => {
    snap = next
    for (const s of subs) s.listener(snap)
  }

  const poller = createPoller<T>({
    fetch: (signal) => opts.fetch(signal, false),
    intervalMs: nextDelay,
    timeoutMs: opts.timeoutMs,
    onData: (data) => {
      received++
      emit({ ...snap, data, status: 'ok' })
    },
    onError: () => {
      if (snap.status !== 'disconnected') emit({ ...snap, status: 'disconnected' })
    },
  })

  const reset = () => {
    poller.stop()
    refreshCtrl?.abort()
    refreshCtrl = null
    snap = EMPTY
    received = 0
  }

  return {
    subscribe(intervalMs, listener) {
      const before = subs.size > 0 ? minInterval() : Infinity
      const sub: Sub = { intervalMs, listener }
      subs.add(sub)
      if (snap.data !== null) listener(snap)
      if (subs.size === 1) poller.start()
      else if (intervalMs < before) poller.reschedule()
      return () => {
        if (!subs.delete(sub)) return
        if (subs.size === 0) reset()
      }
    },
    refresh() {
      if (refreshCtrl || subs.size === 0) return
      const ctrl = new AbortController()
      refreshCtrl = ctrl
      emit({ ...snap, refreshing: true })
      const timer = setTimeout(
        () => ctrl.abort(new Error('request timed out')),
        opts.timeoutMs * REFRESH_TIMEOUT_FACTOR,
      )
      const settle = (patch: Partial<PollSnapshot<T>>) => {
        clearTimeout(timer)
        // 그사이 모든 보기가 떨어졌으면(reset) 결과를 버린다.
        if (refreshCtrl !== ctrl) return
        refreshCtrl = null
        emit({ ...snap, ...patch, refreshing: false })
      }
      opts.fetch(ctrl.signal, true).then(
        (data) => settle({ data, status: 'ok' }),
        () => settle({ status: 'disconnected' }),
      )
    },
  }
}
