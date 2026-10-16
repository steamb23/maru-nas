// 사용자 화면 설정 저장소(테마, 바탕화면, 애니메이션). DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 로그인한 동안 SignedIn이 불러오고(실패하면 다시 시도) 로그아웃하면 지운다. 설정 창이 닫혀 있어도
// 테마·바탕화면·애니메이션에 쓰인다. 저장은 다른 설정 저장소와 같은 저장 큐(api/saveQueue.ts)로 보낸다.
// 비어 있는 값은 관리자가 정한 기본 화면(siteStore.ts)을 따른다.
import { create } from 'zustand'
import * as client from '../api/client'
import { createSaveQueue } from '../api/saveQueue'
import type { AppearancePrefs, ImageInfo, Preferences } from '../api/types.gen'

export type AppearanceStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface AppearanceData {
  status: AppearanceStatus
  prefs: AppearancePrefs
  // 이 사용자가 올린 바탕화면 이미지. 없으면 null.
  image: ImageInfo | null
  saveFailed: boolean
}

export interface AppearanceActions {
  load: () => Promise<void>
  // update는 화면 설정 일부를 바꾼다. 값을 undefined로 주면 그 값을 비워 기본값을 따르게 한다.
  update: (patch: Partial<AppearancePrefs>) => void
  // imageUploaded는 바탕화면 이미지를 올린 뒤 부른다(백엔드가 바탕화면을 그 이미지로 바꾼다).
  imageUploaded: (info: ImageInfo) => void
  // imageDeleted는 올린 이미지를 지운 뒤 부른다(그 이미지였던 바탕화면은 기본값을 따른다).
  imageDeleted: () => void
  clear: () => void
}

export type AppearanceStore = AppearanceData & AppearanceActions

export interface AppearanceApi {
  fetchPreferences: () => Promise<Preferences>
  saveAppearance: (prefs: AppearancePrefs) => Promise<void>
}

export const initialAppearance: AppearanceData = { status: 'idle', prefs: {}, image: null, saveFailed: false }

// withPatch는 patch를 합치고 undefined인 값은 지운다(JSON으로 보낼 때와 같은 모양이 되게).
function withPatch(prefs: AppearancePrefs, patch: Partial<AppearancePrefs>): AppearancePrefs {
  const next: AppearancePrefs = { ...prefs, ...patch }
  for (const k of Object.keys(next) as (keyof AppearancePrefs)[]) {
    if (next[k] === undefined) delete next[k]
  }
  return next
}

export function createAppearanceStore(api: AppearanceApi) {
  return create<AppearanceStore>()((set, get) => {
    // load가 시작될 때마다 늘어난다. 늦게 온 응답이 새 상태를 덮지 않게 한다.
    let generation = 0
    const saves = createSaveQueue(api.saveAppearance, (ok) => set({ saveFailed: !ok }))
    const save = (prefs: AppearancePrefs) => {
      set({ prefs })
      saves.push(prefs)
    }
    return {
      ...initialAppearance,
      load: async () => {
        const gen = ++generation
        saves.reset()
        set({ status: 'loading' })
        try {
          const p = await api.fetchPreferences()
          if (gen !== generation) return
          set({ status: 'ready', prefs: p.appearance ?? {}, image: p.wallpaperImage ?? null, saveFailed: false })
        } catch {
          if (gen === generation) set({ status: 'error' })
        }
      },
      update: (patch) => {
        if (get().status !== 'ready') return
        save(withPatch(get().prefs, patch))
      },
      imageUploaded: (info) => {
        set({ image: info })
        if (get().status === 'ready') save(withPatch(get().prefs, { wallpaper: { kind: 'image' } }))
      },
      imageDeleted: () => {
        set({ image: null })
        const { status, prefs } = get()
        if (status === 'ready' && prefs.wallpaper?.kind === 'image') save(withPatch(prefs, { wallpaper: undefined }))
      },
      clear: () => {
        generation++
        saves.reset()
        set(initialAppearance)
      },
    }
  })
}

export const useAppearanceStore = createAppearanceStore({
  fetchPreferences: () => client.fetchPreferences(),
  saveAppearance: (p) => client.saveAppearance(p),
})

// allowsAnimations는 화면 설정이 애니메이션을 끄지 않았는지다. 값이 비어 있으면(불러오기 전 포함) 켜진 것으로 본다.
export function allowsAnimations(prefs: AppearancePrefs): boolean {
  return prefs.animations !== false
}

// animationsAllowed는 사용자가 애니메이션을 끄지 않았는지다. 불러오기 전에는 켜진 것으로 본다.
export function animationsAllowed(): boolean {
  return allowsAnimations(useAppearanceStore.getState().prefs)
}
