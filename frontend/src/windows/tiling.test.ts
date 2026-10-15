import { describe, expect, it } from 'vitest'
import { getWindowApp } from './apps'
import { FLOAT_GAP, MIN_WIDTH } from './geometry'
import {
  DEFAULT_TILE_SPLIT,
  TILE_EDGE,
  type TileWindow,
  clampSplit,
  layoutOf,
  splitForTiling,
  splitFromDrag,
  tileDividerRect,
  tileMinWidth,
  tilePartner,
  tileRect,
  tileZoneAt,
  visibleTileGroup,
} from './tiling'

const bounds = { width: 1280, height: 720 }

const tw = (id: string, z: number, tile: TileWindow['tile'], minimized = false): TileWindow => ({
  id,
  appId: 'files',
  z,
  minimized,
  maximized: false,
  tile,
})

describe('tileZoneAt', () => {
  it('위·좌·우 가장자리에서 각 영역을 반환한다', () => {
    expect(tileZoneAt({ x: 600, y: 0 }, bounds)).toBe('max')
    expect(tileZoneAt({ x: 600, y: TILE_EDGE }, bounds)).toBe('max')
    expect(tileZoneAt({ x: 0, y: 300 }, bounds)).toBe('left')
    expect(tileZoneAt({ x: bounds.width - 1, y: 300 }, bounds)).toBe('right')
  })

  it('가장자리에서 떨어져 있으면 null', () => {
    expect(tileZoneAt({ x: 600, y: TILE_EDGE + 1 }, bounds)).toBeNull()
    expect(tileZoneAt({ x: TILE_EDGE + 1, y: 300 }, bounds)).toBeNull()
    expect(tileZoneAt({ x: bounds.width - TILE_EDGE - 1, y: 300 }, bounds)).toBeNull()
  })

  it('브라우저 밖 좌표도 가장자리로 본다', () => {
    expect(tileZoneAt({ x: 600, y: -50 }, bounds)).toBe('max')
    expect(tileZoneAt({ x: -50, y: 300 }, bounds)).toBe('left')
    expect(tileZoneAt({ x: bounds.width + 50, y: 300 }, bounds)).toBe('right')
  })

  it('모서리에서는 좌우를 위보다 우선한다', () => {
    expect(tileZoneAt({ x: 0, y: 0 }, bounds)).toBe('left')
    expect(tileZoneAt({ x: bounds.width, y: 0 }, bounds)).toBe('right')
  })

  it('좌우 타일이 최소 너비를 못 가질 만큼 좁으면 최대화만 된다', () => {
    const narrow = { width: 2 * MIN_WIDTH + 3 * FLOAT_GAP - 1, height: 720 }
    expect(tileZoneAt({ x: 0, y: 300 }, narrow)).toBeNull()
    expect(tileZoneAt({ x: 0, y: 0 }, narrow)).toBe('max')
    const enough = { ...narrow, width: narrow.width + 1 }
    expect(tileZoneAt({ x: 0, y: 300 }, enough)).toBe('left')
  })
})

describe('tileRect', () => {
  it('최대화는 가장자리에서 간격만큼 떨어진다', () => {
    expect(tileRect('max', 0.5, bounds)).toEqual({
      x: FLOAT_GAP,
      y: FLOAT_GAP,
      width: bounds.width - 2 * FLOAT_GAP,
      height: bounds.height - 2 * FLOAT_GAP,
    })
  })

  it('좌우 타일은 가장자리와 서로에서 간격만큼 떨어져 너비를 빈틈없이 나눈다', () => {
    for (const split of [0.5, 0.3, 0.71]) {
      const l = tileRect('left', split, bounds)
      const r = tileRect('right', split, bounds)
      expect(l.x).toBe(FLOAT_GAP)
      expect(r.x - (l.x + l.width)).toBe(FLOAT_GAP)
      expect(bounds.width - (r.x + r.width)).toBe(FLOAT_GAP)
      expect(l.height).toBe(bounds.height - 2 * FLOAT_GAP)
      expect(r.height).toBe(l.height)
    }
  })

  it('비율대로 나눈다', () => {
    const total = bounds.width - 3 * FLOAT_GAP
    expect(tileRect('left', 0.25, bounds).width).toBe(Math.round(total * 0.25))
  })
})

