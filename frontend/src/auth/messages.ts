// 로그인 화면 오류 문구의 번역 키. DOM·i18next에 의존하지 않아 node 환경에서 테스트한다.
// 문구가 아닌 키를 돌려주므로, 오류가 떠 있는 동안 언어를 바꿔도 화면이 새 언어로 다시 그린다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'

export interface LoginError {
  key: ParseKeys
  params?: Record<string, string | number>
  // 이 시간(초)이 지날 때까지 다시 시도할 수 없다.
  retryAfterSeconds?: number
}

// loginErrorMessage는 로그인·2차 인증 요청의 오류를 화면 문구의 키로 바꾼다.
export function loginErrorMessage(err: unknown): LoginError {
  if (!(err instanceof ApiError)) {
    return { key: 'login.error.network' }
  }
  switch (err.code) {
    case 'invalid_credentials':
      return { key: 'login.error.invalidCredentials' }
    case 'rate_limited':
      return { key: 'login.error.rateLimited', retryAfterSeconds: err.retryAfterSeconds }
    case 'challenge_failed':
      return { key: 'login.error.challengeFailed' }
    case 'challenge_expired':
      return { key: 'login.error.challengeExpired' }
    case 'omv_unavailable':
      return { key: 'login.error.omvUnavailable' }
    case 'bad_request':
      return { key: 'login.error.badRequest' }
    case 'unauthorized':
      return { key: 'login.error.unauthorized' }
  }
  return { key: 'login.error.httpStatus', params: { status: err.status } }
}
