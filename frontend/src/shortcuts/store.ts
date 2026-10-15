// 바탕화면 바로가기 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 목록은 백엔드에 사용자별로 저장한다(/api/preferences). 로그인하면 load()로 가져오고, 로그아웃·만료로
// 데스크톱을 떠나면 clear()로 비운다. 불러오기·저장 방식은 위젯 배치 저장소(widgets/store.ts)와 같다.
import { create } from 'zustand'
import * as client from '../api/client'
import type { Preferences, Shortcut } from '../api/types.gen'
import type { AppId } from '../windows/apps'
import { defaultShortcuts, knownShortcuts } from './list'

// idle: 아직 불러오지 않음. loading: 불러오는 중. ready: 목록 있음. error: 불러오지 못함(바로가기를 그리지 않음).
export type ShortcutStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface ShortcutData {
  status: ShortcutStatus
  // 이번 로그인에서 한 번이라도 불러오기를 끝냈는지(성공이든 실패든). 다시 시도하는 동안에도 true로 남는다.
  // 위젯 레이어가 아이콘 영역을 믿고 그려도 되는지 판단한다.
  loaded: boolean
  // 저장 목록(바탕화면 순서). 지금 사용자에게 보이지 않는 앱도 들어 있다(list.ts).
  apps: AppId[]
  // 마지막 저장이 실패했는지. 다음 변경 때 다시 보낸다.
  saveFailed: boolean
}

export interface ShortcutActions {
  load: () => Promise<void>
  // set은 새 저장 목록을 반영하고 백엔드에 저장한다. 계산은 list.ts의 placeShortcut 등으로 한다.
  set: (apps: AppId[]) => void
  clear: () => void
}

export type ShortcutStore = ShortcutData & ShortcutActions

export interface ShortcutApi {
  fetchPreferences: () => Promise<Preferences>
  saveShortcuts: (shortcuts: readonly Shortcut[]) => Promise<void>
}

export const initialShortcutData: ShortcutData = { status: 'idle', loaded: false, apps: [], saveFailed: false }

export function createShortcutStore(api: ShortcutApi) {
  return create<ShortcutStore>()((set, get) => {
    // load·clear마다 바뀐다. 로그아웃한 뒤 늦게 도착한 응답과 남은 저장을 버리는 데 쓴다.
    let generation = 0
    // 저장은 한 번에 하나만 보낸다. 보내는 동안 바뀐 목록은 마지막 것만 기다렸다 보낸다.
    let saving = false
    let pending: AppId[] | null = null

    const flush = async () => {
      if (saving || !pending) return
      saving = true
      const gen = generation
      const value = pending
      pending = null
      try {
        await api.saveShortcuts(value.map((app) => ({ app })))
        if (gen === generation) set({ saveFailed: false })
      } catch {
        // 로컬 목록은 그대로 둔다. 401은 API 클라이언트가 로그인 화면으로 보낸다.
        if (gen === generation) set({ saveFailed: true })
      } finally {
        saving = false
      }
      // 보내는 동안 바뀐 목록이 있으면 이어서 보낸다(clear()는 기다리던 목록을 버린다).
      void flush()
    }

    return {
      ...initialShortcutData,

      load: async () => {
        const gen = ++generation
        pending = null
        set({ status: 'loading' })
        try {
          const prefs = await api.fetchPreferences()
          if (gen !== generation) return
          const apps = prefs.shortcuts === undefined ? defaultShortcuts() : knownShortcuts(prefs.shortcuts)
          set({ status: 'ready', loaded: true, apps, saveFailed: false })
        } catch {
          if (gen === generation) set({ status: 'error', loaded: true })
        }
      },

      set: (apps) => {
        if (get().status !== 'ready') return
        set({ apps })
        pending = apps
        void flush()
      },

      clear: () => {
        generation++
        pending = null
        set(initialShortcutData)
      },
    }
  })
}

export const useShortcutStore = createShortcutStore({
  fetchPreferences: () => client.fetchPreferences(),
  saveShortcuts: (s) => client.saveShortcuts(s),
})
