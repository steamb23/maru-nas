import { describe, expect, it, vi } from 'vitest'
import type { Job, Notice, NotificationsResponse } from '../api/types.gen'
import type { SeenMap } from './model'
import { createNotificationStore } from './store'

const notice = (id: string, revision = 'r1', level: Notice['level'] = 'warning'): Notice => ({
  id,
  code: 'sample',
  level,
  revision,
})

const resp = (notices: Notice[], seen: SeenMap = {}): NotificationsResponse => ({ notices, seen: { ...seen }, tasks: [] })

// 저장·지우기를 손으로 끝내는 가짜 API.
function setup() {
  const saves: { seen: SeenMap; resolve: () => void; reject: () => void }[] = []
  const saveSeen = vi.fn(
    (seen: SeenMap) =>
      new Promise<void>((resolve, reject) => {
        saves.push({ seen, resolve, reject: () => reject(new Error('save failed')) })
      }),
  )
  const dismisses: { id: string; resolve: () => void; reject: () => void }[] = []
  const dismiss = vi.fn(
    (id: string) =>
      new Promise<void>((resolve, reject) => {
        dismisses.push({ id, resolve, reject: () => reject(new Error('dismiss failed')) })
      }),
  )
  const store = createNotificationStore({ saveSeen, dismiss, now: () => 1000 })
  store.getState().enable()
  return { store, saves, saveSeen, dismisses, dismiss }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('서버 알림', () => {
  it('첫 응답은 토스트로 띄우지 않고, 리비전이 바뀌면 띄운다', () => {
    const { store } = setup()
    store.getState().received(resp([notice('a')]))
    expect(store.getState().toasts).toEqual([])
    store.getState().received(resp([notice('a', 'r2'), notice('b')]))
    expect(store.getState().toasts.map((t) => t.ref)).toEqual([
      { kind: 'server', id: 'a' },
      { kind: 'server', id: 'b' },
    ])
  })

  it('사라진 알림의 토스트는 내린다', () => {
    const { store } = setup()
    store.getState().received(resp([]))
    store.getState().received(resp([notice('a')]))
    expect(store.getState().toasts).toHaveLength(1)
    store.getState().received(resp([]))
    expect(store.getState().toasts).toEqual([])
  })

  it('markSeen은 지금 목록만 담아 저장한다', () => {
    const { store, saveSeen } = setup()
    store.getState().received(resp([notice('a'), notice('b')], { gone: 'x' }))
    store.getState().markSeen(['a'])
    expect(store.getState().seen).toEqual({ a: 'r1' })
    expect(saveSeen).toHaveBeenLastCalledWith({ a: 'r1' })
    // 이미 읽은 것은 다시 저장하지 않는다.
    store.getState().markSeen(['a'])
    expect(saveSeen).toHaveBeenCalledTimes(1)
  })

  it('저장 전에 받은 응답은 읽음 표시를 되돌리지 않는다', async () => {
    const { store, saves } = setup()
    store.getState().received(resp([notice('a')]))
    store.getState().markSeen()
    store.getState().received(resp([notice('a')], {}))
    expect(store.getState().seen).toEqual({ a: 'r1' })
    saves[0].resolve()
    await flush()
    store.getState().received(resp([notice('a')], { a: 'r1' }))
    expect(store.getState().seen).toEqual({ a: 'r1' })
  })

  it('저장이 실패하면 표시하고 이 탭의 읽음 상태는 유지한다', async () => {
    const { store, saves } = setup()
    store.getState().received(resp([notice('a')]))
    store.getState().markSeen()
    saves[0].reject()
    await flush()
    expect(store.getState().saveFailed).toBe(true)
    store.getState().received(resp([notice('a')]))
    expect(store.getState().seen).toEqual({ a: 'r1' })
  })

  it('다른 탭에서 읽은 것은 다음 응답에서 맞춰진다', () => {
    const { store } = setup()
    store.getState().received(resp([notice('a')]))
    store.getState().received(resp([notice('a')], { a: 'r1' }))
    expect(store.getState().seen).toEqual({ a: 'r1' })
  })

  it('토스트를 닫으면 그 알림을 읽고, 시간이 지나 내려가면 읽지 않은 채로 둔다', () => {
    const { store } = setup()
    store.getState().received(resp([]))
    store.getState().received(resp([notice('a'), notice('b')]))
    const [ta, tb] = store.getState().toasts
    store.getState().expireToast(ta.key)
    store.getState().dismissToast(tb.key)
    expect(store.getState().toasts).toEqual([])
    expect(store.getState().seen).toEqual({ b: 'r1' })
  })
})

describe('클라이언트 알림', () => {
  it('push는 알림을 더하고 토스트를 띄운다', () => {
    const { store } = setup()
    store.getState().push({ code: 'filesDone', level: 'success', params: { count: 2 } })
    const { events, toasts } = store.getState()
    expect(events).toEqual([{ code: 'filesDone', level: 'success', params: { count: 2 }, key: 1, at: 1000, read: false }])
    expect(toasts.map((t) => t.ref)).toEqual([{ kind: 'client', key: 1 }])
  })

  it('로그인하지 않았으면 버린다', () => {
    const { store } = setup()
    store.getState().clear()
    store.getState().push({ code: 'filesDone', level: 'success' })
    expect(store.getState().events).toEqual([])
  })

  it('지우면 그 토스트도 내린다', () => {
    const { store } = setup()
    store.getState().push({ code: 'filesDone', level: 'success' })
    store.getState().push({ code: 'filesCopyFailed', level: 'error' })
    store.getState().dismissEvent(1)
    expect(store.getState().events.map((e) => e.key)).toEqual([2])
    expect(store.getState().toasts.map((t) => t.ref)).toEqual([{ kind: 'client', key: 2 }])
    store.getState().clearEvents()
    expect(store.getState().events).toEqual([])
    expect(store.getState().toasts).toEqual([])
  })

  it('토스트를 닫으면 읽음이 된다', () => {
    const { store } = setup()
    store.getState().push({ code: 'filesDone', level: 'success' })
    store.getState().dismissToast(store.getState().toasts[0].key)
    expect(store.getState().events[0].read).toBe(true)
  })
})

describe('알림 센터', () => {
  it('열면 모두 읽고 토스트를 내리며, 무엇이 새로 왔는지는 열린 동안 남긴다', () => {
    const { store, saveSeen } = setup()
    store.getState().received(resp([]))
    store.getState().received(resp([notice('a'), notice('b')], { b: 'r1' }))
    store.getState().push({ code: 'filesDone', level: 'success' })
    store.getState().setCenterOpen(true)
    const s = store.getState()
    expect(s.toasts).toEqual([])
    expect(s.events[0].read).toBe(true)
    expect(saveSeen).toHaveBeenLastCalledWith({ a: 'r1', b: 'r1' })
    expect([...s.fresh].sort()).toEqual(['c:1', 's:a'])
    store.getState().setCenterOpen(false)
    expect(store.getState().fresh.size).toBe(0)
  })

  it('열린 동안 들어온 알림은 토스트 없이 읽음이 된다', () => {
    const { store } = setup()
    store.getState().received(resp([]))
    store.getState().setCenterOpen(true)
    store.getState().received(resp([notice('a')]))
    store.getState().push({ code: 'filesDone', level: 'success' })
    const s = store.getState()
    expect(s.toasts).toEqual([])
    expect(s.seen).toEqual({ a: 'r1' })
    expect(s.events[0].read).toBe(true)
    expect([...s.fresh].sort()).toEqual(['c:1', 's:a'])
  })
})

describe('clear', () => {
  it('모두 비우고 늦게 끝난 저장의 결과를 버린다', async () => {
    const { store, saves } = setup()
    store.getState().received(resp([notice('a')]))
    store.getState().markSeen()
    store.getState().clear()
    saves[0].reject()
    await flush()
    const s = store.getState()
    expect(s.enabled).toBe(false)
    expect(s.saveFailed).toBe(false)
    expect(s.notices).toEqual([])
    // 다시 로그인하면 첫 응답은 토스트로 띄우지 않는다.
    s.enable()
    store.getState().received(resp([notice('a', 'r2')]))
    expect(store.getState().toasts).toEqual([])
  })

  it('로그아웃한 뒤 도착한 응답은 버린다', () => {
    const { store } = setup()
    store.getState().clear()
    store.getState().received(resp([notice('a')]))
    expect(store.getState().notices).toEqual([])
  })
})

describe('작업 결과 알림', () => {
  const result = (id: string): Notice => ({ ...notice(id, 'r1', 'success'), code: 'jobSucceeded', dismissible: true })

  it('첫 응답에서도 읽지 않은 결과는 토스트로 띄운다', () => {
    const { store } = setup()
    store.getState().received(resp([notice('a'), result('job-1')]))
    expect(store.getState().toasts.map((t) => t.ref)).toEqual([{ kind: 'server', id: 'job-1' }])
  })

  it('지우면 바로 숨기고, 끝나기 전에 받은 응답이 되살리지 않는다', async () => {
    const { store, dismisses, dismiss } = setup()
    store.getState().received(resp([result('job-1')]))
    store.getState().dismissNotice('job-1')
    expect(dismiss).toHaveBeenCalledWith('job-1')
    expect(store.getState().notices).toEqual([])
    expect(store.getState().toasts).toEqual([])
    store.getState().received(resp([result('job-1')]))
    expect(store.getState().notices).toEqual([])
    dismisses[0].resolve()
    await flush()
    store.getState().received(resp([]))
    expect(store.getState().notices).toEqual([])
  })

  it('지우기가 실패하면 되돌린다', async () => {
    const { store, dismisses } = setup()
    store.getState().received(resp([result('job-1')]))
    store.getState().dismissNotice('job-1')
    dismisses[0].reject()
    await flush()
    expect(store.getState().notices.map((n) => n.id)).toEqual(['job-1'])
  })

  it('지울 수 없는 알림은 지우지 않는다', () => {
    const { store, dismiss } = setup()
    store.getState().received(resp([notice('a')]))
    store.getState().dismissNotice('a')
    expect(dismiss).not.toHaveBeenCalled()
    expect(store.getState().notices).toHaveLength(1)
  })

  it('진행 중인 작업(tasks)을 둔다', () => {
    const { store } = setup()
    const task: Job = {
      id: '20261024-101500-1a2b3c4d',
      kind: 'apt-upgrade',
      state: 'running',
      owner: 'admin',
      mine: true,
      startedAt: '2026-10-24T10:15:00Z',
      outputEnd: 0,
    }
    store.getState().received({ ...resp([]), tasks: [task] })
    expect(store.getState().tasks).toEqual([task])
    store.getState().clear()
    expect(store.getState().tasks).toEqual([])
  })
})
