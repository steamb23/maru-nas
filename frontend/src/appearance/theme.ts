// 테마(라이트·다크·자동) 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 실제 적용(<html data-theme>)은 dom.ts가 한다. base.css의 토큰은 light-dark()로 color-scheme을 따른다.
import { isOneOf } from '../api/guards'

export type ThemeMode = 'auto' | 'light' | 'dark'

export const THEME_MODES: readonly ThemeMode[] = ['auto', 'light', 'dark']

export function toThemeMode(v: unknown): ThemeMode | null {
  return isOneOf(v, THEME_MODES) ? v : null
}

// effectiveTheme은 지금 쓸 테마다: 사용자 값 → 관리자가 정한 기본 테마 → 자동.
export function effectiveTheme(user: unknown, siteDefault: unknown): ThemeMode {
  return toThemeMode(user) ?? toThemeMode(siteDefault) ?? 'auto'
}
