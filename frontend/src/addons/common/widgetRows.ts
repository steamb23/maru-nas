// 위젯 본문에 들어가는 목록 줄 수 계산. widgets/grid.ts의 칸 크기와 위젯 틀 CSS(.widget-title,
// .addon-widget)에 맞춘다. DOM에 의존하지 않는다.
import { CELL, GAP } from '../../widgets/grid'

// 위젯 제목 줄 높이(.widget-title)와 본문 아래 여백(.addon-widget의 padding-bottom).
const TITLE_HEIGHT = 28
const BOTTOM_PADDING = 10

// widgetBodyHeight는 rows칸 위젯의 본문 높이(px)다.
export function widgetBodyHeight(rows: number): number {
  return rows * CELL + (rows - 1) * GAP - TITLE_HEIGHT - BOTTOM_PADDING
}

// widgetCapacity는 높이 itemHeight, 간격 gap인 줄이 rows칸 위젯에 몇 개 들어가는지다(최소 1).
export function widgetCapacity(rows: number, itemHeight: number, gap: number): number {
  return Math.max(1, Math.floor((widgetBodyHeight(rows) + gap) / (itemHeight + gap)))
}
