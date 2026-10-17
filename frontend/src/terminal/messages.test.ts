import { describe, expect, it } from 'vitest'
import type { TerminalServerMessage } from '../api/types.gen'
import { closeNotice } from './messages'

const closed = (m: Partial<TerminalServerMessage>): TerminalServerMessage => ({ type: 'status', state: 'closed', ...m })

describe('closeNotice', () => {
  it('셸이 끝나면 종료 코드를 보이고 다시 연결할 수 있다', () => {
    expect(closeNotice(closed({ reason: 'exit', exitCode: 3 }), false)).toEqual({
      key: 'terminal.closed.exit',
      params: { code: 3 },
      reconnect: true,
    })
  })

  it('세션이 끝났거나 관리자가 껐으면 다시 연결 단추가 없다', () => {
    expect(closeNotice(closed({ reason: 'session_ended' }), false).reconnect).toBe(false)
    expect(closeNotice(closed({ reason: 'disabled' }), false).reconnect).toBe(false)
  })

  it('인증 실패는 이유에 맞는 안내를 고른다', () => {
    const auth = (m: Partial<TerminalServerMessage>, asked: boolean) =>
      closeNotice(closed({ reason: 'ssh_auth_failed', ...m }), asked).key
    expect(auth({ notAllowedGroups: ['root', '_ssh'] }, true)).toBe('terminal.closed.authGroups')
    expect(auth({ keyCode: 'ssh_strict_modes' }, true)).toBe('terminal.closed.authFailed')
    expect(auth({ keyCode: 'ssh_strict_modes' }, false)).toBe('terminal.closed.authStrictModes')
    expect(auth({ keyCode: 'no_home' }, false)).toBe('terminal.closed.authNoHome')
    expect(auth({ keyCode: 'helper_unavailable' }, false)).toBe('terminal.closed.authKeyFailed')
    expect(auth({}, false)).toBe('terminal.closed.authNoHelper')
    expect(closeNotice(closed({ reason: 'ssh_auth_failed', notAllowedGroups: ['_ssh'] }), false).params).toEqual({
      groups: '_ssh',
    })
  })

  it('실패 제한은 남은 시간을 넘긴다', () => {
    expect(closeNotice(closed({ reason: 'rate_limited', retryAfterSeconds: 30 }), true).params).toEqual({ seconds: 30 })
  })
})
