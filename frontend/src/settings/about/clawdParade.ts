// 정보 페이지 Clawd 이스터에그의 움직임 계산(DOM 없음). 그리기는 settings/ClawdParade.tsx, 그림은 clawd.ts.
//
// 좌표는 레이어(페이지 영역) 기준 CSS px이고, 시간은 ms다. Clawd 하나는 자기 줄(lane, 쉬고 있을 때 그림 위쪽의 y)을
// 정해 그 줄에서만 좌우로 움직인다.
// - 등장: 첫 마리는 바로, 다음부터 SPAWN_GAP_MS 간격으로 한 마리씩. 왼쪽·오른쪽 가장자리 중 무작위 쪽의 바깥에서
//   천천히 기어 들어와 가장자리 가까운 안쪽 무작위 지점에서 멈춘다
// - 배회: 쉬기(가끔 깜빡임)와 같은 줄에서 좌우로 걷기를 무작위로 반복한다
// - 흩어지기(scatter): 모두 같은 순간 놀라 점프하고, 착지하면 가까운 왼쪽·오른쪽 가장자리 밖으로 달려 나간다.
//   흩어진 뒤에는 새로 나오지 않고, 모두 나가면 done이 된다
import { CLAWD_COLS, CLAWD_ROWS, type ClawdPose } from './clawd'

export type Rng = () => number

// 입력 없이 이만큼 지나면 연출을 시작한다. 정보 페이지는 오래 머무는 곳이 아니므로 로그인 대기 이스터에그(30초)보다 짧다.
export const CLAWD_IDLE_MS = 10_000

// 그림 한 칸의 CSS px. 64칸 격자 로고처럼 정수 px에 맞도록 정수로 둔다.
export const CLAWD_PX = 4
export const CLAWD_WIDTH = CLAWD_COLS * CLAWD_PX
export const CLAWD_HEIGHT = CLAWD_ROWS * CLAWD_PX

// 한 마리가 나온 뒤 다음 마리가 나올 때까지의 간격
export const SPAWN_GAP_MS: readonly [number, number] = [4000, 8000]
// 최대 수: 영역 너비·높이에 비례하되 이 값을 넘지 않는다.
export const MAX_CLAWDS = 5
const WIDTH_PER_CLAWD = 120
const HEIGHT_PER_CLAWD = CLAWD_HEIGHT * 2

// 기어 들어오는 속도와, 가장자리에서 안쪽으로 들어와 멈출 거리
const CRAWL_SPEED = 20 / 1000
const ENTER_DEPTH: readonly [number, number] = [40, 220]
// 배회
const WALK_SPEED: readonly [number, number] = [20 / 1000, 28 / 1000]
const WALK_DISTANCE: readonly [number, number] = [40, 200]
const REST_MS: readonly [number, number] = [1500, 5000]
const WALK_CHANCE = 0.6
const BLINK_GAP_MS: readonly [number, number] = [2000, 4500]
const BLINK_MS = 150
// 다리를 바꾸는 간격(걷기, 도망)
const WALK_STEP_MS = 180
const FLEE_STEP_MS = 60
// 놀라 점프하는 시간과 높이, 도망치는 속도
export const JUMP_MS = 400
export const JUMP_HEIGHT = 24
const FLEE_SPEED = 300 / 1000

// 영역 가장자리와 띄울 여백. 위쪽은 점프한 그림이 잘리지 않게 점프 높이만큼 더 띄운다.
const SIDE_MARGIN = 8
const TOP_MARGIN = JUMP_HEIGHT + 4
const BOTTOM_MARGIN = 4
// 줄을 고를 때 뽑아 볼 후보 수. 이미 있는 줄과 가장 먼 후보를 고른다.
const LANE_CANDIDATES = 4

export type ClawdMode = 'enter' | 'rest' | 'walk' | 'jump' | 'flee'

export interface ClawdView {
  readonly id: number
  readonly mode: ClawdMode
  // 그림 왼쪽 위 좌표. y는 줄에서 점프 높이만큼 올라간 값이다.
  readonly x: number
  readonly y: number
  readonly lane: number
  readonly pose: ClawdPose
}

