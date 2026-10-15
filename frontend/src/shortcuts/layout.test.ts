import { describe, expect, it } from 'vitest'
import {
  ICON_GAP,
  ICON_HEIGHT,
  ICON_WIDTH,
  ICONS_PADDING,
  iconCenter,
  inBounds,
  previewOffsets,
  rowsPerColumn,
  slotAt,
  slotPosition,
} from './layout'

const PITCH_X = ICON_WIDTH + ICON_GAP
const PITCH_Y = ICON_HEIGHT + ICON_GAP

describe('rowsPerColumn', () => {
  it.each([
    [0, 1],
    [100, 1],
    [215, 1],
    [216, 2],
    [315, 2],
    [316, 3],
    [1000, 9],
  ])('높이 %i → %i행', (h, rows) => {
    expect(rowsPerColumn(h)).toBe(rows)
  })
})

describe('slotPosition', () => {
  it('위에서 아래로 채우고 열을 넘긴다', () => {
    expect(slotPosition(0, 3)).toEqual({ x: ICONS_PADDING, y: ICONS_PADDING })
    expect(slotPosition(2, 3)).toEqual({ x: ICONS_PADDING, y: ICONS_PADDING + 2 * PITCH_Y })
    expect(slotPosition(3, 3)).toEqual({ x: ICONS_PADDING + PITCH_X, y: ICONS_PADDING })
    expect(slotPosition(7, 3)).toEqual({ x: ICONS_PADDING + 2 * PITCH_X, y: ICONS_PADDING + PITCH_Y })
  })
})

describe('slotAt', () => {
  it('칸 가운데는 그 칸', () => {
    for (let i = 0; i < 9; i++) expect(slotAt(iconCenter(i, 3), 3, 99)).toBe(i)
  })

  it('가장 가까운 칸으로 맞춘다', () => {
    const c = iconCenter(4, 3)
    expect(slotAt({ x: c.x + PITCH_X * 0.4, y: c.y - PITCH_Y * 0.4 }, 3, 99)).toBe(4)
    expect(slotAt({ x: c.x + PITCH_X * 0.6, y: c.y }, 3, 99)).toBe(7)
  })

  it('왼쪽·위로 벗어나면 첫 열·첫 행, 아래로 벗어나면 마지막 행', () => {
    expect(slotAt({ x: -500, y: -500 }, 3, 99)).toBe(0)
    expect(slotAt({ x: iconCenter(3, 3).x, y: 5000 }, 3, 99)).toBe(5)
  })

  it('maxIndex를 넘지 않는다', () => {
    expect(slotAt({ x: 5000, y: 0 }, 3, 4)).toBe(4)
    expect(slotAt(iconCenter(8, 3), 3, 5)).toBe(5)
    expect(slotAt(iconCenter(2, 3), 3, 0)).toBe(0)
    expect(slotAt(iconCenter(2, 3), 3, -1)).toBe(0)
  })
})

describe('inBounds', () => {
  it('영역 안인지', () => {
    const size = { width: 100, height: 50 }
    expect(inBounds({ x: 0, y: 0 }, size)).toBe(true)
    expect(inBounds({ x: 99, y: 49 }, size)).toBe(true)
    expect(inBounds({ x: 100, y: 10 }, size)).toBe(false)
    expect(inBounds({ x: 10, y: -1 }, size)).toBe(false)
  })
})

describe('previewOffsets', () => {
  const rows = 3
  const delta = (from: number, to: number) => {
    const a = slotPosition(from, rows)
    const b = slotPosition(to, rows)
    return { x: b.x - a.x, y: b.y - a.y }
  }
  const zero = { x: 0, y: 0 }

  it('놓을 칸이 없거나 제자리면 움직이지 않는다', () => {
    expect(previewOffsets(4, 1, null, rows)).toEqual([zero, zero, zero, zero])
    expect(previewOffsets(4, 1, 1, rows)).toEqual([zero, zero, zero, zero])
  })

  it('뒤로 옮기면 사이의 아이콘이 한 칸씩 앞으로', () => {
    // 0 [1] 2 3 4 → 0 2 3 [1] 4 (열을 넘는다)
    expect(previewOffsets(5, 1, 3, rows)).toEqual([zero, delta(1, 3), delta(2, 1), delta(3, 2), zero])
  })

  it('앞으로 옮기면 사이의 아이콘이 한 칸씩 뒤로', () => {
    // 0 1 2 [3] 4 → [3] 0 1 2 4
    expect(previewOffsets(5, 3, 0, rows)).toEqual([delta(0, 1), delta(1, 2), delta(2, 3), delta(3, 0), zero])
  })

  it('밖에서 끼우면 그 칸부터 한 칸씩 뒤로', () => {
    expect(previewOffsets(4, null, 2, rows)).toEqual([zero, zero, delta(2, 3), delta(3, 4)])
    expect(previewOffsets(3, null, 3, rows)).toEqual([zero, zero, zero])
    expect(previewOffsets(0, null, 0, rows)).toEqual([])
  })
})
