import { describe, expect, it } from 'vitest'
import { gridNextIndex } from './gridNav'

// 8개, 3열:
// 0 1 2
// 3 4 5
// 6 7
describe('gridNextIndex', () => {
  const next = (current: number, key: string, count = 8, cols = 3) => gridNextIndex(current, count, cols, key)

  it('좌우는 하나씩, 끝에서 넘어가지 않는다', () => {
    expect(next(4, 'ArrowLeft')).toBe(3)
    expect(next(4, 'ArrowRight')).toBe(5)
    expect(next(0, 'ArrowLeft')).toBe(0)
    expect(next(7, 'ArrowRight')).toBe(7)
    // 줄 끝에서는 다음 줄 첫 항목으로 이어진다(읽는 순서)
    expect(next(2, 'ArrowRight')).toBe(3)
  })

  it('위아래는 한 줄씩', () => {
    expect(next(1, 'ArrowDown')).toBe(4)
    expect(next(4, 'ArrowUp')).toBe(1)
    expect(next(4, 'ArrowDown')).toBe(7)
  })

  it('아래 줄이 덜 찼으면 마지막 항목으로', () => {
    expect(next(5, 'ArrowDown')).toBe(7)
  })

  it('마지막 줄에서 아래는 그대로', () => {
    expect(next(6, 'ArrowDown')).toBe(6)
    expect(next(7, 'ArrowDown')).toBe(7)
  })

  it('첫 줄에서 위는 -1(검색 상자로)', () => {
    expect(next(0, 'ArrowUp')).toBe(-1)
    expect(next(2, 'ArrowUp')).toBe(-1)
  })

  it('Home·End', () => {
    expect(next(4, 'Home')).toBe(0)
    expect(next(4, 'End')).toBe(7)
  })

  it('다른 키는 그대로', () => {
    expect(next(4, 'a')).toBe(4)
    expect(next(4, 'Enter')).toBe(4)
  })

  it('열이 항목보다 많거나 한 열', () => {
    expect(gridNextIndex(1, 3, 5, 'ArrowDown')).toBe(1)
    expect(gridNextIndex(1, 3, 5, 'ArrowUp')).toBe(-1)
    expect(gridNextIndex(1, 3, 1, 'ArrowDown')).toBe(2)
    expect(gridNextIndex(1, 3, 1, 'ArrowUp')).toBe(0)
    expect(gridNextIndex(0, 3, 0, 'ArrowDown')).toBe(1)
  })

  it('항목이 없으면 -1', () => {
    expect(gridNextIndex(0, 0, 3, 'ArrowDown')).toBe(-1)
  })
})
