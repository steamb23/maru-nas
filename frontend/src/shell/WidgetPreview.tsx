import { useLayoutEffect, useRef } from 'react'
import type { PxRect } from '../widgets/grid'
import { animationsEnabled } from './useWindowAnimation'
import {
  PREVIEW_ENTER_MS,
  PREVIEW_MOVE_EASING,
  PREVIEW_MOVE_MS,
  enterKeyframes,
  slideKeyframes,
} from './widgetPreviewAnim'
import { EASING } from './windowAnim'

interface Props {
  // 놓을 자리(위젯 레이어 기준 픽셀). 숨긴 동안에는 마지막으로 보인 자리다.
  rect: PxRect
  // 놓을 수 없는 자리에서는 false. 요소를 지우지 않고 숨겨서, 다시 보일 때 등장 애니메이션 없이
  // 마지막 자리에서 이어서 미끄러진다.
  visible: boolean
}

// WidgetPreview는 위젯을 끌거나 크기를 바꾸는 동안 놓으면 들어갈 자리를 보여 주는 가이드다.
// 모양과 등장 애니메이션은 타일링 가이드(TilePreview)를 따른다. 놓을 칸이 바뀌면 요소를 새로 그리지 않고
// 지금 보이는 자리에서 새 자리로 미끄러지게 옮긴다(FLIP). 끄는 동안에는 계속 그려 두고, 놓을 수 없는 자리에서는
// 숨기기만 한다.
export function WidgetPreview({ rect, visible }: Props) {
  const ref = useRef<HTMLDivElement>(null)
  // 지난번에 놓은 자리와 진행 중인 이동 애니메이션
  const last = useRef<{ rect: PxRect | null; move: Animation | null }>({ rect: null, move: null })
  const { x, y, width, height } = rect

  // 위치는 React가 아니라 여기서 직접 바꾼다. 바꾸기 전에 지금 보이는 자리(진행 중인 애니메이션 포함)를 재야
  // 이동이 끊기지 않고 이어진다.
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const next = { x, y, width, height }
    const animate = animationsEnabled()
    let visual: PxRect | null = null
    if (last.current.rect && animate) {
      const r = el.getBoundingClientRect()
      const origin = (el.offsetParent ?? el.parentElement)?.getBoundingClientRect()
      visual = { x: r.left - (origin?.left ?? 0), y: r.top - (origin?.top ?? 0), width: r.width, height: r.height }
    }
    last.current.move?.cancel()
    last.current.move = null
    Object.assign(el.style, { left: `${x}px`, top: `${y}px`, width: `${width}px`, height: `${height}px` })
    const first = last.current.rect === null
    last.current.rect = next
    if (!animate) return
    if (first) {
      el.animate(enterKeyframes(next), { duration: PREVIEW_ENTER_MS, easing: EASING })
      return
    }
    const frames = visual && slideKeyframes(next, visual)
    // 등장 애니메이션은 그대로 두고(투명도는 이어서 진행) transform만 이동 애니메이션이 덮는다.
    if (frames) last.current.move = el.animate(frames, { duration: PREVIEW_MOVE_MS, easing: PREVIEW_MOVE_EASING })
  }, [x, y, width, height])

  const classes = visible ? 'tile-preview widget-preview' : 'tile-preview widget-preview hidden'
  return <div ref={ref} className={classes} aria-hidden="true" />
}
