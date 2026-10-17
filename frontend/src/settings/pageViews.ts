// 설정 페이지 id별 화면 컴포넌트. 페이지 목록(pages.ts)과 따로 두어 pages.ts를 node 환경에서 테스트한다.
// 페이지는 창의 active(최소화·닫는 중·탭 숨김이면 false)를 받는다. 쓰지 않는 페이지는 props를 무시한다.
import type { ComponentType } from 'react'
import type { WindowViewProps } from '../addons/types'
import { AboutPage } from './AboutPage'
import { DefaultScreenPage } from './DefaultScreenPage'
import { GeneralPage } from './GeneralPage'
import { LoginScreenPage } from './LoginScreenPage'
import { ProfilePage } from './ProfilePage'
import { ScreenPage } from './ScreenPage'
import { SecurityPage } from './SecurityPage'
import { SecurityPolicyPage } from './SecurityPolicyPage'
import { ServerPage } from './ServerPage'
import type { SettingsPageId } from './pages'

export const SETTINGS_PAGE_VIEWS: Record<SettingsPageId, ComponentType<WindowViewProps>> = {
  profile: ProfilePage,
  screen: ScreenPage,
  general: GeneralPage,
  security: SecurityPage,
  'login-screen': LoginScreenPage,
  'default-screen': DefaultScreenPage,
  server: ServerPage,
  'security-policy': SecurityPolicyPage,
  about: AboutPage,
}
