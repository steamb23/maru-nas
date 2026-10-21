// 애드온 capability(/api/capabilities) 저장소와 앱별 표시 판정. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 아직 받지 못했거나 요청이 실패하면 모두 쓸 수 있는 것으로 보고 그린다. 실제 차단은 백엔드가 한다.
import { create } from 'zustand'
import type { AddonCapability, CapabilitiesResponse } from '../api/types.gen'
import { type AppId, isAddonApp, isLinkApp } from '../windows/apps'
import { linkConfigured } from '../windows/links'
import type { WidgetAddonId } from '../widgets/catalog'
import { type AddonId, MANIFESTS } from './manifests.gen'
import type { AddonManifest } from './types'

// helper_missing은 백엔드 애드온 capability가 아니라 프런트가 정하는 사유다(권한 도우미가 필요한 셸 앱).
export type UnavailableReason = NonNullable<AddonCapability['reason']> | 'helper_missing'

export interface AppAccess {
  // 바로가기와 "모든 앱"에 보일지. 관리자 전용 앱은 일반 사용자에게 숨긴다(저장한 바로가기 목록에는 남는다).
  visible: boolean
  // 창 안에 보기 대신 보여 줄 안내. null이면 창 보기를 그대로 그린다.
  notice: UnavailableReason | null
}

const OPEN: AppAccess = { visible: true, notice: null }

const HIDDEN: AppAccess = { visible: false, notice: null }

const HELPER_MISSING: AppAccess = { visible: true, notice: 'helper_missing' }

// appAccess는 capability로 앱 아이콘(바로가기, "모든 앱")과 창 내용을 어떻게 그릴지 정한다.
// 관리자 전용(admin_required)은 아이콘을 숨기고, 플러그인 없음(plugin_missing)은 아이콘을 두고
// 창 안에 안내한다. 이미 열린 창이면 두 경우 모두 창 안에 안내한다.
// 링크 앱은 주소가 설정되어 있을 때만 보인다. capability를 받기 전에는 열 주소를 모르므로
// "받기 전에는 모두 보인다"의 예외로 숨긴다. 웹 터미널도 관리자가 허용했을 때만 보이므로 같은 예외다.
// 파일 탐색기는 권한 도우미가 없으면 쓸 수 없지만, 앱이 있다는 것과 못 쓰는 이유를 알 수 있게
// 아이콘을 두고 창 안에 안내한다(helper_missing). 파일 작업 창과 미리보기 창은 탐색기가 여는 창이라 아이콘을 두지 않는다.
export function appAccess(caps: CapabilitiesResponse | null, id: AppId): AppAccess {
  if (id === 'transfers' || id === 'viewer') return HIDDEN
  if (isLinkApp(id)) return linkConfigured(id, caps) ? OPEN : HIDDEN
  if (id === 'terminal') return caps?.terminal ? OPEN : HIDDEN
  if (id === 'files') return caps && !caps.helper ? HELPER_MISSING : OPEN
  return isAddonApp(id) ? addonAccess(caps, id) : OPEN
}

// widgetAccess는 capability로 위젯을 어떻게 그릴지 정한다. 규칙은 appAccess와 같다.
// 관리자 전용은 위젯을 그리지 않고(저장 배치에는 남는다), 플러그인 없음은 위젯 안에 안내한다.
export function widgetAccess(caps: CapabilitiesResponse | null, id: WidgetAddonId): AppAccess {
  return addonAccess(caps, id)
}

function addonAccess(caps: CapabilitiesResponse | null, id: AddonId): AppAccess {
  if (!caps) return OPEN
  const c = caps.addons.find((a) => a.id === id)
  if (!c || c.available) return OPEN
  const reason = c.reason ?? 'plugin_missing'
  return { visible: reason !== 'admin_required', notice: reason }
}

// requiredPlugins는 애드온 manifest가 선언한 OMV 플러그인 패키지 이름이다. 애드온이 아닌 앱은 빈 목록.
// 안내 문구에 쓴다.
export function requiredPlugins(id: AppId | AddonId): readonly string[] {
  if (!Object.hasOwn(MANIFESTS, id)) return []
  return (MANIFESTS as Record<AddonId, AddonManifest>)[id as AddonId].requires.plugins ?? []
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
