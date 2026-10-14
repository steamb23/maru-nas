import { useEffect, useState } from 'react'
import { fetchAddon } from '../../api/client'
import type { SmartResponse } from '../../api/types.gen'
import { type PolledData, usePolledData } from '../common/usePolledData'
import { isSmartResponse } from './smart'

// 백엔드가 SMART 결과를 보관하고 하루 한 번(기준 시각 이후 첫 요청)만 OMV에 다시 묻는다.
// 그래서 이 주기 요청은 보관한 결과를 받아 오거나 기준 시각을 넘겼는지 확인하는 정도다.
export const SMART_POLL_INTERVAL_MS = 10 * 60_000
// 보관한 결과가 없으면 백엔드가 디스크마다 smartctl을 실행하므로 오래 걸릴 수 있다.
const SMART_TIMEOUT_MS = 60_000

export interface SmartSample {
  response: SmartResponse
  // 새로고침할 수 있게 되는 브라우저 시각(Date.now 기준 밀리초)
  refreshAfterMs: number
}

const fetchSmart = async (signal: AbortSignal, refresh: boolean): Promise<SmartSample> => {
  const response = await fetchAddon('smart', isSmartResponse, { signal, refresh })
  return { response, refreshAfterMs: Date.now() + response.refreshInSeconds * 1000 }
}

export function useSmart(active: boolean): PolledData<SmartSample> {
  return usePolledData(fetchSmart, active, SMART_POLL_INTERVAL_MS, SMART_TIMEOUT_MS)
}

// useRefreshReady는 새로고침할 수 있는 시각이 지났는지다. 남은 시간이 지나면 다시 그린다.
export function useRefreshReady(refreshAfterMs: number | undefined): boolean {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (refreshAfterMs === undefined) return
    const id = setTimeout(() => setNow(Date.now()), Math.max(0, refreshAfterMs - Date.now()) + 50)
    return () => clearTimeout(id)
  }, [refreshAfterMs])
  return refreshAfterMs === undefined || now >= refreshAfterMs
}
