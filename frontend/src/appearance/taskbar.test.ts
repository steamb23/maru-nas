import { describe, expect, it } from 'vitest'
import { taskbarAlignOf } from './taskbar'

describe('taskbarAlignOf', () => {
  it('정하지 않았으면 가운데다', () => {
    expect(taskbarAlignOf(undefined)).toBe('center')
    expect(taskbarAlignOf('')).toBe('center')
  })

  it('정한 값을 쓴다', () => {
    expect(taskbarAlignOf('left')).toBe('left')
    expect(taskbarAlignOf('center')).toBe('center')
  })

  it('모르는 값은 가운데로 본다', () => {
    expect(taskbarAlignOf('right')).toBe('center')
    expect(taskbarAlignOf(1)).toBe('center')
  })
})
