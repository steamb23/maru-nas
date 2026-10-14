// 컨텍스트 메뉴 저장소. 한 번에 메뉴 하나만 열린다. DOM에 의존하지 않아 node 환경에서 테스트한다.
import { create } from 'zustand'
import type { ContextMenuItem, Point } from './contextMenu'

export interface OpenMenu {
  // 열 때마다 바뀐다. 같은 자리에서 다시 열어도 새 메뉴로 그린다(열기 애니메이션).
  key: number
  at: Point
  items: readonly ContextMenuItem[]
  // 키보드(메뉴 키, Shift+F10)로 열었으면 첫 항목에 포커스한다.
  viaKeyboard: boolean
}

interface ContextMenuState {
  menu: OpenMenu | null
  // 항목이 없으면 열지 않는다.
  openAt: (at: Point, items: readonly ContextMenuItem[], viaKeyboard?: boolean) => void
  close: () => void
}

let nextKey = 1

export const useContextMenuStore = create<ContextMenuState>((set) => ({
  menu: null,
  openAt: (at, items, viaKeyboard = false) => {
    if (items.length === 0) return
    set({ menu: { key: nextKey++, at, items, viaKeyboard } })
  },
  close: () => set((s) => (s.menu ? { menu: null } : s)),
}))
