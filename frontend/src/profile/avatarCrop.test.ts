import { describe, expect, it } from 'vitest'
import { INITIAL_CROP, MAX_ZOOM, clampOffset, displayScale, moveBy, sourceRect, zoomTo } from './avatarCrop'

const wide = { width: 400, height: 200 }

describe('displayScale', () => {
  it('확대 1배는 짧은 변이 보기 영역에 맞는다', () => {
    expect(displayScale(wide, 100, 1)).toBe(0.5)
    expect(displayScale(wide, 100, 2)).toBe(1)
    expect(displayScale(wide, 100, 99)).toBe(0.5 * MAX_ZOOM)
  })
})

describe('clampOffset', () => {
  it('이미지가 보기 영역을 다 덮는 범위로 제한한다', () => {
    // 1배: 200×100으로 그려지므로 가로로 ±50까지, 세로로는 움직일 수 없다.
    expect(clampOffset(wide, 100, 1, { x: 80, y: 30 })).toEqual({ x: 50, y: 0 })
    expect(clampOffset(wide, 100, 1, { x: -80, y: -30 })).toEqual({ x: -50, y: 0 })
  })
})

describe('sourceRect', () => {
  it('처음에는 가운데 정사각형이다', () => {
    expect(sourceRect(wide, 100, INITIAL_CROP)).toEqual({ x: 100, y: 0, width: 200, height: 200 })
  })

  it('이미지를 오른쪽으로 끌면 왼쪽 부분이 보인다', () => {
    expect(sourceRect(wide, 100, moveBy(wide, 100, INITIAL_CROP, 50, 0))).toEqual({ x: 0, y: 0, width: 200, height: 200 })
  })

  it('확대하면 더 작은 영역을 자른다', () => {
    expect(sourceRect(wide, 100, { zoom: 2, offset: { x: 0, y: 0 } })).toEqual({ x: 150, y: 50, width: 100, height: 100 })
  })
})

describe('zoomTo', () => {
  it('가운데 지점을 그대로 두고 확대한다', () => {
    const moved = moveBy(wide, 100, INITIAL_CROP, 20, 0)
    const zoomed = zoomTo(wide, 100, moved, 2)
    const before = sourceRect(wide, 100, moved)
    const after = sourceRect(wide, 100, zoomed)
    expect(after.x + after.width / 2).toBeCloseTo(before.x + before.width / 2)
    expect(zoomed.zoom).toBe(2)
  })

  it('줄이면 위치를 다시 제한한다', () => {
    const zoomed = moveBy(wide, 100, { zoom: 4, offset: { x: 0, y: 0 } }, 300, 100)
    expect(zoomTo(wide, 100, zoomed, 1).offset).toEqual({ x: 50, y: 0 })
  })
})
