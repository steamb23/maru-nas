import { describe, expect, it } from 'vitest'
import { effectiveTheme, toThemeMode } from './theme'

describe('effectiveTheme', () => {
  it('사용자 값을 먼저 쓴다', () => {
    expect(effectiveTheme('dark', 'light')).toBe('dark')
  })

  it('사용자 값이 없으면 관리자 기본 테마, 그것도 없으면 자동이다', () => {
    expect(effectiveTheme(undefined, 'light')).toBe('light')
    expect(effectiveTheme('', undefined)).toBe('auto')
  })

  it('모르는 값은 없는 것으로 본다', () => {
    expect(effectiveTheme('sepia', 'dark')).toBe('dark')
    expect(toThemeMode('sepia')).toBeNull()
  })
})
