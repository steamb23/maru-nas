// traceroute 지구본의 순수 계산(DOM 없음): 육지 점 격자, 정사영 카메라, 대원 보간, 화면 배치.
//
// 좌표계: 위경도를 단위 벡터로 바꿀 때 x는 동쪽(경도 90°), y는 북극, z는 경도 0°·위도 0° 방향이다.
// 카메라 회전을 거친 뒤 z가 화면 밖(보는 사람) 방향이며, z < 0이면 지구 뒷면이다.
import type { Rect } from '../../windows/geometry'

const DEG = Math.PI / 180

// dotGrid는 위도 줄 격자의 점(경도, 위도)을 남쪽 줄부터 차례로 돌려준다.
// scripts/gen-globe-dots.py의 dot_grid와 같은 규칙이어야 비트마스크와 맞는다(globe.test.ts가 개수를 검사).
export function dotGrid(stepDeg: number): [number, number][] {
  const points: [number, number][] = []
  const rows = Math.floor(180 / stepDeg + 0.5)
  for (let i = 0; i < rows; i++) {
    const lat = -90 + (i + 0.5) * stepDeg
    const n = Math.max(1, Math.floor((360 * Math.cos(lat * DEG)) / stepDeg + 0.5))
    for (let j = 0; j < n; j++) points.push([-180 + ((j + 0.5) * 360) / n, lat])
  }
  return points
}

// decodeMask는 base64 비트마스크를 점별 참·거짓으로 푼다. 점 k는 바이트 k >> 3의 비트 k & 7이다.
export function decodeMask(base64: string, count: number): boolean[] {
  const bytes = atob(base64)
  const bits: boolean[] = []
  for (let k = 0; k < count; k++) bits.push(((bytes.charCodeAt(k >> 3) >> (k & 7)) & 1) === 1)
  return bits
}

// landVectors는 육지 점만 골라 단위 벡터(x, y, z 차례)로 담는다.
export function landVectors(stepDeg: number, base64: string): Float32Array {
  const grid = dotGrid(stepDeg)
  const land = decodeMask(base64, grid.length)
  const out: number[] = []
  grid.forEach(([lon, lat], k) => {
    if (land[k]) out.push(...toVector(lon, lat))
  })
  return Float32Array.from(out)
}

export type Vec3 = [number, number, number]

export function toVector(lon: number, lat: number): Vec3 {
  const c = Math.cos(lat * DEG)
  return [c * Math.sin(lon * DEG), Math.sin(lat * DEG), c * Math.cos(lon * DEG)]
}

// Camera는 3×3 회전 행렬(행 우선)이다.
export type Camera = [number, number, number, number, number, number, number, number, number]

// cameraMatrix는 (lon, lat) 지점이 화면 가운데(z = 1)에 오도록 지구를 돌리는 행렬이다.
// 경도만큼 y축으로 돌린 뒤, 위도만큼 x축으로 기울인다.
export function cameraMatrix(lon: number, lat: number): Camera {
  const sl = Math.sin(lon * DEG)
  const cl = Math.cos(lon * DEG)
  const sb = Math.sin(lat * DEG)
  const cb = Math.cos(lat * DEG)
  return [cl, 0, -sl, -sb * sl, cb, -sb * cl, cb * sl, sb, cb * cl]
}

export function rotate(m: Camera, v: Vec3): Vec3 {
  const [x, y, z] = v
  return [m[0] * x + m[1] * y + m[2] * z, m[3] * x + m[4] * y + m[5] * z, m[6] * x + m[7] * y + m[8] * z]
}

// occluded는 회전한 점이 지구 뒤에 가려지는지다. 지표보다 높이 뜬 점은 뒷면이어도 테두리 밖이면 보인다.
export function occluded(v: Vec3): boolean {
  return v[2] < 0 && v[0] * v[0] + v[1] * v[1] < 1
}

// angleBetween은 두 단위 벡터 사이 각(라디안)이다.
export function angleBetween(a: Vec3, b: Vec3): number {
  const d = a[0] * b[0] + a[1] * b[1] + a[2] * b[2]
  return Math.acos(Math.max(-1, Math.min(1, d)))
}

