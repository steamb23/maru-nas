// 설정 앱의 페이지 목록. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 사이드바는 이 순서대로 그룹을 묶어 그린다. 페이지를 더하려면 여기에 항목을, pageViews.ts에 화면 컴포넌트를 넣는다
// (빠뜨리면 tsc가 잡는다). 나중에 OMV·시스템 설정을 넣게 되면 그룹을 더한다.
import type { ParseKeys } from 'i18next'
import type { IconName } from '../icons/resolver'

export type SettingsPageId =
  | 'profile'
  | 'screen'
  | 'general'
  | 'security'
  | 'login-screen'
  | 'default-screen'
  | 'server'
  | 'security-policy'
  | 'update'
  | 'about'

export type SettingsGroupId = 'account' | 'user' | 'admin' | 'info'

export interface SettingsPageDef {
  id: SettingsPageId
  labelKey: ParseKeys
  icon: IconName
  group: SettingsGroupId
  // admin이면 관리자에게만 보인다. 전역에 적용되는 설정이다(백엔드도 관리자가 아니면 403으로 막는다).
  requires?: 'admin'
  // card면 사이드바 목록에 넣지 않고 사이드바 위 사용자 카드로 연다(프로필).
  nav?: 'card'
}

// 그룹 머리글의 번역 키. null이면 머리글 없이 구분선만 둔다.
export const SETTINGS_GROUP_LABELS: Record<SettingsGroupId, ParseKeys | null> = {
  account: null,
  user: 'settings.groups.user',
  admin: 'settings.groups.admin',
  info: null,
}

export const SETTINGS_PAGES: readonly SettingsPageDef[] = [
  {
    id: 'profile',
    labelKey: 'settings.pages.profile',
    icon: 'avatar-default-symbolic',
    group: 'account',
    nav: 'card',
  },
  {
    id: 'screen',
    labelKey: 'settings.pages.screen',
    icon: 'preferences-desktop-appearance-symbolic',
    group: 'user',
  },
  { id: 'general', labelKey: 'settings.pages.general', icon: 'preferences-system-symbolic', group: 'user' },
  { id: 'security', labelKey: 'settings.pages.security', icon: 'channel-secure-symbolic', group: 'user' },
  {
    id: 'login-screen',
    labelKey: 'settings.pages.loginScreen',
    icon: 'system-lock-screen-symbolic',
    group: 'admin',
    requires: 'admin',
  },
  {
    id: 'default-screen',
    labelKey: 'settings.pages.defaultScreen',
    icon: 'system-users-symbolic',
    group: 'admin',
    requires: 'admin',
  },
  {
    id: 'server',
    labelKey: 'settings.pages.server',
    icon: 'network-server-symbolic',
    group: 'admin',
    requires: 'admin',
  },
  {
    id: 'security-policy',
    labelKey: 'settings.pages.securityPolicy',
    icon: 'security-high-symbolic',
    group: 'admin',
    requires: 'admin',
  },
  {
    id: 'update',
    labelKey: 'settings.pages.update',
    icon: 'software-update-available-symbolic',
    group: 'admin',
    requires: 'admin',
  },
  { id: 'about', labelKey: 'settings.pages.about', icon: 'help-about-symbolic', group: 'info' },
]

// 설정 창을 처음 열 때와 프로필 메뉴 머리글로 열 때의 페이지.
export const DEFAULT_SETTINGS_PAGE: SettingsPageId = 'screen'

// 설정 창 너비가 이보다 좁으면 사이드바를 숨기고 메뉴 단추로 내용 위에 덮어 연다(SettingsWindow.css와 맞춘다).
export const SETTINGS_NARROW_WIDTH = 640

export interface PageContext {
  admin: boolean
}

// visiblePages는 이 사용자에게 보일 페이지다.
export function visiblePages(ctx: PageContext): SettingsPageDef[] {
  return SETTINGS_PAGES.filter((p) => p.requires !== 'admin' || ctx.admin)
}

export interface PageGroup {
  id: SettingsGroupId
  labelKey: ParseKeys | null
  pages: SettingsPageDef[]
}

// groupPages는 사이드바 목록에 넣을 페이지를 이어진 같은 그룹끼리 묶는다. 페이지가 없는 그룹은 생기지 않고,
// 사용자 카드로 여는 페이지(nav: 'card')는 넣지 않는다.
export function groupPages(pages: readonly SettingsPageDef[]): PageGroup[] {
  const groups: PageGroup[] = []
  for (const p of pages) {
    if (p.nav === 'card') continue
    const last = groups.at(-1)
    if (last && last.id === p.group) last.pages.push(p)
    else groups.push({ id: p.group, labelKey: SETTINGS_GROUP_LABELS[p.group], pages: [p] })
  }
  return groups
}

// resolvePage는 요청한 페이지가 보이면 그 페이지, 아니면(관리자가 아닌 사용자 등) 기본 페이지다.
export function resolvePage(requested: SettingsPageId, pages: readonly SettingsPageDef[]): SettingsPageDef {
  return pages.find((p) => p.id === requested) ?? pages.find((p) => p.id === DEFAULT_SETTINGS_PAGE) ?? pages[0]
}
