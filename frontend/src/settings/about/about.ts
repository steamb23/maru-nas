// 정보 페이지의 빌드 정보 표시 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { AboutResponse } from '../../api/types.gen'

// shortRevision은 git 커밋 해시를 git log --oneline처럼 앞 7자로 줄인다.
export function shortRevision(rev: string): string {
  return rev.slice(0, 7)
}

// formatRevisionTime은 커밋 시각을 화면 언어로 표시한다. 읽을 수 없으면 null.
export function formatRevisionTime(about: AboutResponse, locale: string, timeZone?: string): string | null {
  if (!about.revisionTime) return null
  const d = new Date(about.revisionTime)
  if (Number.isNaN(d.getTime())) return null
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short', timeZone }).format(d)
}
