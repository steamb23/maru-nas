import { useEffect } from 'react'
import { ApiError, fetchSession, sendActivity } from '../api/client'
import { ACTIVITY_EVENT } from './activity'
import { CONFIRM_RETRY_MS, type KeeperState, decide } from './sessionKeeper'
import { useAuthStore } from './store'

// 사용자 입력으로 보는 이벤트. ACTIVITY_EVENT는 입력 없이 이어지는 사용자 작업(파일 올리기)이 알리는 활동이다.
const INPUT_EVENTS = ['pointerdown', 'keydown', 'wheel', ACTIVITY_EVENT] as const

// useSessionKeeper는 로그인한 동안 사용자 입력을 감지해 OMV 세션을 연장하고,
// 예상 만료 시각이 되면 백엔드에 확인해 만료됐으면 로그인 화면으로 보낸다(sessionKeeper.ts).
export function useSessionKeeper() {
  useEffect(() => {
    const { sessionUpdated, expire } = useAuthStore.getState()
    const s: KeeperState = {
      expiresAt: useAuthStore.getState().expiresAt,
      lastInputAt: -Infinity,
      lastSentAt: Date.now(),
    }
    let timer: number | undefined
    let busy = false
    // 확인 요청이 네트워크 오류로 실패하면 이 시각까지 다시 확인하지 않는다.
    let retryAt = 0
    let stopped = false

    const request = (p: Promise<void>) => {
      busy = true
      p.finally(() => {
        busy = false
        if (!stopped) run()
      })
    }

    const run = () => {
      window.clearTimeout(timer)
      timer = undefined
      if (busy || stopped) return
      const now = Date.now()
      const action = decide(s, now)
      if (action.kind === 'renew') {
        s.lastSentAt = now
        // 401이면 API 클라이언트가 로그인 화면으로 보낸다. 그 밖의 실패는 다음 입력에서 다시 보낸다.
        request(sendActivity().then(sessionUpdated, () => {}))
      } else if (action.kind === 'confirm') {
        if (now < retryAt) {
          timer = window.setTimeout(run, retryAt - now)
          return
        }
        request(
          fetchSession().then(sessionUpdated, (err: unknown) => {
            if (err instanceof ApiError && err.status === 401) expire()
            else retryAt = Date.now() + CONFIRM_RETRY_MS
          }),
        )
      } else if (action.kind === 'wait') {
        timer = window.setTimeout(run, Math.max(0, action.at - now))
      }
    }

    const onInput = () => {
      s.lastInputAt = Date.now()
      run()
    }
    for (const type of INPUT_EVENTS) document.addEventListener(type, onInput, { capture: true, passive: true })
    // 만료 시각이 바뀌면(활동 신호·확인 응답) 다시 판단한다.
    const unsubscribe = useAuthStore.subscribe((st, prev) => {
      if (st.expiresAt === prev.expiresAt) return
      s.expiresAt = st.expiresAt
      run()
    })
    run()

    return () => {
      stopped = true
      window.clearTimeout(timer)
      unsubscribe()
      for (const type of INPUT_EVENTS) document.removeEventListener(type, onInput, { capture: true })
    }
  }, [])
}
