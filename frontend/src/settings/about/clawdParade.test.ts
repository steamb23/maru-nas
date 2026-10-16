import { describe, expect, it } from 'vitest'
import {
  CLAWD_HEIGHT,
  CLAWD_WIDTH,
  type ClawdParade,
  JUMP_HEIGHT,
  JUMP_MS,
  MAX_CLAWDS,
  SPAWN_GAP_MS,
  createParade,
  maxClawds,
} from './clawdParade'

// 고정된 순서의 의사 난수(mulberry32)
function seeded(seed: number) {
  let a = seed
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const FRAME = 16

// run은 ms만큼 프레임 단위로 진행하며 프레임마다 check를 부른다.
function run(p: ClawdParade, ms: number, check?: (p: ClawdParade, t: number) => void) {
  for (let t = 0; t < ms; t += FRAME) {
    p.step(FRAME)
    check?.(p, t)
  }
}

function parade(width = 640, height = 480, seed = 1) {
  const p = createParade(seeded(seed))
  p.resize(width, height)
  return p
}

describe('maxClawds', () => {
  it('영역 크기에 비례하고 최대 수를 넘지 않는다', () => {
    expect(maxClawds(640, 480)).toBe(5)
    expect(maxClawds(2000, 2000)).toBe(MAX_CLAWDS)
    expect(maxClawds(360, 480)).toBe(3)
    expect(maxClawds(640, 100)).toBe(1)
  })

  it('한 마리도 들어갈 수 없으면 0이다', () => {
    expect(maxClawds(0, 0)).toBe(0)
    expect(maxClawds(CLAWD_WIDTH, 480)).toBe(0)
    expect(maxClawds(640, CLAWD_HEIGHT - 1)).toBe(0)
  })
})

describe('등장', () => {
  it('첫 마리는 바로 가장자리 바깥에서 나와 안으로 기어 들어온다', () => {
    const p = parade()
    p.step(FRAME)
    expect(p.clawds).toHaveLength(1)
    const c = p.clawds[0]
    expect(c.mode).toBe('enter')
    expect(c.x <= 0 || c.x >= 640 - CLAWD_WIDTH).toBe(true)
    expect(c.x < 0 || c.x > 640 - CLAWD_WIDTH).toBe(true)
    run(p, 20_000)
    const first = p.clawds.find((v) => v.id === c.id)
    expect(first?.mode === 'rest' || first?.mode === 'walk').toBe(true)
  })

  it('한 마리씩 간격을 두고 나오며 최대 수를 넘지 않는다', () => {
    for (const seed of [1, 2, 3]) {
      const p = parade(640, 480, seed)
      const times: number[] = []
      let seen = 0
      run(p, 120_000, (q, t) => {
        if (q.clawds.length > seen) times.push(t)
        seen = q.clawds.length
        expect(q.clawds.length).toBeLessThanOrEqual(maxClawds(640, 480))
      })
      expect(times).toHaveLength(maxClawds(640, 480))
      expect(times[0]).toBe(0)
      for (let i = 1; i < times.length; i++) {
        const gap = times[i] - times[i - 1]
        expect(gap).toBeGreaterThanOrEqual(SPAWN_GAP_MS[0] - FRAME)
        expect(gap).toBeLessThanOrEqual(SPAWN_GAP_MS[1] + FRAME)
      }
    }
  })

  it('좁은 영역에는 한 마리만 나온다', () => {
    const p = parade(200, 120)
    run(p, 60_000)
    expect(p.clawds).toHaveLength(1)
  })

  it('크기를 받기 전에는 나오지 않고, 받으면 바로 나온다', () => {
    const p = createParade(seeded(1))
    run(p, 1000)
    expect(p.clawds).toHaveLength(0)
    p.resize(640, 480)
    p.step(FRAME)
    expect(p.clawds).toHaveLength(1)
  })

  it('왼쪽·오른쪽 모두에서 나오고 줄이 서로 겹치지 않게 퍼진다', () => {
    const sides = new Set<string>()
    for (let seed = 1; seed <= 20; seed++) {
      const p = parade(640, 480, seed)
      p.step(FRAME)
      sides.add(p.clawds[0].x < 0 ? 'left' : 'right')
    }
    expect(sides).toEqual(new Set(['left', 'right']))

    const p = parade()
    run(p, 60_000)
    const lanes = p.clawds.map((c) => c.lane).sort((a, b) => a - b)
    const minGap = Math.min(...lanes.slice(1).map((l, i) => l - lanes[i]))
    expect(minGap).toBeGreaterThan(CLAWD_HEIGHT / 4)
  })
})

describe('배회', () => {
  it('들어온 뒤에는 영역 안 자기 줄에서만 움직인다', () => {
    const p = parade()
    const lanes = new Map<number, number>()
    const settled = new Set<number>()
    // 프레임마다 expect를 부르면 느리므로 어긋난 경우만 모아 마지막에 확인한다.
    const wrong: string[] = []
    run(p, 600_000, (q, t) => {
      for (const c of q.clawds) {
        if (!lanes.has(c.id)) lanes.set(c.id, c.lane)
        if (c.mode !== 'enter') settled.add(c.id)
        const outside = settled.has(c.id) && (c.x < 0 || c.x > 640 - CLAWD_WIDTH)
        const offLane = c.lane !== lanes.get(c.id) || c.y !== c.lane
        const laneRange = c.lane < JUMP_HEIGHT || c.lane > 480 - CLAWD_HEIGHT
        if (outside || offLane || laneRange) wrong.push(`${t}ms #${c.id} x=${c.x} y=${c.y} lane=${c.lane}`)
      }
    })
    expect(wrong.slice(0, 5)).toEqual([])
    expect(settled.size).toBe(maxClawds(640, 480))
  })

  it('쉬기와 걷기를 오가고 가끔 깜빡인다', () => {
    const p = parade()
    const modes = new Set<string>()
    const poses = new Set<string>()
    run(p, 120_000, (q) => {
      for (const c of q.clawds) {
        modes.add(c.mode)
        poses.add(c.pose)
      }
    })
    expect(modes).toEqual(new Set(['enter', 'rest', 'walk']))
    expect(poses).toEqual(new Set(['stand', 'blink', 'walk-a', 'walk-b']))
  })

  it('영역이 줄면 안으로 당겨진다', () => {
    const p = parade()
    run(p, 60_000)
    p.resize(300, 200)
    for (const c of p.clawds) {
      expect(c.lane).toBeLessThanOrEqual(200 - CLAWD_HEIGHT)
      if (c.mode === 'rest' || c.mode === 'walk') expect(c.x).toBeLessThanOrEqual(300 - CLAWD_WIDTH)
    }
  })
})

describe('흩어지기', () => {
  it('모두 같은 순간 점프한 뒤 가까운 가장자리로 나가고 done이 된다', () => {
    const p = parade()
    run(p, 60_000)
    const count = p.clawds.length
    expect(count).toBeGreaterThan(1)
    const dirs = new Map(p.clawds.map((c) => [c.id, c.x + CLAWD_WIDTH / 2 < 320 ? -1 : 1]))
    const startX = new Map(p.clawds.map((c) => [c.id, c.x]))
    p.scatter()

    run(p, JUMP_MS / 2)
    for (const c of p.clawds) {
      expect(c.mode).toBe('jump')
      expect(c.pose).toBe('startled')
      expect(c.lane - c.y).toBeGreaterThan(JUMP_HEIGHT * 0.9)
      expect(c.x).toBe(startX.get(c.id))
    }

    run(p, JUMP_MS / 2 + FRAME * 4)
    for (const c of p.clawds) {
      expect(c.mode).toBe('flee')
      expect(c.y).toBe(c.lane)
      expect(Math.sign(c.x - (startX.get(c.id) ?? 0))).toBe(dirs.get(c.id))
    }

    let done = 0
    run(p, 5000, (q, t) => {
      if (q.done && !done) done = t
    })
    expect(p.clawds).toHaveLength(0)
    expect(p.done).toBe(true)
    expect(done).toBeLessThan(3000)
  })

  it('흩어진 뒤에는 새로 나오지 않는다', () => {
    const p = parade()
    p.step(FRAME)
    p.scatter()
    p.scatter()
    run(p, 60_000)
    expect(p.clawds).toHaveLength(0)
    expect(p.done).toBe(true)
  })

  it('아무도 없을 때 흩어지면 바로 done이다', () => {
    const p = parade()
    expect(p.done).toBe(false)
    p.scatter()
    expect(p.done).toBe(true)
  })
})
