// 모달 범위(창·데스크톱)의 호스트와 쌓임 상태. Desktop과 Window가 컨텍스트로 내려 주고,
// Modal·useDialogs가 자기 범위를 찾는다.
//
// 창 보기 안에서는 두 컨텍스트가 모두 보이므로, 창 안에서도 데스크톱 범위 모달을 열 수 있다.
import { createContext, useCallback, useContext, useMemo, useState } from 'react'
import { popLayer, pushLayer } from './modalStack'

export type ModalScope = 'window' | 'desktop'

export interface ModalLayer {
  // 모달을 그려 넣을 요소(createPortal 대상). 처음 그려지기 전에는 null이다.
  host: HTMLElement | null
  // 열린 모달 key. 비어 있지 않으면 범위를 막는다(inert).
  stack: readonly number[]
  push: (key: number) => void
  pop: (key: number) => void
}

export interface WindowModalLayer extends ModalLayer {
  windowId: string
}

export const DesktopModalContext = createContext<ModalLayer | null>(null)
export const WindowModalContext = createContext<WindowModalLayer | null>(null)

// useModalHost는 범위 하나의 호스트 상태를 만든다. setHost는 호스트 요소의 ref로 넘기고,
// blocked이면 그 범위의 내용에 inert를 건다.
export function useModalHost() {
  const [host, setHost] = useState<HTMLElement | null>(null)
  const [stack, setStack] = useState<readonly number[]>([])
  const push = useCallback((key: number) => setStack((s) => pushLayer(s, key)), [])
  const pop = useCallback((key: number) => setStack((s) => popLayer(s, key)), [])
  const layer = useMemo<ModalLayer>(() => ({ host, stack, push, pop }), [host, stack, push, pop])
  return { setHost, layer, blocked: stack.length > 0 }
}

// useModalLayer는 scope의 범위를 찾는다. scope가 없으면 창 안에서는 그 창, 밖에서는 데스크톱이다.
export function useModalLayer(scope?: ModalScope): ModalLayer | null {
  const desktop = useContext(DesktopModalContext)
  const win = useContext(WindowModalContext)
  if (scope === 'desktop') return desktop
  if (scope === 'window') return win
  return win ?? desktop
}
