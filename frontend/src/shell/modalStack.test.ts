import { describe, expect, it } from 'vitest'
import { isUnder, popLayer, pushLayer } from './modalStack'

describe('modal stack', () => {
  it('pushes on top once and pops by key', () => {
    let s = pushLayer([], 1)
    s = pushLayer(s, 2)
    expect(pushLayer(s, 1)).toBe(s)
    expect(s).toEqual([1, 2])
    expect(popLayer(s, 1)).toEqual([2])
    expect(popLayer(s, 3)).toBe(s)
  })

  it('reports only lower layers as under', () => {
    const s = [1, 2, 3]
    expect(isUnder(s, 1)).toBe(true)
    expect(isUnder(s, 2)).toBe(true)
    expect(isUnder(s, 3)).toBe(false)
    // 아직 올리지 않은 모달은 깔리지 않았다.
    expect(isUnder(s, 4)).toBe(false)
  })
})
