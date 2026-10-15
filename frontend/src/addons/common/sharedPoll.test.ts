import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { type PollSnapshot, REFRESH_TIMEOUT_FACTOR, createSharedPoll } from './sharedPoll'

const TIMEOUT = 5000

// fakeFetch는 호출마다 대기 중인 요청을 만들고, 테스트가 직접 응답하거나 실패시킨다.
function fakeFetch() {
  const pending: {
    resolve: (v: number) => void
    reject: (e: unknown) => void
    signal: AbortSignal
    refresh: boolean
  }[] = []
  const fn = vi.fn(
    (signal: AbortSignal, refresh: boolean) =>
      new Promise<number>((resolve, reject) => {
        pending.push({ resolve, reject, signal, refresh })
        signal.addEventListener('abort', () => reject(signal.reason))
      }),
  )
  const last = () => pending[pending.length - 1]
  return { fn, pending, last }
}

function setup() {
  const f = fakeFetch()
  const poll = createSharedPoll({ fetch: f.fn, timeoutMs: TIMEOUT })
  return { ...f, poll }
}

function listener() {
  const seen: PollSnapshot<number>[] = []
  const fn = (s: PollSnapshot<number>) => seen.push(s)
  return { fn, seen, latest: () => seen[seen.length - 1] }
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('createSharedPoll', () => {
  it('보기 둘이 붙어도 요청은 하나이고 결과는 둘 다 받는다', async () => {
    const t = setup()
    const a = listener()
    const b = listener()
    const offA = t.poll.subscribe(5000, a.fn)
    const offB = t.poll.subscribe(5000, b.fn)
    expect(t.fn).toHaveBeenCalledTimes(1)

    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    expect(a.latest()).toEqual({ data: 1, status: 'ok', refreshing: false })
    expect(b.latest()).toEqual({ data: 1, status: 'ok', refreshing: false })
    offA()
    offB()
  })

  it('주기는 붙어 있는 보기 중 가장 짧은 값이고, 짧은 보기가 떨어지면 긴 주기로 돌아간다', async () => {
    const t = setup()
    const offWidget = t.poll.subscribe(5000, vi.fn())
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(1000)

    // 창이 2초 주기로 붙으면 마지막 응답 시각 기준으로 다시 예약한다.
    const offWindow = t.poll.subscribe(2000, vi.fn())
    expect(t.fn).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(t.fn).toHaveBeenCalledTimes(2)
    t.last().resolve(2)
    await vi.advanceTimersByTimeAsync(2000)
    expect(t.fn).toHaveBeenCalledTimes(3)
    t.last().resolve(3)
    await vi.advanceTimersByTimeAsync(0)

    // 창이 떨어지면 이미 잡힌 예약 뒤부터 5초 주기다.
    offWindow()
    await vi.advanceTimersByTimeAsync(2000)
    expect(t.fn).toHaveBeenCalledTimes(4)
    t.last().resolve(4)
    await vi.advanceTimersByTimeAsync(4999)
    expect(t.fn).toHaveBeenCalledTimes(4)
    await vi.advanceTimersByTimeAsync(1)
    expect(t.fn).toHaveBeenCalledTimes(5)
    offWidget()
  })

  it('긴 주기의 보기가 붙으면 예약을 바꾸지 않는다', async () => {
    const t = setup()
    const offWindow = t.poll.subscribe(2000, vi.fn())
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    const offWidget = t.poll.subscribe(5000, vi.fn())
    await vi.advanceTimersByTimeAsync(2000)
    expect(t.fn).toHaveBeenCalledTimes(2)
    offWindow()
    offWidget()
  })

  it('늦게 붙은 보기는 받아 둔 값을 바로 받는다', async () => {
    const t = setup()
    const off = t.poll.subscribe(5000, vi.fn())
    t.last().resolve(7)
    await vi.advanceTimersByTimeAsync(0)

    const late = listener()
    const offLate = t.poll.subscribe(5000, late.fn)
    expect(late.seen).toEqual([{ data: 7, status: 'ok', refreshing: false }])
    expect(t.fn).toHaveBeenCalledTimes(1)
    off()
    offLate()
  })

  it('실패하면 disconnected를 알리고 마지막 값은 유지한다', async () => {
    const t = setup()
    const a = listener()
    const off = t.poll.subscribe(2000, a.fn)
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(2000)
    t.last().reject(new Error('HTTP 502'))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.latest()).toEqual({ data: 1, status: 'disconnected', refreshing: false })

    // 계속 실패해도 다시 알리지 않는다.
    const count = a.seen.length
    await vi.advanceTimersByTimeAsync(2000)
    t.last().reject(new Error('HTTP 502'))
    await vi.advanceTimersByTimeAsync(0)
    expect(a.seen).toHaveLength(count)
    off()
  })

  it('모두 떨어지면 멈추고 받아 둔 값을 버린다', async () => {
    const t = setup()
    const off = t.poll.subscribe(2000, vi.fn())
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    off()
    off() // 여러 번 불러도 된다
    await vi.advanceTimersByTimeAsync(20_000)
    expect(t.fn).toHaveBeenCalledTimes(1)

    // 다시 붙으면 바로 요청하고, 이전 값은 주지 않는다.
    const again = listener()
    const offAgain = t.poll.subscribe(2000, again.fn)
    expect(t.fn).toHaveBeenCalledTimes(2)
    expect(again.seen).toEqual([])
    offAgain()
  })

  it('모두 떨어지면 진행 중인 요청을 abort한다', () => {
    const t = setup()
    const off = t.poll.subscribe(2000, vi.fn())
    off()
    expect(t.last().signal.aborted).toBe(true)
  })

  it('refresh는 캐시를 건너뛰는 요청을 보내고 결과를 모든 보기에 보낸다', async () => {
    const t = setup()
    const a = listener()
    const b = listener()
    const offA = t.poll.subscribe(2000, a.fn)
    const offB = t.poll.subscribe(30_000, b.fn)
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)

    t.poll.refresh()
    t.poll.refresh() // 진행 중이면 무시한다
    expect(t.fn).toHaveBeenCalledTimes(2)
    expect(t.last().refresh).toBe(true)
    expect(a.latest().refreshing).toBe(true)
    expect(b.latest().refreshing).toBe(true)

    t.last().resolve(9)
    await vi.advanceTimersByTimeAsync(0)
    expect(a.latest()).toEqual({ data: 9, status: 'ok', refreshing: false })
    expect(b.latest()).toEqual({ data: 9, status: 'ok', refreshing: false })
    offA()
    offB()
  })

  it('refresh는 주기 요청보다 길게 기다린 뒤 실패로 처리한다', async () => {
    const t = setup()
    const a = listener()
    const off = t.poll.subscribe(60_000, a.fn)
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    t.poll.refresh()
    const req = t.last()
    await vi.advanceTimersByTimeAsync(TIMEOUT * REFRESH_TIMEOUT_FACTOR - 1)
    expect(req.signal.aborted).toBe(false)
    await vi.advanceTimersByTimeAsync(1)
    expect(req.signal.aborted).toBe(true)
    expect(a.latest()).toEqual({ data: 1, status: 'disconnected', refreshing: false })
    off()
  })

  it('붙어 있는 보기가 없으면 refresh는 아무것도 하지 않는다', () => {
    const t = setup()
    t.poll.refresh()
    expect(t.fn).not.toHaveBeenCalled()
  })

  it('refresh 중에 모두 떨어지면 refresh를 abort하고 결과를 버린다', async () => {
    const t = setup()
    const a = listener()
    const off = t.poll.subscribe(2000, a.fn)
    t.last().resolve(1)
    await vi.advanceTimersByTimeAsync(0)
    t.poll.refresh()
    const req = t.last()
    const count = a.seen.length
    off()
    expect(req.signal.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(0)
    expect(a.seen).toHaveLength(count)
  })
})
