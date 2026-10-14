import { describe, expect, it } from 'vitest'
import { pickEffect } from './effects'

describe('pickEffect', () => {
  it('rng 값에 따라 고르고 범위를 넘지 않는다', () => {
    const list = ['a', 'b', 'c']
    expect(pickEffect(list, () => 0)).toBe('a')
    expect(pickEffect(list, () => 0.5)).toBe('b')
    expect(pickEffect(list, () => 0.9999)).toBe('c')
    expect(pickEffect(list, () => 1)).toBe('c')
  })

  it('빈 목록은 undefined', () => {
    expect(pickEffect([], () => 0)).toBeUndefined()
  })
})
