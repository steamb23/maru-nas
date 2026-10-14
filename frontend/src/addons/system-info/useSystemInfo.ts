import { useEffect, useState } from 'react'
import { fetchSystem } from '../../api/client'
import type { SystemResponse } from '../../api/types.gen'
import { createPoller } from '../common/poller'

// 창은 2초, 위젯은 늘 떠 있으므로 더 길게 5초마다 갱신한다.
export const SYSTEM_POLL_INTERVAL_MS = 2000
export const SYSTEM_WIDGET_POLL_INTERVAL_MS = 5000
const SYSTEM_TIMEOUT_MS = 5000

// loading: 아직 응답이 없음. ok: 마지막 요청 성공. disconnected: 마지막 요청 실패(재시도 중).
export type ConnectionStatus = 'loading' | 'ok' | 'disconnected'

export interface SystemInfoState {
  // 연결이 끊겨도 마지막으로 받은 값을 유지한다.
  data: SystemResponse | null
  status: ConnectionStatus
}

// useSystemInfo는 active인 동안 intervalMs마다 /api/system을 가져온다.
// active가 false가 되거나 컴포넌트가 언마운트되면 요청을 멈춘다.
export function useSystemInfo(active: boolean, intervalMs = SYSTEM_POLL_INTERVAL_MS): SystemInfoState {
  const [state, setState] = useState<SystemInfoState>({ data: null, status: 'loading' })

  useEffect(() => {
    if (!active) return
    const poller = createPoller({
      fetch: fetchSystem,
      intervalMs,
      timeoutMs: SYSTEM_TIMEOUT_MS,
      onData: (data) => setState({ data, status: 'ok' }),
      onError: () => setState((s) => (s.status === 'disconnected' ? s : { ...s, status: 'disconnected' })),
    })
    poller.start()
    return poller.stop
  }, [active, intervalMs])

  return state
}
