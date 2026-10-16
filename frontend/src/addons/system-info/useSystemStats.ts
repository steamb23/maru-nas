import { fetchAddon } from '../../api/client'
import type { SystemStatsResponse } from '../../api/types.gen'
import { DEFAULT_TIMEOUT_MS, createSharedPoll } from '../common/sharedPoll'
import { type PolledState, usePolledData } from '../common/usePolledData'
import { computeProcessCpu } from './processes'
import { type StatsHistory, appendSample, isSystemStatsResponse } from './stats'

// 시스템 정보 창(개요·프로세스 탭)은 2초마다 갱신한다. 값은 두 응답의 차이로 구하므로,
// 창을 열면 두 번째 요청을 빨리 보내 첫 값이 주기만큼 늦지 않게 한다.
export const SYSTEM_STATS_POLL_INTERVAL_MS = 2000
const SYSTEM_STATS_WARMUP_MS = 500

export interface SystemStatsSample {
  response: SystemStatsResponse
  // 그래프 기록(최근 60초)
  history: StatsHistory
  // processKey → 전체 대비 CPU 사용률(0~100). 관리자가 아니거나 첫 응답이면 비어 있다.
  processCpu: ReadonlyMap<string, number>
}

const statsPoll = createSharedPoll({
  fetch: (signal) => fetchAddon('system-info', isSystemStatsResponse, { signal }),
  timeoutMs: DEFAULT_TIMEOUT_MS,
  warmupMs: SYSTEM_STATS_WARMUP_MS,
})

const toSample = (prev: SystemStatsSample | null, response: SystemStatsResponse): SystemStatsSample => ({
  response,
  history: appendSample(prev?.history ?? null, response),
  processCpu: computeProcessCpu(prev?.response ?? null, response),
})

// useSystemStats는 active인 동안 /api/addons/system-info를 가져와 기록을 쌓는다. 기록은 이 보기(창)의 것이다.
export function useSystemStats(active: boolean): PolledState<SystemStatsSample> {
  const { data, status } = usePolledData(statsPoll, active, SYSTEM_STATS_POLL_INTERVAL_MS, toSample)
  return { data, status }
}
