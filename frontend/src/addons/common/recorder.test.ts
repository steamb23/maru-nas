import { describe, expect, it, vi } from 'vitest'
import { createRecorder } from './recorder'
import type { PollSnapshot, SharedPoll } from './sharedPoll'

// fakePoll은 붙은 보기를 기억하고, 테스트가 스냅숏을 보낸다.
function fakePoll() {
  const subs = new Map<number, { intervalMs: number; listener: (s: PollSnapshot<number>) => void }>()
  let nextId = 1
  const poll: SharedPoll<number> = {
    subscribe: (intervalMs, listener) => {
      const id = nextId++
      subs.set(id, { intervalMs, listener })
      return () => subs.delete(id)
    },
    refresh: vi.fn(),
  }
  const emit = (s: PollSnapshot<number>) => [...subs.values()].forEach((sub) => sub.listener(s))
  return { poll, subs, emit }
}

const snap = (data: number | null, status: PollSnapshot<number>['status'] = 'ok'): PollSnapshot<number> => ({
  data,
  status,
  refreshing: false,
})

describe('createRecorder', () => {
  it('retain한 동안 응답을 reduce로 쌓는다(보는 쪽이 없어도)', () => {
    const p = fakePoll()
    const rec = createRecorder<number, number[]>(p.poll, (prev, v) => [...(prev ?? []), v])
    const release = rec.retain(2000)
    expect([...p.subs.values()][0].intervalMs).toBe(2000)
    p.emit(snap(1))
    p.emit(snap(2))
    expect(rec.get()).toEqual({ data: [1, 2], status: 'ok' })
    release()
  })

  it('같은 응답은 두 번 접지 않고, 상태만 바뀌면 값은 그대로 둔다', () => {
    const p = fakePoll()
    const reduce = vi.fn((prev: number[] | null, v: number) => [...(prev ?? []), v])
    const rec = createRecorder(p.poll, reduce)
    const release = rec.retain(1000)
    p.emit(snap(1))
    const before = rec.get()
    p.emit(snap(1))
    expect(rec.get()).toBe(before)
    p.emit(snap(1, 'disconnected'))
    expect(rec.get()).toEqual({ data: [1], status: 'disconnected' })
    expect(reduce).toHaveBeenCalledOnce()
    release()
  })

  it('바뀔 때만 구독자에게 알린다', () => {
    const p = fakePoll()
    const rec = createRecorder<number, number>(p.poll, (_prev, v) => v)
    const release = rec.retain(1000)
    const listener = vi.fn()
    const off = rec.subscribe(listener)
    p.emit(snap(5))
    p.emit(snap(5))
    expect(listener).toHaveBeenCalledOnce()
    off()
    p.emit(snap(6))
    expect(listener).toHaveBeenCalledOnce()
    expect(rec.get().data).toBe(6)
    release()
  })

  it('여러 번 retain해도 폴러에는 한 번 붙고, 모두 그만두면 떨어지고 값을 버린다', () => {
    const p = fakePoll()
    const rec = createRecorder<number, number>(p.poll, (_prev, v) => v)
    const a = rec.retain(1000)
    const b = rec.retain(1000)
    expect(p.subs.size).toBe(1)
    p.emit(snap(3))
    a()
    a()
    expect(p.subs.size).toBe(1)
    expect(rec.get().data).toBe(3)
    b()
    expect(p.subs.size).toBe(0)
    expect(rec.get()).toEqual({ data: null, status: 'loading' })
    // 다시 시작하면 같은 응답도 새로 접는다.
    const c = rec.retain(1000)
    p.emit(snap(3))
    expect(rec.get().data).toBe(3)
    c()
  })
})