describe('tileDividerRect', () => {
  it('좌우 타일 사이 간격과 정확히 맞는다', () => {
    const split = 0.42
    const l = tileRect('left', split, bounds)
    const r = tileRect('right', split, bounds)
    expect(tileDividerRect(split, bounds)).toEqual({ x: l.x + l.width, y: l.y, width: r.x - (l.x + l.width), height: l.height })
  })
})

describe('clampSplit / splitFromDrag', () => {
  const total = bounds.width - 3 * FLOAT_GAP

  it('양쪽 너비가 최소 너비 이상이 되게 제한한다', () => {
    const lo = clampSplit(0, bounds, 300, 400)
    const hi = clampSplit(1, bounds, 300, 400)
    expect(tileRect('left', lo, bounds).width).toBeGreaterThanOrEqual(300)
    expect(tileRect('right', hi, bounds).width).toBeGreaterThanOrEqual(400)
    expect(clampSplit(0.5, bounds, 300, 400)).toBe(0.5)
  })

  it('둘 다 만족할 수 없으면 반반', () => {
    expect(clampSplit(0.2, bounds, total, total)).toBe(DEFAULT_TILE_SPLIT)
  })

  it('끈 거리만큼 비율이 바뀐다', () => {
    expect(splitFromDrag(0.5, total / 10, bounds)).toBeCloseTo(0.6)
  })
})

describe('tileMinWidth', () => {
  it('공통 하한과 그쪽 타일 창들의 앱 최소 너비 중 큰 값', () => {
    const windows: TileWindow[] = [tw('a', 1, 'left'), { ...tw('b', 2, 'right'), appId: 'system-info' }]
    expect(tileMinWidth(windows, 'left')).toBe(MIN_WIDTH)
    expect(tileMinWidth(windows, 'right')).toBe(Math.max(MIN_WIDTH, getWindowApp('system-info').minSize?.width ?? 0))
  })
})

describe('tilePartner / visibleTileGroup', () => {
  const windows = [tw('a', 1, 'left'), tw('b', 2, 'right'), tw('c', 5, 'right', true), tw('d', 4, null), tw('e', 3, 'left')]

  it('반대쪽 타일 창 중 맨 앞 창이 짝이다(최소화된 창 포함)', () => {
    expect(tilePartner(windows, 'a')?.id).toBe('c')
    expect(tilePartner(windows, 'c')?.id).toBe('e')
  })

  it('타일링되지 않은 창이나 반대쪽이 빈 창은 짝이 없다', () => {
    expect(tilePartner(windows, 'd')).toBeNull()
    expect(tilePartner([tw('a', 1, 'left'), tw('e', 2, 'left')], 'a')).toBeNull()
  })

  it('보이는 그룹은 최소화된 창을 뺀다', () => {
    const group = visibleTileGroup(windows)
    expect(group?.left.id).toBe('e')
    expect(group?.right.id).toBe('b')
    expect(visibleTileGroup([tw('a', 1, 'left'), tw('b', 2, 'right', true)])).toBeNull()
  })
})

describe('splitForTiling', () => {
  it('반대쪽에 다른 타일 창이 있으면 비율을 유지하고, 없으면 반반', () => {
    const windows = [tw('a', 1, 'left'), tw('b', 2, null)]
    expect(splitForTiling(windows, 'b', 'right', 0.7)).toBe(0.7)
    expect(splitForTiling(windows, 'b', 'left', 0.7)).toBe(DEFAULT_TILE_SPLIT)
    // 자기 자신은 반대쪽 짝으로 치지 않는다
    expect(splitForTiling(windows, 'a', 'right', 0.7)).toBe(DEFAULT_TILE_SPLIT)
  })
})

describe('layoutOf', () => {
  it('최대화·타일·자유 배치를 구분한다', () => {
    expect(layoutOf({ maximized: true, tile: null })).toBe('max')
    expect(layoutOf({ maximized: false, tile: 'right' })).toBe('right')
    expect(layoutOf({ maximized: false, tile: null })).toBe('free')
  })
})
