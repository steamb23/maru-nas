import { describe, expect, it } from 'vitest'
import { getIcon } from './resolver'

describe('getIcon', () => {
  it('folder는 Colloid places 아이콘으로 해석된다', () => {
    const icon = getIcon('folder')
    expect(icon.kind).toBe('color')
    expect(icon.url).toMatch(/folder\.svg$|^data:image\/svg\+xml/)
  })
})
