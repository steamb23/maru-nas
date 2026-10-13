// 창 최대화·타일링·최소화 애니메이션의 설정값과 위치 계산(DOM 없음).
// 애니메이션은 FLIP 방식으로 transform과 opacity만 움직인다. 위치·크기를 직접 움직이면
// 매 프레임 레이아웃을 다시 계산하게 된다. 실제 재생은 useWindowAnimation.ts가 한다.
import type { Rect, Size } from '../windows/geometry'

// 창 애니메이션 사용 여부. 시스템의 '동작 줄이기'(prefers-reduced-motion)가 켜져 있으면 이 값과 관계없이 끈다.
// TODO: 설정 기능이 생기면 이 값을 사용자 설정에서 읽어 끄고 켤 수 있게 한다.
export const WINDOW_ANIMATIONS = true

// 최대화·타일링 전환 시간
export const MAXIMIZE_MS = 200
// 최대화·타일링 애니메이션은 창 틀만 늘어나 보이도록 창 내용(index.css의 --window-content-opacity)을 숨겼다가,
// 전체 시간의 이 비율 지점부터 끝까지 페이드인한다. 이때는 틀이 거의 제 크기라 내용이 늘어나 보이지 않는다.
export const CONTENT_FADE_START = 0.35
export const MINIMIZE_MS = 220
export const EASING = 'cubic-bezier(0.2, 0, 0, 1)'

// 작업표시줄 버튼을 찾지 못했을 때 최소화 애니메이션이 제자리에서 줄어드는 비율
export const FALLBACK_SCALE = 0.85

// sameRect는 두 사각형이 0.5px 안에서 같은지 비교한다.
export function sameRect(a: Rect, b: Rect): boolean {
  return (
    Math.abs(a.x - b.x) < 0.5 &&
    Math.abs(a.y - b.y) < 0.5 &&
    Math.abs(a.width - b.width) < 0.5 &&
    Math.abs(a.height - b.height) < 0.5
  )
}

// flipTransform은 layout 위치에 놓인 요소를 visual 위치·크기로 보이게 하는 transform 값이다.
// transform-origin이 왼쪽 위(0 0)라고 가정한다. 두 사각형이 같으면 'none'.
export function flipTransform(layout: Rect, visual: Rect): string {
  if (sameRect(layout, visual)) return 'none'
  const dx = visual.x - layout.x
  const dy = visual.y - layout.y
  const sx = visual.width / layout.width
  const sy = visual.height / layout.height
  return `translate(${dx}px, ${dy}px) scale(${sx}, ${sy})`
}

// fitInside는 size의 비율을 유지한 채 target 안에 들어가는 가장 큰 사각형을 target 가운데에 놓는다.
// 최소화할 때 창이 찌그러지지 않고 작업표시줄 버튼 쪽으로 줄어들게 한다.
export function fitInside(size: Size, target: Rect): Rect {
  const k = Math.min(target.width / size.width, target.height / size.height)
  const width = size.width * k
  const height = size.height * k
  return { x: target.x + (target.width - width) / 2, y: target.y + (target.height - height) / 2, width, height }
}

// scaleAbout은 rect를 가운데 기준으로 k배 한 사각형이다.
export function scaleAbout(rect: Rect, k: number): Rect {
  const width = rect.width * k
  const height = rect.height * k
  return { x: rect.x + (rect.width - width) / 2, y: rect.y + (rect.height - height) / 2, width, height }
}
