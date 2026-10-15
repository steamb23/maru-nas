// 위젯 배치 가이드(WidgetPreview)와 위젯 놓기 애니메이션의 설정값과 계산(DOM 없음).
// 모양과 등장 비율은 타일링 가이드(.tile-preview)를 따른다. transform-origin은 왼쪽 위(0 0)다.
import type { Rect } from '../windows/geometry'
import { flipTransform } from './windowAnim'

// 등장: 타일링 가이드(TilePreview.css의 tile-preview-in)와 같은 비율에서 커진다. 커지는 동작은 스프링이 한다.
export const PREVIEW_ENTER_SCALE = 0.96
// 가이드 스프링의 고유 진동수(1/s). 임계 감쇠라 넘치지 않으며, 약 120ms에 거리의 95%를 따라간다.
export const PREVIEW_SPRING = 40
// 한 프레임에 진행하는 최대 시간(s). 프레임이 밀리거나 탭을 다녀와도 한 번에 크게 건너뛰지 않는다.
export const PREVIEW_MAX_STEP = 0.032
// 끌던 위젯을 놓았을 때 놓은 자리(또는 원래 자리)로 미끄러지는 시간
export const DROP_MS = 180

// 이 거리(px)와 속도(px/s) 안이면 멈춘 것으로 본다.
const SETTLE_PX = 0.5
const SETTLE_SPEED = 20

// SpringRect는 가이드가 지금 보이는 자리와 각 값의 속도(px/s)다.
export interface SpringRect extends Rect {
  vx: number
  vy: number
  vw: number
  vh: number
}

// springAt은 r 자리에 멈춰 있는 스프링이다.
export function springAt(r: Rect): SpringRect {
  return { x: r.x, y: r.y, width: r.width, height: r.height, vx: 0, vy: 0, vw: 0, vh: 0 }
}

// enterRect는 가운데를 기준으로 PREVIEW_ENTER_SCALE만큼 줄인 자리다. 가이드가 처음 나타날 때 스프링의 출발점이다.
export function enterRect(r: Rect): Rect {
  const s = PREVIEW_ENTER_SCALE
  return {
    x: r.x + (r.width * (1 - s)) / 2,
    y: r.y + (r.height * (1 - s)) / 2,
    width: r.width * s,
    height: r.height * s,
  }
}

// axis는 임계 감쇠 스프링 한 축을 dt초 진행한 [위치, 속도]다. 닫힌 해라 dt가 커도 발산하지 않는다.
function axis(x: number, v: number, target: number, dt: number, w: number): [number, number] {
  const d = x - target
  const c = v + w * d
  const e = Math.exp(-w * dt)
  return [target + (d + c * dt) * e, (v - w * c * dt) * e]
}

// stepSpring은 s를 target 쪽으로 dt초 진행한 스프링이다. target이 바뀌어도 속도가 이어지므로
// 빠르게 끌어 놓을 칸이 계속 바뀌어도 멈칫하지 않는다.
export function stepSpring(s: SpringRect, target: Rect, dt: number, w = PREVIEW_SPRING): SpringRect {
  const t = Math.min(dt, PREVIEW_MAX_STEP)
  const [x, vx] = axis(s.x, s.vx, target.x, t, w)
  const [y, vy] = axis(s.y, s.vy, target.y, t, w)
  const [width, vw] = axis(s.width, s.vw, target.width, t, w)
  const [height, vh] = axis(s.height, s.vh, target.height, t, w)
  return { x, y, width, height, vx, vy, vw, vh }
}

// springSettled는 s가 target에 충분히 가까이 멈췄는지다.
export function springSettled(s: SpringRect, target: Rect): boolean {
  const near = (a: number, b: number) => Math.abs(a - b) < SETTLE_PX
  const slow = (v: number) => Math.abs(v) < SETTLE_SPEED
  return (
    near(s.x, target.x) &&
    near(s.y, target.y) &&
    near(s.width, target.width) &&
    near(s.height, target.height) &&
    slow(s.vx) &&
    slow(s.vy) &&
    slow(s.vw) &&
    slow(s.vh)
  )
}

// slideKeyframes는 layout 자리에 놓인 요소가 visual 자리(지금 보이는 위치·크기)에서 출발해 제자리로
// 오는 키프레임이다(FLIP). 두 자리가 같으면 null.
export function slideKeyframes(layout: Rect, visual: Rect): Keyframe[] | null {
  const from = flipTransform(layout, visual)
  return from === 'none' ? null : [{ transform: from }, { transform: 'none' }]
}
