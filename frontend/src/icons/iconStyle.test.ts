import { describe, expect, it } from 'vitest'
import { colorIconStyle } from './iconStyle'

const url = 'x.svg'

describe('colorIconStyle', () => {
  it('여백·모서리가 없으면 style을 주지 않는다', () => {
    expect(colorIconStyle({ url, kind: 'color' }, 64, true)).toBeUndefined()
  })

  it('inset은 padding이 된다', () => {
    expect(colorIconStyle({ url, kind: 'color', inset: 4 / 64 }, 64, false)).toEqual({ padding: 4 })
  })

  it('appRadius는 앱 아이콘 자리에서만 여백 안쪽 그림의 모서리를 깎는다', () => {
    const icon = { url, kind: 'color', inset: 4 / 64, appRadius: 13 / 56 } as const
    expect(colorIconStyle(icon, 64, true)).toEqual({ padding: 4, clipPath: 'inset(4px round 13px)' })
    expect(colorIconStyle(icon, 64, false)).toEqual({ padding: 4 })
    expect(colorIconStyle({ url, kind: 'color', appRadius: 0.25 }, 48, true)).toEqual({
      clipPath: 'inset(0px round 12px)',
    })
  })
})
