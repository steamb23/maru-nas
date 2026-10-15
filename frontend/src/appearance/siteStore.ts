// 공개 전역 정보 저장소(GET /api/site): 로그인 배경, 대기 연출, 기본 화면. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 로그인 전에도 쓰므로 앱이 시작할 때 불러오고, 로그인·로그아웃 뒤와 관리자가 전역 설정을 저장한 뒤 다시 불러온다.
// 실패하면 받아 둔 값을 그대로 쓴다(없으면 대시보드 기본값).
import { create } from 'zustand'
import * as client from '../api/client'
import type { SiteInfo } from '../api/types.gen'

export interface SiteState {
  info: SiteInfo | null
  load: () => Promise<void>
  // set은 관리자가 저장한 값을 다시 불러오기 전에 바로 보여 줄 때 쓴다.
  set: (info: SiteInfo) => void
}

export function createSiteStore(fetchSite: () => Promise<SiteInfo>) {
  return create<SiteState>()((set) => {
    let generation = 0
    return {
      info: null,
      load: async () => {
        const gen = ++generation
        try {
          const info = await fetchSite()
          if (gen === generation) set({ info })
        } catch {
          // 받아 둔 값을 그대로 쓴다.
        }
      },
      set: (info) => {
        generation++
        set({ info })
      },
    }
  })
}

export const useSiteStore = createSiteStore(() => client.fetchSite())
