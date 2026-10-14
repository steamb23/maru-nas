// 위젯 배치 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 저장 배치(stored)와 화면 배치(displayed)를 나눈다.
// - 저장 배치는 사용자가 놓은 자리다. 서로 겹치지 않는다(백엔드도 검사한다).
// - 화면 배치는 지금 화면 크기와 아이콘 영역에 맞춘 자리다(fitLayout). 화면이 좁아 저장 자리에
//   들어가지 않는 위젯은 가까운 빈칸으로 옮기거나 숨기고, 저장 배치는 그대로 둔다.
//   그래서 화면을 다시 넓히면 원래 자리로 돌아간다.
// - 사용자가 옮기거나 크기를 바꾸거나 추가한 위젯만 저장 배치를 바꾼다(commitPlacement).
import type { GridSize, WidgetPlacement } from '../api/types.gen'
import { type CellRect, inGrid, overlaps } from './grid'

// MAX_WIDGET_COORD는 저장 좌표의 상한이다(백엔드 addons.MaxWidgetCoord와 같아야 한다).
export const MAX_WIDGET_COORD = 200

export interface FitResult {
  // 화면에 그릴 위젯과 자리. 저장 순서를 따른다.
  shown: WidgetPlacement[]
  // 자리가 없어 숨긴 위젯의 애드온 id.
  hidden: string[]
}

const rectOf = (p: CellRect): CellRect => ({ right: p.right, top: p.top, cols: p.cols, rows: p.rows })

// canPlace는 칸 사각형을 그리드 안, 예약 칸 밖, others와 겹치지 않는 자리에 놓을 수 있는지다.
export function canPlace(r: CellRect, others: readonly CellRect[], grid: GridSize, reserved: CellRect | null): boolean {
  return inGrid(r, grid) && !(reserved && overlaps(r, reserved)) && others.every((o) => !overlaps(r, o))
}

// nearestFree는 from에서 가장 가까운 놓을 수 있는 자리다(같은 크기). 거리가 같으면 위쪽, 그다음 오른쪽.
// 없으면 null.
export function nearestFree(
  from: CellRect,
  others: readonly CellRect[],
  grid: GridSize,
  reserved: CellRect | null,
): CellRect | null {
  let best: CellRect | null = null
  let bestDist = Infinity
  for (let top = 0; top + from.rows <= grid.rows; top++) {
    for (let right = 0; right + from.cols <= grid.cols; right++) {
      const r = { right, top, cols: from.cols, rows: from.rows }
      const dist = (right - from.right) ** 2 + (top - from.top) ** 2
      if (dist < bestDist && canPlace(r, others, grid, reserved)) {
        best = r
        bestDist = dist
      }
    }
  }
  return best
}

// fitLayout은 저장 배치를 지금 그리드에 맞춘 화면 배치다. 저장 자리에 그대로 놓을 수 있는 위젯을 먼저
// 놓고(저장 배치는 서로 겹치지 않으므로 이들끼리는 부딪치지 않는다), 나머지는 저장 순서대로 저장 자리에서
// 가장 가까운 빈칸에 놓는다. 빈칸이 없으면 숨긴다. include로 그리지 않을 위젯(쓸 수 없는 애드온 등)을 뺀다.
export function fitLayout(
  stored: readonly WidgetPlacement[],
  grid: GridSize,
  reserved: CellRect | null,
  include: (p: WidgetPlacement) => boolean = () => true,
): FitResult {
  const items = stored.filter(include)
  const placed = new Map<string, CellRect>()
  for (const p of items) {
    if (canPlace(p, [], grid, reserved)) placed.set(p.addon, rectOf(p))
  }
  const hidden: string[] = []
  for (const p of items) {
    if (placed.has(p.addon)) continue
    const spot = nearestFree(p, [...placed.values()], grid, reserved)
    if (spot) placed.set(p.addon, spot)
    else hidden.push(p.addon)
  }
  const shown = items.flatMap((p) => {
    const r = placed.get(p.addon)
    return r ? [{ addon: p.addon, ...r }] : []
  })
  return { shown, hidden }
}

