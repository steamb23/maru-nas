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

// formatOMVVersion은 OMV 버전을 OMV 대시보드처럼 "버전 (코드명)"으로 보여 준다.
// 코드명이 없으면 버전만, 버전이 없으면 null(코드명만으로는 보여 주지 않는다).
export function formatOMVVersion(about: AboutResponse): string | null {
  if (!about.omvVersion) return null
  return about.omvVersionName ? `${about.omvVersion} (${about.omvVersionName})` : about.omvVersion
}
