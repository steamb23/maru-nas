// 시스템 정보 창 보기 설정(프로세스 CPU 기준, CPU 그래프 모양) 저장소. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 백엔드에 사용자별로 저장한다(/api/preferences의 systemInfo). 시스템 정보 창이 열려 있는 동안 load()로 가져오고,
// 닫히면 clear()로 비운다. 저장은 다른 설정 저장소와 같은 저장 큐(api/saveQueue.ts)를 쓴다(shell/appDrawerPrefsStore.ts와 같은 방식).
import { create } from 'zustand'
import * as client from '../../api/client'
import { createSaveQueue } from '../../api/saveQueue'
import type { Preferences, SystemInfoPrefs } from '../../api/types.gen'
import type { CpuScale } from './processes'

export type { CpuScale } from './processes'
export type CpuGraph = 'overall' | 'cores'

// toCpuScale·toCpuGraph는 저장된 값을 바꾼다. 없거나 모르는 값(다른 버전이 저장한 값 등)이면 기본값이다.
export const toCpuScale = (v: string | undefined): CpuScale => (v === 'core' ? 'core' : 'machine')
export const toCpuGraph = (v: string | undefined): CpuGraph => (v === 'cores' ? 'cores' : 'overall')

// idle: 아직 불러오지 않음. loading: 불러오는 중. ready: 설정 있음. error: 불러오지 못함(기본값을 보이고 바꿀 수 없음).
export type SysinfoPrefsStatus = 'idle' | 'loading' | 'ready' | 'error'

export interface SysinfoPrefsData {
  status: SysinfoPrefsStatus
  cpuScale: CpuScale
  cpuGraph: CpuGraph
  // 마지막 저장이 실패했는지. 다음 변경 때 다시 보낸다.
  saveFailed: boolean
}

export interface SysinfoPrefsActions {
  load: () => Promise<void>
  // set은 설정 일부를 바꾸고 전체를 백엔드에 저장한다. 불러오기 전에는 무시한다(저장된 값을 기본값으로 덮어쓰지 않게).
  set: (patch: Partial<Pick<SysinfoPrefsData, 'cpuScale' | 'cpuGraph'>>) => void
  clear: () => void
}

export type SysinfoPrefsStore = SysinfoPrefsData & SysinfoPrefsActions

export interface SysinfoPrefsApi {
  fetchPreferences: () => Promise<Preferences>
  saveSystemInfoPrefs: (prefs: SystemInfoPrefs) => Promise<void>
}

export const initialSysinfoPrefs: SysinfoPrefsData = {
  status: 'idle',
  cpuScale: 'machine',
  cpuGraph: 'overall',
  saveFailed: false,
}

export function createSysinfoPrefsStore(api: SysinfoPrefsApi) {
  return create<SysinfoPrefsStore>()((set, get) => {
    // load·clear마다 바뀐다. 창을 닫은 뒤 늦게 도착한 응답을 버리는 데 쓴다.
    let generation = 0
    const saves = createSaveQueue(api.saveSystemInfoPrefs, (ok) => set({ saveFailed: !ok }))

    return {
      ...initialSysinfoPrefs,

      load: async () => {
        const gen = ++generation
        saves.reset()
        set({ status: 'loading' })
        try {
          const prefs = await api.fetchPreferences()
          if (gen !== generation) return
          set({
            status: 'ready',
            cpuScale: toCpuScale(prefs.systemInfo?.cpuScale),
            cpuGraph: toCpuGraph(prefs.systemInfo?.cpuGraph),
            saveFailed: false,
          })
        } catch {
          if (gen === generation) set({ status: 'error' })
        }
      },

      set: (patch) => {
        const s = get()
        if (s.status !== 'ready') return
        const next = { cpuScale: patch.cpuScale ?? s.cpuScale, cpuGraph: patch.cpuGraph ?? s.cpuGraph }
        if (next.cpuScale === s.cpuScale && next.cpuGraph === s.cpuGraph) return
        set(next)
        saves.push(next)
      },

      clear: () => {
        generation++
        saves.reset()
        set(initialSysinfoPrefs)
      },
    }
  })
}

export const useSysinfoPrefsStore = createSysinfoPrefsStore({
  fetchPreferences: () => client.fetchPreferences(),
  saveSystemInfoPrefs: (p) => client.saveSystemInfoPrefs(p),
})
