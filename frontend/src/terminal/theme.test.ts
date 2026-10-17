import { describe, expect, it } from 'vitest'
import { isDark, parseRgb, terminalTheme } from './theme'

describe('theme', () => {
  it('계산된 색을 읽는다', () => {
    expect(parseRgb('rgb(27, 29, 33)')).toEqual([27, 29, 33])
    expect(parseRgb('rgba(250, 250, 250, 1)')).toEqual([250, 250, 250])
    expect(parseRgb('color(srgb 1 1 1)')).toBeNull()
  })

  it('배경 밝기로 팔레트를 고른다', () => {
    expect(isDark('rgb(27, 29, 33)')).toBe(true)
    expect(isDark('rgb(250, 250, 250)')).toBe(false)
    expect(isDark('unknown')).toBe(true)
    const light = terminalTheme('rgb(250, 250, 250)', 'rgb(56, 58, 66)')
    const dark = terminalTheme('rgb(27, 29, 33)', 'rgb(215, 218, 224)')
    expect(light.background).toBe('rgb(250, 250, 250)')
    expect(light.cursor).toBe('rgb(56, 58, 66)')
    expect(light.red).not.toBe(dark.red)
  })
})
