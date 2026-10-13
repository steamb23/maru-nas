// 창 위치 계산. 크기는 모두 CSS 픽셀이며 좌표 원점은 데스크톱 영역(작업표시줄 제외) 왼쪽 위다.

export interface Size {
  width: number
  height: number
}

export interface Point {
  x: number
  y: number
}

export interface Rect extends Point, Size {}

// Window.tsx의 제목 표시줄 높이와 맞춰야 한다(index.css의 --titlebar-height).
export const TITLEBAR_HEIGHT = 36
// 창을 좌우로 화면 밖에 끌어도 제목 표시줄이 이만큼은 화면에 남는다.
// 왼쪽으로 밀면 오른쪽 끝의 창 버튼 영역(index.css의 .win-btn 30px × 3 + 간격 + 여백 ≈ 98px)만
// 남게 되는데, 버튼 위에서는 드래그가 시작되지 않으므로 그보다 넉넉히 잡는다.
export const MIN_VISIBLE = 150
// 크기 조절로 줄일 수 있는 최소 크기. 제목 표시줄의 아이콘·제목 일부·버튼 영역이 들어가야 하며,
// MIN_WIDTH ≥ MIN_VISIBLE이어야 resizeRect의 제한이 clampPosition과 어긋나지 않는다.
export const MIN_WIDTH = 240
export const MIN_HEIGHT = 120

// 크기 조절 핸들의 방향. 문자가 끄는 변을 나타낸다(n: 위, s: 아래, e: 오른쪽, w: 왼쪽).
export type ResizeEdge = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

// 새 창의 첫 위치와 계단식 간격. 왼쪽의 바탕화면 아이콘 열을 피한다.
const CASCADE_ORIGIN: Point = { x: 140, y: 40 }
const CASCADE_STEP = 32
const CASCADE_COUNT = 8

function clamp(v: number, min: number, max: number): number {
  return Math.min(Math.max(v, min), max)
}

// clampPosition은 제목 표시줄을 다시 잡을 수 있도록 창 위치를 제한한다.
// 위쪽으로는 화면 밖에 나갈 수 없고, 아래쪽으로는 제목 표시줄 높이만큼 남는다.
// 좌우로는 창 너비 중 MIN_VISIBLE만큼 남는다.
export function clampPosition(rect: Rect, bounds: Size): Point {
  const visible = Math.min(MIN_VISIBLE, rect.width)
  const minX = visible - rect.width
  const maxX = Math.max(minX, bounds.width - visible)
  const maxY = Math.max(0, bounds.height - TITLEBAR_HEIGHT)
  return {
    x: clamp(Math.round(rect.x), minX, maxX),
    y: clamp(Math.round(rect.y), 0, maxY),
  }
}

// clampEdge는 끄는 변의 좌표를 [lo, hi]로 제한한다. 데스크톱이 아주 작아 lo > hi이면 lo를 우선한다.
function clampEdge(v: number, lo: number, hi: number): number {
  return Math.max(Math.min(v, hi), lo)
}

// resizeRect는 start 창의 edge 쪽 변을 (dx, dy)만큼 끌었을 때의 새 위치·크기를 계산한다.
// 끄는 변만 움직이고 반대쪽 변은 고정된다. 제한은 다음과 같다.
//   - 너비·높이는 MIN_WIDTH·MIN_HEIGHT 이상
//   - 데스크톱 밖으로는 키울 수 없다. 이미 밖에 걸쳐 있던 창을 강제로 줄이지는 않는다
//   - 제목 표시줄을 다시 잡을 수 있다(clampPosition과 같은 조건)
export function resizeRect(start: Rect, edge: ResizeEdge, dx: number, dy: number, bounds: Size): Rect {
  let left = start.x
  let top = start.y
  let right = start.x + start.width
  let bottom = start.y + start.height

  if (edge.includes('w')) {
    left = clampEdge(left + dx, Math.min(0, start.x), Math.min(right - MIN_WIDTH, bounds.width - MIN_VISIBLE))
  } else if (edge.includes('e')) {
    right = clampEdge(right + dx, Math.max(left + MIN_WIDTH, MIN_VISIBLE), Math.max(bounds.width, right))
  }
  if (edge.includes('n')) {
    top = clampEdge(top + dy, 0, Math.min(bottom - MIN_HEIGHT, bounds.height - TITLEBAR_HEIGHT))
  } else if (edge.includes('s')) {
    bottom = clampEdge(bottom + dy, top + MIN_HEIGHT, Math.max(bounds.height, bottom))
  }

  const x = Math.round(left)
  const y = Math.round(top)
  return { x, y, width: Math.round(right) - x, height: Math.round(bottom) - y }
}

// cascadePosition은 index번째로 여는 창의 위치를 계단식으로 정한다.
export function cascadePosition(index: number, size: Size, bounds: Size): Point {
  const step = (index % CASCADE_COUNT) * CASCADE_STEP
  return clampPosition(
    { x: CASCADE_ORIGIN.x + step, y: CASCADE_ORIGIN.y + step, ...size },
    bounds,
  )
}
