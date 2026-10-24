// 파일 탐색기 보기 설정(보기 모양, 사이드바에서 숨긴 공유 폴더) 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 백엔드에 사용자별로 저장한다(/api/preferences의 files). 탐색기 창은 열 때 load()로 가져온다.
// - 보기 모양: 창은 이 보기로 시작하고, 창에서 바꾸면 set()으로 저장해 다음에 여는 창의 기본값이 된다. 이미 열린 다른 창의
//   보기는 바꾸지 않는다(창마다 따로 둔다).
// - 숨긴 공유 폴더: 사이드바 표시만 바꾸며 접근과는 관계없다. 보기 모양과 달리 열린 모든 창의 사이드바에 바로 적용한다(창이
//   이 저장소를 구독한다).
// 백엔드는 files 설정 전체를 바꾸므로 저장할 때는 언제나 두 값을 함께 보낸다(한쪽 저장이 다른 쪽을 지우지 않게).
// 저장은 다른 설정 저장소와 같은 저장 큐(api/saveQueue.ts)를 쓴다.
import { create } from 'zustand'
import * as client from '../api/client'
import { createSaveQueue } from '../api/saveQueue'
import { type FilesPrefs, MaxHiddenShares, type Preferences } from '../api/types.gen'

export type FilesLayout = 'list' | 'grid'

// toLayout은 저장된 보기다. 없거나 모르는 값(다른 버전이 저장한 값 등)이면 목록 보기다.
export const toLayout = (v: string | undefined): FilesLayout => (v === 'grid' ? 'grid' : 'list')

// idle: 아직 불러오지 않음. loading: 불러오는 중. ready: 설정 있음. error: 불러오지 못함(목록 보기로 시작하고 저장하지 않는다).
export type FilesPrefsStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface FilesPrefsData {
  status: FilesPrefsStatus
  // settled는 한 번이라도 불러오기를 마쳤는지(ready 또는 error)다. 다시 불러오는 동안(loading)에도 true로 남는다.
  // 창은 이것이 false인 동안 공유 폴더 묶음을 그리지 않는다(숨긴 폴더가 잠깐 보였다 사라지지 않게).
  settled: boolean
  layout: FilesLayout
  // hiddenShares는 사이드바에서 숨긴 공유 폴더의 uuid다. 스냅숏에 없는 uuid는 무시한다.
  hiddenShares: readonly string[]
}

export interface FilesPrefsActions {
  // load는 user의 설정을 받는다. 지난번과 다른 사용자면 받기 전에 비운다(이전 사용자의 숨긴 공유 폴더가 보이지 않게).
  load: (user: string) => Promise<void>
  // set은 보기를 바꾸고 백엔드에 저장한다. 불러오기 전에는 무시한다(저장된 값을 기본값으로 덮어쓰지 않게).
  set: (layout: FilesLayout) => void
  // hideShare·showShare·showAllShares는 숨긴 공유 폴더를 바꾸고 저장한다. 불러오기 전에는 set과 같이 무시한다.
  hideShare: (uuid: string) => void
  showShare: (uuid: string) => void
  showAllShares: () => void
  clear: () => void
}

export type FilesPrefsStore = FilesPrefsData & FilesPrefsActions

export interface FilesPrefsApi {
  fetchPreferences: () => Promise<Preferences>
  saveFilesPrefs: (prefs: FilesPrefs) => Promise<void>
}

export const initialFilesPrefs: FilesPrefsData = { status: 'idle', settled: false, layout: 'list', hiddenShares: [] }

export function createFilesPrefsStore(api: FilesPrefsApi) {
  return create<FilesPrefsStore>()((set, get) => {
    // load·clear마다 바뀐다. 늦게 도착한 응답을 버리는 데 쓴다.
    let generation = 0
    // owner는 지금 값이 누구의 설정인지다.
    let owner: string | null = null
    // 실패는 다음 변경 때 다시 보낸다. 보기 전환은 다시 고르면 되므로 따로 알리지 않는다.
    const saves = createSaveQueue(api.saveFilesPrefs, () => {})

    // apply는 불러온 뒤에만 바꾸고 두 값을 함께 저장한다.
    const apply = (next: Partial<Pick<FilesPrefsData, 'layout' | 'hiddenShares'>>) => {
      const s = get()
      if (s.status !== 'ready') return
      const layout = next.layout ?? s.layout
      const hiddenShares = next.hiddenShares ?? s.hiddenShares
      set({ layout, hiddenShares })
      saves.push({ view: layout, hiddenShares: [...hiddenShares] })
    }

    return {
      ...initialFilesPrefs,

      load: async (user) => {
        const gen = ++generation
        if (owner !== user) {
          owner = user
          saves.reset()
          set(initialFilesPrefs)
        }
        // 창을 열 때마다 다시 받는다(다른 탭에서 바꾼 값).
        set({ status: 'loading' })
        try {
          const prefs = await api.fetchPreferences()
          if (gen !== generation) return
          set({ status: 'ready', settled: true, layout: toLayout(prefs.files?.view), hiddenShares: prefs.files?.hiddenShares ?? [] })
        } catch {
          if (gen === generation) set({ status: 'error', settled: true })
        }
      },

      set: (layout) => {
        if (get().layout === layout) return
        apply({ layout })
      },

      hideShare: (uuid) => {
        const { hiddenShares } = get()
        // 상한을 넘으면 백엔드가 거절하므로 더하지 않는다.
        if (uuid === '' || hiddenShares.includes(uuid) || hiddenShares.length >= MaxHiddenShares) return
        apply({ hiddenShares: [...hiddenShares, uuid] })
      },

      showShare: (uuid) => {
        const { hiddenShares } = get()
        if (!hiddenShares.includes(uuid)) return
        apply({ hiddenShares: hiddenShares.filter((u) => u !== uuid) })
      },

      showAllShares: () => {
        if (get().hiddenShares.length === 0) return
        apply({ hiddenShares: [] })
      },

      clear: () => {
        generation++
        owner = null
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
