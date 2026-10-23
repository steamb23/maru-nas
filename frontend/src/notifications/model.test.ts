import { describe, expect, it } from 'vitest'
import type { Notice } from '../api/types.gen'
import {
  type ClientEvent,
  type Toast,
  actionAvailable,
  badgeText,
  capEvents,
  isUnread,
  itemKey,
  mergeSeen,
  noticesToToast,
  orderNotices,
  pushToast,
  revisionsOf,
  seenAfterMark,
  settleOverrides,
  unreadCount,
} from './model'

const notice = (id: string, revision = 'r1', extra: Partial<Notice> = {}): Notice => ({
  id,
  code: 'sample',
  level: 'info',
  revision,
  ...extra,
})

const event = (key: number, read = false): ClientEvent => ({ key, code: 'filesDone', level: 'success', at: key, read })

describe('읽음 상태', () => {
  it('리비전이 같아야 읽은 것이다', () => {
    expect(isUnread(notice('a'), {})).toBe(true)
    expect(isUnread(notice('a'), { a: 'r1' })).toBe(false)
    expect(isUnread(notice('a', 'r2'), { a: 'r1' })).toBe(true)
  })

  it('읽지 않은 수는 서버 알림과 클라이언트 알림을 함께 센다', () => {
    expect(unreadCount([notice('a'), notice('b')], { a: 'r1' }, [event(1), event(2, true)])).toBe(2)
  })

  it('seenAfterMark는 고른 것만 읽고 사라진 id는 뺀다', () => {
    const notices = [notice('a', 'r2'), notice('b'), notice('c')]
    expect(seenAfterMark(notices, { a: 'r1', gone: 'r9' }, ['b'])).toEqual({ a: 'r1', b: 'r1' })
    expect(seenAfterMark(notices, { gone: 'r9' })).toEqual({ a: 'r2', b: 'r1', c: 'r1' })
  })

  it('저장 전에 받은 응답은 이 탭의 읽음 표시를 되돌리지 않는다', () => {
    const notices = [notice('a', 'r2'), notice('b')]
    const overrides = { a: 'r2' }
    expect(mergeSeen({ a: 'r1', b: 'r1' }, overrides)).toEqual({ a: 'r2', b: 'r1' })
    // 서버가 같은 값을 주면 overrides에서 빠진다. 목록에서 사라진 것도 뺀다.
    expect(settleOverrides({ a: 'r2', b: 'r1', gone: 'x' }, { a: 'r2' }, notices)).toEqual({ b: 'r1' })
  })
})

describe('noticesToToast', () => {
  const seen = { b: 'r1' }

  it('이 탭의 첫 목록은 띄우지 않는다', () => {
    expect(noticesToToast(null, [notice('a')], {})).toEqual([])
  })

  it('새로 생기거나 리비전이 바뀐 읽지 않은 알림만 띄운다', () => {
    const prev = revisionsOf([notice('a'), notice('b'), notice('c')])
    const next = [notice('a'), notice('b', 'r1'), notice('c', 'r2'), notice('d')]
    expect(noticesToToast(prev, next, seen).map((n) => n.id)).toEqual(['c', 'd'])
  })

  it('다른 탭에서 이미 읽었으면 띄우지 않는다', () => {
    const prev = revisionsOf([notice('a')])
    expect(noticesToToast(prev, [notice('a', 'r2')], { a: 'r2' })).toEqual([])
  })
})

describe('목록 상한', () => {
  it('capEvents는 새 것을 남긴다', () => {
    const events = [1, 2, 3, 4].map((k) => event(k))
    expect(capEvents(events, 2).map((e) => e.key)).toEqual([3, 4])
  })

  it('pushToast는 같은 알림의 토스트를 새 것으로 바꾸고 상한을 지킨다', () => {
    const t = (key: string, id: string): Toast => ({ key, ref: { kind: 'server', id }, level: 'info' })
    const toasts = [t('t1', 'a'), t('t2', 'b')]
    expect(pushToast(toasts, t('t3', 'a')).map((x) => x.key)).toEqual(['t2', 't3'])
    expect(pushToast(toasts, t('t3', 'c'), 2).map((x) => x.key)).toEqual(['t2', 't3'])
  })

  it('itemKey는 종류를 구분한다', () => {
    expect(itemKey({ kind: 'server', id: '1' })).not.toBe(itemKey({ kind: 'client', key: 1 }))
  })
})

describe('orderNotices', () => {
  it('급한 것, 최근에 생긴 것, id 순이다', () => {
    const list = [
      notice('b', 'r', { level: 'info', since: '2026-10-01T00:00:00Z' }),
      notice('a', 'r', { level: 'info', since: '2026-10-02T00:00:00Z' }),
      notice('z', 'r', { level: 'error' }),
      notice('c', 'r', { level: 'info' }),
      notice('w', 'r', { level: 'warning' }),
    ]
    expect(orderNotices(list).map((n) => n.id)).toEqual(['z', 'w', 'a', 'b', 'c'])
  })
})

describe('badgeText', () => {
  it.each([
    [0, null],
    [1, '1'],
    [9, '9'],
    [10, '9+'],
  ])('%i → %s', (count, text) => {
    expect(badgeText(count)).toBe(text)
  })
})

describe('actionAvailable', () => {
  const ctx = { admin: false, workbench: false, isApp: (id: string) => id === 'settings' }

  it('없는 동작은 보이지 않는다', () => {
    expect(actionAvailable(undefined, ctx)).toBe(false)
  })

  it('아는 앱만 연다', () => {
    expect(actionAvailable({ kind: 'app', app: 'settings', page: 'about' }, ctx)).toBe(true)
    expect(actionAvailable({ kind: 'app', app: 'later' }, ctx)).toBe(false)
  })

  it('워크벤치는 주소가 있을 때, 전원은 관리자에게만 보인다', () => {
    const route = { kind: 'workbench', route: 'system' } as const
    expect(actionAvailable(route, ctx)).toBe(false)
    expect(actionAvailable(route, { ...ctx, workbench: true })).toBe(true)
    expect(actionAvailable({ kind: 'power' }, ctx)).toBe(false)
    expect(actionAvailable({ kind: 'power' }, { ...ctx, admin: true })).toBe(true)
  })
})
