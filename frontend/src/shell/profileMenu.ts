// 작업표시줄 프로필 메뉴 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 항목은 두 종류다. action 항목은 누르면 ProfileMenu.tsx의 actions에 있는 동작을 실행한다
// (ProfileMenuActionId에 추가하면 동작을 빠뜨렸을 때 타입 오류가 난다). submenu 항목은 누르면 하위 메뉴를 연다.
// 같은 group끼리 묶고 group 사이에 구분선을 그린다.
import type { ParseKeys } from 'i18next'
import type { IconName } from '../icons/resolver'

export type ProfileMenuActionId = 'workbench' | 'power' | 'logout'
export type ProfileMenuSubmenuId = 'language'

interface ItemBase {
  labelKey: ParseKeys
  icon: IconName
  group: string
  // admin이면 관리자에게만, workbench면 OMV 웹 UI 주소가 설정되어 있을 때만 보인다(MenuContext).
  requires?: keyof MenuContext
}

// MenuContext는 항목 표시 조건이다. 로그인한 사용자의 역할과 capability로 정한다.
export interface MenuContext {
  admin: boolean
  workbench: boolean
}

export type ProfileMenuItem = ItemBase &
  ({ kind: 'action'; id: ProfileMenuActionId } | { kind: 'submenu'; id: ProfileMenuSubmenuId })

export const PROFILE_MENU_ITEMS: readonly ProfileMenuItem[] = [
  // OMV 웹 UI를 이름 붙은 브라우저 탭으로 연다(shell/launchApp.ts의 openLink).
  {
    kind: 'action',
    id: 'workbench',
    labelKey: 'profile.openWorkbench',
    icon: 'openmediavault',
    group: 'omv',
    requires: 'workbench',
  },
  {
    kind: 'submenu',
    id: 'language',
    labelKey: 'language.label',
    icon: 'preferences-desktop-locale-symbolic',
    group: 'preferences',
  },
  // 시스템 종료·다시 시작·절전 모달을 연다(power/powerDialog.tsx). OMV의 전원 RPC가 관리자 전용이다.
  {
    kind: 'action',
    id: 'power',
    labelKey: 'profile.power',
    icon: 'system-shutdown-symbolic',
    group: 'session',
    requires: 'admin',
  },
  { kind: 'action', id: 'logout', labelKey: 'profile.logout', icon: 'system-log-out-symbolic', group: 'session' },
]

// visibleItems는 표시 조건(requires)을 만족하는 항목만 남긴다.
export function visibleItems(items: readonly ProfileMenuItem[], ctx: MenuContext): ProfileMenuItem[] {
  return items.filter((it) => it.requires === undefined || ctx[it.requires])
}

// groupItems는 이어진 같은 group 항목을 묶는다. 순서는 그대로 둔다.
export function groupItems(items: readonly ProfileMenuItem[]): ProfileMenuItem[][] {
  const groups: ProfileMenuItem[][] = []
  for (const it of items) {
    const last = groups.at(-1)
    if (last && last[0].group === it.group) last.push(it)
    else groups.push([it])
  }
  return groups
}

// submenuKey는 하위 메뉴를 여닫는 키인지 판단한다. 하위 메뉴는 프로필 메뉴 왼쪽에 열리지만,
// 좌우 어느 화살표로도 열고 닫게 한다(시각적 방향과 "오른쪽 = 안으로" 관례를 둘 다 받는다).
// Enter·Space는 버튼 클릭으로 처리되므로 여기서 다루지 않는다.
export function submenuKey(key: string, inSubmenu: boolean): 'open' | 'close' | null {
  if (inSubmenu) return key === 'ArrowLeft' || key === 'ArrowRight' || key === 'Escape' ? 'close' : null
  return key === 'ArrowLeft' || key === 'ArrowRight' ? 'open' : null
}
