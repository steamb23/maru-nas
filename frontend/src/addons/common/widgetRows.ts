// 위젯 본문에 들어가는 목록 줄 수 계산. widgets/grid.ts의 칸 크기와 위젯 틀 CSS(.widget-title,
// .addon-widget)에 맞춘다. DOM에 의존하지 않는다.
import { CELL, GAP } from '../../widgets/grid'

// 위젯 테두리(.widget, 위아래 1px씩), 제목 줄 높이(.widget-title), 본문 아래 여백(.addon-widget의 padding-bottom).
const BORDER = 2
const TITLE_HEIGHT = 28
const BOTTOM_PADDING = 10

// widgetBodyHeight는 rows칸 위젯의 본문 높이(px)다.
export function widgetBodyHeight(rows: number): number {
  return rows * CELL + (rows - 1) * GAP - BORDER - TITLE_HEIGHT - BOTTOM_PADDING
}

// widgetCapacity는 높이 itemHeight, 간격 gap인 줄이 rows칸 위젯에 몇 개 들어가는지다(최소 1).
export function widgetCapacity(rows: number, itemHeight: number, gap: number): number {
  return Math.max(1, Math.floor((widgetBodyHeight(rows) + gap) / (itemHeight + gap)))
}

// fitList는 capacity줄에 들어갈 항목과 나머지 개수다. 다 들어가지 않으면 마지막 줄을 "외 N개"에
// 내주되, 항목은 적어도 하나 보여 준다.
export function fitList<T>(list: readonly T[], capacity: number): { shown: T[]; hidden: number } {
  const shown = list.length > capacity ? list.slice(0, Math.max(1, capacity - 1)) : [...list]
  return { shown, hidden: list.length - shown.length }
}
