// 알림 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 서버 알림은 폴러(useNotifications.ts)가 받아 received로 넣고, 클라이언트 알림은 notify()가 push로 넣는다.
// 로그인한 동안만 받는다(enable). 로그아웃·만료로 데스크톱을 떠나면 clear로 모두 비우고, 그 뒤에 들어온
// 클라이언트 알림(늦게 끝난 파일 작업 등)은 버린다. 계산은 model.ts에 있다.
import { create } from 'zustand'
import * as client from '../api/client'
import { createSaveQueue } from '../api/saveQueue'
import type { Job, Notice, NotificationsResponse } from '../api/types.gen'
import {
  type ClientEvent,
  type ItemRef,
  type NotifyInput,
  type SeenMap,
  type Toast,
  capEvents,
  isUnread,
  itemKey,
  mergeSeen,
  noticesToToast,
  pushToast,
  revisionsOf,
  seenAfterMark,
  settleOverrides,
} from './model'

// idle: 아직 받지 않음. ready: 서버 알림을 받음. error: 마지막 요청이 실패함(받은 목록은 그대로 둔다).
export type NotificationStatus = 'idle' | 'ready' | 'error'

export interface NotificationData {
  // enabled는 로그인해 알림을 받는 중인지다. false면 push를 버린다.
  enabled: boolean
  status: NotificationStatus
  notices: readonly Notice[]
  // seen은 서버 알림 읽음 상태다(이 탭에서 읽고 아직 저장되지 않은 것을 포함).
  seen: SeenMap
  // 마지막 읽음 상태 저장이 실패했는지. 다음에 읽을 때 다시 보낸다.
  saveFailed: boolean
  // events는 클라이언트 알림이다. 오래된 것이 앞이다.
  events: readonly ClientEvent[]
  // toasts는 띄울 토스트다. 오래된 것이 앞이다.
  toasts: readonly Toast[]
  centerOpen: boolean
  // fresh는 알림 센터를 연 순간 읽지 않았던 항목(itemKey)이다. 센터를 열면 모두 읽음이 되지만,
  // 열려 있는 동안은 무엇이 새로 왔는지 보이게 남긴다. 열린 동안 들어온 항목도 더한다.
  fresh: ReadonlySet<string>
  // tasks는 진행 중인 서버 백그라운드 작업이다(관리자에게만 온다).
  tasks: readonly Job[]
}

export interface NotificationActions {
  enable: () => void
  clear: () => void
  // received는 서버 알림 응답을 반영한다.
  received: (resp: NotificationsResponse) => void
  failed: () => void
  // markSeen은 서버 알림 ids(없으면 모두)를 읽음으로 저장한다.
  markSeen: (ids?: readonly string[]) => void
  // push는 클라이언트 알림을 더하고 토스트를 띄운다.
  push: (input: NotifyInput) => void
  // markRead는 알림 하나를 읽음으로 둔다(동작을 실행했을 때 등).
  markRead: (ref: ItemRef) => void
  dismissEvent: (key: number) => void
  // dismissNotice는 지울 수 있는 서버 알림(작업 결과)을 바로 숨기고 서버에서 지운다. 실패하면 되돌린다.
  dismissNotice: (id: string) => void
  // clearAll은 알림 센터의 "모두 지우기"다. 클라이언트 알림과 지울 수 있는 서버 알림(작업 결과)을 지운다.
  // 상태를 알리는 서버 알림(업데이트, 재부팅 권장)은 남는다. 서버에서 지우지 못하면 작업 결과를 되돌린다.
  clearAll: () => void
  // dismissToast는 사용자가 닫은 토스트를 내리고 그 알림을 읽음으로 둔다.
  dismissToast: (key: string) => void
  // expireToast는 시간이 지난 토스트를 내린다. 알림은 읽지 않은 채로 남는다.
  expireToast: (key: string) => void
  setCenterOpen: (open: boolean) => void
}

export type NotificationStore = NotificationData & NotificationActions

export interface NotificationApi {
  saveSeen: (seen: SeenMap) => Promise<void>
  // dismiss는 서버 알림 하나를 지운다(없으면 아무것도 보내지 않는다).
  dismiss?: (id: string) => Promise<void>
  // dismissThrough는 작업 결과 알림 through와 그보다 먼저 끝난 결과를 모두 지운다(없으면 아무것도 보내지 않는다).
  dismissThrough?: (through: string) => Promise<void>
  now: () => number
}

