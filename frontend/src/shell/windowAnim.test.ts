import { describe, expect, it } from 'vitest'
import { fitInside, flipTransform, sameRect, scaleAbout } from './windowAnim'

describe('flipTransform', () => {
  it('움직임이 없으면 none', () => {
    const r = { x: 10, y: 20, width: 300, height: 200 }
    expect(flipTransform(r, r)).toBe('none')
    expect(flipTransform(r, { ...r, x: 10.2 })).toBe('none')
  })

  it('왼쪽 위 기준으로 layout 사각형을 visual 사각형에 겹친다', () => {
    const layout = { x: 8, y: 8, width: 800, height: 600 }
    const visual = { x: 108, y: 58, width: 400, height: 150 }
    expect(flipTransform(layout, visual)).toBe('translate(100px, 50px) scale(0.5, 0.25)')
  })
})

describe('fitInside', () => {
  it('가로로 긴 사각형은 너비에 맞추고 세로 가운데에 놓는다', () => {
    expect(fitInside({ width: 400, height: 100 }, { x: 0, y: 0, width: 100, height: 100 })).toEqual({
      x: 0,
      y: 37.5,
      width: 100,
      height: 25,
    })
  })

  it('세로로 긴 사각형은 높이에 맞추고 가로 가운데에 놓는다', () => {
    expect(fitInside({ width: 300, height: 600 }, { x: 10, y: 20, width: 200, height: 40 })).toEqual({
      x: 100,
      y: 20,
      width: 20,
      height: 40,
    })
  })
})

describe('scaleAbout', () => {
  it('가운데를 기준으로 줄인다', () => {
    expect(scaleAbout({ x: 0, y: 0, width: 200, height: 100 }, 0.5)).toEqual({ x: 50, y: 25, width: 100, height: 50 })
  })
})

describe('sameRect', () => {
  it('0.5px 넘게 움직이면 다르다', () => {
    expect(sameRect({ x: 0, y: 0, width: 10, height: 10 }, { x: 0, y: 1, width: 10, height: 10 })).toBe(false)
  })
})
