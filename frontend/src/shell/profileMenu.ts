// 작업표시줄 프로필 메뉴 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 항목은 두 종류다. action 항목은 누르면 ProfileMenu.tsx의 actions에 있는 동작을 실행한다
// (ProfileMenuActionId에 추가하면 동작을 빠뜨렸을 때 타입 오류가 난다). submenu 항목은 누르면 하위 메뉴를 연다.
// 나중에 개인 설정, 정보, 재시작·종료(admin) 같은 항목이 들어갈 자리다. 같은 group끼리 묶고 group 사이에 구분선을 그린다.
import type { ParseKeys } from 'i18next'
import type { SessionInfo } from '../api/types.gen'
import type { IconName } from '../icons/resolver'

export type ProfileMenuActionId = 'logout'
export type ProfileMenuSubmenuId = 'language'

interface ItemBase {
  labelKey: ParseKeys
  icon: IconName
  group: string
  // admin이면 관리자에게만 보인다.
  requires?: 'admin'
}

export type ProfileMenuItem = ItemBase &
  ({ kind: 'action'; id: ProfileMenuActionId } | { kind: 'submenu'; id: ProfileMenuSubmenuId })

export const PROFILE_MENU_ITEMS: readonly ProfileMenuItem[] = [
  {
    kind: 'submenu',
    id: 'language',
    labelKey: 'language.label',
    icon: 'preferences-desktop-locale-symbolic',
    group: 'preferences',
  },
  { kind: 'action', id: 'logout', labelKey: 'profile.logout', icon: 'system-log-out-symbolic', group: 'session' },
]

// visibleItems는 사용자 권한으로 볼 수 있는 항목만 남긴다.
export function visibleItems(items: readonly ProfileMenuItem[], user: SessionInfo): ProfileMenuItem[] {
  return items.filter((it) => it.requires !== 'admin' || user.admin)
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
