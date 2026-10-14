// 애드온 조작(시작·정지 등)의 화면 상태. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 대상(컨테이너 이름 등)마다 진행 상태를 두고, 확인은 한 번에 하나만 연다. 확인은 행 버튼·우클릭 메뉴 어디서
// 열든 창 범위 모달(ActionConfirmDialog)로 보여 준다.
import type { ErrorCode } from '../../api/client'

// TargetState는 대상 하나의 상태다.
//   - running: 요청을 보내고 기다리는 중
//   - settling: 성공했고 다음 현황 응답을 기다리는 중(그 전까지는 옛 상태가 보이므로 계속 잠가 둔다)
//   - error: 마지막 조작이 실패했다
export type TargetState =
  | { kind: 'running'; action: string }
  | { kind: 'settling'; action: string }
  | { kind: 'error'; action: string; code: ActionErrorCode }

// ActionErrorCode는 화면에 보여 줄 실패 이유다. 백엔드 코드가 없으면(연결 실패 등) unknown이다.
export type ActionErrorCode = ErrorCode | 'unknown'

export interface PendingConfirm {
  target: string
  action: string
}

export interface ActionState {
  targets: Readonly<Record<string, TargetState>>
  confirm: PendingConfirm | null
}

export const INITIAL_ACTION_STATE: ActionState = { targets: {}, confirm: null }

// isBusy는 대상에 진행 중인 조작이 있어 새 조작을 막아야 하는지다.
export function isBusy(s: ActionState, target: string): boolean {
  const t = s.targets[target]
  return t?.kind === 'running' || t?.kind === 'settling'
}

// askConfirm은 확인을 연다. 이미 열린 확인은 닫고 바꾼다. 진행 중인 대상이면 바꾸지 않는다.
export function askConfirm(s: ActionState, c: PendingConfirm): ActionState {
  if (isBusy(s, c.target)) return s
  return { ...s, confirm: c }
}

export function cancelConfirm(s: ActionState): ActionState {
  return s.confirm ? { ...s, confirm: null } : s
}

// start는 조작을 시작한다. 그 대상의 확인과 이전 오류는 지운다.
export function start(s: ActionState, target: string, action: string): ActionState {
  if (isBusy(s, target)) return s
  return {
    targets: { ...s.targets, [target]: { kind: 'running', action } },
    confirm: s.confirm?.target === target ? null : s.confirm,
  }
}

// finish는 조작 결과를 반영한다. 성공이면 다음 현황 응답까지 settling으로 둔다.
export function finish(s: ActionState, target: string, error: ActionErrorCode | null): ActionState {
  const t = s.targets[target]
  if (t?.kind !== 'running') return s
  const next: TargetState = error ? { kind: 'error', action: t.action, code: error } : { kind: 'settling', action: t.action }
  return { ...s, targets: { ...s.targets, [target]: next } }
}

// settle은 새 현황 응답을 받았을 때 settling 대상을 풀어 준다.
export function settle(s: ActionState): ActionState {
  const entries = Object.entries(s.targets)
  if (!entries.some(([, t]) => t.kind === 'settling')) return s
  return { ...s, targets: Object.fromEntries(entries.filter(([, t]) => t.kind !== 'settling')) }
}

// dismiss는 대상의 오류 표시를 지운다.
export function dismiss(s: ActionState, target: string): ActionState {
  if (s.targets[target]?.kind !== 'error') return s
  const targets = { ...s.targets }
  delete targets[target]
  return { ...s, targets }
}
