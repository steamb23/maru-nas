// 전원 모달이 열릴 때 받는 다시 시작 예약 상태와 이 세션의 도우미 상태. 타일과 예약 화면(PowerSchedule.tsx)이 함께 쓴다.
import { useCallback, useEffect, useState } from 'react'
import { fetchHelperStatus, fetchPowerSchedule } from '../api/client'
import type { HelperStatus, PowerSchedule, PowerScheduleResponse } from '../api/types.gen'
import { clockSkew } from '../notifications/clock'

// ScheduleData는 모달을 열 때 받은 예약 상태와 도우미 상태다. 타일의 "예약됨"과 예약 화면이 함께 쓴다.
export interface ScheduleData {
  status: 'loading' | 'ready' | 'error'
  schedule?: PowerSchedule
  // serverNow·timeZone은 마지막 응답의 NAS 시각과 시간대다. skew는 서버 시계 - 이 브라우저 시계(ms)다.
  serverNow: string
  timeZone?: string
  skew: number
  helper: HelperStatus | null
}

export interface ScheduleState {
  data: ScheduleData
  // update는 예약·취소 응답으로 예약 상태를 바꾼다. setHelper는 상승한 뒤의 도우미 상태다.
  update: (res: PowerScheduleResponse) => void
  setHelper: (helper: HelperStatus) => void
}

function fromResponse(res: PowerScheduleResponse): Pick<ScheduleData, 'status' | 'schedule' | 'serverNow' | 'timeZone' | 'skew'> {
  return { status: 'ready', schedule: res.schedule, serverNow: res.now, timeZone: res.timeZone, skew: clockSkew(res.now, Date.now()) }
}

// useScheduleData는 모달을 열 때 지금 예약과 이 세션의 도우미 상태를 받는다.
export function useScheduleData(): ScheduleState {
  const [data, setData] = useState<ScheduleData>({ status: 'loading', serverNow: '', skew: 0, helper: null })
  useEffect(() => {
    const ctl = new AbortController()
    fetchPowerSchedule(ctl.signal).then(
      (res) => setData((d) => ({ ...d, ...fromResponse(res) })),
      () => {
        if (!ctl.signal.aborted) setData((d) => ({ ...d, status: 'error' }))
      },
    )
    fetchHelperStatus(ctl.signal).then(
      (helper) => setData((d) => ({ ...d, helper })),
      () => {},
    )
    return () => ctl.abort()
  }, [])
  const update = useCallback((res: PowerScheduleResponse) => setData((d) => ({ ...d, ...fromResponse(res) })), [])
  const setHelper = useCallback((helper: HelperStatus) => setData((d) => ({ ...d, helper })), [])
  return { data, update, setHelper }
}
