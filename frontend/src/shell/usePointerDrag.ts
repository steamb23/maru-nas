// 포인터로 끄는 동작의 공통 처리: pointer capture, 임계 거리, 취소. 창·위젯 어디서든 쓸 수 있다.
import { type PointerEvent, useLayoutEffect, useRef } from 'react'

export interface DragDelta {
  dx: number
  dy: number
}

export interface PointerDragOptions {
  // 누른 지점에서 이 거리(px) 이상 움직여야 끌기로 본다. 그 전에 놓으면 onEnd의 moved가 false(클릭)다.
  threshold: number
  // 누를 때 부른다. false를 반환하면 이 포인터 동작을 무시한다.
  onStart?: (e: PointerEvent<HTMLElement>) => boolean | void
  // 임계 거리를 넘은 뒤 움직일 때마다 부른다.
  onMove: (delta: DragDelta, e: PointerEvent<HTMLElement>) => void
  // 놓을 때 부른다. moved가 false면 임계 거리를 넘지 않았다.
  onEnd: (moved: boolean, delta: DragDelta, e: PointerEvent<HTMLElement>) => void
  // 포인터가 취소되면(pointercancel, capture를 잃음) 부른다. 놓기 전에만 불린다.
  onCancel?: () => void
}

interface DragState {
  pointerId: number
  x: number
  y: number
  moved: boolean
  // 누른 요소(pointer capture를 가진 요소)
  el: HTMLElement
}

// usePointerDrag는 요소에 붙일 포인터 이벤트 처리기를 반환한다. 왼쪽 버튼(터치·펜 포함)만 다룬다.
export function usePointerDrag(opts: PointerDragOptions) {
  // 처리기는 렌더마다 새로 만들어지지만 끄는 중 상태는 유지한다. 콜백은 항상 최신 것을 부른다.
  const optsRef = useRef(opts)
  useLayoutEffect(() => {
    optsRef.current = opts
  })
  const drag = useRef<DragState | null>(null)

  const deltaOf = (e: PointerEvent<HTMLElement>, d: DragState): DragDelta => ({ dx: e.clientX - d.x, dy: e.clientY - d.y })

  const cancel = (e: PointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId !== e.pointerId) return
    drag.current = null
    optsRef.current.onCancel?.()
  }

  return {
    onPointerDown: (e: PointerEvent<HTMLElement>) => {
      if (e.button !== 0) return
      const prev = drag.current
      if (prev) {
        // 다른 포인터(두 번째 손가락 등)로 끄는 중이면 무시한다.
        if (prev.el.isConnected && prev.el.hasPointerCapture(prev.pointerId)) return
        // 누른 요소가 끄는 도중 지워지면 pointerup·lostpointercapture가 React까지 오지 않아 끝을 모른다.
        // 다음 누름에서 그 끌기를 취소로 정리하고 새로 시작한다.
        drag.current = null
        optsRef.current.onCancel?.()
      }
      if (optsRef.current.onStart?.(e) === false) return
      drag.current = { pointerId: e.pointerId, x: e.clientX, y: e.clientY, moved: false, el: e.currentTarget }
      e.currentTarget.setPointerCapture(e.pointerId)
    },
    onPointerMove: (e: PointerEvent<HTMLElement>) => {
      const d = drag.current
      if (!d || d.pointerId !== e.pointerId) return
      const delta = deltaOf(e, d)
      if (!d.moved) {
        if (Math.hypot(delta.dx, delta.dy) < optsRef.current.threshold) return
        d.moved = true
      }
      optsRef.current.onMove(delta, e)
    },
    onPointerUp: (e: PointerEvent<HTMLElement>) => {
      const d = drag.current
      if (!d || d.pointerId !== e.pointerId) return
      drag.current = null
      optsRef.current.onEnd(d.moved, deltaOf(e, d), e)
    },
    // pointerup 뒤에 오는 lostpointercapture는 이미 끝난 동작이라 무시된다.
    onPointerCancel: cancel,
    onLostPointerCapture: cancel,
  }
}
