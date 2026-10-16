import { describe, expect, it } from 'vitest'
import { type ChartFrame, areaPath, linePath, nearestIndex, niceMax, pointX, stepIndex } from './chart'

const KiB = 1024
const MiB = 1024 * 1024

describe('niceMax', () => {
  it.each([
    [0, KiB],
    [1, KiB],
    [1500, 2 * KiB],
    [400 * KiB, 500 * KiB],
    [600 * KiB, MiB],
    [1.5 * MiB, 2 * MiB],
    [120 * MiB, 200 * MiB],
  ])('%d B/s → %d', (v, want) => {
    expect(niceMax(v)).toBe(want)
  })
})

const frame: ChartFrame = { now: 60_000, span: 60_000, max: 100, width: 600, height: 100 }

describe('linePath', () => {
  it('시각을 x로, 값을 위에서 아래로 그린다', () => {
    expect(
      linePath(
        [
          { t: 0, v: 0 },
          { t: 30_000, v: 50 },
          { t: 60_000, v: 100 },
        ],
        frame,
      ),
    ).toBe('M0 100L300 50L600 0')
  })

  it('값이 없는 점에서 끊고, 점 하나뿐인 구간은 그리지 않는다', () => {
    expect(
      linePath(
        [
          { t: 0, v: 10 },
          { t: 6000, v: 10 },
          { t: 12_000, v: null },
          { t: 18_000, v: 20 },
          { t: 24_000, v: null },
        ],
        frame,
      ),
    ).toBe('M0 90L60 90')
  })

  it('범위 밖 값은 축 안으로 자른다', () => {
    expect(
      linePath(
        [
          { t: 0, v: -5 },
          { t: 60_000, v: 150 },
        ],
        frame,
      ),
    ).toBe('M0 100L600 0')
  })
})

describe('areaPath', () => {
  it('구간마다 바닥까지 닫는다', () => {
    expect(
      areaPath(
        [
          { t: 30_000, v: 50 },
          { t: 60_000, v: 50 },
        ],
        frame,
      ),
    ).toBe('M300 50L600 50L600 100L300 100Z')
  })
})

describe('nearestIndex·stepIndex', () => {
  const times = [-2000, 20_000, 40_000, 60_000]

  it('x에 가장 가까운 그래프 안의 점이다', () => {
    expect(pointX(frame, 40_000)).toBe(400)
    expect(nearestIndex(times, frame, 0)).toBe(1)
    expect(nearestIndex(times, frame, 390)).toBe(2)
    expect(nearestIndex(times, frame, 600)).toBe(3)
    expect(nearestIndex([], frame, 100)).toBe(-1)
  })

  it('키보드로는 그래프 안의 점 사이에서 움직인다', () => {
    expect(stepIndex(times, frame, -1, -1)).toBe(3)
    expect(stepIndex(times, frame, 3, -1)).toBe(2)
    expect(stepIndex(times, frame, 1, -1)).toBe(1)
    expect(stepIndex(times, frame, 3, 1)).toBe(3)
    expect(stepIndex([], frame, -1, 1)).toBe(-1)
  })
})
