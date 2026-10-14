// 내장 애드온의 보기 컴포넌트 목록. 키는 manifest 목록(manifests.gen.ts)의 애드온 id와 같아야 한다.
// Record<AddonId, …>라서 애드온이 빠지거나 없는 id가 있으면 tsc 오류가 난다.
// 컴포넌트를 import하므로 node 테스트 대상 모듈(windows/apps.ts 등)에서는 import하지 않는다.
import type { AddonId } from './manifests.gen'
import { SystemInfoWindow } from './system-info/SystemInfoWindow'
import type { AddonViews } from './types'

export const ADDON_VIEWS: Record<AddonId, AddonViews> = {
  'system-info': { Window: SystemInfoWindow },
}
