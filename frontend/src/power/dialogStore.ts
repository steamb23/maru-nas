// 전원 모달 하나의 단계 상태. 제목(PowerTitle)과 본문(PowerBody)이 함께 보므로 대화상자마다 하나 만든다.
import type { ParseKeys } from 'i18next'
import { type StoreApi, type UseBoundStore, create } from 'zustand'
import type { PowerAction } from './power'

export interface PowerDialogState {
  // 확인 중인 조작. null이면 타일을 고르는 단계다.
  step: PowerAction | null
  sending: boolean
  error: ParseKeys | null
}

export type PowerDialogStore = UseBoundStore<StoreApi<PowerDialogState>>

export function createPowerDialogStore(): PowerDialogStore {
  return create<PowerDialogState>(() => ({ step: null, sending: false, error: null }))
}
