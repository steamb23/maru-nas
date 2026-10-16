// 설정의 권한 도우미 절 계산(DOM 없음): 상태 문구 키, 상승 남은 시간.
import type { ParseKeys } from 'i18next'
import type { HelperIdentity, HelperStatus } from '../api/types.gen'

type State = HelperStatus['state']

export const HELPER_STATE_KEYS: Record<State, ParseKeys> = {
  disabled: 'settings.helper.state.disabled',
  unavailable: 'settings.helper.state.unavailable',
  none: 'settings.helper.state.none',
  reauth: 'settings.helper.state.reauth',
  active: 'settings.helper.state.active',
}

export const HELPER_STATE_DESC_KEYS: Record<State, ParseKeys> = {
  disabled: 'settings.helper.stateDesc.disabled',
  unavailable: 'settings.helper.stateDesc.unavailable',
  none: 'settings.helper.stateDesc.none',
  reauth: 'settings.helper.stateDesc.reauth',
  active: 'settings.helper.stateDesc.active',
}

// elevationEndsAt은 상승이 끝나는 시각(ms)이다. 상승하지 않았으면 null이다.
// 서버는 시계 차이를 피하려고 남은 초를 주므로, 받은 시각(now) 기준으로 바꿔 둔다.
export function elevationEndsAt(status: HelperStatus, now: number): number | null {
  return status.state === 'active' && status.elevatedSeconds > 0 ? now + status.elevatedSeconds * 1000 : null
}

// secondsLeft는 endsAt까지 남은 초다(올림). 끝났거나 상승하지 않았으면 0이다.
export function secondsLeft(endsAt: number | null, now: number): number {
  return endsAt === null ? 0 : Math.max(0, Math.ceil((endsAt - now) / 1000))
}

// groupNames는 그룹 이름을 기본 그룹부터 쉼표로 잇는다.
export function groupNames(id: HelperIdentity): string {
  return id.groups.map((g) => g.name).join(', ')
}
