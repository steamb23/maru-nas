// 설정 페이지 id별 화면 컴포넌트. 페이지 목록(pages.ts)과 따로 두어 pages.ts를 node 환경에서 테스트한다.
import type { ComponentType } from 'react'
import { PlaceholderWindow } from '../shell/PlaceholderWindow'
import { AboutPage } from './AboutPage'
import { GeneralPage } from './GeneralPage'
import { ScreenPage } from './ScreenPage'
import type { SettingsPageId } from './pages'

export const SETTINGS_PAGE_VIEWS: Record<SettingsPageId, ComponentType> = {
  screen: ScreenPage,
  general: GeneralPage,
  'login-screen': PlaceholderWindow,
  'default-screen': PlaceholderWindow,
  server: PlaceholderWindow,
  about: AboutPage,
}
