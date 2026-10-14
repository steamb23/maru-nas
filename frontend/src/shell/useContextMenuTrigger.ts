// 컨텍스트 메뉴를 여는 입력 처리: 우클릭, 키보드(메뉴 키, Shift+F10), 터치 길게 누르기.
import { type KeyboardEvent, type MouseEvent, type PointerEvent, useLayoutEffect, useRef } from 'react'
import { type ContextMenuItem, isContextMenuKey } from './contextMenu'
import { useContextMenuStore } from './contextMenuStore'

// 터치를 이 시간 동안 누르고 있으면 메뉴를 연다. 그 전에 이 거리 이상 움직이면 취소한다(끌기).
export const LONG_PRESS_MS = 500
const LONG_PRESS_SLOP = 8

interface TriggerEvent {
  target: EventTarget
  currentTarget: EventTarget
}

export interface ContextMenuTriggerOptions {
  // 메뉴를 열 때 항목을 만든다. 비어 있으면 열지 않는다.
  items: () => readonly ContextMenuItem[]
  // 이 요소에서 시작한 입력만 받을지. 없으면 모두 받는다(자식 요소의 입력 포함).
  accept?: (e: TriggerEvent) => boolean
}

export interface ContextMenuTrigger {
  // 요소에 붙일 처리기. 다른 처리기와 함께 쓰려면 mergeHandlers로 합친다.
  handlers: {
    onContextMenu: (e: MouseEvent<HTMLElement>) => void
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => void
    onPointerDown: (e: PointerEvent<HTMLElement>) => void
    onPointerMove: (e: PointerEvent<HTMLElement>) => void
    onPointerUp: (e: PointerEvent<HTMLElement>) => void
    onPointerCancel: (e: PointerEvent<HTMLElement>) => void
  }
  // 지금(또는 마지막) 누름이 메뉴를 열었는지. 끌기·클릭 처리가 그 누름을 무시하는 데 쓴다.
  // 다음 누름을 시작하면 false로 돌아간다.
  openedByPress: () => boolean
}

export function useContextMenuTrigger(opts: ContextMenuTriggerOptions): ContextMenuTrigger {
  const optsRef = useRef(opts)
  useLayoutEffect(() => {
    optsRef.current = opts
  })
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  // 누르고 있는 왼쪽 버튼·터치. 오른쪽 버튼은 contextmenu 이벤트만으로 처리한다.
  const down = useRef<{ pointerId: number; x: number; y: number } | null>(null)
  const opened = useRef(false)

  const accepts = (e: TriggerEvent) => optsRef.current.accept?.(e) ?? true
  const open = (x: number, y: number, viaKeyboard: boolean) =>
    useContextMenuStore.getState().openAt({ x, y }, optsRef.current.items(), viaKeyboard)
  const cancelTimer = () => {
    if (timer.current !== null) clearTimeout(timer.current)
    timer.current = null
  }
  const release = (e: PointerEvent<HTMLElement>) => {
    if (down.current?.pointerId !== e.pointerId) return
    cancelTimer()
    down.current = null
  }

  return {
    handlers: {
      onContextMenu: (e) => {
        if (!accepts(e)) return
        e.preventDefault()
        // 위젯 안에서 연 메뉴가 바탕화면 메뉴로 다시 열리지 않게 한다.
        e.stopPropagation()
        cancelTimer()
        // Android 등은 길게 누르면 contextmenu도 보낸다. 이미 길게 누르기로 열었으면 다시 열지 않는다.
        if (down.current) {
          if (opened.current) return
          opened.current = true
        }
        open(e.clientX, e.clientY, false)
      },
      onKeyDown: (e) => {
        if (!isContextMenuKey(e.key, e.shiftKey) || !accepts(e)) return
        e.preventDefault()
        e.stopPropagation()
        const r = e.currentTarget.getBoundingClientRect()
        open(r.x + 16, r.y + 16, true)
      },
      onPointerDown: (e) => {
        if (e.button !== 0 || !accepts(e)) return
        cancelTimer()
        opened.current = false
        const { pointerId, clientX: x, clientY: y } = e
        down.current = { pointerId, x, y }
        if (e.pointerType !== 'touch') return
        timer.current = setTimeout(() => {
          timer.current = null
          opened.current = true
          open(x, y, false)
        }, LONG_PRESS_MS)
      },
      onPointerMove: (e) => {
        const d = down.current
        if (!d || d.pointerId !== e.pointerId) return
        if (Math.hypot(e.clientX - d.x, e.clientY - d.y) >= LONG_PRESS_SLOP) cancelTimer()
      },
      onPointerUp: release,
      onPointerCancel: release,
    },
    openedByPress: () => opened.current,
  }
}

type Handler = ((e: never) => void) | undefined

// mergeHandlers는 같은 이벤트의 처리기를 차례로 부르는 처리기 묶음을 만든다.
export function mergeHandlers<A extends Record<string, Handler>, B extends Record<string, Handler>>(a: A, b: B): A & B {
  const out: Record<string, Handler> = { ...a, ...b }
  for (const key of Object.keys(a)) {
    const fa = a[key] as ((e: unknown) => void) | undefined
    const fb = b[key] as ((e: unknown) => void) | undefined
    if (fa && fb)
      out[key] = (e: never) => {
        fa(e)
        fb(e)
      }
  }
  return out as A & B
}
