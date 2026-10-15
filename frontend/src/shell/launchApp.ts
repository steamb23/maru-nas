// 앱 열기. 창 앱은 창 저장소에 창을 열고, 링크 앱은 이름 붙은 브라우저 탭으로 연다.
// 팝업 차단을 피하려고 사용자 입력 처리기(클릭, 메뉴 선택) 안에서 기다리지 않고 바로 부른다.
// 안내 대화상자를 거치면 그 "열기" 클릭 안에서 연다.
import { useCapabilitiesStore } from '../addons/capabilities'
import { type AppId, type LinkAppId, isLinkApp } from '../windows/apps'
import type { Size } from '../windows/geometry'
import { LINK_TAB_NAMES, linkHref, workbenchHref } from '../windows/links'
import { useWindowStore } from '../windows/store'
import { confirmLinkOpen } from './linkNoticeDialog'

// openLink는 링크 앱을 그 앱의 이름 붙은 탭으로 연다. 주소를 모르면(설정 없음) 아무것도 하지 않는다.
// 먼저 새 탭에서 열린다고 알리고(다음에 알리지 않음을 고르지 않았으면, linkNoticeDialog.tsx) "열기"를 눌러야 연다.
// noopener를 주면 브라우저가 이름으로 기존 탭을 찾지 않고 늘 새 탭을 만들므로 주지 않는다.
// 열린 탭은 opener로 대시보드 탭을 가리키지만 상대는 같은 NAS의 OMV 웹 UI다.
export function openLink(id: LinkAppId): void {
  const href = linkHref(id, useCapabilitiesStore.getState().caps, document.baseURI)
  if (href) confirmLinkOpen(id, () => window.open(href, LINK_TAB_NAMES[id]))
}

// openWorkbenchRoute는 OMV 워크벤치의 특정 화면(route)을 워크벤치 앱과 같은 이름 붙은 탭으로 연다.
// 열기 전 안내와 팝업 차단 회피는 openLink와 같다.
export function openWorkbenchRoute(route: string): void {
  const href = workbenchHref(useCapabilitiesStore.getState().caps?.workbenchUrl, document.baseURI, route)
  if (href) confirmLinkOpen('workbench', () => window.open(href, LINK_TAB_NAMES.workbench))
}

export function launchApp(id: AppId, bounds: Size): void {
  if (isLinkApp(id)) openLink(id)
  else useWindowStore.getState().open(id, bounds)
}
