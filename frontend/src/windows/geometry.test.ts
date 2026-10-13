import { describe, expect, it } from 'vitest'
import {
  MIN_HEIGHT,
  MIN_VISIBLE,
  MIN_WIDTH,
  type Rect,
  type ResizeEdge,
  TITLEBAR_HEIGHT,
  cascadePosition,
  clampPosition,
  resizeRect,
} from './geometry'

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

describe('resizeRect', () => {
  const start: Rect = { x: 200, y: 100, width: 400, height: 300 }

  it('오른쪽·아래 변을 끌면 위치는 그대로 두고 크기만 바꾼다', () => {
    expect(resizeRect(start, 'e', 50, 999, bounds)).toEqual({ ...start, width: 450 })
    expect(resizeRect(start, 's', 999, 40, bounds)).toEqual({ ...start, height: 340 })
  })

  it('왼쪽·위쪽 변을 끌면 반대쪽 변은 고정된다', () => {
    const w = resizeRect(start, 'w', -50, 0, bounds)
    expect(w).toEqual({ x: 150, y: 100, width: 450, height: 300 })
    const n = resizeRect(start, 'n', 0, 30, bounds)
    expect(n).toEqual({ x: 200, y: 130, width: 400, height: 270 })
  })

  it('모서리는 두 축을 함께 바꾼다', () => {
    expect(resizeRect(start, 'nw', -20, -30, bounds)).toEqual({ x: 180, y: 70, width: 420, height: 330 })
    expect(resizeRect(start, 'se', 20, 30, bounds)).toEqual({ x: 200, y: 100, width: 420, height: 330 })
  })

  it('최소 크기보다 줄어들지 않는다', () => {
    const se = resizeRect(start, 'se', -5000, -5000, bounds)
    expect(se).toEqual({ x: 200, y: 100, width: MIN_WIDTH, height: MIN_HEIGHT })
  })

  it('왼쪽·위쪽 변으로 최소 크기까지 줄여도 반대쪽 변은 고정된다', () => {
    const nw = resizeRect(start, 'nw', 5000, 5000, bounds)
    expect(nw.width).toBe(MIN_WIDTH)
    expect(nw.height).toBe(MIN_HEIGHT)
    expect(nw.x + nw.width).toBe(start.x + start.width)
    expect(nw.y + nw.height).toBe(start.y + start.height)
  })

  it('위쪽 변은 화면 위로 나가지 않는다', () => {
    const n = resizeRect(start, 'n', 0, -5000, bounds)
    expect(n.y).toBe(0)
    expect(n.y + n.height).toBe(start.y + start.height)
  })

  it('데스크톱 밖으로는 키울 수 없다', () => {
    const se = resizeRect(start, 'se', 5000, 5000, bounds)
    expect(se.x + se.width).toBe(bounds.width)
    expect(se.y + se.height).toBe(bounds.height)
    expect(resizeRect(start, 'w', -5000, 0, bounds).x).toBe(0)
  })

  it('이미 데스크톱 밖에 걸친 창은 강제로 줄이지 않는다', () => {
    const over: Rect = { x: 1000, y: 600, width: 400, height: 300 }
    expect(resizeRect(over, 'se', 10, 10, bounds)).toEqual(over)
    const left: Rect = { x: -100, y: 100, width: 400, height: 300 }
    expect(resizeRect(left, 'w', -10, 0, bounds)).toEqual(left)
  })

  it('결과는 정수 픽셀이다', () => {
    const r = resizeRect(start, 'nw', 10.4, -7.6, bounds)
    for (const v of Object.values(r)) expect(Number.isInteger(v)).toBe(true)
  })

  it('minSize를 넘기면 그 크기에서 멈춘다', () => {
    const minSize = { width: 320, height: 260 }
    expect(resizeRect(start, 'se', -5000, -5000, bounds, minSize)).toEqual({ x: 200, y: 100, ...minSize })
    const nw = resizeRect(start, 'nw', 5000, 5000, bounds, minSize)
    expect(nw).toEqual({ x: start.x + start.width - 320, y: start.y + start.height - 260, ...minSize })
  })

  it('공통 하한보다 작은 minSize는 공통 하한으로 올라간다', () => {
    const r = resizeRect(start, 'se', -5000, -5000, bounds, { width: 10, height: 10 })
    expect(r.width).toBe(MIN_WIDTH)
    expect(r.height).toBe(MIN_HEIGHT)
  })

  it('크기를 조절해도 제목 표시줄은 다시 잡을 수 있다', () => {
    const edges: ResizeEdge[] = ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw']
    const starts = [
      start,
      { ...clampPosition({ x: -5000, y: 0, ...size }, bounds), ...size },
      { ...clampPosition({ x: 5000, y: 5000, ...size }, bounds), ...size },
    ]
    const deltas = [-5000, -100, 0, 100, 5000]
    const minSizes = [undefined, { width: 360, height: 280 }]
    for (const minSize of minSizes)
      for (const s of starts)
        for (const edge of edges)
          for (const dx of deltas)
            for (const dy of deltas) {
              const r = resizeRect(s, edge, dx, dy, bounds, minSize)
              expect(clampPosition(r, bounds)).toEqual({ x: r.x, y: r.y })
              expect(r.width).toBeGreaterThanOrEqual(minSize?.width ?? MIN_WIDTH)
              expect(r.height).toBeGreaterThanOrEqual(minSize?.height ?? MIN_HEIGHT)
            }
  })
})