const EMPTY_FRESH: ReadonlySet<string> = new Set()

export const initialNotificationData: NotificationData = {
  enabled: false,
  status: 'idle',
  notices: [],
  seen: {},
  saveFailed: false,
  events: [],
  toasts: [],
  centerOpen: false,
  fresh: EMPTY_FRESH,
  tasks: [],
}

export function createNotificationStore(api: NotificationApi) {
  return create<NotificationStore>()((set, get) => {
    // clear마다 바뀐다. 로그아웃한 뒤 늦게 끝난 저장의 결과를 버리는 데 쓴다(저장 큐가 함께 버린다).
    const saves = createSaveQueue(api.saveSeen, (ok) => set({ saveFailed: !ok }))
    // 이 탭에서 읽었지만 서버가 아직 같은 값을 주지 않은 읽음 상태(model.ts의 mergeSeen).
    let overrides: Record<string, string> = {}
    // 마지막으로 받은 서버 알림의 리비전. 이 탭에서 처음 받기 전에는 null이다(첫 목록은 토스트로 띄우지 않는다).
    let lastRevisions: Map<string, string> | null = null
    let eventSeq = 0
    let toastSeq = 0
    // 지우는 중인 서버 알림 id. 지우기가 끝나기 전에 받은 응답이 다시 보이지 않게 거른다(서버가 더는 주지 않으면 뺀다).
    let dismissing = new Set<string>()

    const newToast = (ref: ItemRef, level: Toast['level']): Toast => ({ key: `t${++toastSeq}`, ref, level })

    const saveSeen = (ids?: readonly string[]) => {
      const { notices, seen } = get()
      const marked = ids ?? notices.map((n) => n.id)
      if (!notices.some((n) => marked.includes(n.id) && isUnread(n, seen))) return
      const next = seenAfterMark(notices, seen, marked)
      for (const n of notices) if (marked.includes(n.id)) overrides[n.id] = n.revision
      set({ seen: next })
      saves.push(next)
    }

    return {
      ...initialNotificationData,

      enable: () => set({ enabled: true }),

      clear: () => {
        saves.reset()
        overrides = {}
        lastRevisions = null
        dismissing = new Set()
        set(initialNotificationData)
      },

      received: (raw) => {
        if (!get().enabled) return
        for (const id of dismissing) if (!raw.notices.some((n) => n.id === id)) dismissing.delete(id)
        const resp = { ...raw, notices: raw.notices.filter((n) => !dismissing.has(n.id)) }
        overrides = settleOverrides(overrides, resp.seen, resp.notices)
        const seen = mergeSeen(resp.seen, overrides)
        const news = noticesToToast(lastRevisions, resp.notices, seen)
        lastRevisions = revisionsOf(resp.notices)
        const ids = new Set(resp.notices.map((n) => n.id))
        let { toasts, fresh } = get()
        // 사라진 서버 알림의 토스트는 내린다.
        toasts = toasts.filter((t) => t.ref.kind !== 'server' || ids.has(t.ref.id))
        const { centerOpen } = get()
        if (centerOpen) {
          fresh = new Set([...fresh, ...news.map((n) => itemKey({ kind: 'server', id: n.id }))])
        } else {
          for (const n of news) toasts = pushToast(toasts, newToast({ kind: 'server', id: n.id }, n.level))
        }
        set({ status: 'ready', notices: resp.notices, seen, toasts, fresh, tasks: resp.tasks ?? [] })
        // 센터가 열린 동안 들어온 알림은 사용자가 보고 있으므로 읽음이다.
        if (centerOpen && news.length > 0) saveSeen(news.map((n) => n.id))
      },

      failed: () => {
        if (get().enabled) set({ status: 'error' })
      },

      markSeen: (ids) => saveSeen(ids),

      push: (input) => {
        const { enabled, centerOpen, events, toasts, fresh } = get()
        if (!enabled) return
        const key = ++eventSeq
        const kept = capEvents([...events, { ...input, key, at: api.now(), read: centerOpen }])
        const live = new Set(kept.map((e) => e.key))
        const ref: ItemRef = { kind: 'client', key }
        if (centerOpen) {
          set({ events: kept, fresh: new Set([...fresh, itemKey(ref)]) })
          return
        }
        set({
          events: kept,
          toasts: pushToast(
            toasts.filter((t) => t.ref.kind !== 'client' || live.has(t.ref.key)),
            newToast(ref, input.level),
          ),
        })
      },

      markRead: (ref) => {
        if (ref.kind === 'server') {
          saveSeen([ref.id])
          return
        }
        set({ events: get().events.map((e) => (e.key === ref.key && !e.read ? { ...e, read: true } : e)) })
      },

      dismissEvent: (key) =>
        set({
          events: get().events.filter((e) => e.key !== key),
          toasts: get().toasts.filter((t) => t.ref.kind !== 'client' || t.ref.key !== key),
        }),

      dismissNotice: (id) => {
        const notice = get().notices.find((n) => n.id === id)
        if (!notice?.dismissible || !api.dismiss) return
        dismissing.add(id)
        const hide = () =>
          set({
            notices: get().notices.filter((n) => n.id !== id),
            toasts: get().toasts.filter((t) => t.ref.kind !== 'server' || t.ref.id !== id),
          })
        hide()
        const generation = dismissing
        api.dismiss(id).catch(() => {
          // 로그아웃했으면(clear) 되돌리지 않는다.
          if (generation !== dismissing || !get().enabled) return
          dismissing.delete(id)
          if (!get().notices.some((n) => n.id === id)) set({ notices: [...get().notices, notice] })
        })
      },

      clearAll: () => {
        const { notices, toasts } = get()
        const removed = api.dismissThrough ? notices.filter((n) => n.dismissible) : []
        const ids = new Set(removed.map((n) => n.id))
        set({
          events: [],
          notices: notices.filter((n) => !ids.has(n.id)),
          toasts: toasts.filter((t) => t.ref.kind === 'server' && !ids.has(t.ref.id)),
        })
        const through = newestNotice(removed)
        if (!through || !api.dismissThrough) return
        // 서버는 through와 그보다 먼저 끝난 결과를 지운다. 화면에 오지 않은 오래된 결과도 함께 지워져
        // 다음 응답에 다시 나오지 않고, 이 탭이 아직 받지 못한 더 최근 결과는 남는다.
        for (const id of ids) dismissing.add(id)
        const generation = dismissing
        api.dismissThrough(through.id).catch(() => {
          // 로그아웃했으면(clear) 되돌리지 않는다.
          if (generation !== dismissing || !get().enabled) return
          for (const id of ids) dismissing.delete(id)
          const shown = new Set(get().notices.map((n) => n.id))
          set({ notices: [...get().notices, ...removed.filter((n) => !shown.has(n.id))] })
        })
      },

      dismissToast: (key) => {
        const toast = get().toasts.find((t) => t.key === key)
        if (!toast) return
        set({ toasts: get().toasts.filter((t) => t.key !== key) })
        get().markRead(toast.ref)
      },

      expireToast: (key) => set({ toasts: get().toasts.filter((t) => t.key !== key) }),

      setCenterOpen: (open) => {
        const state = get()
        if (state.centerOpen === open) return
        if (!open) {
          set({ centerOpen: false, fresh: EMPTY_FRESH })
          return
        }
        const fresh = new Set([
          ...state.notices.filter((n) => isUnread(n, state.seen)).map((n) => itemKey({ kind: 'server', id: n.id })),
          ...state.events.filter((e) => !e.read).map((e) => itemKey({ kind: 'client', key: e.key })),
        ])
        set({
          centerOpen: true,
          fresh,
          toasts: [],
          events: state.events.map((e) => (e.read ? e : { ...e, read: true })),
        })
        saveSeen()
      },
    }
  })
}

// newestNotice는 가장 최근에 생긴 알림이다(since가 가장 늦은 것). 작업 결과의 since는 끝난 시각(UTC RFC 3339)이라
// 문자열로 비교한다. 단독 지우기를 되돌리면 목록 끝에 붙으므로 목록 순서를 믿지 않는다.
function newestNotice(notices: readonly Notice[]): Notice | undefined {
  let newest: Notice | undefined
  for (const n of notices) if (!newest || (n.since ?? '') > (newest.since ?? '')) newest = n
  return newest
}

export const useNotificationStore = createNotificationStore({
  saveSeen: (seen) => client.saveNotificationPrefs(seen),
  dismiss: (id) => client.dismissNotice(id),
  dismissThrough: (through) => client.dismissNotices(through),
  now: () => Date.now(),
})
