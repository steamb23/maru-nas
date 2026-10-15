// 링크 앱(창 없이 브라우저 탭으로 여는 앱)의 주소. DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { CapabilitiesResponse } from '../api/types.gen'
import type { LinkAppId } from './apps'

// 링크 앱마다 여는 브라우저 탭 이름. 같은 이름으로 열면 이미 열린 탭을 다시 써서 새 탭이 쌓이지 않고,
// 그 탭의 OMV 로그인(탭마다 따로다)도 유지된다.
export const LINK_TAB_NAMES: Record<LinkAppId, string> = {
  workbench: 'onmaru-omv-workbench',
}

// linkConfigured는 링크 앱의 주소가 설정되어 있는지다. capability를 아직 받지 못했으면 주소를 모르므로 false.
export function linkConfigured(id: LinkAppId, caps: CapabilitiesResponse | null): boolean {
  switch (id) {
    case 'workbench':
      return Boolean(caps?.workbenchUrl)
  }
}

// linkHref는 링크 앱을 열 주소다. 설정이 없거나 해석할 수 없으면 null.
export function linkHref(id: LinkAppId, caps: CapabilitiesResponse | null, documentBase: string): string | null {
  switch (id) {
    case 'workbench':
      return workbenchHref(caps?.workbenchUrl, documentBase)
  }
}

// workbenchHref는 OMV 웹 UI를 열 주소다. configured는 백엔드 설정(-workbench-url) 그대로이며,
// 경로 참조(/, ../)는 대시보드 문서 주소(documentBase) 기준으로 해석한다.
// 경로 끝 슬래시를 보정하고 OMV 라우트(#/<route>, 기본은 첫 화면 #/)를 붙인다. OMV는 hash 라우터라, 이미 열린
// OMV 탭을 다시 열 때 경로가 같으면 새로 고침 없이 그 라우트로만 이동한다. #/는 역할과 상관없이 열리는 기본 라우트다.
export function workbenchHref(configured: string | undefined, documentBase: string, route = ''): string | null {
  if (!configured) return null
  let url: URL
  try {
    url = new URL(configured, documentBase)
  } catch {
    return null
  }
  if (url.protocol !== 'http:' && url.protocol !== 'https:') return null
  if (!url.pathname.endsWith('/')) url.pathname += '/'
  url.search = ''
  url.hash = `/${route}`
  return url.href
}

// userEditRoute는 OMV 사용자 관리의 사용자 편집 라우트다(관리자 전용 화면).
export function userEditRoute(username: string): string {
  return `usermgmt/users/edit/${encodeURIComponent(username)}`
}
