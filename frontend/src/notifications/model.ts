// 알림 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 알림은 두 가지다.
// - 서버 알림(Notice): 백엔드가 알리는 상태(업데이트가 있음 등). 조건이 풀리면 사라지고 지울 수 없다.
//   읽음 상태는 사용자별로 백엔드에 저장한다(id → 읽을 때의 리비전). 리비전이 바뀌면 다시 읽지 않은 알림이다.
// - 클라이언트 알림(ClientEvent): 이 탭에서 생긴 일(파일 작업 완료 등). 이 탭의 메모리에만 있고 지울 수 있다.
import type { Notice, NoticeAction } from '../api/types.gen'

export type NoticeLevel = Notice['level']

// 클라이언트 알림 코드. 문구는 messages.ts가 코드로 고른다.
export type ClientEventCode = 'filesCopied' | 'filesUploaded' | 'filesDone' | 'filesCopyFailed' | 'filesUploadFailed'

// NotifyInput은 notify()로 넣는 클라이언트 알림이다. 문구 대신 코드와 치환값을 둔다(언어를 바꾸면 다시 그린다).
export interface NotifyInput {
  code: ClientEventCode
  level: NoticeLevel
  params?: Readonly<Record<string, string | number>>
  action?: NoticeAction
}

export interface ClientEvent extends NotifyInput {
  // key는 이 탭에서 붙인 번호다.
  key: number
  // at은 생긴 시각(Date.now)이다.
  at: number
  read: boolean
}

// SeenMap은 서버 알림 읽음 상태(id → 읽을 때의 리비전)다.
export type SeenMap = Readonly<Record<string, string>>

// ItemRef는 알림 하나를 가리킨다. 토스트와 알림 센터의 새 표시가 쓴다.
export type ItemRef = { kind: 'server'; id: string } | { kind: 'client'; key: number }

export interface Toast {
  // key는 토스트마다 새로 붙인다. 같은 알림이 다시 떠도 새 토스트로 그린다.
  key: string
  ref: ItemRef
  level: NoticeLevel
}

// 이 탭에 두는 클라이언트 알림 수. 넘치면 오래된 것부터 버린다.
export const EVENT_LIMIT = 50
// 기다리는 토스트까지 합한 수. 넘치면 오래된 것부터 버린다.
export const TOAST_LIMIT = 10
// 저장하는 읽음 상태의 수. 백엔드의 상한(maxSeenEntries)과 같다.
export const SEEN_LIMIT = 128

// itemKey는 알림 하나의 문자열 키다.
export function itemKey(ref: ItemRef): string {
  return ref.kind === 'server' ? `s:${ref.id}` : `c:${ref.key}`
}

export function isUnread(n: Notice, seen: SeenMap): boolean {
  return seen[n.id] !== n.revision
}

export function unreadCount(notices: readonly Notice[], seen: SeenMap, events: readonly ClientEvent[]): number {
  return notices.filter((n) => isUnread(n, seen)).length + events.filter((e) => !e.read).length
}

// unreadError는 읽지 않은 오류 알림이 있는지다(알림 버튼 배지를 빨갛게 한다).
export function unreadError(notices: readonly Notice[], seen: SeenMap, events: readonly ClientEvent[]): boolean {
  return (
    notices.some((n) => n.level === 'error' && isUnread(n, seen)) || events.some((e) => e.level === 'error' && !e.read)
  )
}

// seenAfterMark는 ids(없으면 모두)를 읽은 뒤의 읽음 상태다. 지금 목록에 없는 id는 뺀다(저장 크기를 묶어 둔다).
export function seenAfterMark(notices: readonly Notice[], seen: SeenMap, ids?: readonly string[]): Record<string, string> {
  const marked = ids ? new Set(ids) : null
  const out: Record<string, string> = {}
  for (const n of notices.slice(0, SEEN_LIMIT)) {
    if (!marked || marked.has(n.id)) out[n.id] = n.revision
    else if (n.id in seen) out[n.id] = seen[n.id]
  }
  return out
}

