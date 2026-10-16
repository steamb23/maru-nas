// 사용량 그래프의 좌표 계산. DOM에 의존하지 않아 node 환경에서 테스트한다. 그리기는 UsageChart.tsx가 한다.

// niceMax는 초당 바이트 그래프의 y축 최댓값이다. 값보다 크거나 같은 1·2·5 × 1024^n(1 KiB/s 이상)이며,
// 단위 경계에서 formatRate가 깔끔한 수(1 MiB/s, 500 KiB/s)로 보이게 한다.
export function niceMax(value: number): number {
  let unit = 1024
  while (value > unit * 1000) unit *= 1024
  for (const step of [1, 2, 5, 10, 20, 50, 100, 200, 500, 1000]) {
    if (value <= step * unit) return step === 1000 ? unit * 1024 : step * unit
  }
  return unit * 1024
}

export interface ChartFrame {
  // 그래프 오른쪽 끝(지금) 시각과 보여 줄 시간(밀리초)
  now: number
  span: number
  // y축 최댓값(0부터)
  max: number
  width: number
  height: number
}

export interface Sample {
  t: number
  v: number | null
}

const xOf = (f: ChartFrame, t: number) => ((t - (f.now - f.span)) / f.span) * f.width
const yOf = (f: ChartFrame, v: number) => f.height - (Math.min(Math.max(v, 0), f.max) / f.max) * f.height

const fmt = (n: number) => String(Math.round(n * 10) / 10)

// segments는 값이 이어진 구간들이다. 값이 없는 점(null)에서 선을 끊는다.
function segments(samples: readonly Sample[]): { t: number; v: number }[][] {
  const out: { t: number; v: number }[][] = []
  let cur: { t: number; v: number }[] = []
  for (const s of samples) {
    if (s.v === null) {
      if (cur.length > 0) out.push(cur)
      cur = []
    } else {
      cur.push({ t: s.t, v: s.v })
    }
  }
  if (cur.length > 0) out.push(cur)
  return out
}

// linePath는 선 그래프의 SVG path다. 점 하나뿐인 구간은 그리지 않는다(선이 될 수 없다).
export function linePath(samples: readonly Sample[], f: ChartFrame): string {
  return segments(samples)
    .filter((seg) => seg.length > 1)
    .map((seg) => seg.map((p, i) => `${i === 0 ? 'M' : 'L'}${fmt(xOf(f, p.t))} ${fmt(yOf(f, p.v))}`).join(''))
    .join('')
}

// areaPath는 선 아래를 채우는 SVG path다(구간마다 바닥까지 닫는다).
export function areaPath(samples: readonly Sample[], f: ChartFrame): string {
  return segments(samples)
    .filter((seg) => seg.length > 1)
    .map((seg) => {
      const first = fmt(xOf(f, seg[0].t))
      const last = fmt(xOf(f, seg[seg.length - 1].t))
      const top = seg.map((p, i) => `${i === 0 ? 'M' : 'L'}${fmt(xOf(f, p.t))} ${fmt(yOf(f, p.v))}`).join('')
      return `${top}L${last} ${fmt(f.height)}L${first} ${fmt(f.height)}Z`
    })
    .join('')
}

// pointX는 시각 t의 x 좌표다(십자선).
export function pointX(f: ChartFrame, t: number): number {
  return xOf(f, t)
}

// nearestIndex는 x 좌표에 가장 가까운 시각의 번호다. 그래프 안(지금 - span 이후)의 점만 보며, 없으면 -1이다.
export function nearestIndex(times: readonly number[], f: ChartFrame, x: number): number {
  const t = f.now - f.span + (x / f.width) * f.span
  let best = -1
  let bestDist = Infinity
  times.forEach((ti, i) => {
    if (ti < f.now - f.span) return
    const d = Math.abs(ti - t)
    if (d < bestDist) {
      best = i
      bestDist = d
    }
  })
  return best
}

// stepIndex는 키보드로 십자선을 옮긴 번호다. 그래프 안의 점 사이에서만 움직이고, 처음 누르면 가장 최근 점이다.
export function stepIndex(times: readonly number[], f: ChartFrame, cur: number, delta: -1 | 1): number {
  const visible = times.map((t, i) => (t >= f.now - f.span ? i : -1)).filter((i) => i >= 0)
  if (visible.length === 0) return -1
  const pos = visible.indexOf(cur)
  if (pos < 0) return visible[visible.length - 1]
  return visible[Math.min(visible.length - 1, Math.max(0, pos + delta))]
}
