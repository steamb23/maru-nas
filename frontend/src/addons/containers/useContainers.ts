import { fetchAddon } from '../../api/client'
import type { ContainersResponse } from '../../api/types.gen'
import { createSharedPoll } from '../common/sharedPoll'
import { type PolledData, usePolledData } from '../common/usePolledData'
import { isContainersResponse } from './containers'

// OMV가 PHP에서 docker CLI를 실행하므로 느리다. 창은 15초, 위젯은 30초마다 갱신한다.
export const CONTAINERS_POLL_INTERVAL_MS = 15_000
export const CONTAINERS_WIDGET_POLL_INTERVAL_MS = 30_000
const CONTAINERS_TIMEOUT_MS = 20_000

const containersPoll = createSharedPoll({
  fetch: (signal, refresh) => fetchAddon('containers', isContainersResponse, { signal, refresh }),
  timeoutMs: CONTAINERS_TIMEOUT_MS,
})

export function useContainers(
  active: boolean,
  intervalMs = CONTAINERS_POLL_INTERVAL_MS,
): PolledData<ContainersResponse> {
  return usePolledData(containersPoll, active, intervalMs)
}
