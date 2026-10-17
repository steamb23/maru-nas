import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPoller } from './poller'

const INTERVAL = 2000
const TIMEOUT = 5000

// FakeFetch는 호출마다 대기 중인 요청을 만들고, 테스트가 직접 응답하거나 실패시킨다.
// respectAbort가 true면 abort 시 요청이 바로 실패한다(실제 fetch와 같다).
function fakeFetch(respectAbort = true) {
  const pending: { resolve: (v: number) => void; reject: (e: unknown) => void; signal: AbortSignal }[] = []
  const fn = vi.fn(
    (signal: AbortSignal) =>
      new Promise<number>((resolve, reject) => {
        pending.push({ resolve, reject, signal })
        if (respectAbort) signal.addEventListener('abort', () => reject(signal.reason))
      }),
  )
  const last = () => pending[pending.length - 1]
  return { fn, pending, last }
}

function setup(respectAbort = true) {
  const f = fakeFetch(respectAbort)
  const onData = vi.fn()
  const onError = vi.fn()
  const poller = createPoller({ fetch: f.fn, intervalMs: INTERVAL, timeoutMs: TIMEOUT, onData, onError })
  return { ...f, onData, onError, poller }
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('createPoller', () => {
  it('시작하면 바로 요청하고, 응답 후 2초마다 다시 요청한다', async () => {
    const t = setup()
    t.poller.start()
    expect(t.fn).toHaveBeenCalledTimes(1)

    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    expect(t.onData).toHaveBeenLastCalledWith(1)

    await vi.advanceTimersByTimeAsync(INTERVAL - 1)
    expect(t.fn).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(t.fn).toHaveBeenCalledTimes(2)
    t.poller.stop()
  })

  it('응답이 늦어도 요청이 겹치지 않고, 타임아웃이면 오류로 처리한다', async () => {
    const t = setup()
    t.poller.start()
    await vi.advanceTimersByTimeAsync(TIMEOUT - 1)
    expect(t.fn).toHaveBeenCalledTimes(1)
    expect(t.onError).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(1)
    expect(t.last().signal.aborted).toBe(true)
    expect(t.onError).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect(t.fn).toHaveBeenCalledTimes(2)
    t.poller.stop()
  })

  it('오류가 나도 계속 재시도하고, 성공하면 다시 데이터를 보낸다', async () => {
    const t = setup()
    t.poller.start()
    t.last().reject(new TypeError('Failed to fetch'))
    await vi.advanceTimersByTimeAsync(0)
    expect(t.onError).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(INTERVAL)
    t.last().reject(new Error('HTTP 502'))
    await vi.advanceTimersByTimeAsync(0)
    expect(t.onError).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(INTERVAL)
    t.last().resolve(7)
    await vi.advanceTimersByTimeAsync(0)
    expect(t.onData).toHaveBeenCalledWith(7)
    expect(t.fn).toHaveBeenCalledTimes(3)
    t.poller.stop()
  })

  it('stop하면 대기 중인 예약이 취소되어 더 이상 요청하지 않는다', async () => {
    const t = setup()
    t.poller.start()
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)

    t.poller.stop()
    t.poller.stop() // 여러 번 불러도 된다
    await vi.advanceTimersByTimeAsync(INTERVAL * 10)
    expect(t.fn).toHaveBeenCalledTimes(1)
  })

  it('stop하면 진행 중인 요청을 abort하고 그 결과는 보내지 않는다', async () => {
    const t = setup()
    t.poller.start()
    t.poller.stop()
    expect(t.last().signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(INTERVAL * 10)
    expect(t.onError).not.toHaveBeenCalled()
    expect(t.fn).toHaveBeenCalledTimes(1)
  })

  it('abort를 무시한 요청이 stop 뒤에 응답해도 버린다', async () => {
    const t = setup(false)
    t.poller.start()
    t.poller.stop()
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(INTERVAL * 10)
    expect(t.onData).not.toHaveBeenCalled()
    expect(t.fn).toHaveBeenCalledTimes(1)
  })

  it('stop 후 바로 다시 start해도 요청 주기가 하나만 돈다', async () => {
    const t = setup(false)
    t.poller.start()
    const first = t.last()
    t.poller.stop()
    t.poller.start()
    expect(t.fn).toHaveBeenCalledTimes(2)

    // 이전 실행의 늦은 응답은 예약을 만들지 않는다.
    first.resolve(1)
    t.last().resolve(2)
    await vi.advanceTimersByTimeAsync(0)
    expect(t.onData).toHaveBeenCalledTimes(1)
    expect(t.onData).toHaveBeenCalledWith(2)

    await vi.advanceTimersByTimeAsync(INTERVAL)
    expect(t.fn).toHaveBeenCalledTimes(3)
    t.poller.stop()
  })
  it('intervalMs가 함수면 예약할 때마다 다시 읽는다', async () => {
    const f = fakeFetch()
    let interval = 5000
    const poller = createPoller({ fetch: f.fn, intervalMs: () => interval, timeoutMs: TIMEOUT, onData: vi.fn(), onError: vi.fn() })
    poller.start()
    f.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    interval = 2000
    // 이미 잡힌 예약은 그대로다.
    await vi.advanceTimersByTimeAsync(4999)
    expect(f.fn).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(f.fn).toHaveBeenCalledTimes(2)
    f.last().resolve(2)
    await vi.advanceTimersByTimeAsync(2000)
    expect(f.fn).toHaveBeenCalledTimes(3)
    poller.stop()
  })

  it('reschedule은 마지막 응답 시각 기준으로 지금 주기에 맞춰 다시 예약한다', async () => {
    const f = fakeFetch()
    let interval = 5000
    const poller = createPoller({ fetch: f.fn, intervalMs: () => interval, timeoutMs: TIMEOUT, onData: vi.fn(), onError: vi.fn() })
    poller.start()
    f.last().resolve(1)
    await vi.advanceTimersByTimeAsync(1000)
    interval = 2000
    poller.reschedule()
    await vi.advanceTimersByTimeAsync(999)
    expect(f.fn).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(f.fn).toHaveBeenCalledTimes(2)

    // 이미 주기가 지났으면 바로 요청한다.
    f.last().resolve(2)
    interval = 5000
    await vi.advanceTimersByTimeAsync(3000)
    interval = 2000
    poller.reschedule()
    await vi.advanceTimersByTimeAsync(0)
    expect(f.fn).toHaveBeenCalledTimes(3)

    // 요청 중이면 아무것도 하지 않는다.
    poller.reschedule()
    expect(f.fn).toHaveBeenCalledTimes(3)
    poller.stop()
  })

  it('schedule을 주면 다음 요청 예약과 요청 제한 시간에 그 타이머를 쓴다', async () => {
    // 테스트용 타이머: 시간을 직접 넘기는 대신 걸린 예약을 꺼내 부른다.
    const pending: { fn: () => void; ms: number; cancelled: boolean }[] = []
    const schedule = vi.fn((fn: () => void, ms: number) => {
      const entry = { fn, ms, cancelled: false }
      pending.push(entry)
      return () => {
        entry.cancelled = true
      }
    })
    const f = fakeFetch()
    const onError = vi.fn()
    const poller = createPoller({ fetch: f.fn, intervalMs: INTERVAL, timeoutMs: TIMEOUT, onData: vi.fn(), onError, schedule })
    poller.start()
    // 첫 요청의 제한 시간
    expect(pending.map((p) => p.ms)).toEqual([TIMEOUT])
    f.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    // 응답하면 제한 시간을 거두고 다음 요청을 예약한다.
    expect(pending[0].cancelled).toBe(true)
    expect(pending[1].ms).toBe(INTERVAL)
    pending[1].fn()
    expect(f.fn).toHaveBeenCalledTimes(2)
    // 제한 시간이 되면 abort하고 오류로 처리한다.
    pending[2].fn()
    await vi.advanceTimersByTimeAsync(0)
    expect(onError).toHaveBeenCalledOnce()
    // stop은 대기 중인 예약을 거둔다.
    poller.stop()
    expect(pending[pending.length - 1].cancelled).toBe(true)
  })
})
