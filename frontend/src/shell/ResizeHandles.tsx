import { type PointerEvent, useRef } from 'react'
import { type Rect, type ResizeEdge, type Size, resizeRect } from '../windows/geometry'
import { useWindowStore } from '../windows/store'

interface Props {
  id: string
  // 현재 창의 위치·크기. 크기 조절을 시작할 때의 기준이 된다.
  rect: Rect
  bounds: Size
}

interface Resize {
  pointerId: number
  edge: ResizeEdge
  // 시작할 때의 포인터 위치와 창 위치·크기
  startX: number
  startY: number
  start: Rect
}

const EDGES: ResizeEdge[] = ['n', 's', 'e', 'w', 'ne', 'nw', 'se', 'sw']

const { resize } = useWindowStore.getState()

// ResizeHandles는 창 테두리 안쪽의 변·모서리 핸들이다. 최대화된 창에는 그리지 않는다.
// 포커스는 창(section)의 onPointerDownCapture가 처리한다.
export function ResizeHandles({ id, rect, bounds }: Props) {
  const drag = useRef<Resize | null>(null)

  const onPointerDown = (edge: ResizeEdge) => (e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0) return
    // 끄는 동안 텍스트 선택이 시작되지 않게 한다.
    e.preventDefault()
    drag.current = { pointerId: e.pointerId, edge, startX: e.clientX, startY: e.clientY, start: rect }
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  const onPointerMove = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current
    if (!d || d.pointerId !== e.pointerId) return
    resize(id, resizeRect(d.start, d.edge, e.clientX - d.startX, e.clientY - d.startY, bounds))
  }
  const endDrag = (e: PointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId === e.pointerId) drag.current = null
  }

  return EDGES.map((edge) => (
    <div
      key={edge}
      className={`resize-handle resize-${edge}`}
      aria-hidden="true"
      onPointerDown={onPointerDown(edge)}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onLostPointerCapture={endDrag}
    />
  ))
}
