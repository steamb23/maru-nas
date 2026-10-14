import { describe, expect, it } from 'vitest'
import { enterKeyframes, slideKeyframes } from './widgetPreviewAnim'

describe('enterKeyframes', () => {
  it('가운데 기준으로 줄인 투명한 상태에서 시작한다', () => {
    const [from, to] = enterKeyframes({ x: 10, y: 10, width: 200, height: 100 })
    expect(from).toEqual({ opacity: 0, transform: 'translate(4px, 2px) scale(0.96)' })
    expect(to).toEqual({ opacity: 1, transform: 'none' })
  })
})

describe('slideKeyframes', () => {
  it('지금 보이는 자리에서 새 자리로 미끄러진다', () => {
    const layout = { x: 100, y: 50, width: 80, height: 80 }
    expect(slideKeyframes(layout, { x: 12, y: 50, width: 80, height: 80 })).toEqual([
      { transform: 'translate(-88px, 0px) scale(1, 1)' },
      { transform: 'none' },
    ])
  })
  it('크기가 바뀌면 크기도 함께 맞춘다', () => {
    const [from] = slideKeyframes({ x: 0, y: 0, width: 168, height: 80 }, { x: 0, y: 0, width: 84, height: 80 }) ?? []
    expect(from).toEqual({ transform: 'translate(0px, 0px) scale(0.5, 1)' })
  })
  it('같은 자리면 null', () => {
    const r = { x: 1, y: 2, width: 3, height: 4 }
    expect(slideKeyframes(r, { ...r, x: 1.2 })).toBeNull()
  })
})
