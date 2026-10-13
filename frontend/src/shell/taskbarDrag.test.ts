import { describe, expect, it } from 'vitest'
import { type Slot, dragLayout } from './taskbarDrag'

// 너비가 서로 다른 버튼 세 개, 간격 4
//   0: [10, 110)  중심 60
//   1: [114, 164) 중심 139
//   2: [168, 318) 중심 243
const gap = 4
const slots: Slot[] = [
  { left: 10, width: 100 },
  { left: 114, width: 50 },
  { left: 168, width: 150 },
]

// packedLefts는 order 순서로 버튼을 빈틈없이 늘어놓았을 때 각 버튼(원래 인덱스 기준)의 왼쪽 위치다.
function packedLefts(order: number[]): number[] {
  const lefts: number[] = []
  let x = slots[0].left
  for (const i of order) {
    lefts[i] = x
    x += slots[i].width + gap
  }
  return lefts
}

describe('dragLayout', () => {
  it('조금 끌면 자리가 바뀌지 않는다', () => {
    const l = dragLayout(slots, gap, 0, 20)
    expect(l).toEqual({ dx: 20, to: 0, shifts: [0, 0, 0], settle: 0 })
  })

  it('오른쪽 끝이 이웃 버튼의 중심을 넘으면 그 버튼이 비켜난다', () => {
    // 버튼 0의 오른쪽 끝 110 + 28 = 138 < 139
    expect(dragLayout(slots, gap, 0, 28).to).toBe(0)
    // 110 + 30 = 140 > 139
    const l = dragLayout(slots, gap, 0, 30)
    expect(l.to).toBe(1)
    expect(l.shifts).toEqual([0, -104, 0])
  })

  it('왼쪽으로 끌면 사이의 버튼이 오른쪽으로 비켜난다', () => {
    // 버튼 2의 왼쪽 끝 168 - 110 = 58 < 60
    const l = dragLayout(slots, gap, 2, -110)
    expect(l.to).toBe(0)
    expect(l.shifts).toEqual([154, 154, 0])
  })

  it('이웃보다 넓은 버튼도 양 끝 자리까지 옮길 수 있다', () => {
    expect(dragLayout(slots, gap, 2, -999).to).toBe(0)
    expect(dragLayout([slots[1], slots[2]], gap, 1, -999).to).toBe(0)
  })

  it('목록 범위 밖으로는 끌리지 않는다', () => {
    expect(dragLayout(slots, gap, 0, -50).dx).toBe(0)
    expect(dragLayout(slots, gap, 0, 999).dx).toBe(318 - 110)
    expect(dragLayout(slots, gap, 2, -999).dx).toBe(10 - 168)
  })

  it('비켜난 버튼과 도착 위치가 실제로 옮긴 뒤의 배치와 같다', () => {
    const cases: [number, number, number[]][] = [
      [0, 999, [1, 2, 0]],
      [0, 30, [1, 0, 2]],
      [2, -999, [2, 0, 1]],
      [1, -60, [1, 0, 2]],
      [1, 100, [0, 2, 1]],
    ]
    for (const [from, dx, order] of cases) {
      const l = dragLayout(slots, gap, from, dx)
      const after = packedLefts(order)
      expect(l.to).toBe(order.indexOf(from))
      slots.forEach((s, i) => {
        const offset = i === from ? l.settle : l.shifts[i]
        expect(s.left + offset).toBe(after[i])
      })
    }
  })

  it('버튼이 하나면 움직이지 않는다', () => {
    expect(dragLayout([slots[0]], gap, 0, 50)).toEqual({ dx: 0, to: 0, shifts: [0], settle: 0 })
  })
})
