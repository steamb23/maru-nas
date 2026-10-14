import { describe, expect, it } from 'vitest'
import {
  CITIES,
  HEADER,
  MAX_LINE_LENGTH,
  type Route,
  TARGET,
  createCycles,
  makeRoute,
  mulberry32,
  routeFrame,
} from './traceroute'

const DOC_IP = /\((192\.0\.2|198\.51\.100|203\.0\.113)\.(\d{1,3})\)/
const HOP = /^[ \d]\d {2}(\S+) \((\S+)\) {2}\d+\.\d{3} ms$/
const STARS = /^[ \d]\d {2}\* \* \*$/

const routes = Array.from({ length: 300 }, (_, i) => makeRoute(mulberry32(i * 7919 + 1)))

describe('CITIES', () => {
  it('좌표 범위 안이고 코드가 겹치지 않는다', () => {
    for (const c of CITIES) {
      expect(Math.abs(c.lon), c.code).toBeLessThanOrEqual(180)
      expect(Math.abs(c.lat), c.code).toBeLessThanOrEqual(90)
      expect(c.code).toMatch(/^[a-z]{3}$/)
    }
    expect(new Set(CITIES.map((c) => c.code)).size).toBe(CITIES.length)
  })
})

describe('makeRoute', () => {
  it('도착지는 출발지와 같고, 경유지는 4~8곳이며 서로·출발지와 다르다', () => {
    for (const r of routes) {
      const codes = r.stops.map((s) => s.city.code)
      expect(codes.at(-1)).toBe(codes[0])
      expect(new Set(codes.slice(0, -1)).size).toBe(codes.length - 1)
      expect(codes.length - 2).toBeGreaterThanOrEqual(4)
      expect(codes.length - 2).toBeLessThanOrEqual(8)
      expect(r.legs).toHaveLength(codes.length - 1)
    }
  })

  it('모든 줄이 ASCII이고 최대 길이 이하', () => {
    for (const r of routes)
      for (const { text } of r.lines) {
        expect(text, text).toMatch(/^[\x20-\x7e]+$/)
        expect(text.length, text).toBeLessThanOrEqual(MAX_LINE_LENGTH)
      }
  })

  it('IP는 문서용 대역, 호스트 이름은 예약된 이름만 쓴다', () => {
    for (const r of routes) {
      expect(r.lines[0].text).toBe(HEADER)
      const hops = r.lines.slice(1, -1).map((l) => l.text)
      for (const line of hops) {
        if (STARS.test(line)) continue
        const m = HOP.exec(line)
        expect(m, line).not.toBeNull()
        const [, host] = m!
        expect(host, line).toMatch(/(\.example|\.home\.arpa)$/)
        const ip = DOC_IP.exec(line)
        expect(ip, line).not.toBeNull()
        expect(Number(ip![2])).toBeLessThanOrEqual(255)
      }
      expect(hops.at(-1)).toContain(TARGET)
      // 마지막 줄은 드립이며 프롬프트처럼 보이는 기호로 시작하지 않는다.
      expect(r.lines.at(-1)!.text).toMatch(/^---/)
    }
  })

  it('줄과 도착 시각은 시간 순서이고, 같은 시드는 같은 경로', () => {
    for (const r of routes.slice(0, 20)) {
      const at = r.lines.map((l) => l.at)
      expect(at).toEqual([...at].sort((a, b) => a - b))
      expect(r.clearAt).toBeGreaterThan(at.at(-1)!)
      expect(r.cycle).toBeGreaterThan(r.clearAt)
    }
    expect(makeRoute(mulberry32(42))).toEqual(makeRoute(mulberry32(42)))
  })

  it('경로가 다양하다', () => {
    expect(new Set(routes.map((r) => r.stops.map((s) => s.city.code).join())).size).toBeGreaterThan(290)
    expect(new Set(routes.map((r) => r.stops[0].city.code)).size).toBeGreaterThan(20)
  })
})

describe('routeFrame', () => {
  const r = routes[0]
  const next = routes[1].stops[0].city

  it('처음에는 아무 줄도 없고 출발지를 본다', () => {
    const f = routeFrame(r, 0, next)
    expect(f.lines).toEqual([])
    expect(f.stopsReached).toBe(1)
    expect(f.camLon).toBeCloseTo(r.stops[0].city.lon, 6)
  })

  it('구간 중간에는 진행도가 0~1이고, 끝나면 도착 줄이 나온다', () => {
    const leg = r.legs[0]
    const mid = routeFrame(r, leg.start + leg.duration / 2, next)
    expect(mid.activeProgress).toBeCloseTo(0.5, 6)
    expect(mid.legsDone).toBe(0)
    const before = routeFrame(r, leg.start + leg.duration - 1, next).lines.length
    const after = routeFrame(r, leg.start + leg.duration, next)
    expect(after.lines.length).toBe(before + 1)
    expect(after.activeProgress).toBeNull()
    expect(after.stopsReached).toBe(2)
  })

  it('지운 뒤에는 경로 없이 다음 출발지로 카메라가 간다', () => {
    const cleared = routeFrame(r, r.clearAt, next)
    expect(cleared.lines).toEqual([])
    expect(cleared.stopsReached).toBe(0)
    const end = routeFrame(r, r.cycle - 0.001, next)
    expect(end.camLon).toBeCloseTo(next.lon, 1)
  })

  it('같은 시각은 같은 결과', () => {
    expect(routeFrame(r, 5000, next)).toEqual(routeFrame(r, 5000, next))
  })
})

describe('createCycles', () => {
  const fixed = (cycle: number) => (): Route => ({ stops: [], legs: [], lines: [], clearAt: cycle - 1, cycle })

  it('주기 경계를 넘으면 다음 주기의 처음이 된다', () => {
    const c = createCycles(1, fixed(1000))
    expect(c.at(999).t).toBe(999)
    expect(c.at(1000).t).toBe(0)
    expect(c.at(2500).t).toBe(500)
  })

  it('시간을 건너뛰거나 거꾸로 물어도 같은 결과', () => {
    const a = createCycles(7)
    const b = createCycles(7)
    const later = a.at(300_000)
    expect(a.at(1000)).toEqual(b.at(1000))
    expect(b.at(300_000)).toEqual(later)
  })

  it('주기마다 다른 경로', () => {
    const c = createCycles(3)
    const first = c.at(0)
    const second = c.at(first.route.cycle)
    expect(second.t).toBe(0)
    expect(second.route).toBe(first.next)
    expect(second.route).not.toEqual(first.route)
  })
})
