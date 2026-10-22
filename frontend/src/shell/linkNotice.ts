// 링크 앱을 열기 전에 띄우는 "새 탭에서 열립니다" 안내를 건너뛸지(다음에 알리지 않음).
// 브라우저별 편의 설정이라 쿠키에 둔다. DOM에 의존하지 않아 node 환경에서 테스트한다(document.cookie 문자열을 받는다).
import type { LinkAppId } from '../windows/apps'

// 쿠키를 두는 기간(초). 브라우저가 허용하는 최대(400일)보다 짧게 1년으로 둔다.
export const NOTICE_COOKIE_MAX_AGE = 365 * 24 * 60 * 60

export function noticeCookieName(id: LinkAppId): string {
  return `maru_link_notice_${id}`
}

// noticeSkipped는 cookie(document.cookie)에 그 앱의 안내를 끈 기록이 있는지다.
export function noticeSkipped(cookie: string, id: LinkAppId): boolean {
  const entry = `${noticeCookieName(id)}=off`
  return cookie.split(';').some((part) => part.trim() === entry)
}

// skipNoticeCookie는 안내를 끄는 쿠키(document.cookie에 넣을 문자열)다. Path는 주지 않아 대시보드 문서
// 경로 아래에 둔다(경로 접두사 아래에 두어도 그 대시보드에만 적용된다). HTTPS면 Secure를 붙인다.
export function skipNoticeCookie(id: LinkAppId, secure: boolean): string {
  return `${noticeCookieName(id)}=off; Max-Age=${NOTICE_COOKIE_MAX_AGE}; SameSite=Strict${secure ? '; Secure' : ''}`
}
