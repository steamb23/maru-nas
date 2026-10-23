// 알림의 동작(앱 열기, 워크벤치 화면 열기, 전원 대화상자)을 실행한다. 알림 센터와 토스트가 함께 쓴다.
import type { NoticeAction } from '../api/types.gen'
import { useCapabilitiesStore } from '../addons/capabilities'
import { useAuthStore } from '../auth/store'
import type { ActionContext } from '../notifications/model'
import { openPowerDialog } from '../power/powerDialog'
import { SETTINGS_PAGES, type SettingsPageId } from '../settings/pages'
import { type AppId, isAppId } from '../windows/apps'
import { openWorkbenchRoute } from './launchApp'

export interface NoticeActionDeps {
  // openApp·openSettings는 창 위치 계산에 데스크톱 크기가 필요해 Desktop이 넘긴다.
  openApp: (id: AppId) => void
  openSettings: (page: SettingsPageId) => void
}

const isSettingsPage = (page: string): page is SettingsPageId => SETTINGS_PAGES.some((p) => p.id === page)

// runNoticeAction은 action을 실행한다. 워크벤치 탭은 팝업 차단을 피하려고 누르기 처리 안에서 바로 연다.
export function runNoticeAction(action: NoticeAction, deps: NoticeActionDeps): void {
  switch (action.kind) {
    case 'app':
      if (action.app === 'settings' && action.page && isSettingsPage(action.page)) deps.openSettings(action.page)
      else if (action.app && isAppId(action.app)) deps.openApp(action.app)
      return
    case 'workbench':
      openWorkbenchRoute(action.route ?? '')
      return
    case 'power':
      openPowerDialog()
  }
}

// useNoticeActionContext는 지금 보일 수 있는 동작을 가리는 데 쓰는 값이다(notifications/model.ts의 actionAvailable).
export function useNoticeActionContext(): ActionContext {
  const admin = useAuthStore((s) => s.user?.admin ?? false)
  const workbench = useCapabilitiesStore((s) => Boolean(s.caps?.workbenchUrl))
  return { admin, workbench, isApp: isAppId }
}
