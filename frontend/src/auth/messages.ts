// 로그인 화면 오류 문구의 번역 키. DOM·i18next에 의존하지 않아 node 환경에서 테스트한다.
// 문구가 아닌 키를 돌려주므로, 오류가 떠 있는 동안 언어를 바꿔도 화면이 새 언어로 다시 그린다.
import type { ParseKeys } from 'i18next'
import { ApiError } from '../api/client'
import { PasskeyLoginBrokenError, PrfUnsupportedError } from '../security/passkeyLoginFlow'

export interface LoginError {
  key: ParseKeys
  params?: Record<string, string | number>
  // 이 시간(초)이 지날 때까지 다시 시도할 수 없다.
  retryAfterSeconds?: number
}

// loginErrorMessage는 로그인·2차 인증·패스키로 로그인 요청의 오류를 화면 문구의 키로 바꾼다.
export function loginErrorMessage(err: unknown): LoginError {
  // 패스키 창의 결과(DOMException). 취소·시간 초과는 NotAllowedError로 온다.
  if (typeof DOMException !== 'undefined' && err instanceof DOMException) {
    const canceled = err.name === 'NotAllowedError' || err.name === 'AbortError'
    return { key: canceled ? 'login.error.passkeyCanceled' : 'login.error.passkeyFailed' }
  }
  if (err instanceof PrfUnsupportedError) return { key: 'login.error.passkeyPrf' }
  if (err instanceof PasskeyLoginBrokenError) return { key: 'login.error.passkeyBroken' }
  if (!(err instanceof ApiError)) {
    return { key: 'login.error.network' }
  }
  switch (err.code) {
    // 패스키로 로그인: 앞의 셋은 서버에 그 등록이 없어졌다는 뜻이다(브라우저 항목도 지운다).
    case 'passkey_login_unknown':
      return { key: 'login.error.passkeyUnknown' }
    case 'passkey_login_expired':
      return { key: 'login.error.passkeyExpired' }
    case 'passkey_login_stale':
      return { key: 'login.error.passkeyStale' }
    case 'factor_failed':
      return { key: 'login.error.passkeyFailed' }
    case 'passkey_unavailable':
      return { key: 'login.error.passkeyUnavailable' }
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
