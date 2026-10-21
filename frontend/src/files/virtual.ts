// 가상 스크롤 계산. 줄 높이가 같은 목록에서 보이는 줄만 그린다. 전체 높이가 브라우저의 요소 높이 한계(Firefox 약
// 1,790만 px)에 가까우면 스크롤 높이를 MAX_SCROLL_HEIGHT로 줄이고 스크롤 위치를 비율로 바꾼다.
// DOM에 의존하지 않아 node 환경에서 테스트한다.

// ROW_HEIGHT는 목록 한 줄의 높이(px)다. CSS(.files-row)와 맞춘다.
export const ROW_HEIGHT = 34

// MAX_SCROLL_HEIGHT는 스크롤 영역의 최대 높이(px)다. 넘으면 비율로 줄인다.
export const MAX_SCROLL_HEIGHT = 8_000_000

// OVERSCAN은 보이는 줄 위아래로 더 그리는 줄 수다.
export const OVERSCAN = 6

export interface Layout {
  // scrollHeight는 스크롤 영역의 높이(px)다.
  scrollHeight: number
  // first, last는 그릴 줄의 범위다(last는 포함하지 않는다).
  first: number
  last: number
  // top은 first 줄을 그릴 스크롤 영역 안의 위치(px)다. 다음 줄은 ROW_HEIGHT씩 아래다.
  top: number
}

// contentHeight는 줄이지 않은 전체 높이다.
const contentHeight = (total: number, rowHeight: number) => total * rowHeight

// scrollHeightOf는 스크롤 영역의 높이다.
export function scrollHeightOf(total: number, rowHeight = ROW_HEIGHT): number {
  return Math.min(contentHeight(total, rowHeight), MAX_SCROLL_HEIGHT)
}

// virtualOffset은 스크롤 위치(px)에 해당하는 전체 목록 안의 위치(px)다. 줄이지 않았으면 같다.
export function virtualOffset(scrollTop: number, viewHeight: number, total: number, rowHeight = ROW_HEIGHT): number {
  const full = contentHeight(total, rowHeight)
  const physical = scrollHeightOf(total, rowHeight)
  if (physical >= full || physical <= viewHeight) return scrollTop
  return (scrollTop * (full - viewHeight)) / (physical - viewHeight)
}

// scrollTopFor는 전체 목록 안의 위치(px)를 스크롤 위치로 바꾼다(virtualOffset의 역).
export function scrollTopFor(offset: number, viewHeight: number, total: number, rowHeight = ROW_HEIGHT): number {
  const full = contentHeight(total, rowHeight)
  const physical = scrollHeightOf(total, rowHeight)
  if (physical >= full || physical <= viewHeight) return offset
  return (offset * (physical - viewHeight)) / (full - viewHeight)
}

// layout은 스크롤 위치에서 그릴 줄과 그 자리다.
export function layout(
  total: number,
  scrollTop: number,
  viewHeight: number,
  rowHeight = ROW_HEIGHT,
  overscan = OVERSCAN,
): Layout {
  const scrollHeight = scrollHeightOf(total, rowHeight)
  if (total === 0) return { scrollHeight, first: 0, last: 0, top: 0 }
  const maxTop = Math.max(0, scrollHeight - viewHeight)
  const st = Math.min(Math.max(0, scrollTop), maxTop)
  const offset = virtualOffset(st, viewHeight, total, rowHeight)
  const firstVisible = Math.min(total - 1, Math.floor(offset / rowHeight))
  const first = Math.max(0, firstVisible - overscan)
  const last = Math.min(total, Math.ceil((offset + viewHeight) / rowHeight) + overscan)
  // first 줄의 화면 위치: 보이는 영역 위끝(st)에서 offset과의 차이만큼 떨어진 곳
  const top = st + (first * rowHeight - offset)
  return { scrollHeight, first, last, top }
}

