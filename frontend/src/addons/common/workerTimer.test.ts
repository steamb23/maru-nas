import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { type WorkerLike, createWorkerScheduler } from './workerTimer'

// fakeWorker는 받은 메시지를 모으고, 테스트가 예약이 끝났다고(fire) 또는 실패했다고(fail)알린다.
function fakeWorker() {
  const posted: { id: number; ms?: number }[] = []
  const w: WorkerLike = {
    postMessage: (m) => posted.push(m),
    terminate: vi.fn(),
    onmessage: null,
    onerror: null,
  }
  const fire = (id: number) => w.onmessage?.({ data: id } as MessageEvent<number>)
  const fail = () => w.onerror?.(new Event('error') as ErrorEvent)
  return { w, posted, fire, fail }
}

beforeEach(() => {
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('createWorkerScheduler', () => {
  it('Worker에 예약을 맡기고 돌아온 id의 함수를 한 번 부른다', () => {
    const f = fakeWorker()
    const make = vi.fn(() => f.w)
    const schedule = createWorkerScheduler(make)
    const a = vi.fn()
    const b = vi.fn()
    schedule(a, 1000)
    schedule(b, 2000)
    expect(make).toHaveBeenCalledOnce()
    expect(f.posted).toEqual([
      { id: 1, ms: 1000 },
      { id: 2, ms: 2000 },
    ])
    f.fire(2)
    f.fire(2)
    expect(b).toHaveBeenCalledOnce()
    expect(a).not.toHaveBeenCalled()
  })

  it('취소하면 Worker의 예약도 거두고 늦게 온 메시지는 버린다', () => {
    const f = fakeWorker()
    const schedule = createWorkerScheduler(() => f.w)
    const fn = vi.fn()
    const cancel = schedule(fn, 1000)
    cancel()
    expect(f.posted[1]).toEqual({ id: 1 })
    f.fire(1)
    expect(fn).not.toHaveBeenCalled()
  })

  it('Worker를 만들 수 없으면 setTimeout을 쓴다', () => {
    for (const make of [() => null, () => { throw new Error('no worker') }]) {
      const schedule = createWorkerScheduler(make)
      const fn = vi.fn()
      schedule(fn, 500)
      vi.advanceTimersByTime(499)
      expect(fn).not.toHaveBeenCalled()
      vi.advanceTimersByTime(1)
      expect(fn).toHaveBeenCalledOnce()
    }
  })

  it('Worker가 실패하면 기다리던 예약을 남은 시간만큼 setTimeout으로 옮긴다', () => {
    const f = fakeWorker()
    const make = vi.fn(() => f.w)
    const schedule = createWorkerScheduler(make)
    const kept = vi.fn()
    const dropped = vi.fn()
    schedule(kept, 1000)
    const cancel = schedule(dropped, 1000)
    vi.advanceTimersByTime(400)
    f.fail()
    expect(f.w.terminate).toHaveBeenCalled()
    cancel()
    vi.advanceTimersByTime(599)
    expect(kept).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(kept).toHaveBeenCalledOnce()
    expect(dropped).not.toHaveBeenCalled()

    // 그 뒤의 예약은 Worker를 다시 만들지 않고 setTimeout으로 한다.
    const later = vi.fn()
    schedule(later, 100)
    vi.advanceTimersByTime(100)
    expect(later).toHaveBeenCalledOnce()
    expect(make).toHaveBeenCalledOnce()
  })
})
