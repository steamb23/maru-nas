import { describe, expect, it } from 'vitest'
import {
  type Vec3,
  angleBetween,
  cameraMatrix,
  decodeMask,
  dotGrid,
  globeLayout,
  landVectors,
  lerpLon,
  occluded,
  rotate,
  slerp,
  toVector,
} from './globe'
import { DOT_COUNT, DOT_STEP_DEG, LAND_COUNT, LAND_MASK_BASE64 } from './globeDots.gen'

const close = (a: Vec3, b: Vec3) => a.forEach((v, i) => expect(v).toBeCloseTo(b[i], 6))

// (lon, lat)에 가장 가까운 격자점이 육지인지
function isLand(lon: number, lat: number): boolean {
  const grid = dotGrid(DOT_STEP_DEG)
  const land = decodeMask(LAND_MASK_BASE64, grid.length)
  const target = toVector(lon, lat)
  let best = 0
  grid.forEach(([x, y], k) => {
    if (angleBetween(toVector(x, y), target) < angleBetween(toVector(...grid[best]), target)) best = k
  })
  return land[best]
}

describe('육지 점 데이터', () => {
  it('격자 점 수가 생성 스크립트와 같다', () => {
    expect(dotGrid(DOT_STEP_DEG)).toHaveLength(DOT_COUNT)
  })

  it('디코드한 육지 점 수가 생성물과 같고 비율이 그럴듯하다', () => {
    expect(landVectors(DOT_STEP_DEG, LAND_MASK_BASE64).length / 3).toBe(LAND_COUNT)
    const ratio = LAND_COUNT / DOT_COUNT
    expect(ratio).toBeGreaterThan(0.25)
    expect(ratio).toBeLessThan(0.35)
  })

  it('대륙과 바다가 제자리에 있다', () => {
    expect(isLand(105, 35)).toBe(true) // 중국 내륙
    expect(isLand(20, 5)).toBe(true) // 아프리카
    expect(isLand(-100, 40)).toBe(true) // 북아메리카
    expect(isLand(-150, 0)).toBe(false) // 태평양
    expect(isLand(-30, 30)).toBe(false) // 대서양
  })

  it('비트마스크는 LSB부터 읽는다', () => {
    // 0b00000101, 0b10000000
    expect(decodeMask(btoa('\x05\x80'), 16)).toEqual([
      true, false, true, false, false, false, false, false,
      false, false, false, false, false, false, false, true,
    ])
  })
})

describe('카메라', () => {
  it('카메라가 보는 지점은 화면 가운데 앞(z = 1), 대척점은 뒤', () => {
    const cam = cameraMatrix(127, 37)
    close(rotate(cam, toVector(127, 37)), [0, 0, 1])
    close(rotate(cam, toVector(-53, -37)), [0, 0, -1])
  })

  it('동쪽은 오른쪽, 북쪽은 위', () => {
    const cam = cameraMatrix(0, 0)
    expect(rotate(cam, toVector(10, 0))[0]).toBeGreaterThan(0)
    expect(rotate(cam, toVector(0, 10))[1]).toBeGreaterThan(0)
  })

  it('뒷면이어도 테두리 밖으로 뜬 점은 가려지지 않는다', () => {
    expect(occluded([0, 0, -1])).toBe(true)
    expect(occluded([1.1, 0, -0.1])).toBe(false)
    expect(occluded([0, 0, 1])).toBe(false)
  })
})

describe('보간', () => {
  it('slerp는 대원 위에서 움직인다', () => {
    close(slerp(toVector(0, 0), toVector(90, 0), 0.5), toVector(45, 0))
    close(slerp(toVector(0, 0), toVector(90, 0), 0), toVector(0, 0))
    close(slerp(toVector(10, 10), toVector(10, 10), 0.5), toVector(10, 10))
  })

  it('경도는 ±180°를 넘는 짧은 쪽으로 보간한다', () => {
    expect(lerpLon(170, -170, 0.5)).toBeCloseTo(-180 + 0, 6)
    expect(lerpLon(170, -170, 0.25)).toBeCloseTo(175, 6)
    expect(lerpLon(-170, 170, 0.25)).toBeCloseTo(-175, 6)
    expect(lerpLon(10, 50, 0.5)).toBeCloseTo(30, 6)
  })
})

describe('globeLayout', () => {
  const overlaps = (a: { x: number; y: number; width: number; height: number }, b: typeof a) =>
    a.x < b.x + b.width && b.x < a.x + a.width && a.y < b.y + b.height && b.y < a.y + a.height

  it('넓은 화면은 카드 왼쪽에 지구본, 오른쪽에 출력', () => {
    const card = { x: 780, y: 340, width: 360, height: 400 }
    const l = globeLayout(1920, 1080, card)
    expect(l.cx + l.r).toBeLessThanOrEqual(card.x)
    expect(l.text).not.toBeNull()
    expect(l.text!.x).toBeGreaterThanOrEqual(card.x + card.width)
    expect(overlaps(l.text!, card)).toBe(false)
  })

  it('좁은 화면은 카드 위·아래 넓은 쪽에 지구본, 다른 쪽에 출력', () => {
    const card = { x: 15, y: 300, width: 360, height: 400 }
    const l = globeLayout(390, 844, card)
    expect(l.cy).toBeLessThan(card.y)
    expect(l.cy - l.r).toBeGreaterThanOrEqual(0)
    expect(l.text).not.toBeNull()
    expect(l.text!.y).toBeGreaterThanOrEqual(card.y + card.height)
    expect(overlaps(l.text!, card)).toBe(false)
  })

  it('출력할 자리가 없으면 text는 null', () => {
    const l = globeLayout(390, 700, { x: 15, y: 250, width: 360, height: 450 })
    expect(l.cy).toBeLessThan(250)
    expect(l.text).toBeNull()
  })

  it('영역이 좁아도 지구본은 최소 크기를 지키고 화면 밖으로 나가지 않는다', () => {
    const above = globeLayout(390, 844, { x: 15, y: 207, width: 360, height: 430 })
    expect(above.r).toBe(160)
    expect(above.cy - above.r).toBe(24)
    // 카드 아래에 둘 때는 아래 가장자리에 맞춘다.
    const below = globeLayout(390, 844, { x: 15, y: 250, width: 360, height: 300 })
    expect(below.r).toBe(160)
    expect(below.cy + below.r).toBe(844 - 24)
    // 카드 옆에 둘 때는 왼쪽 가장자리에 맞춘다.
    const side = globeLayout(900, 400, { x: 270, y: 0, width: 360, height: 400 })
    expect(side.r).toBe(160)
    expect(side.cx - side.r).toBe(24)
    // 화면이 아주 작으면 화면에 들어가는 만큼만
    expect(globeLayout(300, 280, { x: 0, y: 40, width: 300, height: 200 }).r).toBeCloseTo(119, 0)
  })

  it('카드가 없으면 화면 가운데', () => {
    const l = globeLayout(800, 600, null)
    expect([l.cx, l.cy, l.r]).toEqual([400, 300, 255])
  })
})