// scrollToShow는 index 줄이 보이도록 할 스크롤 위치다. 이미 보이면 지금 위치 그대로다.
export function scrollToShow(
  index: number,
  total: number,
  scrollTop: number,
  viewHeight: number,
  rowHeight = ROW_HEIGHT,
): number {
  const offset = virtualOffset(scrollTop, viewHeight, total, rowHeight)
  const rowTop = index * rowHeight
  let target = offset
  if (rowTop < offset) target = rowTop
  else if (rowTop + rowHeight > offset + viewHeight) target = rowTop + rowHeight - viewHeight
  if (target === offset) return scrollTop
  return scrollTopFor(target, viewHeight, total, rowHeight)
}

// indexAt은 스크롤 위치에서 보이는 첫 줄이다(스크롤 위치 말풍선).
export function indexAt(total: number, scrollTop: number, viewHeight: number, rowHeight = ROW_HEIGHT): number {
  if (total === 0) return 0
  return Math.min(total - 1, Math.max(0, Math.floor(virtualOffset(scrollTop, viewHeight, total, rowHeight) / rowHeight)))
}

// 아이콘 보기는 칸(타일)을 시각 행으로 묶어 같은 계산을 쓴다. 목록 보기는 한 행에 한 칸인 경우다.

// TILE_MIN_WIDTH는 아이콘 보기 칸의 최소 너비(px), TILE_HEIGHT는 칸 높이(px)다. CSS(.files-tile)와 맞춘다.
// GRID_PAD는 스크롤 영역 좌우 안쪽 여백(px)이다(.files-spacer .files-row의 left·right와 같다).
export const TILE_MIN_WIDTH = 104
export const TILE_HEIGHT = 108
export const GRID_PAD = 8

export interface Grid {
  // cols는 한 행의 칸 수, rowHeight는 행 높이, tileWidth는 칸 너비(px)다(남는 폭을 칸에 나눠 준다).
  cols: number
  rowHeight: number
  tileWidth: number
}

// gridOf는 보기 모양과 스크롤 영역 너비로 정한 칸 배치다. 너비를 아직 모르면 한 칸으로 본다.
export function gridOf(mode: 'list' | 'grid', width: number): Grid {
  const inner = Math.max(0, width - 2 * GRID_PAD)
  if (mode === 'list') return { cols: 1, rowHeight: ROW_HEIGHT, tileWidth: inner }
  const cols = Math.max(1, Math.floor(inner / TILE_MIN_WIDTH))
  return { cols, rowHeight: TILE_HEIGHT, tileWidth: inner / cols }
}

// rowsOf는 칸 total개를 담는 시각 행 수다.
export const rowsOf = (total: number, cols: number) => Math.ceil(total / Math.max(1, cols))

// itemRange는 그릴 행 범위(layout의 first·last)에 든 칸 범위다(last는 포함하지 않는다).
export function itemRange(lay: Layout, cols: number, total: number): { first: number; last: number } {
  return { first: Math.min(total, lay.first * cols), last: Math.min(total, lay.last * cols) }
}

// moveKey는 방향 키로 옮겨 갈 칸이다. cur는 지금 칸(없으면 -1), pageRows는 PageUp/Down이 넘기는 행 수다.
// 처리하지 않는 키면 null이다. 목록 보기(cols 1)에서는 왼쪽·오른쪽 화살표를 쓰지 않는다.
export function moveKey(key: string, cur: number, cols: number, total: number, pageRows: number): number | null {
  if (total === 0) return null
  const clamp = (i: number) => Math.min(Math.max(0, i), total - 1)
  switch (key) {
    case 'ArrowDown':
      return cur < 0 ? 0 : clamp(cur + cols)
    case 'ArrowUp':
      return cur < 0 ? 0 : clamp(cur - cols)
    case 'ArrowRight':
      return cols > 1 ? clamp(cur + 1) : null
    case 'ArrowLeft':
      return cols > 1 ? (cur < 0 ? 0 : clamp(cur - 1)) : null
    case 'PageDown':
      return clamp(cur + pageRows * cols)
    case 'PageUp':
      return clamp(cur - pageRows * cols)
    case 'Home':
      return 0
    case 'End':
      return total - 1
    default:
      return null
  }
}
