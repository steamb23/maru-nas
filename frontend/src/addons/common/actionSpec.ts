// 애드온 조작의 화면 정의와 우클릭 메뉴 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import type { IconName } from '../../icons/resolver'
import type { ContextMenuItem } from '../../shell/contextMenu'
import { type AddonId, MANIFESTS } from '../manifests.gen'
import { type ActionState, isBusy } from './actionState'

type ManifestOf<I extends AddonId> = (typeof MANIFESTS)[I]

// AddonActionName은 manifest가 선언한 조작 이름이다. 조작이 없는 애드온이면 never다.
export type AddonActionName<I extends AddonId> =
  ManifestOf<I> extends { actions: readonly { name: infer N extends string }[] } ? N : never

// ActionSpec은 조작 하나를 화면에 그리는 방법이다. 확인이 필요한지는 manifest의 confirm이 정한다.
export interface ActionSpec<A extends string = string> {
  action: A
  icon: IconName
  // 버튼·메뉴 항목 이름(정지)
  labelKey: ParseKeys
  // 진행 중 표시(정지 중…)
  runningKey: ParseKeys
  // 확인 문구. {{name}}에 대상 이름이 들어간다(web을(를) 정지할까요?).
  confirmKey: ParseKeys
  // 확인 단추를 위험 색으로 그린다(정지, 끄기).
  danger?: boolean
}

// needsConfirm은 manifest에서 그 조작이 실행 전 확인을 요구하는지 읽는다.
export function needsConfirm(id: AddonId, action: string): boolean {
  const actions = (MANIFESTS[id] as { actions?: readonly { name: string; confirm?: boolean }[] }).actions
  return actions?.find((a) => a.name === action)?.confirm === true
}

export interface ActionMenuHandlers {
  // 조작을 고른다. 확인이 필요하면 호출자가 확인 대화상자를 연다.
  select: (spec: ActionSpec) => void
}

// actionMenuItems는 대상 하나의 우클릭 메뉴 항목이다. 진행 중이면 모두 비활성이다.
export function actionMenuItems(
  state: ActionState,
  target: string,
  specs: readonly ActionSpec[],
  handlers: ActionMenuHandlers,
): ContextMenuItem[] {
  const busy = isBusy(state, target)
  return specs.map((spec) => ({
    kind: 'action',
    id: spec.action,
    labelKey: spec.labelKey,
    icon: spec.icon,
    disabled: busy,
    onSelect: () => handlers.select(spec),
  }))
}