// placeNew는 새 위젯을 놓을 자리다. 오른쪽 열부터 위에서 아래로 첫 빈칸을 찾는다. 없으면 null.
export function placeNew(
  size: GridSize,
  displayed: readonly CellRect[],
  grid: GridSize,
  reserved: CellRect | null,
): CellRect | null {
  for (let right = 0; right + size.cols <= grid.cols; right++) {
    for (let top = 0; top + size.rows <= grid.rows; top++) {
      const r = { right, top, cols: size.cols, rows: size.rows }
      if (canPlace(r, displayed, grid, reserved)) return r
    }
  }
  return null
}

// belowGrid는 저장 좌표에서 그리드 바로 아래(top ≥ grid.rows)부터 others와 겹치지 않는 첫 자리다.
// 화면에 들어갈 자리가 없는 위젯을 저장할 때 쓴다. 상한 안에서 찾지 못하면 null.
function belowGrid(size: CellRect, others: readonly CellRect[], grid: GridSize): CellRect | null {
  for (let top = grid.rows; top + size.rows <= MAX_WIDGET_COORD; top++) {
    for (let right = 0; right + size.cols <= MAX_WIDGET_COORD; right++) {
      const r = { right, top, cols: size.cols, rows: size.rows }
      if (others.every((o) => !overlaps(r, o))) return r
    }
  }
  return null
}

// commitPlacement는 위젯 하나(edited)를 옮기거나 크기를 바꾸거나 새로 추가했을 때의 저장 배치다.
// edited의 자리는 화면 배치에서 비어 있는 자리여야 한다(canPlace로 확인한 뒤 부른다).
// 다른 위젯의 저장 자리는 바꾸지 않는다. 다만 edited가 화면이 좁아 밀려난 위젯의 저장 자리를 덮으면
// 그 위젯은 지금 화면에 보이는 자리로, 그 자리도 다른 저장 자리와 겹치면 그리드 아래로 옮긴다.
// 결과는 저장 좌표에서 서로 겹치지 않는다. 순서는 유지하되 새 위젯과 밀려난 위젯은 맨 뒤에 붙는다
// (fitLayout은 뒤쪽 위젯부터 화면 밖으로 밀어낸다).
export function commitPlacement(
  stored: readonly WidgetPlacement[],
  displayed: readonly WidgetPlacement[],
  edited: WidgetPlacement,
  grid: GridSize,
): WidgetPlacement[] {
  const shownAt = new Map(displayed.map((p) => [p.addon, p]))
  let next = stored.filter((p) => p.addon !== edited.addon).map((p) => ({ ...p }))
  const bumped = next.filter((p) => overlaps(p, edited)).map((p) => p.addon)
  // 겹친 위젯을 빼고 edited를 넣은 뒤, 겹쳤던 위젯을 하나씩 새 자리에 다시 넣는다.
  next = next.filter((p) => !bumped.includes(p.addon))
  const at = stored.findIndex((p) => p.addon === edited.addon)
  next.splice(at < 0 ? next.length : Math.min(at, next.length), 0, { ...edited })
  for (const addon of bumped) {
    const p = stored.find((o) => o.addon === addon)!
    const shown = shownAt.get(addon)
    const spot = shown && next.every((o) => !overlaps(shown, o)) ? rectOf(shown) : belowGrid(p, next, grid)
    // 상한 안에 자리가 없으면(위젯이 아주 많을 때) 그 위젯을 뺀다. 겹친 배치는 저장할 수 없다.
    if (spot) next.push({ addon, ...spot })
  }
  return next
}

// removePlacement는 위젯 하나를 뺀 저장 배치다.
export function removePlacement(stored: readonly WidgetPlacement[], addon: string): WidgetPlacement[] {
  return stored.filter((p) => p.addon !== addon)
}
