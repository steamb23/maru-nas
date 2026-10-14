// 바탕화면에 둘 수 있는 위젯 목록(위젯 보기가 있는 애드온). DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { GridSize, WidgetPlacement } from '../api/types.gen'
import { type AddonId, MANIFESTS } from '../addons/manifests.gen'
import type { AddonManifest } from '../addons/types'

// WidgetAddonId는 위젯 보기가 있는 애드온 id다.
export type WidgetAddonId = {
  [K in AddonId]: (typeof MANIFESTS)[K]['views'] extends { widget: object } ? K : never
}[AddonId]

export interface WidgetDef {
  id: WidgetAddonId
  manifest: AddonManifest
  defaultSize: GridSize
  // 크기 조절 범위. manifest에 없으면 기본 크기다(백엔드 addons.WidgetSizeRange와 같은 규칙).
  minSize: GridSize
  maxSize: GridSize
}

const WIDGETS: readonly WidgetDef[] = Object.values(MANIFESTS as Record<AddonId, AddonManifest>).flatMap((m) => {
  const w = m.views.widget
  if (!w) return []
  const def: WidgetDef = {
    id: m.id as WidgetAddonId,
    manifest: m,
    defaultSize: { ...w.defaultSize },
    minSize: { ...(w.minSize ?? w.defaultSize) },
    maxSize: { ...(w.maxSize ?? w.defaultSize) },
  }
  return [def]
})

// WIDGET_IDS는 manifest 순서의 위젯 id 목록이다(위젯 추가 메뉴 순서).
export const WIDGET_IDS: readonly WidgetAddonId[] = WIDGETS.map((w) => w.id)

export function isWidgetAddon(id: string): id is WidgetAddonId {
  return WIDGETS.some((w) => w.id === id)
}

export function getWidget(id: WidgetAddonId): WidgetDef {
  return WIDGETS.find((w) => w.id === id)!
}

// 처음 쓰는 사용자(배치를 한 번도 저장하지 않음)의 배치. 시스템 정보 위젯을 오른쪽 위에 둔다.
const DEFAULT_WIDGETS: readonly WidgetAddonId[] = ['system-info']

export function defaultLayout(): WidgetPlacement[] {
  let top = 0
  return DEFAULT_WIDGETS.map((id) => {
    const size = getWidget(id).defaultSize
    const p = { addon: id, right: 0, top, ...size }
    top += size.rows
    return p
  })
}

// knownPlacements는 저장 배치에서 지금 빌드에 없는 위젯(지워진 애드온 등)을 뺀다.
export function knownPlacements(stored: readonly WidgetPlacement[]): WidgetPlacement[] {
  return stored.filter((p) => isWidgetAddon(p.addon))
}
