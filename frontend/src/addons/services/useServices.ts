import { fetchAddon } from '../../api/client'
import type { ServicesResponse } from '../../api/types.gen'
import { type PolledData, usePolledData } from '../common/usePolledData'
import { isServicesResponse } from './services'

// OMV가 서비스마다 systemctl을 부르므로 가볍지 않다. 창은 10초, 위젯은 30초마다 갱신한다.
export const SERVICES_POLL_INTERVAL_MS = 10_000
export const SERVICES_WIDGET_POLL_INTERVAL_MS = 30_000

const fetchServices = (signal: AbortSignal) => fetchAddon('services', isServicesResponse, { signal })

export function useServices(active: boolean, intervalMs = SERVICES_POLL_INTERVAL_MS): PolledData<ServicesResponse> {
  return usePolledData(fetchServices, active, intervalMs)
}
