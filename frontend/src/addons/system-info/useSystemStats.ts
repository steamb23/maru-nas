import { useCallback, useEffect, useSyncExternalStore } from 'react'
import { fetchAddon } from '../../api/client'
import type { SystemStatsResponse } from '../../api/types.gen'
import { type RecordedState, createRecorder } from '../common/recorder'
import { DEFAULT_TIMEOUT_MS, createSharedPoll } from '../common/sharedPoll'
import { workerScheduler } from '../common/workerTimer'
import { computeProcessCpu } from './processes'
import { type StatsHistory, appendSample, isSystemStatsResponse } from './stats'

// 시스템 정보 창은 보이는 동안 1초마다 갱신한다(작업 관리자와 같다). 탭을 숨기거나 창을 최소화해도 창이 열려 있으면
// 2초마다 계속 기록해 돌아왔을 때 그사이 그래프가 이어진다. 숨긴 탭의 타이머는 브라우저가 늦추므로 Worker 타이머를 쓴다.
// 값은 두 응답의 차이로 구하므로, 창을 열면 두 번째 요청을 빨리 보내 첫 값이 주기만큼 늦지 않게 한다.
export const SYSTEM_STATS_INTERVAL_MS = 1000
const SYSTEM_STATS_BACKGROUND_INTERVAL_MS = 2000
const SYSTEM_STATS_WARMUP_MS = 250

export interface SystemStatsSample {
  response: SystemStatsResponse
  // 그래프 기록(최근 60초)
  history: StatsHistory
  // processKey → 전체 대비 CPU 사용률(0~100). 프로세스 목록을 받기 전이나 첫 응답이면 비어 있다.
  processCpu: ReadonlyMap<string, number>
}

// 프로세스 목록(수백 개)은 프로세스 페이지가 보일 때만 받는다(setProcessesWanted).
let processesWanted = false

const statsPoll = createSharedPoll({
  fetch: (signal, refresh) =>
    fetchAddon('system-info', isSystemStatsResponse, {
      signal,
      refresh,
      query: processesWanted ? { processes: '1' } : undefined,
    }),
  timeoutMs: DEFAULT_TIMEOUT_MS,
  warmupMs: SYSTEM_STATS_WARMUP_MS,
  schedule: workerScheduler,
})

const toSample = (prev: SystemStatsSample | null, response: SystemStatsResponse): SystemStatsSample => ({
  response,
  history: appendSample(prev?.history ?? null, response),
  processCpu: computeProcessCpu(prev?.response ?? null, response),
})

const statsRecorder = createRecorder(statsPoll, toSample)

// setProcessesWanted는 다음 요청부터 프로세스 목록을 받을지 정한다. 켜면 주기를 기다리지 않고 바로 한 번 가져온다
// (프로세스 CPU 값은 그다음 응답부터 나온다).
export function setProcessesWanted(on: boolean) {
  if (processesWanted === on) return
  processesWanted = on
  if (on) statsPoll.refresh()
}

// useSystemStats는 창이 열려 있는 동안 /api/addons/system-info를 기록한다. active(보이는 동안)일 때만 다시 그리고,
// 다시 보이면 그동안 쌓인 기록을 바로 받는다. 창을 닫으면(언마운트) 기록을 버린다.
export function useSystemStats(active: boolean): RecordedState<SystemStatsSample> {
  useEffect(() => statsRecorder.retain(SYSTEM_STATS_BACKGROUND_INTERVAL_MS), [])
  // 보이는 동안은 더 짧은 주기로 붙는다(공유 폴러는 가장 짧은 주기를 쓴다).
  useEffect(() => (active ? statsPoll.subscribe(SYSTEM_STATS_INTERVAL_MS, () => {}) : undefined), [active])
  const subscribe = useCallback((onChange: () => void) => (active ? statsRecorder.subscribe(onChange) : () => {}), [active])
  return useSyncExternalStore(subscribe, statsRecorder.get)
}