export interface ClawdParade {
  // resize는 처음과 영역 크기가 바뀔 때 부른다. 줄과 위치를 새 범위 안으로 당긴다.
  resize: (width: number, height: number) => void
  // step은 dtMs만큼 진행한다. 첫 호출에서 첫 마리가 나온다.
  step: (dtMs: number) => void
  // scatter는 모두 놀라 도망치게 한다. 여러 번 불러도 한 번과 같다.
  scatter: () => void
  readonly clawds: readonly ClawdView[]
  // 흩어진 뒤 모두 나갔으면 true
  readonly done: boolean
}

interface Clawd {
  id: number
  mode: ClawdMode
  x: number
  y: number
  lane: number
  pose: ClawdPose
  // 걷기·기어 들어오기의 목표 x와 속도(px/ms)
  target: number
  speed: number
  // 쉬기의 남은 시간, 점프의 지난 시간
  timer: number
  blinkIn: number
  blinkLeft: number
  stepClock: number
  // 도망칠 방향(-1 왼쪽, 1 오른쪽)
  dir: -1 | 1
}

const between = (rng: Rng, [min, max]: readonly [number, number]) => min + rng() * (max - min)
const clamp = (v: number, min: number, max: number) => Math.min(max, Math.max(min, v))

// maxClawds는 영역 크기에 맞는 최대 수다. 한 마리도 들어갈 수 없으면 0.
export function maxClawds(width: number, height: number): number {
  if (width < CLAWD_WIDTH + SIDE_MARGIN * 2 || height < CLAWD_HEIGHT) return 0
  const byWidth = Math.floor(width / WIDTH_PER_CLAWD)
  const byHeight = Math.floor(height / HEIGHT_PER_CLAWD)
  return clamp(Math.min(byWidth, byHeight), 1, MAX_CLAWDS)
}

