// 바탕화면 위젯 그리드 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 그리드는 데스크톱 영역(작업표시줄 제외)의 오른쪽 위를 기준으로 한다. 칸 좌표는 오른쪽 끝에서
// 위젯 오른쪽 끝까지의 칸 수(right)와 위쪽 끝에서 위젯 위쪽 끝까지의 칸 수(top)다. 브라우저 너비가
// 바뀌어도 위젯은 오른쪽에 붙어 있고, 위젯끼리 겹치는지는 화면 크기와 무관하게 정해진다.
// 창 시스템(src/windows)과는 서로 import하지 않는다.
import type { GridSize } from '../api/types.gen'

// 칸 한 변의 크기, 칸 사이 간격, 데스크톱 영역 가장자리와의 간격(CSS 픽셀).
// 가장자리 간격은 바탕화면 아이콘 목록의 여백(index.css의 .desktop-icons top·left)과 같다.
export const CELL = 80
export const GAP = 8
export const PADDING = 12
const PITCH = CELL + GAP

// CellRect는 오른쪽 위 기준 칸 좌표의 사각형이다(api.WidgetPlacement에서 addon을 뺀 것).
export interface CellRect {
  right: number
  top: number
  cols: number
  rows: number
}

export interface PxSize {
  width: number
  height: number
}

// PxRect는 데스크톱 영역 왼쪽 위 기준 픽셀 사각형이다.
export interface PxRect extends PxSize {
  x: number
  y: number
}

// spanCells는 길이 px 안에 들어가는 칸 수다.
const spanCells = (px: number) => Math.max(0, Math.floor((px - 2 * PADDING + GAP) / PITCH))

// gridSize는 데스크톱 영역 크기에 들어가는 그리드 열·행 수다.
export function gridSize(area: PxSize): GridSize {
  return { cols: spanCells(area.width), rows: spanCells(area.height) }
}

// spanPx는 칸 n개가 차지하는 픽셀 길이다(칸 사이 간격 포함).
export const spanPx = (n: number) => Math.max(0, n * PITCH - GAP)

// cellToPx는 칸 사각형의 픽셀 위치다. 그리드 밖(화면에 들어가지 않는 자리)도 계산은 한다.
export function cellToPx(r: CellRect, area: PxSize): PxRect {
  const width = spanPx(r.cols)
  const rightEdge = area.width - PADDING - r.right * PITCH
  return { x: rightEdge - width, y: PADDING + r.top * PITCH, width, height: spanPx(r.rows) }
}

// overlaps는 두 칸 사각형이 같은 칸을 차지하는지다.
export function overlaps(a: CellRect, b: CellRect): boolean {
  return a.right < b.right + b.cols && b.right < a.right + a.cols && a.top < b.top + b.rows && b.top < a.top + a.rows
}

// inGrid는 칸 사각형이 그리드 안에 모두 들어가는지다.
export function inGrid(r: CellRect, grid: GridSize): boolean {
  return r.right >= 0 && r.top >= 0 && r.right + r.cols <= grid.cols && r.top + r.rows <= grid.rows
}

// reservedCells는 픽셀 사각형(바탕화면 아이콘 목록)에 조금이라도 걸치는 칸들을 감싸는 칸 사각형이다.
// 걸치는 칸이 없으면 null.
export function reservedCells(px: PxRect | null, area: PxSize): CellRect | null {
  if (!px || px.width <= 0 || px.height <= 0) return null
  const grid = gridSize(area)
  const rights: number[] = []
  for (let r = 0; r < grid.cols; r++) {
    const c = cellToPx({ right: r, top: 0, cols: 1, rows: 1 }, area)
    if (c.x < px.x + px.width && px.x < c.x + c.width) rights.push(r)
  }
  const tops: number[] = []
  for (let t = 0; t < grid.rows; t++) {
    const c = cellToPx({ right: 0, top: t, cols: 1, rows: 1 }, area)
    if (c.y < px.y + px.height && px.y < c.y + c.height) tops.push(t)
  }
  if (rights.length === 0 || tops.length === 0) return null
  const right = Math.min(...rights)
  const top = Math.min(...tops)
  return { right, top, cols: Math.max(...rights) - right + 1, rows: Math.max(...tops) - top + 1 }
}

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), hi)

// snapCell은 위젯 왼쪽 위 픽셀 위치 pos에 가장 가까운 칸 자리다. 그리드 안으로 끌어 넣는다.
// 위젯이 그리드보다 크면 null.
export function snapCell(pos: { x: number; y: number }, size: GridSize, area: PxSize): CellRect | null {
  const grid = gridSize(area)
  if (size.cols > grid.cols || size.rows > grid.rows) return null
  // 오른쪽 끝에서 위젯 왼쪽 끝까지의 거리로 오른쪽 기준 칸을 구한다.
  const fromRight = area.width - PADDING - pos.x - spanPx(size.cols)
  return {
    right: clamp(Math.round(fromRight / PITCH), 0, grid.cols - size.cols),
    top: clamp(Math.round((pos.y - PADDING) / PITCH), 0, grid.rows - size.rows),
    cols: size.cols,
    rows: size.rows,
  }
}

// resizeCells는 오른쪽 아래 모서리를 (dx, dy)픽셀 끌었을 때의 칸 사각형이다. 왼쪽 위 모서리는 그대로다.
// 크기는 [min, max]와 그리드 안으로 제한한다(이미 그리드 밖에 걸친 위젯을 강제로 줄이지는 않는다).
export function resizeCells(
  start: CellRect,
  dx: number,
  dy: number,
  min: GridSize,
  max: GridSize,
  grid: GridSize,
): CellRect {
  const want = (cells: number, d: number) => Math.round((spanPx(cells) + d + GAP) / PITCH)
  // 왼쪽 끝을 고정하므로 넓히면 오른쪽 끝(right)이 줄어든다. right가 0보다 작아지면 그리드 밖이다.
  const leftFromRight = start.right + start.cols
  const maxCols = Math.max(Math.min(max.cols, leftFromRight), Math.min(start.cols, max.cols))
  const maxRows = Math.max(Math.min(max.rows, grid.rows - start.top), Math.min(start.rows, max.rows))
  const cols = clamp(want(start.cols, dx), min.cols, Math.max(min.cols, maxCols))
  const rows = clamp(want(start.rows, dy), min.rows, Math.max(min.rows, maxRows))
  return { right: leftFromRight - cols, top: start.top, cols, rows }
}
