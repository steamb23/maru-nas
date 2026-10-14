import { describe, expect, it } from 'vitest'
import type { WidgetPlacement } from '../api/types.gen'
import { overlaps } from './grid'
import { MAX_WIDGET_COORD, canPlace, commitPlacement, fitLayout, nearestFree, placeNew, removePlacement } from './layout'

const grid = { cols: 11, rows: 6 }
const w = (addon: string, right: number, top: number, cols = 3, rows = 2): WidgetPlacement => ({
  addon,
  right,
  top,
  cols,
  rows,
})

// noOverlap은 배치 안의 위젯이 서로 겹치지 않는지다(저장 배치의 불변 조건).
function noOverlap(list: readonly WidgetPlacement[]): boolean {
  return list.every((a, i) => list.slice(i + 1).every((b) => !overlaps(a, b)))
}

describe('canPlace', () => {
  it('그리드 밖, 예약 칸, 다른 위젯과 겹치면 놓을 수 없다', () => {
    const r = w('a', 0, 0)
    expect(canPlace(r, [], grid, null)).toBe(true)
    expect(canPlace(w('a', 9, 0), [], grid, null)).toBe(false)
    expect(canPlace(r, [], grid, { right: 2, top: 1, cols: 1, rows: 1 })).toBe(false)
    expect(canPlace(r, [w('b', 2, 1)], grid, null)).toBe(false)
    expect(canPlace(r, [w('b', 3, 0)], grid, null)).toBe(true)
  })
})

describe('nearestFree', () => {
  it('가장 가까운 빈자리, 같으면 위쪽·오른쪽', () => {
    expect(nearestFree(w('a', 0, 0), [w('b', 0, 0)], grid, null)).toEqual({ right: 0, top: 2, cols: 3, rows: 2 })
    expect(nearestFree(w('a', 0, 0), [w('b', 0, 0, 11, 6)], grid, null)).toBeNull()
  })
})

describe('fitLayout', () => {
  it('들어가면 저장 자리 그대로', () => {
    const stored = [w('a', 0, 0), w('b', 3, 0)]
    expect(fitLayout(stored, grid, null)).toEqual({ shown: stored, hidden: [] })
  })

  it('그리드 밖으로 나간 위젯은 가까운 빈칸으로 옮기고 저장값은 바꾸지 않는다', () => {
    const stored = [w('a', 0, 0), w('b', 9, 0)]
    const copy = structuredClone(stored)
    expect(fitLayout(stored, grid, null).shown).toEqual([w('a', 0, 0), w('b', 8, 0)])
    expect(stored).toEqual(copy)
  })

  it('예약 칸(아이콘 영역)을 피한다', () => {
    const reserved = { right: 10, top: 0, cols: 1, rows: 4 }
    expect(fitLayout([w('a', 8, 0)], grid, reserved).shown).toEqual([w('a', 7, 0)])
  })

  it('제자리에 놓을 수 있는 위젯이 먼저 자리를 갖는다', () => {
    // b는 순서가 앞이지만 저장 자리가 그리드 밖이다. a의 자리를 빼앗지 않는다.
    const small = { cols: 11, rows: 3 }
    const fit = fitLayout([w('b', 0, 5), w('a', 0, 0)], small, null)
    expect(fit.shown).toEqual([w('b', 3, 1), w('a', 0, 0)])
  })

  it('자리가 없으면 숨기고, 다시 넓어지면 원래 자리로 돌아간다', () => {
    const stored = [w('a', 0, 0), w('b', 3, 0)]
    const tiny = { cols: 4, rows: 2 }
    expect(fitLayout(stored, tiny, null)).toEqual({ shown: [w('a', 0, 0)], hidden: ['b'] })
    expect(fitLayout(stored, grid, null).shown).toEqual(stored)
  })

  it('include로 뺀 위젯은 그리지도 숨김으로 세지도 않는다', () => {
    const fit = fitLayout([w('a', 0, 0), w('b', 3, 0)], grid, null, (p) => p.addon !== 'a')
    expect(fit).toEqual({ shown: [w('b', 3, 0)], hidden: [] })
  })
})

