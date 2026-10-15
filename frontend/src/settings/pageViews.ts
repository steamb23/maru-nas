// 설정 페이지 id별 화면 컴포넌트. 페이지 목록(pages.ts)과 따로 두어 pages.ts를 node 환경에서 테스트한다.
import type { ComponentType } from 'react'
import { AboutPage } from './AboutPage'
import { DefaultScreenPage } from './DefaultScreenPage'
import { GeneralPage } from './GeneralPage'
import { LoginScreenPage } from './LoginScreenPage'
import { ProfilePage } from './ProfilePage'
import { ScreenPage } from './ScreenPage'
import { ServerPage } from './ServerPage'
import type { SettingsPageId } from './pages'

export const SETTINGS_PAGE_VIEWS: Record<SettingsPageId, ComponentType> = {
  profile: ProfilePage,
  screen: ScreenPage,
  general: GeneralPage,
  'login-screen': LoginScreenPage,
  'default-screen': DefaultScreenPage,
  server: ServerPage,
  about: AboutPage,
}
