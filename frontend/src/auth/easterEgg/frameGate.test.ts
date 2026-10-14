import { describe, expect, it } from 'vitest'
import { createFrameGate } from './frameGate'

// count는 주사율 hz의 rAF를 seconds초 동안 흘렸을 때 그린 프레임 수다.
function count(fps: number, hz: number, seconds: number): number {
  const gate = createFrameGate(fps)
  let drawn = 0
  for (let i = 0; i <= hz * seconds; i++) {
    if (gate.tick((i * 1000) / hz)) drawn++
  }
  return drawn
}

describe('createFrameGate', () => {
  it('첫 프레임은 그린다', () => {
    expect(createFrameGate(30).tick(1234)).toBe(true)
  })

  it('60Hz·144Hz 화면에서 30fps를 지킨다', () => {
    expect(count(30, 60, 10)).toBeGreaterThanOrEqual(299)
    expect(count(30, 60, 10)).toBeLessThanOrEqual(301)
    expect(count(30, 144, 10)).toBeGreaterThanOrEqual(299)
    expect(count(30, 144, 10)).toBeLessThanOrEqual(301)
  })

  it('15fps로 바꿀 수 있다', () => {
    expect(count(15, 60, 10)).toBeGreaterThanOrEqual(149)
    expect(count(15, 60, 10)).toBeLessThanOrEqual(151)
  })

  it('주사율이 fps보다 낮으면 모든 프레임을 그린다', () => {
    expect(count(30, 24, 10)).toBe(241)
  })

  it('오래 멈췄다 돌아오면 바로 그리고 다시 일정하게 그린다', () => {
    const gate = createFrameGate(30)
    gate.tick(0)
    expect(gate.tick(5000)).toBe(true)
    expect(gate.tick(5016)).toBe(false)
    expect(gate.tick(5033)).toBe(true)
  })
})
