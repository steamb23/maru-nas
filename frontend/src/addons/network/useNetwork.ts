import { useRef } from 'react'
import { fetchAddon } from '../../api/client'
import type { NetworkResponse } from '../../api/types.gen'
import { type PolledState, usePolledData } from '../common/usePolledData'
import { type Rate, computeRates, isNetworkResponse } from './network'

// 속도는 보기마다 직전 응답과의 차이로 계산하므로 창·위젯이 서로 영향을 주지 않는다.
export const NETWORK_POLL_INTERVAL_MS = 2000
export const NETWORK_WIDGET_POLL_INTERVAL_MS = 5000

export interface NetworkSample {
  response: NetworkResponse
  // 인터페이스 이름 → 초당 바이트. 첫 응답이나 계산할 수 없을 때는 비어 있다.
  rates: Map<string, Rate>
}

export function useNetwork(active: boolean, intervalMs = NETWORK_POLL_INTERVAL_MS): PolledState<NetworkSample> {
  const prev = useRef<NetworkResponse | null>(null)
  const fetchSample = async (signal: AbortSignal): Promise<NetworkSample> => {
    const response = await fetchAddon('network', isNetworkResponse, { signal })
    const rates = computeRates(prev.current, response)
    prev.current = response
    return { response, rates }
  }
  const { data, status } = usePolledData(fetchSample, active, intervalMs)
  return { data, status }
}
