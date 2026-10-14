// 위젯 배치 가이드(WidgetPreview)와 위젯 놓기 애니메이션의 설정값과 키프레임 계산(DOM 없음).
// 모양과 등장 애니메이션은 타일링 가이드(.tile-preview)를 따른다. transform-origin은 왼쪽 위(0 0)다.
import type { Rect } from '../windows/geometry'
import { flipTransform } from './windowAnim'

// 등장: 타일링 가이드(index.css의 tile-preview-in)와 같은 시간·비율
export const PREVIEW_ENTER_MS = 150
export const PREVIEW_ENTER_SCALE = 0.96
// 놓을 칸이 바뀔 때 새 칸으로 미끄러지는 시간
export const PREVIEW_MOVE_MS = 140
// 끌던 위젯을 놓았을 때 놓은 자리(또는 원래 자리)로 미끄러지는 시간
export const DROP_MS = 180

// enterKeyframes는 가운데를 기준으로 조금 작고 투명한 상태에서 나타나는 키프레임이다.
export function enterKeyframes(r: Rect): Keyframe[] {
  const s = PREVIEW_ENTER_SCALE
  // 소수 오차(4.000000000000001px 등)를 없애려고 0.01px 단위로 맞춘다.
  const dx = Math.round(r.width * (1 - s) * 50) / 100
  const dy = Math.round(r.height * (1 - s) * 50) / 100
  return [
    { opacity: 0, transform: `translate(${dx}px, ${dy}px) scale(${s})` },
    { opacity: 1, transform: 'none' },
  ]
}

// slideKeyframes는 layout 자리에 놓인 요소가 visual 자리(지금 보이는 위치·크기)에서 출발해 제자리로
// 오는 키프레임이다(FLIP). 두 자리가 같으면 null.
export function slideKeyframes(layout: Rect, visual: Rect): Keyframe[] | null {
  const from = flipTransform(layout, visual)
  return from === 'none' ? null : [{ transform: from }, { transform: 'none' }]
}
