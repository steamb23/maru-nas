// 프로필 사진 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 로그인한 동안 SignedIn이 불러오고(실패하면 다시 시도) 로그아웃하면 지운다. ProfileAvatar가 이 값으로 그린다.
import { create } from 'zustand'
import * as client from '../api/client'
import type { ImageInfo, Preferences } from '../api/types.gen'

export type AvatarStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface AvatarData {
  status: AvatarStatus
  // 프로필 사진. 없으면 null이고 기본 아바타를 그린다.
  image: ImageInfo | null
}

export interface AvatarActions {
  load: () => Promise<void>
  // uploaded·deleted는 사진을 올리거나 지운 뒤 부른다.
  uploaded: (info: ImageInfo) => void
  deleted: () => void
  clear: () => void
}

export type AvatarStore = AvatarData & AvatarActions

export const initialAvatar: AvatarData = { status: 'idle', image: null }

export function createAvatarStore(fetchPreferences: () => Promise<Preferences>) {
  return create<AvatarStore>()((set) => {
    let generation = 0
    return {
      ...initialAvatar,
      load: async () => {
        const gen = ++generation
        set({ status: 'loading' })
        try {
          const p = await fetchPreferences()
          if (gen === generation) set({ status: 'ready', image: p.avatarImage ?? null })
        } catch {
          if (gen === generation) set({ status: 'error' })
        }
      },
      uploaded: (info) => {
        generation++
        set({ status: 'ready', image: info })
      },
      deleted: () => {
        generation++
        set({ status: 'ready', image: null })
      },
      clear: () => {
        generation++
        set(initialAvatar)
      },
    }
  })
}

export const useAvatarStore = createAvatarStore(() => client.fetchPreferences())
