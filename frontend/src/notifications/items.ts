// 알림 센터·토스트가 그릴 항목 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { Notice, NoticeAction } from '../api/types.gen'
import { type NoticeText, clientText, serverText } from './messages'
import { type ClientEvent, type ItemRef, type NoticeLevel, type Toast, itemKey, orderNotices } from './model'

// Item은 화면에 그릴 알림 하나다.
export interface Item {
  ref: ItemRef
  key: string
  level: NoticeLevel
  text: NoticeText
  action: NoticeAction | undefined
  // at은 생긴 시각(Date.now 기준 ms)이다. 서버 알림이 시각을 주지 않았으면 null이다.
  at: number | null
  // dismissible이면 사용자가 지울 수 있다(클라이언트 알림, 서버의 작업 결과 알림). 그 밖의 서버 알림은 조건이 풀려야
  // 사라진다.
  dismissible: boolean
  // job은 작업 결과 알림의 작업 id다. 동작이 그 작업의 출력을 연다.
  job?: string
}

function serverItem(n: Notice): Item {
  const at = n.since ? Date.parse(n.since) : NaN
  const ref: ItemRef = { kind: 'server', id: n.id }
  return {
    ref,
    key: itemKey(ref),
    level: n.level,
    text: serverText(n),
    action: n.action,
    at: Number.isNaN(at) ? null : at,
    dismissible: n.dismissible === true,
    job: n.job?.id,
  }
}

function clientItem(e: ClientEvent): Item {
  const ref: ItemRef = { kind: 'client', key: e.key }
  return { ref, key: itemKey(ref), level: e.level, text: clientText(e), action: e.action, at: e.at, dismissible: true }
}

// resolveItem은 토스트가 가리키는 알림이다. 그사이 사라졌으면 null이다.
export function resolveItem(ref: ItemRef, notices: readonly Notice[], events: readonly ClientEvent[]): Item | null {
  if (ref.kind === 'server') {
    const n = notices.find((x) => x.id === ref.id)
    return n ? serverItem(n) : null
  }
  const e = events.find((x) => x.key === ref.key)
  return e ? clientItem(e) : null
}

export interface CenterGroups {
  today: Item[]
  earlier: Item[]
}

// buildCenter는 알림 센터의 묶음(오늘, 이전)이다. 묶음 안은 최근 것이 앞이고, 시각을 모르는 서버 알림은
// 오늘의 맨 앞에 급한 순서로 둔다(지금 계속되는 상태라서).
export function buildCenter(notices: readonly Notice[], events: readonly ClientEvent[], now: number): CenterGroups {
  const untimed: Item[] = []
  const timed: Item[] = []
  for (const n of orderNotices(notices)) (n.since ? timed : untimed).push(serverItem(n))
  // events는 오래된 것이 앞이다. 거꾸로 넣어 같은 시각이면(정렬은 안정적이다) 나중에 생긴 것이 앞에 오게 한다.
  for (const e of [...events].reverse()) timed.push(clientItem(e))
  timed.sort((a, b) => (b.at ?? 0) - (a.at ?? 0))
  const today = [...untimed]
  const earlier: Item[] = []
  for (const item of timed) (item.at === null || calendarDays(item.at, now) <= 0 ? today : earlier).push(item)
  return { today, earlier }
}

// calendarDays는 at에서 now까지 지난 날 수(이 브라우저의 시간대 달력 기준)다.
function calendarDays(at: number, now: number): number {
  const a = new Date(at)
  const b = new Date(now)
  const dayA = Date.UTC(a.getFullYear(), a.getMonth(), a.getDate())
  const dayB = Date.UTC(b.getFullYear(), b.getMonth(), b.getDate())
  return Math.round((dayB - dayA) / 86_400_000)
}

// ageText는 지난 시간을 "방금", "3분 전", "2시간 전", "어제", "3일 전", 일주일이 넘으면 날짜로 보인다.
export function ageText(at: number, now: number, locale: string): string {
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' })
  const minutes = Math.floor(Math.max(0, now - at) / 60_000)
  if (minutes < 1) return rtf.format(0, 'second')
  if (minutes < 60) return rtf.format(-minutes, 'minute')
  const days = calendarDays(at, now)
  if (days <= 0) return rtf.format(-Math.floor(minutes / 60), 'hour')
  if (days < 7) return rtf.format(-days, 'day')
  return new Intl.DateTimeFormat(locale, { month: 'numeric', day: 'numeric' }).format(at)
}

// 한 번에 보이는 토스트 수. 나머지는 "+N개 더"로 접는다.
export const TOAST_VISIBLE = 3

// visibleToasts는 보일 토스트(오래된 것이 위)와 접힌 수다. 펼치면 모두 보인다.
export function visibleToasts(toasts: readonly Toast[], expanded: boolean): { shown: Toast[]; hidden: number } {
  if (expanded || toasts.length <= TOAST_VISIBLE) return { shown: [...toasts], hidden: 0 }
  return { shown: toasts.slice(toasts.length - TOAST_VISIBLE), hidden: toasts.length - TOAST_VISIBLE }
}

// toastDuration은 토스트가 저절로 닫히기까지의 시간이다. 오류는 사용자가 닫을 때까지 둔다(null).
export function toastDuration(level: NoticeLevel): number | null {
  switch (level) {
    case 'error':
      return null
    case 'warning':
      return 8000
    default:
      return 5000
  }
}