// slerp는 두 단위 벡터 사이 대원 위의 점이다(t = 0이면 a, 1이면 b).
export function slerp(a: Vec3, b: Vec3, t: number): Vec3 {
  const omega = angleBetween(a, b)
  if (omega < 1e-6) return [a[0], a[1], a[2]]
  const s = Math.sin(omega)
  const wa = Math.sin((1 - t) * omega) / s
  const wb = Math.sin(t * omega) / s
  return [a[0] * wa + b[0] * wb, a[1] * wa + b[1] * wb, a[2] * wa + b[2] * wb]
}

// lerpLon은 경도를 ±180°를 넘는 짧은 쪽으로 보간한다. 결과는 -180~180이다.
export function lerpLon(a: number, b: number, t: number): number {
  const d = ((((b - a) % 360) + 540) % 360) - 180
  const v = a + d * t
  return ((((v + 180) % 360) + 360) % 360) - 180
}

export function easeInOut(t: number): number {
  return t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2
}

export interface GlobeLayout {
  cx: number
  cy: number
  r: number
  // traceroute 출력을 그릴 영역. 들어갈 자리가 없으면 null
  text: Rect | null
}

// 카드 좌우가 이만큼 넓으면 지구본과 출력을 카드 양옆에 둔다.
const MIN_SIDE = 240
// 반지름은 지구본 영역 짧은 변의 절반에 이 비율을 곱한다.
const FILL = 0.85
const PAD = 24
// 영역이 좁아도 지구본 반지름은 이보다 작아지지 않는다(화면에 들어가는 한). 이때는 카드 뒤로 가려져도 된다.
const MIN_RADIUS = 160

// grow는 반지름을 MIN_RADIUS까지 키운다. 화면 짧은 변에 들어가는 크기를 넘지 않는다.
function grow(r: number, width: number, height: number): number {
  return Math.max(r, Math.min(MIN_RADIUS, (Math.min(width, height) / 2) * FILL))
}

// globeLayout은 화면 크기와 로그인 카드(avoid)로 지구본과 출력 영역을 정한다.
// - 카드 좌우가 넓으면: 왼쪽에 지구본, 오른쪽에 출력(카드 위쪽 높이부터)
// - 좁으면: 카드 위·아래 중 넓은 쪽에 지구본, 다른 쪽에 출력
// 출력 영역은 카드와 겹치지 않는다. 지구본은 카드 뒤로 일부 가려져도 된다.
// 영역이 좁아 지구본을 키우면 화면 가장자리 밖으로 나가지 않게 카드 쪽으로 민다.
export function globeLayout(width: number, height: number, avoid: Rect | null): GlobeLayout {
  if (!avoid) {
    const r = (Math.min(width, height) / 2) * FILL
    return { cx: width / 2, cy: height / 2, r, text: { x: PAD, y: PAD, width: width / 3, height: height - PAD * 2 } }
  }
  const left = Math.max(0, avoid.x)
  const rightStart = Math.min(width, avoid.x + avoid.width)
  const right = width - rightStart
  if (Math.min(left, right) >= MIN_SIDE) {
    const top = Math.max(PAD, avoid.y)
    const r = grow((Math.min(left, height) / 2) * FILL, width, height)
    return {
      cx: Math.max(left / 2, PAD + r),
      cy: height / 2,
      r,
      text: { x: rightStart + PAD, y: top, width: right - PAD * 2, height: height - top - PAD },
    }
  }
  const above = Math.max(0, avoid.y)
  const belowStart = Math.min(height, avoid.y + avoid.height)
  const below = height - belowStart
  const globeAbove = above >= below
  const [gTop, gHeight] = globeAbove ? [0, above] : [belowStart, below]
  const [tTop, tHeight] = globeAbove ? [belowStart, below] : [0, above]
  const textHeight = tHeight - PAD * 2
  const r = grow((Math.min(width, gHeight) / 2) * FILL, width, height)
  const center = gTop + gHeight / 2
  return {
    cx: width / 2,
    cy: globeAbove ? Math.max(center, PAD + r) : Math.min(center, height - PAD - r),
    r,
    text: textHeight > 0 ? { x: PAD, y: tTop + PAD, width: width - PAD * 2, height: textHeight } : null,
  }
}
