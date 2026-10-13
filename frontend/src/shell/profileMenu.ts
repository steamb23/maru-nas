// 작업표시줄 프로필 메뉴 항목. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 항목을 추가하려면 PROFILE_MENU_ITEMS에 한 줄을 넣고 ProfileMenu.tsx의 actions에 동작을 넣는다
// (ProfileMenuItemId에 추가하면 동작을 빠뜨렸을 때 타입 오류가 난다). 나중에 개인 설정, 정보,
// 재시작·종료(admin) 같은 항목이 들어갈 자리다. 같은 group끼리 묶고 group 사이에 구분선을 그린다.
import type { SessionInfo } from '../api/types.gen'
import type { IconName } from '../icons/resolver'

export type ProfileMenuItemId = 'logout'

export interface ProfileMenuItem {
  id: ProfileMenuItemId
  label: string
  icon: IconName
  group: string
  // admin이면 관리자에게만 보인다.
  requires?: 'admin'
}

export const PROFILE_MENU_ITEMS: readonly ProfileMenuItem[] = [
  { id: 'logout', label: '로그아웃', icon: 'system-log-out-symbolic', group: 'session' },
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

// nextIndex는 화살표 키로 옮길 항목 위치다. 끝에서는 반대쪽 끝으로 넘어간다.
export function nextIndex(current: number, count: number, key: string): number {
  if (count === 0) return -1
  switch (key) {
    case 'ArrowDown':
      return (current + 1 + count) % count
    case 'ArrowUp':
      return current < 0 ? count - 1 : (current - 1 + count) % count
    case 'Home':
      return 0
    case 'End':
      return count - 1
  }
  return current
}