describe('placeNew', () => {
  it('오른쪽 열부터 위에서 아래로 채운다', () => {
    const size = { cols: 3, rows: 2 }
    expect(placeNew(size, [], grid, null)).toEqual({ right: 0, top: 0, ...size })
    expect(placeNew(size, [w('a', 0, 0)], grid, null)).toEqual({ right: 0, top: 2, ...size })
    expect(placeNew(size, [w('a', 0, 0, 3, 6)], grid, null)).toEqual({ right: 3, top: 0, ...size })
  })
  it('자리가 없으면 null', () => {
    expect(placeNew({ cols: 3, rows: 2 }, [w('a', 0, 0, 11, 6)], grid, null)).toBeNull()
  })
})

describe('commitPlacement', () => {
  it('옮긴 위젯만 바꾸고 순서를 유지한다', () => {
    const stored = [w('a', 0, 0), w('b', 0, 2)]
    expect(commitPlacement(stored, stored, w('a', 4, 0), grid)).toEqual([w('a', 4, 0), w('b', 0, 2)])
  })

  it('새 위젯은 맨 뒤에 붙는다', () => {
    const stored = [w('a', 0, 0)]
    expect(commitPlacement(stored, stored, w('c', 0, 2, 2, 2), grid)).toEqual([w('a', 0, 0), w('c', 0, 2, 2, 2)])
  })

  it('밀려난 위젯의 저장 자리는 덮지 않으면 그대로 둔다', () => {
    // b는 화면이 좁아 (8, 0)에 그려져 있지만 저장 자리는 (9, 0)이다.
    const stored = [w('a', 0, 0), w('b', 9, 0)]
    const displayed = [w('a', 0, 0), w('b', 8, 0)]
    expect(commitPlacement(stored, displayed, w('a', 0, 3), grid)).toEqual([w('a', 0, 3), w('b', 9, 0)])
  })

  it('밀려난 위젯의 저장 자리를 덮으면 그 위젯은 지금 보이는 자리로 간다', () => {
    const stored = [w('a', 0, 0), w('b', 9, 0)]
    const displayed = [w('a', 0, 0), w('b', 0, 3)]
    const next = commitPlacement(stored, displayed, w('a', 8, 0), grid)
    expect(next).toEqual([w('a', 8, 0), w('b', 0, 3)])
    expect(noOverlap(next)).toBe(true)
  })

  it('숨겨진 위젯의 저장 자리를 덮으면 그리드 아래로 옮긴다', () => {
    const stored = [w('a', 0, 0), w('b', 9, 0)]
    const displayed = [w('a', 0, 0)]
    const next = commitPlacement(stored, displayed, w('a', 8, 0), grid)
    expect(next).toEqual([w('a', 8, 0), w('b', 0, grid.rows)])
    expect(noOverlap(next)).toBe(true)
  })

  it('보이는 자리가 다른 저장 자리와 겹치면 그리드 아래로 옮긴다', () => {
    // b가 보이는 자리(0, 3)는 c의 저장 자리와 겹친다(c는 숨겨져 있음).
    const stored = [w('a', 0, 0), w('b', 9, 0), w('c', 1, 3)]
    const displayed = [w('a', 0, 0), w('b', 0, 3)]
    const next = commitPlacement(stored, displayed, w('a', 8, 0), grid)
    expect(next.find((p) => p.addon === 'b')).toEqual(w('b', 0, grid.rows))
    expect(noOverlap(next)).toBe(true)
  })

  it('저장 좌표 상한을 넘지 않는다', () => {
    const stored = [w('a', 0, 0), w('b', 0, 0, 3, 2)]
    const next = commitPlacement(stored, [], w('a', 0, 0, 2, 2), { cols: 11, rows: MAX_WIDGET_COORD })
    // 그리드 아래에 자리가 없으면 그 위젯은 빠진다(겹친 배치는 저장하지 않는다).
    expect(next).toEqual([w('a', 0, 0, 2, 2)])
  })
})

describe('removePlacement', () => {
  it('위젯 하나를 뺀다', () => {
    expect(removePlacement([w('a', 0, 0), w('b', 3, 0)], 'a')).toEqual([w('b', 3, 0)])
  })
})
