// 전원 모달 하나의 단계 상태. 제목(PowerTitle)과 본문(PowerBody)이 함께 보므로 대화상자마다 하나 만든다.
import type { ParseKeys } from 'i18next'
import { type StoreApi, type UseBoundStore, create } from 'zustand'
import type { PowerAction } from './power'

// PowerWhen은 다시 시작 확인 단계의 "지금"·"시간 정해서"다. 다른 조작에서는 쓰지 않는다.
export type PowerWhen = 'now' | 'later'

export interface PowerDialogState {
  // 확인 중인 조작. null이면 타일을 고르는 단계다.
  step: PowerAction | null
  when: PowerWhen
  // sending은 조작이나 예약 요청을 보내는 중이다. 그동안 단계를 바꾸지 못한다.
  sending: boolean
  error: ParseKeys | null
}

export type PowerDialogStore = UseBoundStore<StoreApi<PowerDialogState>>

export function createPowerDialogStore(init: Partial<Pick<PowerDialogState, 'step' | 'when'>> = {}): PowerDialogStore {
  return create<PowerDialogState>(() => ({ step: init.step ?? null, when: init.when ?? 'now', sending: false, error: null }))
}