// mergeSeen은 서버가 준 읽음 상태 위에 이 탭에서 읽었지만 서버가 아직 같은 값을 주지 않은 것(overrides)을 얹는다.
// 저장이 끝나기 전에 받은 응답이 읽음 표시를 되돌리지 않게 한다.
export function mergeSeen(server: SeenMap, overrides: SeenMap): Record<string, string> {
  return { ...server, ...overrides }
}

// settleOverrides는 서버가 같은 값을 주었거나 목록에서 사라진 알림을 overrides에서 뺀다.
export function settleOverrides(overrides: SeenMap, server: SeenMap, notices: readonly Notice[]): Record<string, string> {
  const ids = new Set(notices.map((n) => n.id))
  const out: Record<string, string> = {}
  for (const [id, rev] of Object.entries(overrides)) {
    if (ids.has(id) && server[id] !== rev) out[id] = rev
  }
  return out
}

export function revisionsOf(notices: readonly Notice[]): Map<string, string> {
  return new Map(notices.map((n) => [n.id, n.revision]))
}

// noticesToToast는 새로 토스트로 띄울 서버 알림이다. 이 탭에서 처음 받은 목록(prev가 null)은 띄우지 않는다
// (로그인할 때마다 같은 알림이 뜨지 않게). 그 뒤로는 새로 생기거나 리비전이 바뀐 읽지 않은 알림만 띄운다.
// 예외로 작업 결과(dismissible)는 첫 목록에서도 읽지 않았으면 띄운다. 창을 닫았거나 로그아웃한 동안 끝난 작업을
// 다음 로그인 때 알려야 하기 때문이다. 읽거나 지운 결과는 다시 뜨지 않는다.
export function noticesToToast(
  prev: ReadonlyMap<string, string> | null,
  next: readonly Notice[],
  seen: SeenMap,
): Notice[] {
  if (!prev) return next.filter((n) => n.dismissible === true && isUnread(n, seen))
  return next.filter((n) => prev.get(n.id) !== n.revision && isUnread(n, seen))
}

export function capEvents(events: readonly ClientEvent[], limit = EVENT_LIMIT): ClientEvent[] {
  return events.slice(Math.max(0, events.length - limit))
}

// pushToast는 toast를 맨 뒤(가장 새 것)에 더한다. 같은 알림의 토스트가 있으면 그것은 뺀다.
export function pushToast(toasts: readonly Toast[], toast: Toast, limit = TOAST_LIMIT): Toast[] {
  const key = itemKey(toast.ref)
  const out = [...toasts.filter((t) => itemKey(t.ref) !== key), toast]
  return out.slice(Math.max(0, out.length - limit))
}

const LEVEL_RANK: Record<NoticeLevel, number> = { error: 0, warning: 1, info: 2, success: 3 }

// orderNotices는 서버 알림을 급한 것, 최근에 생긴 것, id 순으로 놓는다.
export function orderNotices(notices: readonly Notice[]): Notice[] {
  return [...notices].sort(
    (a, b) =>
      LEVEL_RANK[a.level] - LEVEL_RANK[b.level] ||
      (b.since ?? '').localeCompare(a.since ?? '') ||
      a.id.localeCompare(b.id),
  )
}

// badgeText는 알림 버튼 배지의 글자다. 0이면 배지가 없다.
export function badgeText(count: number): string | null {
  if (count <= 0) return null
  return count > 9 ? '9+' : String(count)
}

export interface ActionContext {
  admin: boolean
  // workbench는 OMV 워크벤치 주소가 설정되어 있는지다.
  workbench: boolean
  // isApp은 이 화면이 아는 앱 id인지다(windows/apps.ts의 isAppId).
  isApp: (id: string) => boolean
}

// actionAvailable은 알림의 동작을 지금 보일 수 있는지다. 할 수 없는 동작은 단추를 그리지 않는다.
export function actionAvailable(action: NoticeAction | undefined, ctx: ActionContext): action is NoticeAction {
  if (!action) return false
  switch (action.kind) {
    case 'app':
      return action.app !== undefined && ctx.isApp(action.app)
    case 'workbench':
      return ctx.workbench && action.route !== undefined
    case 'power':
      return ctx.admin
  }
}
