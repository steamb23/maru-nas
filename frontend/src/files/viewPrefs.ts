// 파일 탐색기 보기 설정(목록·아이콘 보기) 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 백엔드에 사용자별로 저장한다(/api/preferences의 files). 탐색기 창은 열 때 load()로 가져와 그 보기로 시작하고, 창에서
// 보기를 바꾸면 set()으로 저장해 다음에 여는 창의 기본값이 된다. 이미 열린 다른 창의 보기는 바꾸지 않는다(창마다 따로 둔다).
// 저장은 다른 설정 저장소와 같은 저장 큐(api/saveQueue.ts)를 쓴다.
import { create } from 'zustand'
import * as client from '../api/client'
import { createSaveQueue } from '../api/saveQueue'
import type { FilesPrefs, Preferences } from '../api/types.gen'

export type FilesLayout = 'list' | 'grid'

// toLayout은 저장된 보기다. 없거나 모르는 값(다른 버전이 저장한 값 등)이면 목록 보기다.
export const toLayout = (v: string | undefined): FilesLayout => (v === 'grid' ? 'grid' : 'list')

// idle: 아직 불러오지 않음. loading: 불러오는 중. ready: 설정 있음. error: 불러오지 못함(목록 보기로 시작하고 저장하지 않는다).
export type FilesPrefsStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface FilesPrefsData {
  status: FilesPrefsStatus
  layout: FilesLayout
}

export interface FilesPrefsActions {
  load: () => Promise<void>
  // set은 보기를 바꾸고 백엔드에 저장한다. 불러오기 전에는 무시한다(저장된 값을 기본값으로 덮어쓰지 않게).
  set: (layout: FilesLayout) => void
  clear: () => void
}

export type FilesPrefsStore = FilesPrefsData & FilesPrefsActions

export interface FilesPrefsApi {
  fetchPreferences: () => Promise<Preferences>
  saveFilesPrefs: (prefs: FilesPrefs) => Promise<void>
}

export const initialFilesPrefs: FilesPrefsData = { status: 'idle', layout: 'list' }

export function createFilesPrefsStore(api: FilesPrefsApi) {
  return create<FilesPrefsStore>()((set, get) => {
    // load·clear마다 바뀐다. 늦게 도착한 응답을 버리는 데 쓴다.
    let generation = 0
    // 실패는 다음 변경 때 다시 보낸다. 보기 전환은 다시 고르면 되므로 따로 알리지 않는다.
    const saves = createSaveQueue(api.saveFilesPrefs, () => {})

    return {
      ...initialFilesPrefs,

      load: async () => {
        const gen = ++generation
        // 창을 열 때마다 다시 받는다(다른 탭에서 바꾼 값, 로그아웃 뒤 다른 사용자).
        set({ status: 'loading' })
        try {
          const prefs = await api.fetchPreferences()
          if (gen !== generation) return
          set({ status: 'ready', layout: toLayout(prefs.files?.view) })
        } catch {
          if (gen === generation) set({ status: 'error' })
        }
      },

      set: (layout) => {
        const s = get()
        if (s.status !== 'ready' || s.layout === layout) return
        set({ layout })
        saves.push({ view: layout })
      },

      clear: () => {
        generation++
        saves.reset()
        set(initialFilesPrefs)
      },
    }
  })
}

export const useFilesPrefsStore = createFilesPrefsStore({
  fetchPreferences: () => client.fetchPreferences(),
  saveFilesPrefs: (p) => client.saveFilesPrefs(p),
})
