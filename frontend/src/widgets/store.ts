// 위젯 저장 배치 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 배치는 백엔드에 사용자별로 저장한다(/api/preferences). 로그인하면 load()로 가져오고,
// 로그아웃·만료로 데스크톱을 떠나면 clear()로 비운다. 화면 배치 계산(layout.ts)은 이 저장소 밖에서 한다.
import { create } from 'zustand'
import * as client from '../api/client'
import { createSaveQueue } from '../api/saveQueue'
import type { Preferences, WidgetPlacement } from '../api/types.gen'
import { defaultLayout, knownPlacements } from './catalog'

// idle: 아직 불러오지 않음. loading: 불러오는 중. ready: 배치 있음. error: 불러오지 못함(위젯을 그리지 않음).
export type WidgetStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface WidgetData {
  status: WidgetStatus
  // 저장 배치(오른쪽 위 기준 칸 좌표). 서로 겹치지 않는다.
  placements: WidgetPlacement[]
  // 마지막 저장이 실패했는지. 다음 변경 때 다시 보낸다.
  saveFailed: boolean
}

export interface WidgetActions {
  load: () => Promise<void>
  // set은 새 저장 배치를 반영하고 백엔드에 저장한다. 계산은 layout.ts의 commitPlacement 등으로 한다.
  set: (placements: WidgetPlacement[]) => void
  clear: () => void
}

export type WidgetStore = WidgetData & WidgetActions

export interface WidgetApi {
  fetchPreferences: () => Promise<Preferences>
  saveWidgets: (widgets: readonly WidgetPlacement[]) => Promise<void>
}

export const initialWidgetData: WidgetData = { status: 'idle', placements: [], saveFailed: false }

export function createWidgetStore(api: WidgetApi) {
  return create<WidgetStore>()((set, get) => {
    // load·clear마다 바뀐다. 로그아웃한 뒤 늦게 도착한 응답을 버리는 데 쓴다.
    let generation = 0
    // 실패해도 로컬 배치는 그대로 두고 다음 변경 때 다시 보낸다. load·clear는 남은 저장을 버린다.
    const saves = createSaveQueue(api.saveWidgets, (ok) => set({ saveFailed: !ok }))

    return {
      ...initialWidgetData,

      load: async () => {
        const gen = ++generation
        saves.reset()
        set({ status: 'loading' })
        try {
          const prefs = await api.fetchPreferences()
          if (gen !== generation) return
          const placements = prefs.widgets === undefined ? defaultLayout() : knownPlacements(prefs.widgets)
          set({ status: 'ready', placements, saveFailed: false })
        } catch {
          if (gen === generation) set({ status: 'error' })
        }
      },

      set: (placements) => {
        if (get().status !== 'ready') return
        set({ placements })
        saves.push(placements)
      },

      clear: () => {
        generation++
        saves.reset()
        set(initialWidgetData)
      },
    }
  })
}

export const useWidgetStore = createWidgetStore({
  fetchPreferences: () => client.fetchPreferences(),
  saveWidgets: (w) => client.saveWidgets(w),
})
