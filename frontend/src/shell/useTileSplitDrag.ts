import { type PointerEvent, useRef, useState } from 'react'
import type { Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'
import { clampSplit, splitFromDrag, tileMinWidth } from '../windows/tiling'

interface SplitDrag {
  pointerId: number
  startX: number
  startSplit: number
  // 시작할 때 잰 좌우 최소 너비. 끄는 동안 타일 창이 바뀌지 않으므로 다시 재지 않는다.
  minLeft: number
  minRight: number
}

const { setTileSplit } = useWindowStore.getState()

// useTileSplitDrag는 좌우 타일 그룹의 경계를 끌어 비율을 바꾸는 포인터 핸들러다. 타일 창의 안쪽 변
// (TileResizeHandle)과 두 타일 사이 간격(TileDivider)이 함께 쓴다. 양쪽 창이 같은 비율을 쓰므로 함께 크기가 바뀐다.
export function useTileSplitDrag(bounds: Size) {
  const drag = useRef<SplitDrag | null>(null)
  const [dragging, setDragging] = useState(false)

  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0) return
    // 끄는 동안 텍스트 선택이 시작되지 않게 한다.
    e.preventDefault()
    const s = useWindowStore.getState()
    drag.current = {
      pointerId: e.pointerId,
      startX: e.clientX,
      startSplit: s.tileSplit,
      minLeft: tileMinWidth(s.windows, 'left'),
      minRight: tileMinWidth(s.windows, 'right'),
    }
    e.currentTarget.setPointerCapture(e.pointerId)
    setDragging(true)
  }
  const onPointerMove = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current
    if (!d || d.pointerId !== e.pointerId) return
    const split = splitFromDrag(d.startSplit, e.clientX - d.startX, bounds)
    setTileSplit(clampSplit(split, bounds, d.minLeft, d.minRight))
  }
  const endDrag = (e: PointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId !== e.pointerId) return
    drag.current = null
    setDragging(false)
  }

  return {
    dragging,
    handlers: {
      onPointerDown,
      onPointerMove,
      onPointerUp: endDrag,
      onPointerCancel: endDrag,
      onLostPointerCapture: endDrag,
    },
  }
}
