import { describe, expect, it } from 'vitest'
import {
  PREVIEW_MAX_STEP,
  type SpringRect,
  enterRect,
  slideKeyframes,
  springAt,
  springSettled,
  stepSpring,
} from './widgetPreviewAnim'

describe('enterRect', () => {
  it('가운데를 기준으로 줄인 자리다', () => {
    const r = enterRect({ x: 10, y: 10, width: 200, height: 100 })
    expect(r.x).toBeCloseTo(14)
    expect(r.y).toBeCloseTo(12)
    expect(r.width).toBeCloseTo(192)
    expect(r.height).toBeCloseTo(96)
  })
})

// run은 dt초씩 n프레임 진행한 스프링이다.
const run = (s: SpringRect, target: Parameters<typeof stepSpring>[1], n: number, dt = 1 / 60) => {
  for (let i = 0; i < n; i++) s = stepSpring(s, target, dt)
  return s
}

describe('stepSpring', () => {
  const from = { x: 0, y: 0, width: 100, height: 100 }
  const to = { x: 100, y: 50, width: 200, height: 100 }

  it('넘치지 않고 목표에 다가가 멈춘다', () => {
    let s = springAt(from)
    let prev = s.x
    for (let i = 0; i < 30; i++) {
      s = stepSpring(s, to, 1 / 60)
      expect(s.x).toBeGreaterThanOrEqual(prev)
      expect(s.x).toBeLessThanOrEqual(to.x)
      prev = s.x
    }
    expect(springSettled(s, to)).toBe(true)
  })

  it('8프레임(약 130ms)이면 95% 넘게 따라간다', () => {
    const s = run(springAt(from), to, 8)
    expect(s.x).toBeGreaterThan(95)
    expect(springSettled(s, to)).toBe(false)
  })

  it('목표가 바뀌어도 속도가 이어진다', () => {
    const moving = run(springAt(from), to, 3)
    const next = stepSpring(moving, { ...to, x: 200 }, 1 / 60)
    // 같은 방향으로 더 먼 목표가 생겨도 멈추지 않고 계속 빨라진다.
    expect(next.vx).toBeGreaterThan(moving.vx)
  })

  it('한 프레임이 길어도 최대 시간만큼만 진행한다', () => {
    const s = springAt(from)
    expect(stepSpring(s, to, 1)).toEqual(stepSpring(s, to, PREVIEW_MAX_STEP))
  })
})

describe('springSettled', () => {
  it('가까워도 빠르게 움직이면 멈춘 것이 아니다', () => {
    const r = { x: 0, y: 0, width: 10, height: 10 }
    expect(springSettled(springAt(r), r)).toBe(true)
    expect(springSettled({ ...springAt(r), vx: 100 }, r)).toBe(false)
  })
})

describe('slideKeyframes', () => {
  it('지금 보이는 자리에서 새 자리로 미끄러진다', () => {
    const layout = { x: 100, y: 50, width: 80, height: 80 }
    expect(slideKeyframes(layout, { x: 12, y: 50, width: 80, height: 80 })).toEqual([
      { transform: 'translate(-88px, 0px) scale(1, 1)' },
      { transform: 'none' },
    ])
  })
  it('크기가 바뀌면 크기도 함께 맞춘다', () => {
    const [from] = slideKeyframes({ x: 0, y: 0, width: 168, height: 80 }, { x: 0, y: 0, width: 84, height: 80 }) ?? []
    expect(from).toEqual({ transform: 'translate(0px, 0px) scale(0.5, 1)' })
  })
  it('같은 자리면 null', () => {
    const r = { x: 1, y: 2, width: 3, height: 4 }
    expect(slideKeyframes(r, { ...r, x: 1.2 })).toBeNull()
  })
})
