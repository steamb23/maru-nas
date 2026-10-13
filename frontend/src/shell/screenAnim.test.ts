import { describe, expect, it } from 'vitest'
import { SHOWN, screenFrames, viewportOrigin } from './screenAnim'

describe('viewportOrigin', () => {
  it('요소 기준으로 화면 가운데를 가리킨다', () => {
    const viewport = { width: 1000, height: 800 }
    expect(viewportOrigin({ x: 0, y: 0, width: 1000, height: 740 }, viewport)).toBe('500px 400px')
    expect(viewportOrigin({ x: 8, y: 744, width: 984, height: 48 }, viewport)).toBe('492px -344px')
  })
})

describe('screenFrames', () => {
  it('크기 비율·투명도·흐림을 바꾸고, 1배·흐림 0은 transform·filter를 두지 않는다', () => {
    expect(screenFrames({ scale: 0.94, opacity: 0, blur: 8 }, SHOWN, '1px 2px')).toEqual([
      { transform: 'scale(0.94)', opacity: 0, filter: 'blur(8px)', transformOrigin: '1px 2px' },
      { transform: 'none', opacity: 1, filter: 'none', transformOrigin: '1px 2px' },
    ])
  })
})
