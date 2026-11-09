// 예약된 다시 시작·종료가 다가올 때의 경고 계산(DOM 없음): 10분 전 알림, 1분 전 카운트다운, 시각이 되면 전원 화면.
// 알림 저장소의 서버 알림(rebootScheduled·shutdownScheduled)과 서버 시계 차이로 정한다. 화면은 ScheduleWarnings.tsx다.
import type { Notice } from '../api/types.gen'

// SOON_MS 안으로 다가오면 알림을 띄우고, IMMINENT_MS 안이면 카운트다운을 보인다.
export const SOON_MS = 10 * 60_000
export const IMMINENT_MS = 60_000
// REFRESH_BEFORE_MS 전에 알림을 한 번 더 받아 예약이 그대로인지 본다(다른 곳에서 취소했을 수 있다).
export const REFRESH_BEFORE_MS = 5_000
// 시각이 DUE_GRACE_MS 넘게 지난 알림은 낡은 것으로 보고 전원 화면으로 가지 않는다(서버 알림은 30초 보관된다).
export const DUE_GRACE_MS = 60_000
// MAX_WAIT_MS는 단계를 다시 볼 때까지 가장 오래 기다리는 시간이다(잠자기·시계 변경 뒤에도 늦지 않게).
export const MAX_WAIT_MS = 30_000

export interface ScheduledShutdown {
  at: number
  reboot: boolean
  // by는 대시보드에서 예약한 사용자다. 대시보드 밖에서 예약했으면 없다.
  by?: string
  // revision은 그 예약 알림의 리비전이다. 경고를 예약마다 한 번만 띄우는 데 쓴다.
  revision: string
}

// scheduledShutdown은 서버 알림에서 예약된 다시 시작·종료를 찾는다. 시각을 읽을 수 없으면 null이다.
export function scheduledShutdown(notices: readonly Notice[]): ScheduledShutdown | null {
  const n = notices.find((x) => x.code === 'rebootScheduled' || x.code === 'shutdownScheduled')
  if (!n?.at) return null
  const at = Date.parse(n.at)
  if (Number.isNaN(at)) return null
  return { at, reboot: n.code === 'rebootScheduled', by: n.items?.[0], revision: n.revision }
}

// later: 아직 멀다. soon: 10분 안. imminent: 1분 안. due: 시각이 됨. past: 시각이 지난 지 오래된 낡은 알림
export type WarningPhase = 'later' | 'soon' | 'imminent' | 'due' | 'past'

// warningPhase는 남은 시간(ms)의 단계다.
export function warningPhase(left: number): WarningPhase {
  if (left <= -DUE_GRACE_MS) return 'past'
  if (left <= 0) return 'due'
  if (left <= IMMINENT_MS) return 'imminent'
  if (left <= SOON_MS) return 'soon'
  return 'later'
}

// nextCheck는 다음에 남은 시간을 다시 볼 때까지의 시간(ms)이다. 1분 안이면 표시하는 초가 바뀔 때, 아니면 다음
// 단계의 경계까지 기다리되 MAX_WAIT_MS를 넘지 않는다.
export function nextCheck(left: number): number {
  if (left <= IMMINENT_MS) {
    const toNextSecond = left > 0 ? left % 1000 || 1000 : 1000
    return Math.max(50, toNextSecond)
  }
  const boundary = left > SOON_MS ? left - SOON_MS : left - IMMINENT_MS
  return Math.min(MAX_WAIT_MS, Math.max(50, boundary))
}

// countdownText는 카운트다운 시계 문구다(초를 올림, 예: 0:59).
export function countdownText(left: number): string {
  const s = Math.max(0, Math.ceil(left / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// minutesLeft는 10분 전 알림에 쓰는 남은 분이다(올림).
export function minutesLeft(left: number): number {
  return Math.max(1, Math.ceil(left / 60_000))
}

// progress는 카운트다운 막대의 남은 비율(0~1)이다.
export function progress(left: number): number {
  return Math.min(1, Math.max(0, left / IMMINENT_MS))
}
