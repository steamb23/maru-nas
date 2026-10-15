import { fetchAddon } from '../../api/client'
import type { NetworkResponse } from '../../api/types.gen'
import { DEFAULT_TIMEOUT_MS, createSharedPoll } from '../common/sharedPoll'
import { type PolledState, usePolledData } from '../common/usePolledData'
import { type Rate, computeRates, isNetworkResponse } from './network'

// 창과 위젯이 요청을 함께 쓰고, 속도는 보기마다 자기가 받은 직전 응답과의 차이로 계산한다.
export const NETWORK_POLL_INTERVAL_MS = 2000
export const NETWORK_WIDGET_POLL_INTERVAL_MS = 5000

export interface NetworkSample {
  response: NetworkResponse
  // 인터페이스 이름 → 초당 바이트. 첫 응답이나 계산할 수 없을 때는 비어 있다.
  rates: Map<string, Rate>
}

const networkPoll = createSharedPoll({
  fetch: (signal) => fetchAddon('network', isNetworkResponse, { signal }),
  timeoutMs: DEFAULT_TIMEOUT_MS,
})

const toSample = (prev: NetworkSample | null, response: NetworkResponse): NetworkSample => ({
  response,
  rates: computeRates(prev?.response ?? null, response),
})

export function useNetwork(active: boolean, intervalMs = NETWORK_POLL_INTERVAL_MS): PolledState<NetworkSample> {
  const { data, status } = usePolledData(networkPoll, active, intervalMs, toSample)
  return { data, status }
}
