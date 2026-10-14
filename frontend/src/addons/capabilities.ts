// 애드온 capability(/api/capabilities) 저장소와 앱별 표시 판정. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 아직 받지 못했거나 요청이 실패하면 모두 쓸 수 있는 것으로 보고 그린다. 실제 차단은 백엔드가 한다.
import { create } from 'zustand'
import type { AddonCapability, CapabilitiesResponse } from '../api/types.gen'
import { type AppId, isAddonApp } from '../windows/apps'
import { type AddonId, MANIFESTS } from './manifests.gen'
import type { AddonManifest } from './types'

export type UnavailableReason = NonNullable<AddonCapability['reason']>

export interface AppAccess {
  // 바탕화면 아이콘을 보일지. 관리자 전용 앱은 일반 사용자에게 숨긴다.
  visible: boolean
  // 창 안에 보기 대신 보여 줄 안내. null이면 창 보기를 그대로 그린다.
  notice: UnavailableReason | null
}

const OPEN: AppAccess = { visible: true, notice: null }

// appAccess는 capability로 앱 아이콘과 창 내용을 어떻게 그릴지 정한다.
// 관리자 전용(admin_required)은 아이콘을 숨기고, 플러그인 없음(plugin_missing)은 아이콘을 두고
// 창 안에 안내한다. 이미 열린 창이면 두 경우 모두 창 안에 안내한다.
export function appAccess(caps: CapabilitiesResponse | null, id: AppId): AppAccess {
  if (!caps || !isAddonApp(id)) return OPEN
  const c = caps.addons.find((a) => a.id === id)
  if (!c || c.available) return OPEN
  const reason = c.reason ?? 'plugin_missing'
  return { visible: reason !== 'admin_required', notice: reason }
}

// requiredPlugins는 앱 manifest가 선언한 OMV 플러그인 패키지 이름이다. 안내 문구에 쓴다.
export function requiredPlugins(id: AppId): readonly string[] {
  if (!isAddonApp(id)) return []
  return (MANIFESTS as Record<AddonId, AddonManifest>)[id].requires.plugins ?? []
}

interface CapabilitiesState {
  // 로그인한 사용자 기준 capability. 아직 받지 못했으면 null.
  caps: CapabilitiesResponse | null
  set: (caps: CapabilitiesResponse) => void
  clear: () => void
}

export const useCapabilitiesStore = create<CapabilitiesState>((set) => ({
  caps: null,
  set: (caps) => set({ caps }),
  clear: () => set({ caps: null }),
}))
