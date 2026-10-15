// "모든 앱" 화면의 열림 상태. 작업표시줄 버튼과 Desktop이 그리는 화면(AppDrawer)이 함께 쓴다.
// DOM에 의존하지 않아 node 환경에서 테스트한다.
import { create } from 'zustand'

// 화면 요소의 id. 작업표시줄 버튼의 aria-controls가 가리킨다.
export const APP_DRAWER_ID = 'app-drawer'

interface AppDrawerState {
  open: boolean
  // 터치로 열었는지. 그러면 검색 상자에 포커스하지 않는다(가상 키보드가 화면을 가리지 않게).
  viaTouch: boolean
  // 닫기 애니메이션 없이 닫았는지. 앱을 바탕화면으로 끌어 놓을 때는 화면이 이미 사라져 있다.
  instant: boolean
  show: (viaTouch?: boolean) => void
  toggle: (viaTouch?: boolean) => void
  close: (instant?: boolean) => void
}

export const useAppDrawerStore = create<AppDrawerState>((set, get) => ({
  open: false,
  viaTouch: false,
  instant: false,
  show: (viaTouch = false) => set({ open: true, viaTouch, instant: false }),
  toggle: (viaTouch = false) => (get().open ? get().close() : get().show(viaTouch)),
  close: (instant = false) => set((s) => (s.open ? { open: false, instant } : s)),
}))
