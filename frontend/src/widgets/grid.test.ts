import { describe, expect, it } from 'vitest'
import { cellToPx, gridSize, inGrid, overlaps, reservedCells, resizeCells, snapCell, spanPx } from './grid'

// 1000×600 영역: (1000 - 24 + 8) / 88 = 11열, (600 - 24 + 8) / 88 = 6행
const area = { width: 1000, height: 600 }
const grid = { cols: 11, rows: 6 }

describe('gridSize', () => {
  it('가장자리 여백과 간격을 빼고 들어가는 칸 수', () => {
    expect(gridSize(area)).toEqual(grid)
    expect(gridSize({ width: 104, height: 104 })).toEqual({ cols: 1, rows: 1 })
    expect(gridSize({ width: 103, height: 0 })).toEqual({ cols: 0, rows: 0 })
  })
})

describe('cellToPx', () => {
  it('오른쪽 위 기준으로 픽셀 위치를 계산한다', () => {
    expect(cellToPx({ right: 0, top: 0, cols: 3, rows: 2 }, area)).toEqual({ x: 732, y: 12, width: 256, height: 168 })
    expect(cellToPx({ right: 1, top: 1, cols: 1, rows: 1 }, area)).toEqual({ x: 820, y: 100, width: 80, height: 80 })
  })

  it('영역 너비가 바뀌어도 오른쪽 끝에서의 거리는 같다', () => {
    const wide = cellToPx({ right: 2, top: 0, cols: 2, rows: 1 }, { width: 1400, height: 600 })
    const narrow = cellToPx({ right: 2, top: 0, cols: 2, rows: 1 }, area)
    expect(1400 - (wide.x + wide.width)).toBe(1000 - (narrow.x + narrow.width))
  })

  it('spanPx는 칸 사이 간격을 포함한다', () => {
    expect(spanPx(1)).toBe(80)
    expect(spanPx(4)).toBe(344)
    expect(spanPx(0)).toBe(0)
  })
})

describe('overlaps / inGrid', () => {
  const a = { right: 0, top: 0, cols: 3, rows: 2 }
  it('맞닿은 것은 겹치지 않는다', () => {
    expect(overlaps(a, { right: 3, top: 0, cols: 1, rows: 1 })).toBe(false)
    expect(overlaps(a, { right: 0, top: 2, cols: 1, rows: 1 })).toBe(false)
    expect(overlaps(a, { right: 2, top: 1, cols: 2, rows: 2 })).toBe(true)
  })
  it('그리드 안에 모두 들어가야 한다', () => {
    expect(inGrid({ right: 8, top: 4, cols: 3, rows: 2 }, grid)).toBe(true)
    expect(inGrid({ right: 9, top: 0, cols: 3, rows: 2 }, grid)).toBe(false)
    expect(inGrid({ right: 0, top: 5, cols: 3, rows: 2 }, grid)).toBe(false)
  })
})

describe('reservedCells', () => {
  it('아이콘 영역에 걸치는 칸을 감싼다', () => {
    // 왼쪽 위 아이콘 한 열(88px 너비, 4칸 남짓 높이). 맨 왼쪽 열(right 10, x 28~108)만 걸친다.
    expect(reservedCells({ x: 12, y: 12, width: 88, height: 300 }, area)).toEqual({ right: 10, top: 0, cols: 1, rows: 4 })
  })
  it('두 열에 걸치면 두 열을 예약한다', () => {
    expect(reservedCells({ x: 12, y: 12, width: 184, height: 80 }, area)).toEqual({ right: 9, top: 0, cols: 2, rows: 1 })
  })
  it('걸치는 칸이 없으면 null', () => {
    expect(reservedCells(null, area)).toBeNull()
    expect(reservedCells({ x: 0, y: 0, width: 20, height: 20 }, area)).toBeNull()
    expect(reservedCells({ x: 12, y: 12, width: 0, height: 100 }, area)).toBeNull()
  })
})

describe('snapCell', () => {
  const size = { cols: 3, rows: 2 }
  it('가장 가까운 칸에 맞춘다', () => {
    expect(snapCell({ x: 732, y: 12 }, size, area)).toEqual({ right: 0, top: 0, ...size })
    expect(snapCell({ x: 700, y: 50 }, size, area)).toEqual({ right: 0, top: 0, ...size })
    expect(snapCell({ x: 680, y: 60 }, size, area)).toEqual({ right: 1, top: 1, ...size })
  })
  it('그리드 밖이면 안으로 끌어 넣는다', () => {
    expect(snapCell({ x: -500, y: 9999 }, size, area)).toEqual({ right: 8, top: 4, ...size })
    expect(snapCell({ x: 2000, y: -50 }, size, area)).toEqual({ right: 0, top: 0, ...size })
  })
  it('그리드보다 큰 위젯은 null', () => {
    expect(snapCell({ x: 0, y: 0 }, { cols: 12, rows: 1 }, area)).toBeNull()
  })
})

describe('resizeCells', () => {
  const min = { cols: 2, rows: 2 }
  const max = { cols: 4, rows: 3 }
  it('왼쪽 위를 고정하고 칸 단위로 늘린다', () => {
    expect(resizeCells({ right: 2, top: 0, cols: 3, rows: 2 }, 88, 88, min, max, grid)).toEqual({
      right: 1,
      top: 0,
      cols: 4,
      rows: 3,
    })
    // 반 칸 넘게 끌어야 바뀐다.
    expect(resizeCells({ right: 2, top: 0, cols: 3, rows: 2 }, 40, -40, min, max, grid)).toMatchObject({ cols: 3, rows: 2 })
    expect(resizeCells({ right: 2, top: 0, cols: 3, rows: 2 }, 50, -50, min, max, grid)).toMatchObject({ cols: 4, rows: 2 })
  })
  it('최소·최대 크기로 제한한다', () => {
    expect(resizeCells({ right: 0, top: 0, cols: 3, rows: 2 }, -999, -999, min, max, grid)).toEqual({
      right: 1,
      top: 0,
      cols: 2,
      rows: 2,
    })
    expect(resizeCells({ right: 5, top: 0, cols: 3, rows: 2 }, 999, 999, min, max, grid)).toMatchObject({ cols: 4, rows: 3 })
  })
  it('그리드 오른쪽·아래 끝을 넘지 않는다', () => {
    expect(resizeCells({ right: 0, top: 0, cols: 3, rows: 2 }, 200, 0, min, max, grid)).toEqual({
      right: 0,
      top: 0,
      cols: 3,
      rows: 2,
    })
    expect(resizeCells({ right: 3, top: 5, cols: 3, rows: 1 }, 0, 200, { cols: 1, rows: 1 }, max, grid)).toMatchObject({
      rows: 1,
    })
  })
})