export function createParade(rng: Rng): ClawdParade {
  const clawds: Clawd[] = []
  let width = 0
  let height = 0
  let nextId = 1
  let spawnIn = 0
  let scattered = false

  // 쉬거나 걷는 동안 x의 범위와 줄의 범위. 영역이 너무 작으면 최소값에 붙는다.
  const minX = () => SIDE_MARGIN
  const maxX = () => Math.max(SIDE_MARGIN, width - CLAWD_WIDTH - SIDE_MARGIN)
  const minLane = () => Math.min(TOP_MARGIN, Math.max(0, height - CLAWD_HEIGHT))
  const maxLane = () => Math.max(minLane(), height - CLAWD_HEIGHT - BOTTOM_MARGIN)

  const pickLane = () => {
    let best = 0
    let bestGap = -1
    for (let i = 0; i < LANE_CANDIDATES; i++) {
      const lane = minLane() + rng() * (maxLane() - minLane())
      const gap = clawds.reduce((g, c) => Math.min(g, Math.abs(c.lane - lane)), Infinity)
      if (gap > bestGap) {
        best = lane
        bestGap = gap
      }
    }
    return best
  }

  const spawn = () => {
    const fromLeft = rng() < 0.5
    const depth = between(rng, ENTER_DEPTH)
    const lane = pickLane()
    clawds.push({
      id: nextId++,
      mode: 'enter',
      x: fromLeft ? -CLAWD_WIDTH : width,
      y: lane,
      lane,
      pose: 'walk-a',
      target: fromLeft ? clamp(minX() + depth, minX(), maxX()) : clamp(maxX() - depth, minX(), maxX()),
      speed: CRAWL_SPEED,
      timer: 0,
      blinkIn: between(rng, BLINK_GAP_MS),
      blinkLeft: 0,
      stepClock: 0,
      dir: fromLeft ? -1 : 1,
    })
  }

  const rest = (c: Clawd) => {
    c.mode = 'rest'
    c.timer = between(rng, REST_MS)
  }

  // wander는 쉬기가 끝났을 때 다음 행동을 고른다. 걸을 곳이 너무 가까우면 다시 쉰다.
  const wander = (c: Clawd) => {
    if (rng() >= WALK_CHANCE) return rest(c)
    const distance = between(rng, WALK_DISTANCE)
    const sign = rng() < 0.5 ? -1 : 1
    let target = c.x + sign * distance
    // 가장자리에 막히면 반대쪽으로 간다.
    if (target < minX() || target > maxX()) target = c.x - sign * distance
    target = clamp(target, minX(), maxX())
    if (Math.abs(target - c.x) < CLAWD_PX * 2) return rest(c)
    c.mode = 'walk'
    c.target = target
    c.speed = between(rng, WALK_SPEED)
  }

  // moveToward는 목표까지 움직이고 닿았으면 true다.
  const moveToward = (c: Clawd, dt: number) => {
    const step = c.speed * dt
    if (Math.abs(c.target - c.x) <= step) {
      c.x = c.target
      return true
    }
    c.x += Math.sign(c.target - c.x) * step
    return false
  }

  const legs = (c: Clawd, dt: number, every: number) => {
    c.stepClock = (c.stepClock + dt) % (every * 2)
    return c.stepClock < every ? 'a' : 'b'
  }

  const advance = (c: Clawd, dt: number) => {
    switch (c.mode) {
      case 'enter':
      case 'walk':
        c.pose = `walk-${legs(c, dt, WALK_STEP_MS)}`
        if (moveToward(c, dt)) {
          rest(c)
          c.pose = 'stand'
        }
        break
      case 'rest':
        c.timer -= dt
        if (c.blinkLeft > 0) c.blinkLeft -= dt
        else if ((c.blinkIn -= dt) <= 0) {
          c.blinkLeft = BLINK_MS
          c.blinkIn = between(rng, BLINK_GAP_MS)
        }
        c.pose = c.blinkLeft > 0 ? 'blink' : 'stand'
        if (c.timer <= 0) wander(c)
        break
      case 'jump': {
        c.timer += dt
        c.pose = 'startled'
        if (c.timer >= JUMP_MS) {
          c.mode = 'flee'
          c.y = c.lane
          c.stepClock = 0
          c.target = c.dir < 0 ? -CLAWD_WIDTH - 1 : width + 1
          c.speed = FLEE_SPEED
        } else {
          const t = c.timer / JUMP_MS
          c.y = c.lane - JUMP_HEIGHT * 4 * t * (1 - t)
        }
        break
      }
      case 'flee':
        c.pose = `flee-${legs(c, dt, FLEE_STEP_MS)}`
        moveToward(c, dt)
        break
    }
  }

  const gone = (c: Clawd) => c.mode === 'flee' && (c.x <= -CLAWD_WIDTH || c.x >= width)

  return {
    resize(w, h) {
      width = w
      height = h
      for (const c of clawds) {
        if (c.mode === 'flee') continue
        c.lane = clamp(c.lane, minLane(), maxLane())
        if (c.mode !== 'jump') c.y = c.lane
        if (c.mode === 'rest' || c.mode === 'walk') {
          c.x = clamp(c.x, minX(), maxX())
          c.target = clamp(c.target, minX(), maxX())
        } else if (c.mode === 'enter') {
          c.target = clamp(c.target, minX(), maxX())
        }
      }
    },

    step(dt) {
      if (!scattered) {
        spawnIn -= dt
        if (spawnIn <= 0 && clawds.length < maxClawds(width, height)) {
          spawn()
          spawnIn = between(rng, SPAWN_GAP_MS)
        }
      }
      for (const c of clawds) advance(c, dt)
      for (let i = clawds.length - 1; i >= 0; i--) if (gone(clawds[i])) clawds.splice(i, 1)
    },

    scatter() {
      if (scattered) return
      scattered = true
      for (const c of clawds) {
        c.mode = 'jump'
        c.timer = 0
        c.y = c.lane
        c.pose = 'startled'
        // 가까운 쪽으로 도망친다. 들어오던 중이면 대개 들어온 쪽이다.
        c.dir = c.x + CLAWD_WIDTH / 2 < width / 2 ? -1 : 1
      }
    },

    get clawds() {
      return clawds
    },

    get done() {
      return scattered && clawds.length === 0
    },
  }
}
