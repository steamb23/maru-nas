import { describe, expect, it } from 'vitest'
import { fitList, widgetBodyHeight, widgetCapacity } from './widgetRows'

describe('widgetRows', () => {
  it('본문 높이는 칸 높이에서 제목과 아래 여백을 뺀 값이다', () => {
    expect(widgetBodyHeight(2)).toBe(2 * 80 + 8 - 2 - 28 - 10)
  })
  it('들어가는 줄 수를 센다', () => {
    // 본문 128px: 30px 줄 + 8px 간격이면 3줄(30*3 + 8*2 = 106), 4줄은 144px라 넘친다.
    expect(widgetCapacity(2, 30, 8)).toBe(3)
    expect(widgetCapacity(1, 200, 8)).toBe(1)
  })
})

describe('fitList', () => {
  it('다 들어가면 모두 보여 준다', () => {
    expect(fitList([1, 2, 3], 3)).toEqual({ shown: [1, 2, 3], hidden: 0 })
  })
  it('넘치면 마지막 줄을 나머지 개수에 쓴다', () => {
    expect(fitList([1, 2, 3, 4], 3)).toEqual({ shown: [1, 2], hidden: 2 })
  })
  it('한 줄뿐이면 항목 하나만 보여 주고 나머지 개수 줄은 두지 않는다', () => {
    expect(fitList([1, 2], 1)).toEqual({ shown: [1], hidden: 0 })
  })
})
