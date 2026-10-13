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
export const MIN_VISIBLE = 80

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

// cascadePosition은 index번째로 여는 창의 위치를 계단식으로 정한다.
export function cascadePosition(index: number, size: Size, bounds: Size): Point {
  const step = (index % CASCADE_COUNT) * CASCADE_STEP
  return clampPosition(
    { x: CASCADE_ORIGIN.x + step, y: CASCADE_ORIGIN.y + step, ...size },
    bounds,
  )
}
