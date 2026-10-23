import { useEffect } from 'react'
import { fetchNotifications } from '../api/client'
import { createPoller } from '../addons/common/poller'
import { usePageVisible } from '../addons/common/usePageVisible'
import { useNotificationStore } from './store'

// 서버 알림을 다시 가져오는 주기. 백엔드는 알림을 30초 보관한다.
export const NOTIFICATIONS_POLL_INTERVAL_MS = 60_000
const NOTIFICATIONS_TIMEOUT_MS = 10_000

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
      intervalMs: NOTIFICATIONS_POLL_INTERVAL_MS,
      timeoutMs: NOTIFICATIONS_TIMEOUT_MS,
      onData: store.received,
      onError: store.failed,
    })
    poller.start()
    return () => poller.stop()
  }, [visible])
}
