// 바탕화면 바로가기 칸 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 바로가기는 데스크톱 영역의 왼쪽 위부터 위에서 아래로 채우고, 열이 차면 오른쪽 다음 열로 넘어간다(열 우선).
// 칸 좌표는 데스크톱 영역 왼쪽 위 기준 픽셀이다. 끌어서 순서를 바꾸거나 "모든 앱"에서 끌어 놓을 때 쓴다.
import type { Point, Size } from '../windows/geometry'

// 아이콘 크기, 아이콘 사이 간격, 데스크톱 영역 가장자리와의 간격(CSS 픽셀).
// shell/DesktopShortcuts.css의 .desktop-icons(top·left, gap, 행 높이), shell/DesktopIcon.css의 .desktop-icon과 맞춘다.
export const ICON_WIDTH = 88
export const ICON_HEIGHT = 92
export const ICON_GAP = 8
export const ICONS_PADDING = 12

// rowsPerColumn은 높이 areaHeight인 데스크톱 영역에서 한 열에 들어가는 아이콘 수다(최소 1).
// CSS의 grid-template-rows: repeat(auto-fill, 92px)와 max-height: calc(100% - 24px)가 만드는 행 수와 같다.
export function rowsPerColumn(areaHeight: number): number {
  return Math.max(1, Math.floor((areaHeight - 2 * ICONS_PADDING + ICON_GAP) / (ICON_HEIGHT + ICON_GAP)))
}

// slotPosition은 index번째 칸의 왼쪽 위다.
export function slotPosition(index: number, rows: number): Point {
  const col = Math.floor(index / rows)
  const row = index % rows
  return { x: ICONS_PADDING + col * (ICON_WIDTH + ICON_GAP), y: ICONS_PADDING + row * (ICON_HEIGHT + ICON_GAP) }
}

// slotAt은 아이콘 가운데가 center에 있을 때 가장 가까운 칸이다. 0~maxIndex로 제한한다
// (순서 바꾸기는 count - 1, 새 아이콘을 끼울 때는 count까지).
export function slotAt(center: Point, rows: number, maxIndex: number): number {
  const col = Math.max(0, Math.round((center.x - ICONS_PADDING - ICON_WIDTH / 2) / (ICON_WIDTH + ICON_GAP)))
  const row = Math.round((center.y - ICONS_PADDING - ICON_HEIGHT / 2) / (ICON_HEIGHT + ICON_GAP))
  const index = col * rows + Math.min(Math.max(row, 0), rows - 1)
  return Math.min(Math.max(index, 0), Math.max(maxIndex, 0))
}

// iconCenter는 index번째 칸에 놓인 아이콘의 가운데다.
export function iconCenter(index: number, rows: number): Point {
  const p = slotPosition(index, rows)
  return { x: p.x + ICON_WIDTH / 2, y: p.y + ICON_HEIGHT / 2 }
}

// inBounds는 점이 데스크톱 영역 안에 있는지다.
export function inBounds(p: Point, size: Size): boolean {
  return p.x >= 0 && p.y >= 0 && p.x < size.width && p.y < size.height
}

// previewOffsets는 끄는 동안 각 아이콘(지금 순서의 index)이 원래 칸에서 미리보기 칸까지 움직일 거리다.
// from은 끄는 아이콘의 지금 index(밖에서 들어오는 새 아이콘이면 null), to는 놓을 칸(놓을 곳이 없으면 null)이다.
// 끄는 아이콘 자리에는 그 아이콘이 놓일 칸까지의 거리를 준다(포인터를 따라가는 쪽은 부르는 쪽이 정한다).
export function previewOffsets(count: number, from: number | null, to: number | null, rows: number): Point[] {
  const offsets: Point[] = []
  for (let i = 0; i < count; i++) {
    if (to === null) {
      offsets.push({ x: 0, y: 0 })
      continue
    }
    let target: number
    if (i === from) target = to
    else {
      // 끄는 아이콘을 뺀 순서에서의 자리, 그리고 to 칸을 비운 뒤의 자리
      const j = from !== null && i > from ? i - 1 : i
      target = j >= to ? j + 1 : j
    }
    const a = slotPosition(i, rows)
    const b = slotPosition(target, rows)
    offsets.push({ x: b.x - a.x, y: b.y - a.y })
  }
  return offsets
}
