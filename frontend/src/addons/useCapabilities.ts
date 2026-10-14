import { useEffect } from 'react'
import { fetchCapabilities } from '../api/client'
import { useCapabilitiesStore } from './capabilities'
import { createPoller } from './common/poller'

// 플러그인 설치·제거를 반영하는 주기. 백엔드의 감지 결과 보관 시간(5분)과 같다.
export const CAPABILITIES_POLL_INTERVAL_MS = 5 * 60_000
const CAPABILITIES_TIMEOUT_MS = 10_000

// useCapabilities는 로그인한 동안 capability를 가져와 저장소에 둔다. 언마운트(로그아웃·만료)되면
// 저장소를 비워 다음 사용자에게 이전 사용자의 값이 보이지 않게 한다.
// 실패하면 마지막 값을 유지한다(401은 API 클라이언트가 로그인 화면으로 보낸다).
export function useCapabilities() {
  useEffect(() => {
    const store = useCapabilitiesStore.getState()
    const poller = createPoller({
      fetch: fetchCapabilities,
      intervalMs: CAPABILITIES_POLL_INTERVAL_MS,
      timeoutMs: CAPABILITIES_TIMEOUT_MS,
      onData: store.set,
      onError: () => {},
    })
    poller.start()
    return () => {
      poller.stop()
      store.clear()
    }
  }, [])
}
