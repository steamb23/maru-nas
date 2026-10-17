// 설정 창이 보여 줄 페이지. 창 저장소는 앱마다 창 하나와 위치만 다루므로, 어느 페이지를 열지는 여기에 둔다.
// 프로필 메뉴 머리글처럼 창 밖에서 특정 페이지로 열 때 show를 부르고 창을 연다.
import { create } from 'zustand'
import { type WindowStore, useWindowStore } from '../windows/store'
import { DEFAULT_SETTINGS_PAGE, type SettingsPageId } from './pages'

interface SettingsNavState {
  page: SettingsPageId
  // resetPending은 설정 창이 닫힌 뒤 아직 페이지를 고르지 않았다는 뜻이다. 다음에 열릴 때 기본 페이지로 바꾼다.
  resetPending: boolean
  show: (page: SettingsPageId) => void
}

export const initialSettingsNav = { page: DEFAULT_SETTINGS_PAGE, resetPending: false }

export const useSettingsNavStore = create<SettingsNavState>()((set) => ({
  ...initialSettingsNav,
  show: (page) => set({ page, resetPending: false }),
}))

const settingsOpen = (s: WindowStore) => s.windows.some((w) => w.appId === 'settings')

// 설정 창이 닫히면 다음에 기본 페이지로 열리게 한다. 창 컴포넌트의 정리 함수는 개발 모드(StrictMode)에서
// 마운트 직후에도 불려 방금 고른 페이지를 지우므로 쓰지 않고, 창 저장소에서 창이 사라지는 것을 본다.
// 닫힌 창은 닫기 애니메이션 동안 이 저장소의 페이지로 계속 그려지므로, 닫힐 때는 표시만 해 두고 페이지는
// 다시 열릴 때 바꾼다. 그 전에 show로 페이지를 골랐으면 표시가 지워져 그 페이지로 열린다.
useWindowStore.subscribe((s, prev) => {
  const wasOpen = settingsOpen(prev)
  const isOpen = settingsOpen(s)
  if (wasOpen && !isOpen) useSettingsNavStore.setState({ resetPending: true })
  else if (!wasOpen && isOpen && useSettingsNavStore.getState().resetPending) {
    useSettingsNavStore.setState(initialSettingsNav)
  }
})
