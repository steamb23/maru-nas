import { describe, expect, it } from 'vitest'
import { MIN_VISIBLE, TITLEBAR_HEIGHT, cascadePosition, clampPosition } from './geometry'

const bounds = { width: 1280, height: 720 }
const size = { width: 400, height: 300 }

describe('clampPosition', () => {
  it('화면 안의 위치는 그대로 둔다', () => {
    expect(clampPosition({ x: 100, y: 50, ...size }, bounds)).toEqual({ x: 100, y: 50 })
  })

  it('위쪽으로는 화면 밖에 나가지 않는다', () => {
    expect(clampPosition({ x: 100, y: -500, ...size }, bounds).y).toBe(0)
  })

  it('아래쪽으로 끌어도 제목 표시줄 높이만큼 남는다', () => {
    expect(clampPosition({ x: 100, y: 5000, ...size }, bounds).y).toBe(bounds.height - TITLEBAR_HEIGHT)
  })

  it('왼쪽으로 끌어도 오른쪽 끝 MIN_VISIBLE만큼 남는다', () => {
    const { x } = clampPosition({ x: -5000, y: 0, ...size }, bounds)
    expect(x + size.width).toBe(MIN_VISIBLE)
  })

  it('오른쪽으로 끌어도 왼쪽 끝 MIN_VISIBLE만큼 남는다', () => {
    const { x } = clampPosition({ x: 5000, y: 0, ...size }, bounds)
    expect(bounds.width - x).toBe(MIN_VISIBLE)
  })

  it('데스크톱이 창보다 작아도 제목 표시줄은 잡을 수 있다', () => {
    const tiny = { width: 200, height: 100 }
    const p = clampPosition({ x: 5000, y: 5000, ...size }, tiny)
    expect(p.y).toBe(tiny.height - TITLEBAR_HEIGHT)
    expect(p.x).toBeLessThanOrEqual(tiny.width - MIN_VISIBLE)
    expect(p.x + size.width).toBeGreaterThanOrEqual(MIN_VISIBLE)
  })

  it('데스크톱 높이가 제목 표시줄보다 작으면 y는 0', () => {
    expect(clampPosition({ x: 0, y: 50, ...size }, { width: 800, height: 10 }).y).toBe(0)
  })
})

describe('cascadePosition', () => {
  it('창마다 오른쪽 아래로 비켜 놓는다', () => {
    const a = cascadePosition(0, size, bounds)
    const b = cascadePosition(1, size, bounds)
    expect(b.x).toBeGreaterThan(a.x)
    expect(b.y).toBeGreaterThan(a.y)
  })

  it('일정 개수마다 처음 위치로 돌아간다', () => {
    expect(cascadePosition(8, size, bounds)).toEqual(cascadePosition(0, size, bounds))
  })
})
