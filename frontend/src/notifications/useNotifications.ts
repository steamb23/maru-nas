import { useEffect } from 'react'
import { fetchNotifications } from '../api/client'
import { type Poller, createPoller } from '../addons/common/poller'
import { usePageVisible } from '../addons/common/usePageVisible'
import { useNotificationStore } from './store'

// 서버 알림을 다시 가져오는 주기. 백엔드는 알림을 30초 보관한다. 서버 백그라운드 작업이 진행 중이면(tasks) 결과를
// 곧 알리도록 자주 받는다(작업 결과 알림은 보관하지 않는다).
export const NOTIFICATIONS_POLL_INTERVAL_MS = 60_000
export const NOTIFICATIONS_TASK_INTERVAL_MS = 10_000

// pollInterval은 진행 중인 작업이 있는지에 따른 주기다.
export function pollInterval(tasks: number): number {
  return tasks > 0 ? NOTIFICATIONS_TASK_INTERVAL_MS : NOTIFICATIONS_POLL_INTERVAL_MS
}
const NOTIFICATIONS_TIMEOUT_MS = 10_000

// active는 지금 도는 알림 폴러다(탭이 보이는 동안).
let active: Poller | null = null

// refreshNotifications는 알림을 바로 다시 받는다. 서버 백그라운드 작업을 시작한 직후처럼 다음 주기를 기다리지 않고
// 알려야 할 때 부른다(작업이 생기면 주기도 짧아진다). 탭이 보이지 않으면 아무것도 하지 않는다.
export function refreshNotifications(): void {
  active?.stop()
  active?.start()
}

// useNotifications는 로그인한 동안 서버 알림을 가져와 저장소에 둔다. 탭이 보이지 않으면 멈추고, 다시 보이면
// 바로 한 번 가져온다. 언마운트(로그아웃·만료)되면 저장소를 비워 다음 사용자에게 이전 사용자의 알림이 보이지 않게 한다.
// 실패하면 받은 목록을 유지한다(401은 API 클라이언트가 로그인 화면으로 보낸다).
export function useNotifications() {
  const visible = usePageVisible()

  useEffect(() => {
    const store = useNotificationStore.getState()
    store.enable()
    return () => store.clear()
  }, [])

  useEffect(() => {
    if (!visible) return
    const store = useNotificationStore.getState()
    const poller = createPoller({
      fetch: fetchNotifications,
      intervalMs: () => pollInterval(useNotificationStore.getState().tasks.length),
      timeoutMs: NOTIFICATIONS_TIMEOUT_MS,
      onData: store.received,
      onError: store.failed,
    })
    // 작업이 생기거나 끝나면 다음 요청을 새 주기로 다시 잡는다.
    const unsubscribe = useNotificationStore.subscribe((s, prev) => {
      if (pollInterval(s.tasks.length) !== pollInterval(prev.tasks.length)) poller.reschedule()
    })
    poller.start()
    active = poller
    return () => {
      unsubscribe()
      poller.stop()
      if (active === poller) active = null
    }
  }, [visible])
}
