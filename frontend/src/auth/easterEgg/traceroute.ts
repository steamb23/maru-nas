// 가짜 traceroute 지구본 효과. 점으로 그린 지구본 위로 홉이 하나씩 이어지며 경로가 그려지고,
// 옆에는 traceroute 출력이 한 줄씩 나온다. 패킷은 홈 네트워크에서 출발해 세계를 돌고 같은 홈 네트워크의 NAS로 돌아온다.
// 출발지(=도착지)와 경유지는 주기마다 무작위로 바뀐다.
//
// 전부 가짜 연출이다. IP는 문서용 대역(192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24)만,
// 호스트 이름은 예약된 이름(.example, home.arpa)만 쓴다. 출발지는 브라우저 시간대·로캘에서 유추하지 않는다.
// `$ traceroute ...` 같은 명령줄이나 커서는 그리지 않는다(프롬프트로 오해하지 않게). 문구는 영어로 고정한다.
//
// draw는 timeMs의 순수 함수다. 주기 k의 경로는 처음에 정한 시드와 k로 만든 PRNG로 정해지므로,
// 탭이 숨겨졌다 돌아와도 그동안 재생된 것과 같은 화면이 된다.
import type { Rect } from '../../windows/geometry'
import type { EggEffect, Rng } from './effects'
import {
  type Vec3,
  angleBetween,
  cameraMatrix,
  easeInOut,
  globeLayout,
  landVectors,
  lerpLon,
  occluded,
  rotate,
  slerp,
  toVector,
} from './globe'
import { DOT_STEP_DEG, LAND_MASK_BASE64 } from './globeDots.gen'

export interface City {
  code: string
  lon: number
  lat: number
  // 이 도시를 지날 때 쓰는 전용 호스트 이름
  host?: string
}

// 도시 좌표는 지도에 점을 찍기 위한 대략값이다.
export const CITIES: readonly City[] = [
  { code: 'sel', lon: 126.98, lat: 37.57 },
  { code: 'tyo', lon: 139.69, lat: 35.68 },
  { code: 'hkg', lon: 114.17, lat: 22.32 },
  { code: 'sin', lon: 103.82, lat: 1.35 },
  { code: 'syd', lon: 151.21, lat: -33.87 },
  { code: 'akl', lon: 174.76, lat: -36.85 },
  { code: 'bom', lon: 72.88, lat: 19.08 },
  { code: 'dxb', lon: 55.27, lat: 25.2 },
  { code: 'ist', lon: 28.98, lat: 41.01 },
  { code: 'cai', lon: 31.24, lat: 30.04 },
  { code: 'nbo', lon: 36.82, lat: -1.29 },
  { code: 'jnb', lon: 28.05, lat: -26.2 },
  { code: 'los', lon: 3.38, lat: 6.52 },
  { code: 'fra', lon: 8.68, lat: 50.11 },
  { code: 'ams', lon: 4.9, lat: 52.37 },
  { code: 'lon', lon: -0.13, lat: 51.51 },
  { code: 'par', lon: 2.35, lat: 48.86 },
  { code: 'mad', lon: -3.7, lat: 40.42 },
  { code: 'sto', lon: 18.07, lat: 59.33 },
  { code: 'rkv', lon: -21.94, lat: 64.15 },
  { code: 'nyc', lon: -74.01, lat: 40.71 },
  { code: 'chi', lon: -87.63, lat: 41.88 },
  { code: 'yyz', lon: -79.38, lat: 43.65 },
  { code: 'sfo', lon: -122.42, lat: 37.77 },
  { code: 'sea', lon: -122.33, lat: 47.61 },
  { code: 'mex', lon: -99.13, lat: 19.43 },
  { code: 'gru', lon: -46.63, lat: -23.55 },
  { code: 'bue', lon: -58.38, lat: -34.6 },
  { code: 'lim', lon: -77.04, lat: -12.05 },
  { code: 'hnl', lon: -157.86, lat: 21.31, host: 'aloha-relay.hnl.example' },
  { code: 'anc', lon: -149.9, lat: 61.22, host: 'moose-crossing.anc.example' },
  { code: 'mcm', lon: 166.67, lat: -77.85, host: 'penguin-relay.mcm.example' },
]

