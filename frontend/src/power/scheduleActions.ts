// 다시 시작 예약·취소 요청. 권한 도우미의 root 연산이므로 상승하지 않았으면 비밀번호를 묻고 한 번 더 보낸다.
// 전원 모달(PowerSchedule.tsx), 1분 전 카운트다운(ScheduleWarnings.tsx), 시스템 업데이트 창이 함께 쓴다.
import { ApiError, cancelPowerSchedule, schedulePowerRestart } from '../api/client'
import type { HelperStatus, PowerScheduleResponse } from '../api/types.gen'
import { refreshNotifications } from '../notifications/useNotifications'
import type { DialogScope } from '../shell/dialogs'
import { requestElevation } from '../shell/elevationDialog'

// withElevation은 req를 보낸다. elevation_required면 scope에 권한 상승 대화상자를 열고 상승한 뒤 다시 보낸다.
// 묻기를 취소하면 null이다. onElevated는 상승한 뒤의 도우미 상태를 받는다.
export async function withElevation<T>(
  scope: DialogScope,
  req: () => Promise<T>,
  onElevated?: (helper: HelperStatus) => void,
): Promise<T | null> {
  try {
    return await req()
  } catch (err) {
    if (!(err instanceof ApiError && err.code === 'elevation_required')) throw err
    const helper = await requestElevation(scope)
    if (!helper) return null
    onElevated?.(helper)
    return await req()
  }
}

// scheduleRestart는 at(Unix ms)에 다시 시작하도록 예약하고 알림을 다시 받는다.
export async function scheduleRestart(
  scope: DialogScope,
  at: number,
  onElevated?: (helper: HelperStatus) => void,
): Promise<PowerScheduleResponse | null> {
  const res = await withElevation(scope, () => schedulePowerRestart(new Date(at).toISOString()), onElevated)
  if (res) refreshNotifications()
  return res
}

// cancelSchedule은 예약된 다시 시작·종료를 취소하고 알림을 다시 받는다.
export async function cancelSchedule(
  scope: DialogScope,
  onElevated?: (helper: HelperStatus) => void,
): Promise<PowerScheduleResponse | null> {
  const res = await withElevation(scope, cancelPowerSchedule, onElevated)
  if (res) refreshNotifications()
  return res
}
