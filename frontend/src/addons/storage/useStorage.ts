import { fetchAddon } from '../../api/client'
import type { StorageResponse } from '../../api/types.gen'
import { DEFAULT_TIMEOUT_MS, createSharedPoll } from '../common/sharedPoll'
import { type PolledData, usePolledData } from '../common/usePolledData'
import { isStorageResponse } from './storage'

// 용량은 천천히 바뀐다. 창은 15초, 위젯은 1분마다 갱신한다.
export const STORAGE_POLL_INTERVAL_MS = 15_000
export const STORAGE_WIDGET_POLL_INTERVAL_MS = 60_000

const storagePoll = createSharedPoll({
  fetch: (signal) => fetchAddon('storage', isStorageResponse, { signal }),
  timeoutMs: DEFAULT_TIMEOUT_MS,
})

export function useStorage(active: boolean, intervalMs = STORAGE_POLL_INTERVAL_MS): PolledData<StorageResponse> {
  return usePolledData(storagePoll, active, intervalMs)
}