// 중간 홉 호스트 이름. {c}는 도시 코드, {n}은 작은 숫자다.
const ROUTER_HOSTS = [
  'ae-{n}.{c}.transit.example',
  'xe-{n}-0-1.cr{n}.{c}.example',
  'be{n}.{c}.peering.example',
  'ix-{c}.exchange.example',
  'cr{n}.{c}.carrier.example',
]
// 가끔 섞는 드립 호스트 이름
const JOKE_HOSTS = [
  'pigeon-relay.{c}.rfc1149.example',
  'definitely-not-a-cdn.{c}.example',
  'scenic-route.{c}.example',
  'bgp-had-a-bad-day.{c}.example',
  'who-routed-this.{c}.example',
  'router-under-a-desk.{c}.example',
  'its-always-dns.{c}.example',
]
// 대양을 건너는 긴 구간의 도착 홉
const SUBSEA_HOSTS = [
  '{c}-landing.subsea.example',
  'shark-approved.{c}.subsea.example',
  'kraken-ok.{c}.subsea.example',
  'very-long-cable.{c}.example',
]
// 마지막 줄 드립. {km}는 다닌 거리, {n}은 들른 도시 수다.
const OUTROS = [
  '--- {km} km travelled to reach the NAS ---',
  '--- {n} cities visited. The NAS was right here. ---',
  '--- scenic route enabled. Please log in. ---',
  '--- no packets were harmed in this traceroute ---',
  '--- route optimized by a very confused BGP ---',
  '--- the packet sends its regards ---',
  '--- around the world and back home again ---',
]

export const TARGET = 'nas.home.arpa'
export const TARGET_IP = '192.0.2.10'
const GATEWAY = 'gw.home.arpa'
const GATEWAY_IP = '192.0.2.1'
export const HEADER = `traceroute to ${TARGET} (${TARGET_IP}), 30 hops max`

// 좁은 화면을 고려한 한 줄 최대 길이(traceroute.test.ts가 검사)
export const MAX_LINE_LENGTH = 72

// 시간(ms)
const LEAD_MS = 400
const LOCAL_HOP_MS = 700
const STARS_MS = 1500
// 구간 이동 시간은 기본 + 거리 비례(반 바퀴면 MOVE_FAR_MS 추가)
const MOVE_BASE_MS = 900
const MOVE_FAR_MS = 1600
const DWELL_MS = 500
const OUTRO_MS = 1200
const HOLD_MS = 6000
const GAP_MS = 2500
const PULSE_MS = 1400

// 이 각도(라디안)보다 긴 구간은 해저 케이블 호스트를 쓴다.
const SUBSEA_ANGLE = 0.7
// 지구 반지름(km)과 광섬유 왕복 지연(km당 ms)
const EARTH_KM = 6371
const RTT_PER_KM = 0.01

export interface Stop {
  city: City
  // 이 지점에 도착하는 시각(주기 시작 기준 ms)
  at: number
}

export interface Leg {
  from: number
  to: number
  start: number
  duration: number
}

export interface Route {
  stops: Stop[]
  legs: Leg[]
  lines: { at: number; text: string }[]
  // 경로를 지우는 시각과 주기 길이
  clearAt: number
  cycle: number
}

