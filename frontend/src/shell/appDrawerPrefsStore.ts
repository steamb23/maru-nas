// "모든 앱" 화면 설정(정렬 방식) 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 백엔드에 사용자별로 저장한다(/api/preferences의 drawer). "모든 앱" 화면이 로그인한 동안 load()로 가져오고,
// 데스크톱을 떠나면 clear()로 비운다. 저장은 다른 설정 저장소와 같은 저장 큐(api/saveQueue.ts)를 쓴다.
import { create } from 'zustand'
import * as client from '../api/client'
import { createSaveQueue } from '../api/saveQueue'
import type { DrawerPrefs, Preferences } from '../api/types.gen'
import { type AppSort, DEFAULT_APP_SORT, toAppSort } from './appSort'

// idle: 아직 불러오지 않음. loading: 불러오는 중. ready: 설정 있음. error: 불러오지 못함(기본값을 보이고 바꿀 수 없음).
export type AppDrawerPrefsStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface AppDrawerPrefsData {
  status: AppDrawerPrefsStatus
  sort: AppSort
  // 마지막 저장이 실패했는지. 다음 변경 때 다시 보낸다.
  saveFailed: boolean
}

export interface AppDrawerPrefsActions {
  load: () => Promise<void>
  // setSort는 정렬 방식을 바꾸고 백엔드에 저장한다. 불러오기 전에는 무시한다(저장된 값을 기본값으로 덮어쓰지 않게).
  setSort: (sort: AppSort) => void
  clear: () => void
}

export type AppDrawerPrefsStore = AppDrawerPrefsData & AppDrawerPrefsActions

export interface AppDrawerPrefsApi {
  fetchPreferences: () => Promise<Preferences>
  saveDrawer: (drawer: DrawerPrefs) => Promise<void>
}

export const initialAppDrawerPrefs: AppDrawerPrefsData = { status: 'idle', sort: DEFAULT_APP_SORT, saveFailed: false }

export function createAppDrawerPrefsStore(api: AppDrawerPrefsApi) {
  return create<AppDrawerPrefsStore>()((set, get) => {
    // load·clear마다 바뀐다. 로그아웃한 뒤 늦게 도착한 응답을 버리는 데 쓴다.
    let generation = 0
    // 설정 전체({sort})를 한 번에 저장한다.
    const saves = createSaveQueue(api.saveDrawer, (ok) => set({ saveFailed: !ok }))

    return {
      ...initialAppDrawerPrefs,

      load: async () => {
        const gen = ++generation
        saves.reset()
        set({ status: 'loading' })
        try {
          const prefs = await api.fetchPreferences()
          if (gen !== generation) return
          set({ status: 'ready', sort: toAppSort(prefs.drawer?.sort), saveFailed: false })
        } catch {
          if (gen === generation) set({ status: 'error' })
        }
      },

      setSort: (sort) => {
        if (get().status !== 'ready' || get().sort === sort) return
        set({ sort })
        saves.push({ sort })
      },

      clear: () => {
        generation++
        saves.reset()
        set(initialAppDrawerPrefs)
      },
    }
  })
}

export const useAppDrawerPrefsStore = createAppDrawerPrefsStore({
  fetchPreferences: () => client.fetchPreferences(),
  saveDrawer: (d) => client.saveDrawer(d),
})
