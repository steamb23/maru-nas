import { describe, expect, it } from 'vitest'
import { NOTICE_COOKIE_MAX_AGE, noticeSkipped, skipNoticeCookie } from './linkNotice'

describe('linkNotice', () => {
  it('안내를 끈 쿠키가 있을 때만 건너뛴다', () => {
    expect(noticeSkipped('', 'workbench')).toBe(false)
    expect(noticeSkipped('maru_link_notice_workbench=off', 'workbench')).toBe(true)
    expect(noticeSkipped('a=1; maru_link_notice_workbench=off; b=2', 'workbench')).toBe(true)
    expect(noticeSkipped('maru_link_notice_workbench=on', 'workbench')).toBe(false)
    expect(noticeSkipped('xmaru_link_notice_workbench=off', 'workbench')).toBe(false)
  })

  it('끄는 쿠키는 1년 동안 두고 HTTPS면 Secure를 붙인다', () => {
    expect(skipNoticeCookie('workbench', false)).toBe(
      `maru_link_notice_workbench=off; Max-Age=${NOTICE_COOKIE_MAX_AGE}; SameSite=Strict`,
    )
    expect(skipNoticeCookie('workbench', true)).toMatch(/; Secure$/)
  })
})
