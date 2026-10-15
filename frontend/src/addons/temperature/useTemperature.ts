import { fetchAddon } from '../../api/client'
import type { TemperatureResponse } from '../../api/types.gen'
import { DEFAULT_TIMEOUT_MS, createSharedPoll } from '../common/sharedPoll'
import { type PolledData, usePolledData } from '../common/usePolledData'
import { isTemperatureResponse } from './temperature'

export const TEMPERATURE_POLL_INTERVAL_MS = 5000
export const TEMPERATURE_WIDGET_POLL_INTERVAL_MS = 10_000

const temperaturePoll = createSharedPoll({
  fetch: (signal) => fetchAddon('temperature', isTemperatureResponse, { signal }),
  timeoutMs: DEFAULT_TIMEOUT_MS,
})

export function useTemperature(
  active: boolean,
  intervalMs = TEMPERATURE_POLL_INTERVAL_MS,
): PolledData<TemperatureResponse> {
  return usePolledData(temperaturePoll, active, intervalMs)
}
