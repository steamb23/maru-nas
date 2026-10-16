// 권한 상승(관리자 비밀번호 재확인) 계산(DOM 없음): 실패 문구 키, 남은 시간 표기.
// 대화상자는 elevationDialog.tsx, 설정의 도우미 절은 settings/HelperSection.tsx가 쓴다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'

export interface ElevationError {
  key: ParseKeys
  // rate_limited일 때 다시 시도할 수 있기까지 남은 초
  retryAfter?: number
}

// elevationError는 상승 요청 실패를 화면 문구 키로 바꾼다.
export function elevationError(err: unknown): ElevationError {
  if (!(err instanceof ApiError)) return { key: 'elevation.failed' }
  switch (err.code) {
    case 'elevation_failed':
      return { key: 'elevation.wrongPassword' }
    case 'rate_limited':
      return { key: 'elevation.rateLimited', retryAfter: err.retryAfterSeconds ?? 0 }
    case 'elevation_not_allowed':
    case 'admin_required':
      return { key: 'elevation.notAllowed' }
    case 'helper_reauth':
      return { key: 'elevation.reauth' }
    case 'helper_unavailable':
      return { key: 'elevation.unavailable' }
    default:
      return { key: 'elevation.failed' }
  }
}

// formatClock은 남은 초를 m:ss로 쓴다(숫자만이라 번역하지 않는다).
export function formatClock(seconds: number): string {
  const s = Math.max(0, Math.ceil(seconds))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}
