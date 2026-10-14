import { describe, expect, it } from 'vitest'
import { widgetBodyHeight, widgetCapacity } from './widgetRows'

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
