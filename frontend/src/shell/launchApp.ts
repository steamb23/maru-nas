// 앱 열기. 창 앱은 창 저장소에 창을 열고, 링크 앱은 이름 붙은 브라우저 탭으로 연다.
// 팝업 차단을 피하려고 사용자 입력 처리기(클릭, 메뉴 선택) 안에서 기다리지 않고 바로 부른다.
import { useCapabilitiesStore } from '../addons/capabilities'
import { type AppId, type LinkAppId, isLinkApp } from '../windows/apps'
import type { Size } from '../windows/geometry'
import { LINK_TAB_NAMES, linkHref } from '../windows/links'
import { useWindowStore } from '../windows/store'

// openLink는 링크 앱을 그 앱의 이름 붙은 탭으로 연다. 주소를 모르면(설정 없음) 아무것도 하지 않는다.
// noopener를 주면 브라우저가 이름으로 기존 탭을 찾지 않고 늘 새 탭을 만들므로 주지 않는다.
// 열린 탭은 opener로 대시보드 탭을 가리키지만 상대는 같은 NAS의 OMV 웹 UI다.
export function openLink(id: LinkAppId): void {
  const href = linkHref(id, useCapabilitiesStore.getState().caps, document.baseURI)
  if (href) window.open(href, LINK_TAB_NAMES[id])
}

export function launchApp(id: AppId, bounds: Size): void {
  if (isLinkApp(id)) openLink(id)
  else useWindowStore.getState().open(id, bounds)
}