// mulberry32는 32비트 시드로 정해지는 PRNG다.
export function mulberry32(seed: number): Rng {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const pick = <T>(list: readonly T[], rng: Rng): T => list[Math.min(list.length - 1, Math.floor(rng() * list.length))]
const randInt = (min: number, max: number, rng: Rng) => min + Math.min(max - min, Math.floor(rng() * (max - min + 1)))

const cityVector = (c: City): Vec3 => toVector(c.lon, c.lat)

function fill(template: string, c: City, rng: Rng): string {
  return template.replaceAll('{c}', c.code).replaceAll('{n}', () => String(randInt(0, 9, rng)))
}

// 중간 홉 IP는 198.51.100.0/24, 203.0.113.0/24 중에서 고른다.
function transitIp(rng: Rng): string {
  return `${pick(['198.51.100', '203.0.113'], rng)}.${randInt(1, 254, rng)}`
}

// formatHop은 traceroute 홉 한 줄이다. 프로브는 하나만 보인다(좁은 화면에 맞추려고).
export function formatHop(n: number, host: string, ip: string, rttMs: number): string {
  return `${String(n).padStart(2)}  ${host} (${ip})  ${rttMs.toFixed(3)} ms`
}

const formatStars = (n: number) => `${String(n).padStart(2)}  * * *`

// makeRoute는 무작위 경로 하나와 그 시간표를 만든다.
// 도착지는 출발지와 같은 홈 네트워크다(대상이 같은 LAN의 NAS이므로). 경유지(4~8곳)는 서로 다르고 출발지와도 다르며,
// 가까운 곳부터 들러 카메라가 크게 휘청이지 않게 한다.
export function makeRoute(rng: Rng): Route {
  const pool = [...CITIES]
  const take = () => pool.splice(Math.min(pool.length - 1, Math.floor(rng() * pool.length)), 1)[0]
  const origin = take()
  const waypoints = Array.from({ length: randInt(4, 8, rng) }, take)

  const ordered: City[] = []
  let here = origin
  while (waypoints.length > 0) {
    let best = 0
    waypoints.forEach((c, i) => {
      if (angleBetween(cityVector(here), cityVector(c)) < angleBetween(cityVector(here), cityVector(waypoints[best])))
        best = i
    })
    here = waypoints.splice(best, 1)[0]
    ordered.push(here)
  }

  const stops: Stop[] = [{ city: origin, at: 0 }]
  const legs: Leg[] = []
  const lines: Route['lines'] = []
  let t = LEAD_MS
  let hop = 0
  let km = 0
  let rtt = 0
  lines.push({ at: t, text: HEADER })

  const local = (host: string, ip: string, extraMs: number) => {
    t += LOCAL_HOP_MS
    rtt += extraMs
    lines.push({ at: t, text: formatHop(++hop, host, ip, rtt) })
  }
  const move = (to: City, host: (angle: number) => string, ip: string) => {
    const from = stops.length - 1
    const angle = angleBetween(cityVector(stops[from].city), cityVector(to))
    const duration = MOVE_BASE_MS + (MOVE_FAR_MS * angle) / Math.PI
    legs.push({ from, to: from + 1, start: t, duration })
    t += duration
    stops.push({ city: to, at: t })
    km += angle * EARTH_KM
    rtt = 3 + km * RTT_PER_KM + rng() * 4
    lines.push({ at: t, text: formatHop(++hop, host(angle), ip, rtt) })
    t += DWELL_MS
  }

  rtt = 0.3 + rng() * 0.5
  lines.push({ at: (t += LOCAL_HOP_MS), text: formatHop(++hop, GATEWAY, GATEWAY_IP, rtt) })
  local(fill('be{n}.{c}.isp.example', origin, rng), transitIp(rng), 2 + rng() * 3)

  let starsLeft = 1
  for (const city of ordered) {
    if (starsLeft > 0 && rng() < 0.2) {
      starsLeft--
      t += STARS_MS
      lines.push({ at: t, text: formatStars(++hop) })
    }
    const joke = rng() < 0.15
    move(
      city,
      (angle) =>
        city.host ??
        fill(angle > SUBSEA_ANGLE ? pick(SUBSEA_HOSTS, rng) : joke ? pick(JOKE_HOSTS, rng) : pick(ROUTER_HOSTS, rng), city, rng),
      transitIp(rng),
    )
  }
  move(origin, () => fill('be{n}.{c}.isp.example', origin, rng), transitIp(rng))
  local(TARGET, TARGET_IP, 0.3 + rng() * 0.5)

  t += OUTRO_MS
  const outro = pick(OUTROS, rng)
    .replace('{km}', Math.round(km).toLocaleString('en-US'))
    .replace('{n}', String(stops.length - 1))
  lines.push({ at: t, text: outro })

  const clearAt = t + HOLD_MS
  return { stops, legs, lines, clearAt, cycle: clearAt + GAP_MS }
}

export interface CycleAt {
  route: Route
  next: Route
  // 이 주기 안의 시간
  t: number
}

// createCycles는 주기별 경로를 시드로 차례로 만들어 두고, 연출 시간이 몇 번째 주기의 어디인지 찾는다.
// 주기 길이가 경로마다 달라 나머지 연산을 못 쓰므로, 필요한 주기까지 시작 시각을 이어 붙인다.
export function createCycles(seed: number, make: (rng: Rng) => Route = makeRoute) {
  const routes: Route[] = []
  const starts: number[] = []
  const route = (k: number) => {
    while (routes.length <= k) {
      const i = routes.length
      starts.push(i === 0 ? 0 : starts[i - 1] + routes[i - 1].cycle)
      routes.push(make(mulberry32((seed + Math.imul(i, 0x9e3779b9)) >>> 0)))
    }
    return routes[k]
  }
  // 마지막으로 찾은 주기. 시간은 보통 앞으로만 흐르므로 여기서부터 찾는다.
  let hint = 0
  return {
    at(timeMs: number): CycleAt {
      const time = Math.max(0, timeMs)
      route(hint)
      let k = time >= starts[hint] ? hint : 0
      // route(k)가 starts[k]를 채우므로 먼저 부른다.
      while (route(k).cycle + starts[k] <= time) k++
      hint = k
      return { route: route(k), next: route(k + 1), t: time - starts[k] }
    },
  }
}

export interface RouteFrame {
  lines: string[]
  // 카메라가 볼 지점
  camLon: number
  camLat: number
  // 다 그린 구간 수와, 그리는 중인 구간의 진행도(없으면 null)
  legsDone: number
  activeProgress: number | null
  // 도착한 지점 수(출발지 포함). 경로를 지운 뒤에는 0
  stopsReached: number
}

// 카메라 위도는 도시 위도를 따라가되 덜 기울여 극지방에서도 지구본 모양이 잘 보이게 한다.
const camLatOf = (c: City) => Math.max(-35, Math.min(50, c.lat * 0.6 + 8))

// routeFrame은 주기 안의 시간 t일 때 보일 출력·경로·카메라다.
// 경로를 지운 뒤(clearAt 이후)에는 카메라가 다음 주기의 출발지로 돌아간다.
export function routeFrame(route: Route, t: number, nextOrigin: City): RouteFrame {
  const last = route.stops[route.stops.length - 1].city
  if (t >= route.clearAt) {
    const p = easeInOut(Math.min(1, (t - route.clearAt) / GAP_MS))
    return {
      lines: [],
      camLon: lerpLon(last.lon, nextOrigin.lon, p),
      camLat: camLatOf(last) + (camLatOf(nextOrigin) - camLatOf(last)) * p,
      legsDone: 0,
      activeProgress: null,
      stopsReached: 0,
    }
  }
  const lines = route.lines.filter((l) => l.at <= t).map((l) => l.text)
  const stopsReached = route.stops.filter((s) => s.at <= t).length
  const leg = route.legs.find((l) => t >= l.start && t < l.start + l.duration)
  if (leg) {
    const progress = (t - leg.start) / leg.duration
    const a = route.stops[leg.from].city
    const b = route.stops[leg.to].city
    const p = easeInOut(progress)
    return {
      lines,
      camLon: lerpLon(a.lon, b.lon, p),
      camLat: camLatOf(a) + (camLatOf(b) - camLatOf(a)) * p,
      legsDone: leg.from,
      activeProgress: progress,
      stopsReached,
    }
  }
  const here = route.stops[stopsReached - 1].city
  return {
    lines,
    camLon: here.lon,
    camLat: camLatOf(here),
    legsDone: stopsReached - 1,
    activeProgress: null,
    stopsReached,
  }
}

// --- 그리기 ---

// 지구본(점·몸체·테두리) 색. 미래지향적인 하늘색(#40c4ff)
const DOT_RGB = '64, 196, 255'
const BUCKETS = 4
const BUCKET_ALPHA = [0.18, 0.32, 0.5, 0.72]
const ROUTE_COLOR = '#ffcc33'
const ROUTE_RGB = '255, 204, 51'
const MARKER_COLOR = '#ffffff'
const TEXT_COLOR = '#c8c8c8'
const HEADER_COLOR = '#ffffff'
const OUTRO_COLOR = '#4cd964'
const FONT_FAMILY = 'ui-monospace, "DejaVu Sans Mono", Menlo, Consolas, monospace'
const CHAR_WIDTH_EM = 0.6
const ARC_SAMPLES = 48

// arcPoint는 구간 위 t 지점이다. 거리에 비례해 지표에서 띄운다.
function arcPoint(a: Vec3, b: Vec3, angle: number, t: number): Vec3 {
  const p = slerp(a, b, t)
  const lift = 1 + (0.03 + (0.1 * angle) / Math.PI) * Math.sin(Math.PI * t)
  return [p[0] * lift, p[1] * lift, p[2] * lift]
}

export function createEffect(rng: Rng): EggEffect {
  const dots = landVectors(DOT_STEP_DEG, LAND_MASK_BASE64)
  const dotCount = dots.length / 3
  // 앞면 점의 화면 좌표와 밝기 버킷. 프레임마다 다시 쓰고 새로 할당하지 않는다.
  const projX = new Float32Array(dotCount)
  const projY = new Float32Array(dotCount)
  const projS = new Float32Array(dotCount)
  const projB = new Uint8Array(dotCount)
  const cycles = createCycles(Math.floor(rng() * 2 ** 32))

  let width = 0
  let height = 0
  let layout = globeLayout(0, 0, null)
  let dotSize = 2
  let fontSize = 12
  let gradient: CanvasGradient | null = null

  return {
    resize(w: number, h: number, avoid: Rect | null) {
      width = w
      height = h
      layout = globeLayout(w, h, avoid)
      // 점 간격(px)의 절반쯤을 점 크기로 한다.
      dotSize = Math.max(1, Math.min(4, layout.r * DOT_STEP_DEG * (Math.PI / 180) * 0.5))
      fontSize = layout.text ? Math.max(10, Math.min(14, layout.text.width / MAX_LINE_LENGTH / CHAR_WIDTH_EM)) : 12
      gradient = null
    },
    draw(ctx, timeMs) {
      ctx.clearRect(0, 0, width, height)
      const { cx, cy, r } = layout
      if (r <= 0) return
      const { route, next, t } = cycles.at(timeMs)
      const frame = routeFrame(route, t, next.stops[0].city)
      const cam = cameraMatrix(frame.camLon, frame.camLat)

      // 구 몸체: 옅은 그라데이션과 테두리
      if (!gradient) {
        gradient = ctx.createRadialGradient(cx - r * 0.3, cy - r * 0.3, r * 0.1, cx, cy, r)
        gradient.addColorStop(0, `rgba(${DOT_RGB}, 0.10)`)
        gradient.addColorStop(1, `rgba(${DOT_RGB}, 0.02)`)
      }
      ctx.beginPath()
      ctx.arc(cx, cy, r, 0, Math.PI * 2)
      ctx.fillStyle = gradient
      ctx.fill()
      ctx.strokeStyle = `rgba(${DOT_RGB}, 0.35)`
      ctx.lineWidth = 1
      ctx.stroke()

      // 육지 점: 앞면만 투영하고, 밝기 버킷별로 한 번씩 채운다.
      let visible = 0
      for (let i = 0; i < dotCount; i++) {
        const x = dots[i * 3]
        const y = dots[i * 3 + 1]
        const z = dots[i * 3 + 2]
        const rz = cam[6] * x + cam[7] * y + cam[8] * z
        if (rz <= 0) continue
        const s = dotSize * (0.4 + 0.6 * rz)
        projX[visible] = cx + r * (cam[0] * x + cam[1] * y + cam[2] * z) - s / 2
        projY[visible] = cy - r * (cam[3] * x + cam[4] * y + cam[5] * z) - s / 2
        projS[visible] = s
        projB[visible] = Math.min(BUCKETS - 1, Math.floor(rz * BUCKETS))
        visible++
      }
      for (let b = 0; b < BUCKETS; b++) {
        ctx.beginPath()
        for (let i = 0; i < visible; i++) if (projB[i] === b) ctx.rect(projX[i], projY[i], projS[i], projS[i])
        ctx.fillStyle = `rgba(${DOT_RGB}, ${BUCKET_ALPHA[b]})`
        ctx.fill()
      }

      const screen = (v: Vec3) => {
        const p = rotate(cam, v)
        return { x: cx + r * p[0], y: cy - r * p[1], hidden: occluded(p), z: p[2] }
      }

      // 경로: 다 그린 구간은 옅게, 그리는 중인 구간은 넓고 옅은 선 + 얇고 밝은 선으로 빛나게 한다.
      const strokeLeg = (index: number, upTo: number) => {
        const leg = route.legs[index]
        const a = cityVector(route.stops[leg.from].city)
        const b = cityVector(route.stops[leg.to].city)
        const angle = angleBetween(a, b)
        const steps = Math.max(2, Math.ceil(ARC_SAMPLES * upTo))
        ctx.beginPath()
        let pen = false
        for (let s = 0; s <= steps; s++) {
          const p = screen(arcPoint(a, b, angle, (s / steps) * upTo))
          if (p.hidden) pen = false
          else if (pen) ctx.lineTo(p.x, p.y)
          else {
            ctx.moveTo(p.x, p.y)
            pen = true
          }
        }
        ctx.stroke()
        return screen(arcPoint(a, b, angle, upTo))
      }
      ctx.lineCap = 'round'
      ctx.lineJoin = 'round'
      ctx.strokeStyle = `rgba(${ROUTE_RGB}, 0.45)`
      ctx.lineWidth = 1.2
      for (let i = 0; i < frame.legsDone; i++) strokeLeg(i, 1)
      if (frame.activeProgress !== null) {
        ctx.strokeStyle = `rgba(${ROUTE_RGB}, 0.18)`
        ctx.lineWidth = 5
        strokeLeg(frame.legsDone, frame.activeProgress)
        ctx.strokeStyle = ROUTE_COLOR
        ctx.lineWidth = 1.6
        const head = strokeLeg(frame.legsDone, frame.activeProgress)
        if (!head.hidden) {
          ctx.fillStyle = MARKER_COLOR
          ctx.beginPath()
          ctx.arc(head.x, head.y, 2.5, 0, Math.PI * 2)
          ctx.fill()
        }
      }

      // 도착한 지점: 점과 도시 코드, 막 도착한 곳에는 퍼지는 고리
      const labelSize = Math.max(9, Math.min(12, r / 20))
      ctx.font = `${labelSize}px ${FONT_FAMILY}`
      ctx.textBaseline = 'middle'
      for (let i = 0; i < frame.stopsReached; i++) {
        const stop = route.stops[i]
        const p = screen(cityVector(stop.city))
        if (p.z <= 0) continue
        ctx.fillStyle = ROUTE_COLOR
        ctx.fillRect(p.x - 2, p.y - 2, 4, 4)
        ctx.fillStyle = `rgba(255, 255, 255, ${0.35 + 0.5 * p.z})`
        ctx.fillText(stop.city.code.toUpperCase(), p.x + 6, p.y)
        const age = t - stop.at
        if (age < PULSE_MS) {
          const k = age / PULSE_MS
          ctx.strokeStyle = `rgba(${ROUTE_RGB}, ${1 - k})`
          ctx.lineWidth = 1.5
          ctx.beginPath()
          ctx.arc(p.x, p.y, 3 + k * r * 0.08, 0, Math.PI * 2)
          ctx.stroke()
        }
      }

      // traceroute 출력: 들어가는 만큼 마지막 줄들을 보이고, 넘치는 글자는 자른다.
      const area = layout.text
      if (!area || frame.lines.length === 0) return
      ctx.font = `${fontSize}px ${FONT_FAMILY}`
      ctx.textBaseline = 'top'
      const lineHeight = Math.round(fontSize * 1.5)
      const rows = Math.floor(area.height / lineHeight)
      if (rows <= 0) return
      const maxChars = Math.max(1, Math.floor(area.width / ctx.measureText('M').width))
      const shown = frame.lines.slice(-rows)
      const outroShown = route.lines.at(-1)?.text === frame.lines.at(-1)
      shown.forEach((line, i) => {
        const isHeader = line === HEADER
        const isOutro = outroShown && i === shown.length - 1
        ctx.fillStyle = isHeader ? HEADER_COLOR : isOutro ? OUTRO_COLOR : TEXT_COLOR
        ctx.fillText(line.slice(0, maxChars), area.x, area.y + i * lineHeight)
      })
    },
  }
}
