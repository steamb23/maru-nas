import { describe, expect, it } from 'vitest'
import type { Notice } from '../api/types.gen'
import { ageText, buildCenter, resolveItem, toastDuration, visibleToasts } from './items'
import { actionLabel, clientText, serverText } from './messages'
import { type ClientEvent, type Toast, unreadError } from './model'

// 시각은 이 브라우저(테스트 환경)의 시간대 달력으로 만든다.
const at = (day: number, hour: number, minute = 0) => new Date(2026, 9, day, hour, minute).getTime()
const now = at(23, 12)

const notice = (id: string, extra: Partial<Notice> = {}): Notice => ({ id, code: 'sample', level: 'warning', revision: 'r1', ...extra })
const event = (key: number, when: number, extra: Partial<ClientEvent> = {}): ClientEvent => ({
  key,
  code: 'filesCopied',
  level: 'success',
  params: { count: 2 },
  at: when,
  read: false,
  ...extra,
})

describe('buildCenter', () => {
  it('오늘과 이전으로 묶고 최근 것을 앞에 둔다', () => {
    const notices = [notice('timed', { since: new Date(at(23, 9)).toISOString() }), notice('untimed', { level: 'error' })]
    const events = [event(1, at(23, 11)), event(2, at(22, 23)), event(3, at(20, 8))]
    const { today, earlier } = buildCenter(notices, events, now)
    expect(today.map((i) => i.key)).toEqual(['s:untimed', 'c:1', 's:timed'])
    expect(earlier.map((i) => i.key)).toEqual(['c:2', 'c:3'])
  })

  it('같은 시각이면 나중에 생긴 알림이 앞이다', () => {
    const { today } = buildCenter([], [event(1, at(23, 11)), event(2, at(23, 11))], now)
    expect(today.map((i) => i.key)).toEqual(['c:2', 'c:1'])
  })

  it('서버 알림은 지울 수 없고 클라이언트 알림은 지울 수 있다', () => {
    const { today } = buildCenter([notice('a')], [event(1, at(23, 11))], now)
    expect(today.map((i) => i.dismissible)).toEqual([false, true])
  })
})

describe('ageText', () => {
  it.each([
    [now - 30_000, 'now'],
    [at(23, 11, 57), '3 minutes ago'],
    [at(23, 9), '3 hours ago'],
    [at(22, 23), 'yesterday'],
    [at(20, 12), '3 days ago'],
  ])('%s → %s', (when, text) => {
    expect(ageText(when, now, 'en')).toBe(text)
  })

  it('일주일이 넘으면 날짜를 보인다', () => {
    expect(ageText(at(1, 12), now, 'en')).toBe('10/1')
  })

  it('한국어로도 보인다', () => {
    expect(ageText(at(22, 23), now, 'ko')).toBe('어제')
  })
})

describe('토스트', () => {
  const toast = (n: number): Toast => ({ key: `t${n}`, ref: { kind: 'client', key: n }, level: 'info' })

  it('최근 세 개만 보이고 나머지는 접는다', () => {
    const toasts = [1, 2, 3, 4, 5].map(toast)
    expect(visibleToasts(toasts, false)).toEqual({ shown: toasts.slice(2), hidden: 2 })
    expect(visibleToasts(toasts, true)).toEqual({ shown: toasts, hidden: 0 })
    expect(visibleToasts(toasts.slice(0, 2), false)).toEqual({ shown: toasts.slice(0, 2), hidden: 0 })
  })

  it('오류는 저절로 닫히지 않는다', () => {
    expect(toastDuration('error')).toBeNull()
    expect(toastDuration('warning')).toBe(8000)
    expect(toastDuration('info')).toBe(5000)
    expect(toastDuration('success')).toBe(5000)
  })

  it('가리키는 알림이 사라졌으면 그리지 않는다', () => {
    expect(resolveItem({ kind: 'server', id: 'gone' }, [], [])).toBeNull()
    expect(resolveItem({ kind: 'client', key: 9 }, [], [event(1, now)])).toBeNull()
    expect(resolveItem({ kind: 'client', key: 1 }, [], [event(1, now)])?.key).toBe('c:1')
  })
})

describe('문구', () => {
  it('수가 없는 실패는 멈춤 문구다', () => {
    expect(clientText(event(1, now, { code: 'filesCopyFailed', level: 'error', params: undefined })).body).toBe(
      'notifications.client.filesCopyFailed.bodyStopped',
    )
    expect(clientText(event(1, now, { code: 'filesCopyFailed', level: 'error' })).body).toBe(
      'notifications.client.filesCopyFailed.body',
    )
  })

  it('서버 알림은 수에 따라 본문이 다르다', () => {
    expect(serverText(notice('a')).body).toBe('notifications.server.sample.body')
    expect(serverText(notice('a', { count: 3 }))).toEqual({
      title: 'notifications.server.sample.title',
      body: 'notifications.server.sample.bodyCount',
      params: { count: 3 },
    })
  })

  it('파일 작업 창은 "자세히 보기"다', () => {
    expect(actionLabel({ kind: 'app', app: 'transfers' })).toBe('notifications.action.details')
    expect(actionLabel({ kind: 'app', app: 'settings' })).toBe('notifications.action.open')
    expect(actionLabel({ kind: 'power' })).toBe('notifications.action.power')
  })
})

describe('unreadError', () => {
  it('읽지 않은 오류가 있는지 본다', () => {
    expect(unreadError([notice('a', { level: 'error' })], {}, [])).toBe(true)
    expect(unreadError([notice('a', { level: 'error' })], { a: 'r1' }, [])).toBe(false)
    expect(unreadError([], {}, [event(1, now, { level: 'error' })])).toBe(true)
    expect(unreadError([], {}, [event(1, now, { level: 'error', read: true })])).toBe(false)
  })
})
