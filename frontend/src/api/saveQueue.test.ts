import { describe, expect, it, vi } from 'vitest'
import { createSaveQueue } from './saveQueue'

// deferred는 밖에서 끝낼 수 있는 Promise다.
function deferred<T = void>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const flushPromises = () => new Promise((r) => setTimeout(r, 0))

describe('createSaveQueue', () => {
  it('저장하고 결과를 알린다', async () => {
    const save = vi.fn(async (_v: string) => {})
    const onResult = vi.fn()
    createSaveQueue(save, onResult).push('a')
    await flushPromises()
    expect(save).toHaveBeenCalledExactlyOnceWith('a')
    expect(onResult).toHaveBeenCalledExactlyOnceWith(true)
  })

  it('하나씩 보내고, 보내는 동안 들어온 값은 마지막 것만 보낸다', async () => {
    const first = deferred()
    const save = vi.fn((_v: string) => (save.mock.calls.length === 1 ? first.promise : Promise.resolve()))
    const q = createSaveQueue(save, () => {})
    q.push('a')
    q.push('b')
    q.push('c')
    expect(save).toHaveBeenCalledTimes(1)
    first.resolve()
    await flushPromises()
    expect(save.mock.calls.map(([v]) => v)).toEqual(['a', 'c'])
  })

  it('실패를 알리고 다음 값은 그대로 보낸다', async () => {
    const save = vi.fn(async (_v: string) => {
      if (save.mock.calls.length === 1) throw new Error('down')
    })
    const onResult = vi.fn()
    const q = createSaveQueue(save, onResult)
    q.push('a')
    await flushPromises()
    q.push('b')
    await flushPromises()
    expect(onResult.mock.calls.map(([ok]) => ok)).toEqual([false, true])
  })

  it('거짓값도 보낸다', async () => {
    const save = vi.fn(async (_v: number) => {})
    createSaveQueue(save, () => {}).push(0)
    await flushPromises()
    expect(save).toHaveBeenCalledExactlyOnceWith(0)
  })

  it('reset은 기다리던 값을 버리고 보내는 중인 요청의 결과를 알리지 않는다', async () => {
    const first = deferred()
    const save = vi.fn((_v: string) => first.promise)
    const onResult = vi.fn()
    const q = createSaveQueue(save, onResult)
    q.push('a')
    q.push('b')
    q.reset()
    first.resolve()
    await flushPromises()
    expect(save).toHaveBeenCalledTimes(1)
    expect(onResult).not.toHaveBeenCalled()
  })

  it('reset 뒤에 들어온 값은 앞 요청이 끝난 뒤 보낸다', async () => {
    const first = deferred()
    const save = vi.fn((_v: string) => (save.mock.calls.length === 1 ? first.promise : Promise.resolve()))
    const onResult = vi.fn()
    const q = createSaveQueue(save, onResult)
    q.push('a')
    q.reset()
    q.push('b')
    expect(save).toHaveBeenCalledTimes(1)
    first.resolve()
    await flushPromises()
    expect(save.mock.calls.map(([v]) => v)).toEqual(['a', 'b'])
    expect(onResult).toHaveBeenCalledExactlyOnceWith(true)
  })
})
