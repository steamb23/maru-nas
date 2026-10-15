// 설정 창이 보여 줄 페이지. 창 저장소는 앱마다 창 하나와 위치만 다루므로, 어느 페이지를 열지는 여기에 둔다.
// 프로필 메뉴 머리글처럼 창 밖에서 특정 페이지로 열 때 show를 부르고 창을 연다.
import { create } from 'zustand'
import { type WindowStore, useWindowStore } from '../windows/store'
import { DEFAULT_SETTINGS_PAGE, type SettingsPageId } from './pages'

interface SettingsNavState {
  page: SettingsPageId
  show: (page: SettingsPageId) => void
  // reset은 다음에 기본 페이지로 열리게 한다. 설정 창이 닫히면 저절로 부른다.
  reset: () => void
}

export const useSettingsNavStore = create<SettingsNavState>()((set) => ({
  page: DEFAULT_SETTINGS_PAGE,
  show: (page) => set({ page }),
  reset: () => set({ page: DEFAULT_SETTINGS_PAGE }),
}))

const settingsOpen = (s: WindowStore) => s.windows.some((w) => w.appId === 'settings')

// 설정 창이 닫히면 다음에 기본 페이지로 열리게 한다. 창 컴포넌트의 정리 함수는 개발 모드(StrictMode)에서
// 마운트 직후에도 불려 방금 고른 페이지를 지우므로 쓰지 않고, 창 저장소에서 창이 사라지는 것을 본다.
useWindowStore.subscribe((s, prev) => {
  if (settingsOpen(prev) && !settingsOpen(s)) useSettingsNavStore.getState().reset()
})
