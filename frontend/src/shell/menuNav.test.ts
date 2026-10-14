import { describe, expect, it } from 'vitest'
import { nextIndex } from './menuNav'

describe('nextIndex', () => {
  it('화살표 키 이동은 양 끝에서 넘어간다', () => {
    expect(nextIndex(-1, 3, 'ArrowDown')).toBe(0)
    expect(nextIndex(2, 3, 'ArrowDown')).toBe(0)
    expect(nextIndex(-1, 3, 'ArrowUp')).toBe(2)
    expect(nextIndex(0, 3, 'ArrowUp')).toBe(2)
    expect(nextIndex(1, 3, 'End')).toBe(2)
    expect(nextIndex(1, 3, 'Home')).toBe(0)
    expect(nextIndex(1, 3, 'x')).toBe(1)
    expect(nextIndex(0, 0, 'ArrowDown')).toBe(-1)
  })
})
