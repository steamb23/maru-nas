// 세션 유지·만료 판단. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// OMV 세션은 사용자 조작이 없으면 자동 로그아웃 시간(OMV 웹 관리 설정의 timeout)이 지나 만료된다.
// 주기 요청(시스템 정보 갱신 등)은 세션을 연장하지 않으므로, 이 모듈이 언제 연장 신호
// (/api/session/activity)를 보내고 언제 만료를 확인(/api/session)할지 정한다.
//
// - 입력이 있으면 ACTIVITY_INTERVAL_MS에 한 번 연장 신호를 보낸다(요청 수를 줄이기 위한 묶음).
// - 묶음 때문에 아직 보내지 않은 입력이 있으면, 만료 RENEW_MARGIN_MS 전에는 반드시 보낸다.
//   그래서 timeout이 묶음 간격보다 짧아도(예: 1분) 조작하는 동안에는 만료되지 않는다.
// - 입력 없이 만료 시각이 지나면 CONFIRM_GRACE_MS 뒤에 백엔드에 확인한다. 바로 로그아웃하지
//   않는 이유는 다른 탭의 조작으로 세션이 연장됐을 수 있어서다(확인 응답으로 새 만료 시각을 받는다).

export const ACTIVITY_INTERVAL_MS = 60_000
// 만료 이만큼 전에 밀린 입력을 보낸다. 요청이 OMV에 닿는 시간과 타이머 지연을 감안한 여유다.
export const RENEW_MARGIN_MS = 10_000
// 예상 만료 시각 이만큼 뒤에 확인한다. 예상 시각은 OMV보다 약간 이르게 잡히므로 여유를 둔다.
export const CONFIRM_GRACE_MS = 2_000
// 확인 요청이 네트워크 오류로 실패하면 이만큼 뒤에 다시 확인한다.
export const CONFIRM_RETRY_MS = 10_000

export interface KeeperState {
  // OMV 세션의 예상 만료 시각(ms). 모르거나 자동 로그아웃이 꺼져 있으면 null.
  expiresAt: number | null
  // 마지막 사용자 입력 시각(ms). 입력이 없었으면 -Infinity.
  lastInputAt: number
  // 마지막으로 연장 신호를 보낸 시각(ms). 로그인·복원 시점으로 시작한다.
  lastSentAt: number
}

// renew: 지금 연장 신호를 보낸다. confirm: 지금 만료를 확인한다.
// wait: at(ms)에 다시 판단한다. idle: 입력이 올 때까지 할 일이 없다.
export type KeeperAction =
  | { kind: 'renew' }
  | { kind: 'confirm' }
  | { kind: 'wait'; at: number }
  | { kind: 'idle' }

// decide는 지금(now) 할 일을 정한다.
export function decide(s: KeeperState, now: number): KeeperAction {
  const pending = s.lastInputAt > s.lastSentAt
  const throttleEnds = s.lastSentAt + ACTIVITY_INTERVAL_MS
  const renewBy = s.expiresAt === null ? Infinity : s.expiresAt - RENEW_MARGIN_MS

  if (pending && (now >= throttleEnds || now >= renewBy)) return { kind: 'renew' }
  if (s.expiresAt === null) {
    return pending ? { kind: 'wait', at: throttleEnds } : { kind: 'idle' }
  }
  const confirmAt = s.expiresAt + CONFIRM_GRACE_MS
  if (now >= confirmAt) return { kind: 'confirm' }
  return { kind: 'wait', at: pending ? Math.min(throttleEnds, renewBy) : confirmAt }
}
