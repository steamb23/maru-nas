// 주기적으로 데이터를 가져오는 폴러. React와 무관하게 동작해 node 환경에서 테스트한다.
//
// - 응답이 끝난 뒤에 다음 요청을 예약하므로 요청이 겹치지 않는다.
// - 요청마다 timeoutMs가 지나면 abort하고 오류로 처리한다.
// - stop()은 대기 중인 예약을 지우고 진행 중인 요청을 abort한다. 여러 번 불러도 된다.
//   stop() 뒤에 도착한 응답은 버린다.
// - intervalMs는 함수로 줄 수 있다. 다음 요청을 예약할 때마다 다시 읽는다(sharedPoll.ts).

export interface PollerOptions<T> {
  fetch: (signal: AbortSignal) => Promise<T>
  intervalMs: number | (() => number)
  timeoutMs: number
  onData: (data: T) => void
  // 네트워크 오류, HTTP 오류, 응답 형식 오류, 타임아웃을 모두 여기로 보낸다.
  onError: (err: unknown) => void
}

export interface Poller {
  start: () => void
  stop: () => void
  // reschedule은 대기 중인 예약을 지금의 intervalMs로 다시 잡는다(마지막 응답 시각 기준).
  // 요청이 진행 중이거나 멈춰 있으면 아무것도 하지 않는다. 주기가 짧아졌을 때 부른다.
  reschedule: () => void
}

export function createPoller<T>(opts: PollerOptions<T>): Poller {
  // start/stop마다 바뀐다. 이전 실행에서 늦게 끝난 요청을 구분하는 데 쓴다.
  let generation = 0
  let running = false
  let timer: ReturnType<typeof setTimeout> | undefined
  let inflight: AbortController | undefined
  // 마지막 응답(성공·실패)을 처리한 시각(Date.now)
  let doneAt = 0
  const interval = () => (typeof opts.intervalMs === 'function' ? opts.intervalMs() : opts.intervalMs)
  const schedule = (gen: number, delay: number) => {
    timer = setTimeout(() => void tick(gen), Math.max(0, delay))
  }

  const tick = async (gen: number) => {
    timer = undefined
    const ctrl = new AbortController()
    inflight = ctrl
    const timeout = setTimeout(() => ctrl.abort(new Error('request timed out')), opts.timeoutMs)
    let result: { ok: true; data: T } | { ok: false; err: unknown }
    try {
      result = { ok: true, data: await opts.fetch(ctrl.signal) }
    } catch (err) {
      result = { ok: false, err }
    } finally {
      clearTimeout(timeout)
    }
    if (gen !== generation) return
    inflight = undefined
    if (result.ok) opts.onData(result.data)
    else opts.onError(result.err)
    // 콜백 안에서 stop()을 불렀을 수 있다.
    if (gen === generation) {
      doneAt = Date.now()
      schedule(gen, interval())
    }
  }

  return {
    start() {
      if (running) return
      running = true
      void tick(++generation)
    },
    stop() {
      if (!running) return
      running = false
      generation++
      clearTimeout(timer)
      timer = undefined
      inflight?.abort()
      inflight = undefined
    },
    reschedule() {
      if (timer === undefined) return
      clearTimeout(timer)
      schedule(generation, doneAt + interval() - Date.now())
    },
  }
}
