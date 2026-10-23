// 프로필 메뉴 "전원" 모달의 계산(DOM 없음): 조작 목록, 절전을 쓸 수 있는지, 조작 실패 문구.
// 모달은 powerDialog.tsx·PowerOptions.tsx, 조작 뒤 감시와 화면은 watch.ts·PowerScreen.tsx다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'
import type { PowerInfo, SystemStatsResponse } from '../api/types.gen'
import type { IconName } from '../icons/resolver'

// PowerAction은 시스템 정보 애드온의 전원 조작 이름이다(backend sysinfo/power.go).
export type PowerAction = 'shutdown' | 'reboot' | 'standby'

// POWER_ADDON은 전원 조작과 절전 방식을 가진 애드온이다.
export const POWER_ADDON = 'system-info'

// POWER_SOURCE는 절전 방식을 읽는 provider 이름이다(?power=1).
export const POWER_SOURCE = 'omv-power'

export interface PowerActionSpec {
  id: PowerAction
  labelKey: ParseKeys
  icon: IconName
  // 확인 단계의 제목과 설명
  confirmTitleKey: ParseKeys
  confirmMessageKey: ParseKeys
  // 타일을 위험 색으로 그린다(시스템 종료).
  danger: boolean
}

// POWER_ACTIONS는 모달의 타일 순서다.
export const POWER_ACTIONS: readonly PowerActionSpec[] = [
  {
    id: 'shutdown',
    labelKey: 'power.shutdown',
    icon: 'system-shutdown-symbolic',
    confirmTitleKey: 'power.confirm.shutdownTitle',
    confirmMessageKey: 'power.confirm.shutdownMessage',
    danger: true,
  },
  {
    id: 'reboot',
    labelKey: 'power.reboot',
    icon: 'system-reboot-symbolic',
    confirmTitleKey: 'power.confirm.rebootTitle',
    confirmMessageKey: 'power.confirm.rebootMessage',
    danger: false,
  },
  {
    id: 'standby',
    labelKey: 'power.standby',
    icon: 'system-suspend-symbolic',
    confirmTitleKey: 'power.confirm.standbyTitle',
    confirmMessageKey: 'power.confirm.standbyMessage',
    danger: false,
  },
]

export function powerActionSpec(id: PowerAction): PowerActionSpec {
  return POWER_ACTIONS.find((a) => a.id === id) ?? POWER_ACTIONS[0]
}

// 대기 방식의 표시 이름. 이 밖의 방식(suspendhybrid)은 OMV가 지원한다고 알리지 않는다.
const MODE_KEYS: Partial<Record<string, ParseKeys>> = {
  suspend: 'power.mode.suspend',
  hibernate: 'power.mode.hibernate',
}

// SleepState는 절전 타일의 상태다.
//   - loading: 절전 방식을 받는 중
//   - available: 쓸 수 있다. modeKey는 대기 방식의 표시 이름(모르는 방식이면 없음)
//   - unavailable: 쓸 수 없다. reasonKey는 그 이유
export type SleepState =
  | { kind: 'loading' }
  | { kind: 'available'; modeKey?: ParseKeys }
  | { kind: 'unavailable'; reasonKey: ParseKeys }

// sleepState는 받은 절전 방식으로 절전 타일의 상태를 정한다. OMV의 System.standby는 설정된 대기 방식을
// 따르고, 그 방식을 이 기기가 지원하지 않으면 전원을 끈다. 그래서 대기 방식이 전원 끄기이거나 지원 목록에
// 없으면 쓸 수 없게 둔다. 받지 못했으면(failed) 확인할 수 없으므로 쓸 수 없다.
export function sleepState(info: PowerInfo | undefined, failed: boolean): SleepState {
  if (failed) return { kind: 'unavailable', reasonKey: 'power.sleepUnknown' }
  if (!info) return { kind: 'loading' }
  if (info.standbyMode === 'poweroff') return { kind: 'unavailable', reasonKey: 'power.sleepPoweroff' }
  if (!info.standbyModes.includes(info.standbyMode)) return { kind: 'unavailable', reasonKey: 'power.sleepUnsupported' }
  return { kind: 'available', modeKey: MODE_KEYS[info.standbyMode] }
}

// powerInfoOf는 ?power=1 응답에서 절전 방식을 꺼낸다. 출처가 실패했거나 빠졌으면 failed다.
export function powerInfoOf(res: SystemStatsResponse): { info?: PowerInfo; failed: boolean } {
  if (res.power && res.sources.includes(POWER_SOURCE)) return { info: res.power, failed: false }
  return { failed: true }
}

// 조작 실패 코드별 문구. 애드온 조작의 공통 문구(actions.error)를 쓴다.
const ERROR_KEYS: Partial<Record<string, ParseKeys>> = {
  action_busy: 'power.error.busy',
  action_timeout: 'actions.error.action_timeout',
  admin_required: 'actions.error.admin_required',
  omv_error: 'power.error.omv',
  omv_unavailable: 'actions.error.omv_error',
}

// sendOutcome은 조작 요청의 실패를 판단한다. 백엔드가 오류 코드와 함께 거절했으면 그 문구 키를 돌려준다.
// 그 밖(연결 끊김, 프록시의 5xx 오류 페이지)은 시스템이 내려가며 응답이 끊긴 것으로 보고 'sent'다.
// 401은 API 클라이언트가 로그인 화면으로 보내므로 여기서는 실패 문구만 정한다.
export function sendOutcome(err: unknown): 'sent' | ParseKeys {
  if (!(err instanceof ApiError)) return 'sent'
  if (!err.code) return err.status >= 500 ? 'sent' : 'actions.error.unknown'
  return ERROR_KEYS[err.code] ?? 'actions.error.unknown'
}
