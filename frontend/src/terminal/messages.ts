// 터미널이 닫힌 이유(status closed)를 안내 문구로 바꾼다. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import type { TerminalServerMessage } from '../api/types.gen'

export interface CloseNotice {
  key: ParseKeys
  params?: Record<string, string | number>
  // 다시 연결 단추를 보일지. 세션이 끝났거나 관리자가 껐으면 다시 연결해도 소용없다.
  reconnect: boolean
}

// closeNotice는 닫힘 메시지의 안내다. asked는 이 접속에서 비밀번호 등을 물었는지다(물었는데 실패했으면
// 비밀번호가 틀린 것이고, 묻지도 못했으면 sshd가 이 계정에 쓸 수 있는 인증 방법이 없는 것이다).
export function closeNotice(msg: TerminalServerMessage, asked: boolean): CloseNotice {
  switch (msg.reason) {
    case 'exit':
      return { key: 'terminal.closed.exit', params: { code: msg.exitCode ?? 0 }, reconnect: true }
    case 'session_ended':
      return { key: 'terminal.closed.sessionEnded', reconnect: false }
    case 'disabled':
      return { key: 'terminal.closed.disabled', reconnect: false }
    case 'connection_lost':
      return { key: 'terminal.closed.connectionLost', reconnect: true }
    case 'ssh_unavailable':
      return { key: 'terminal.closed.unavailable', reconnect: true }
    case 'ssh_host_key':
      return { key: 'terminal.closed.hostKey', reconnect: true }
    case 'ssh_auth_failed':
      return { ...authFailedNotice(msg, asked), reconnect: true }
    case 'canceled':
      return { key: 'terminal.closed.canceled', reconnect: true }
    case 'too_many_windows':
      return { key: 'terminal.closed.tooMany', reconnect: true }
    case 'busy':
      return { key: 'terminal.closed.busy', reconnect: true }
    case 'rate_limited':
      return { key: 'terminal.closed.rateLimited', params: { seconds: msg.retryAfterSeconds ?? 0 }, reconnect: true }
    default:
      return { key: 'terminal.closed.failed', reconnect: true }
  }
}

function authFailedNotice(msg: TerminalServerMessage, asked: boolean): Omit<CloseNotice, 'reconnect'> {
  if (msg.notAllowedGroups?.length) {
    return { key: 'terminal.closed.authGroups', params: { groups: msg.notAllowedGroups.join(', ') } }
  }
  if (asked) return { key: 'terminal.closed.authFailed' }
  // 묻지도 못했다: 비밀번호를 받지 않는 sshd에서 임시 키로 들어가지 못했다.
  switch (msg.keyCode) {
    case 'ssh_strict_modes':
      return { key: 'terminal.closed.authStrictModes' }
    case 'no_home':
      return { key: 'terminal.closed.authNoHome' }
    case 'key_rejected':
      return { key: 'terminal.closed.authKeyRejected' }
    case undefined:
      return { key: 'terminal.closed.authNoHelper' }
    default:
      return { key: 'terminal.closed.authKeyFailed' }
  }
}
