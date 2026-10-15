import { fetchSystem } from '../../api/client'
import type { SystemResponse } from '../../api/types.gen'
import { createSharedPoll } from '../common/sharedPoll'
import { type PolledState, usePolledData } from '../common/usePolledData'

// 창은 2초, 위젯은 늘 떠 있으므로 더 길게 5초마다 갱신한다.
export const SYSTEM_POLL_INTERVAL_MS = 2000
export const SYSTEM_WIDGET_POLL_INTERVAL_MS = 5000
const SYSTEM_TIMEOUT_MS = 5000

export type { ConnectionStatus } from '../common/usePolledData'

export type SystemInfoState = PolledState<SystemResponse>

const systemPoll = createSharedPoll({ fetch: (signal) => fetchSystem(signal), timeoutMs: SYSTEM_TIMEOUT_MS })

// useSystemInfo는 active인 동안 intervalMs마다 /api/system을 가져온다. 창과 위젯이 요청을 함께 쓴다.
// active가 false가 되거나 컴포넌트가 언마운트되면 떨어진다.
export function useSystemInfo(active: boolean, intervalMs = SYSTEM_POLL_INTERVAL_MS): SystemInfoState {
  const { data, status } = usePolledData(systemPoll, active, intervalMs)
  return { data, status }
}
