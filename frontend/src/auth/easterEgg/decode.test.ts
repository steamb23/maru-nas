import { describe, expect, it } from 'vitest'
import {
  type DecodeFrame,
  MAX_PHRASE_LENGTH,
  PHRASES,
  type PhrasePlan,
  decodeFrame,
  planPhrases,
  textCenterY,
} from './decode'

// 0, 0.1, ..., 0.9를 반복하는 결정적 rng
function seqRng(): () => number {
  let i = 0
  return () => (i++ % 10) / 10
}

const text = (f: DecodeFrame) => f.cells.map((c) => c.ch).join('')
const end = (p: PhrasePlan) => p.resolvedAt
const HOLD_MS = 3500
const GAP_MS = 1000
const duration = (p: PhrasePlan) => p.resolvedAt + HOLD_MS + GAP_MS

describe('PHRASES', () => {
  it('한 줄에 들어가는 길이의 ASCII 문구다', () => {
    for (const p of PHRASES) {
      expect(p.length, p).toBeLessThanOrEqual(MAX_PHRASE_LENGTH)
      expect(p, p).toMatch(/^[\x20-\x7e]+$/)
    }
  })
})

describe('planPhrases', () => {
  it('공백은 바로 확정하고, 나머지는 앞 글자부터 대략 순서대로 확정한다', () => {
    const [plan] = planPhrases(['ab cd'], () => 0)
    expect(plan.resolveAt).toEqual([400, 460, 0, 580, 640])
    expect(plan.resolvedAt).toBe(640)
  })
})

describe('decodeFrame', () => {
  const plans = planPhrases(['Wake up', 'The NAS'], seqRng())

  it('시작하면 공백 외 모든 글자가 뒤섞인 채 같은 길이로 보인다', () => {
    const f = decodeFrame(plans, 0, () => 0)
    expect(f.cells).toHaveLength(7)
    expect(text(f)).toBe('!!!! !!')
    expect(f.cells.map((c) => c.done)).toEqual([false, false, false, false, true, false, false])
  })

  it('뒤섞인 글자는 ASCII 33~126 범위다', () => {
    for (const r of [0, 0.5, 0.9999]) {
      for (const c of decodeFrame(plans, 0, () => r).cells) {
        if (c.done) continue
        const code = c.ch.charCodeAt(0)
        expect(code).toBeGreaterThanOrEqual(33)
        expect(code).toBeLessThanOrEqual(126)
      }
    }
  })

  it('모두 확정되면 원문이 보이고 유지 시간 동안 그대로다', () => {
    expect(text(decodeFrame(plans, end(plans[0]), () => 0))).toBe('Wake up')
    expect(text(decodeFrame(plans, end(plans[0]) + HOLD_MS - 1, () => 0))).toBe('Wake up')
  })

  it('유지 시간이 지나면 다음 문구 전까지 빈 화면이다', () => {
    expect(decodeFrame(plans, end(plans[0]) + HOLD_MS, () => 0).cells).toEqual([])
    expect(decodeFrame(plans, duration(plans[0]) - 1, () => 0).cells).toEqual([])
  })

  it('다음 문구로 넘어가고, 마지막 뒤에는 처음으로 돌아간다', () => {
    const second = duration(plans[0])
    expect(decodeFrame(plans, second, () => 0).cells).toHaveLength(7)
    expect(text(decodeFrame(plans, second + end(plans[1]), () => 0))).toBe('The NAS')
    const cycle = duration(plans[0]) + duration(plans[1])
    expect(text(decodeFrame(plans, cycle + end(plans[0]), () => 0))).toBe('Wake up')
  })

  it('문구가 없으면 빈 화면이다', () => {
    expect(decodeFrame([], 1000, () => 0).cells).toEqual([])
  })
})

describe('textCenterY', () => {
  it('카드가 없으면 화면 가운데', () => {
    expect(textCenterY(800, null)).toBe(400)
  })

  it('카드 위아래 중 넓은 쪽의 가운데', () => {
    expect(textCenterY(800, { x: 0, y: 200, width: 360, height: 400 })).toBe(100)
    expect(textCenterY(800, { x: 0, y: 100, width: 360, height: 400 })).toBe(650)
  })

  it('카드가 화면 밖으로 넘쳐도 화면 안에서 고른다', () => {
    expect(textCenterY(600, { x: 0, y: -50, width: 360, height: 500 })).toBe(525)
  })
})
