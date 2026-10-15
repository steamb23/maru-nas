// 아이콘 리졸버. 컴포넌트는 아이콘 경로 대신 freedesktop 아이콘 이름으로 getIcon()을 부른다.
//
// 아이콘은 출처별 테이블(sources/<출처>.ts)에 두고 여기서 합친다. 기본 출처는 Colloid 테마이고,
// 테마에 없는 제3자 에셋(브랜드 로고 등)은 vendor/icons/<출처>/에 원본과 출처·라이선스를 두고
// 출처 테이블을 하나 더 만든다(.claude/rules/icons.md). 테마를 바꿀 때는 출처 테이블만 바꾼다.
import { COLLOID_ICONS } from './sources/colloid'
import { OPENMEDIAVAULT_ICONS } from './sources/openmediavault'
import type { Icon } from './types'

export type { Icon, IconKind } from './types'

// ICON_SOURCES는 출처 id(vendor/icons/<id>/ 폴더 이름) → 아이콘 테이블이다.
// 출처를 더하면 vendor/icons/README.md에도 절을 추가한다(resolver.test.ts가 검사한다).
export const ICON_SOURCES = {
  colloid: COLLOID_ICONS,
  openmediavault: OPENMEDIAVAULT_ICONS,
} as const

const ICONS = { ...COLLOID_ICONS, ...OPENMEDIAVAULT_ICONS } as const satisfies Record<string, Icon>

export type IconName = keyof typeof ICONS

export function getIcon(name: IconName): Icon {
  return ICONS[name]
}
