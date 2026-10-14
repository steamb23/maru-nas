// 애드온 조작(시작·정지 등)의 화면 상태. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 대상(컨테이너 이름 등)마다 진행 상태를 두고, 확인은 한 번에 하나만 연다. 확인은 행 버튼·우클릭 메뉴 어디서
// 열든 창 범위 모달(ActionConfirmDialog)로 보여 준다.
import type { ErrorCode } from '../../api/client'

// TargetState는 대상 하나의 상태다.
//   - running: 요청을 보내고 기다리는 중
//   - settling: 성공했고 현황 응답이 바뀐 상태를 보여 줄 때까지 기다리는 중(그 전까지는 옛 상태가 보이므로 계속
//     잠가 둔다). since는 성공한 시각(ms)이다
//   - error: 마지막 조작이 실패했다
export type TargetState =
  | { kind: 'running'; action: string }
  | { kind: 'settling'; action: string; since: number }
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

// finish는 조작 결과를 반영한다. 성공이면 now(ms)부터 settling으로 둔다.
export function finish(s: ActionState, target: string, error: ActionErrorCode | null, now: number): ActionState {
  const t = s.targets[target]
  if (t?.kind !== 'running') return s
  const next: TargetState = error
    ? { kind: 'error', action: t.action, code: error }
    : { kind: 'settling', action: t.action, since: now }
  return { ...s, targets: { ...s.targets, [target]: next } }
}

// SettleCheck는 새 현황 응답에서 대상이 조작의 결과 상태에 이르렀는지다. 대상이 목록에서 사라졌으면 true로 본다.
export type SettleCheck = (target: string, action: string) => boolean

// 결과 상태에 이르지 않아도 이만큼 지나면 잠금을 푼다. 그 뒤에도 다르면 실제 상태로 보여 준다.
export const SETTLE_TIMEOUT_MS = 15_000

// settle은 새 현황 응답을 받았을 때 settling 대상을 풀어 준다. done이 없으면 모두 풀고, 있으면 결과 상태에
// 이른 대상과 SETTLE_TIMEOUT_MS가 지난 대상만 푼다(서비스처럼 설정이 바뀐 뒤 실제 상태가 늦게 따라오는 경우).
export function settle(s: ActionState, now: number, done?: SettleCheck): ActionState {
  const entries = Object.entries(s.targets)
  const release = ([target, t]: [string, TargetState]) =>
    t.kind === 'settling' && (!done || done(target, t.action) || now - t.since >= SETTLE_TIMEOUT_MS)
  if (!entries.some(release)) return s
  return { ...s, targets: Object.fromEntries(entries.filter((e) => !release(e))) }
}

// dismiss는 대상의 오류 표시를 지운다.
export function dismiss(s: ActionState, target: string): ActionState {
  if (s.targets[target]?.kind !== 'error') return s
  const targets = { ...s.targets }
  delete targets[target]
  return { ...s, targets }
}
