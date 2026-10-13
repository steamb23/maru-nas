// 로그인 화면 오류 문구. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { ApiError } from '../api/client'

export interface LoginError {
  message: string
  // 이 시간(초)이 지날 때까지 다시 시도할 수 없다.
  retryAfterSeconds?: number
}

// loginErrorMessage는 로그인·2차 인증 요청의 오류를 화면 문구로 바꾼다.
export function loginErrorMessage(err: unknown): LoginError {
  if (!(err instanceof ApiError)) {
    return { message: '서버에 연결할 수 없습니다. 네트워크 상태를 확인하세요.' }
  }
  switch (err.code) {
    case 'invalid_credentials':
      return { message: '사용자 이름 또는 비밀번호가 올바르지 않습니다.' }
    case 'rate_limited':
      return {
        message: '로그인 시도가 너무 많습니다. 잠시 후 다시 시도하세요.',
        retryAfterSeconds: err.retryAfterSeconds,
      }
    case 'challenge_failed':
      return { message: '인증 코드가 올바르지 않습니다.' }
    case 'challenge_expired':
      return { message: '인증 시간이 지났습니다. 다시 로그인하세요.' }
    case 'omv_unavailable':
      return { message: 'OpenMediaVault에 연결할 수 없습니다.' }
    case 'bad_request':
      return { message: '사용자 이름과 비밀번호를 입력하세요.' }
  }
  return { message: `로그인하지 못했습니다 (HTTP ${err.status}).` }
}
