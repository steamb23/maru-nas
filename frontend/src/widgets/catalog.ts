// 바탕화면에 둘 수 있는 위젯 목록(위젯 보기가 있는 내장 애드온과 애드온 패키지). DOM에 의존하지 않아 node 환경에서 테스트한다.
import type { ParseKeys } from 'i18next'
import {
  type PackageAppId,
  currentPackages,
  isKnownPackage,
  knownPackage,
  packageTitleKey,
} from '../addonpkg/catalog'
import type { AddonPackageView, GridSize, WidgetPlacement } from '../api/types.gen'
import { type AddonId, MANIFESTS } from '../addons/manifests.gen'
import type { AddonManifest } from '../addons/types'
import type { AppIcon } from '../icons/appIcon'

// BuiltinWidgetId는 위젯 보기가 있는 내장 애드온 id다.
export type BuiltinWidgetId = {
  [K in AddonId]: (typeof MANIFESTS)[K]['views'] extends { widget: object } ? K : never
}[AddonId]

// WidgetAddonId는 위젯 id다(내장 애드온 또는 애드온 패키지).
export type WidgetAddonId = BuiltinWidgetId | PackageAppId

export interface WidgetDef {
  id: WidgetAddonId
  // kind는 위젯을 그리는 쪽이다. addon은 addons/registry.ts의 보기, addonpkg는 샌드박스 프레임이다.
  kind: 'addon' | 'addonpkg'
  titleKey: ParseKeys
  icon: AppIcon
  defaultSize: GridSize
  // 크기 조절 범위. manifest에 없으면 기본 크기다(백엔드 addons.WidgetSizeRange와 같은 규칙).
  minSize: GridSize
  maxSize: GridSize
}

const BUILTIN: readonly WidgetDef[] = Object.values(MANIFESTS as Record<AddonId, AddonManifest>).flatMap((m) => {
  const w = m.views.widget
  if (!w) return []
  const def: WidgetDef = {
    id: m.id as BuiltinWidgetId,
    kind: 'addon',
    titleKey: m.title,
    icon: m.icon,
    defaultSize: { ...w.defaultSize },
    minSize: { ...(w.minSize ?? w.defaultSize) },
    maxSize: { ...(w.maxSize ?? w.defaultSize) },
  }
  return [def]
})

function isBuiltinWidget(id: string): id is BuiltinWidgetId {
  return BUILTIN.some((w) => w.id === id)
}

// packageWidget은 애드온 패키지의 위젯 정의다. 위젯 보기가 없으면 null이다.
export function packageWidget(view: AddonPackageView): WidgetDef | null {
  const w = view.views.widget
  if (!w) return null
  return {
    id: view.id as PackageAppId,
    kind: 'addonpkg',
    titleKey: packageTitleKey(view.id) as ParseKeys,
    icon: { url: view.icon },
    defaultSize: { ...w.defaultSize },
    minSize: { ...(w.minSize ?? w.defaultSize) },
    maxSize: { ...(w.maxSize ?? w.defaultSize) },
  }
}

// WIDGET_IDS는 내장 위젯 id다(manifest 순서).
export const WIDGET_IDS: readonly BuiltinWidgetId[] = BUILTIN.map((w) => w.id as BuiltinWidgetId)

// widgetIds는 지금 둘 수 있는 위젯 id다(내장 다음에 위젯 보기가 있는 애드온 패키지, 위젯 추가 메뉴 순서).
export function widgetIds(): WidgetAddonId[] {
  const pkgs = currentPackages()
    .filter((p) => p.views.widget && !isBuiltinWidget(p.id))
    .map((p) => p.id as PackageAppId)
  return [...WIDGET_IDS, ...pkgs]
}

// isWidgetAddon은 지금 그릴 수 있는 위젯 id인지다(내장, 또는 지금 목록에 위젯 보기가 있는 애드온 패키지).
// 저장 배치의 다른 id(꺼지거나 지운 패키지)는 지우지 않고 그리지만 않는다(layout.ts).
export function isWidgetAddon(id: string): id is WidgetAddonId {
  return isBuiltinWidget(id) || currentPackages().some((p) => p.id === id && p.views.widget !== undefined)
}

// getWidget은 위젯 정의다. 사라진 애드온 패키지는 이 탭에서 마지막으로 본 정의(묘비)를 준다.
export function getWidget(id: WidgetAddonId): WidgetDef {
  const builtin = BUILTIN.find((w) => w.id === id)
  if (builtin) return builtin
  const view = isKnownPackage(id) ? knownPackage(id) : undefined
  return (
    (view && packageWidget(view)) ?? {
      id,
      kind: 'addonpkg',
      titleKey: 'addonPackages.unavailableTitle',
      icon: 'application-x-addon',
      defaultSize: { cols: 1, rows: 1 },
      minSize: { cols: 1, rows: 1 },
      maxSize: { cols: 1, rows: 1 },
    }
  )
}

// widgetSizeRange는 id 위젯의 크기 범위다. 지금 그릴 수 없는 id(꺼지거나 지운 패키지)는 null이다(맞추지 않는다).
export function widgetSizeRange(id: string): { min: GridSize; max: GridSize } | null {
  if (!isWidgetAddon(id)) return null
  const def = getWidget(id)
  return { min: def.minSize, max: def.maxSize }
}

// 처음 쓰는 사용자(배치를 한 번도 저장하지 않음)의 배치. 시스템 정보 위젯을 오른쪽 위에 둔다.
const DEFAULT_WIDGETS: readonly BuiltinWidgetId[] = ['system-info']

export function defaultLayout(): WidgetPlacement[] {
  let top = 0
  return DEFAULT_WIDGETS.map((id) => {
    const size = getWidget(id).defaultSize
    const p = { addon: id, right: 0, top, ...size }
    top += size.rows
    return p
  })
}

// WIDGET_ID는 백엔드가 받는 위젯 id 형식이다(addons.ValidateWidgets).
const WIDGET_ID = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/

// storedPlacements는 저장 배치에서 형식이 틀린 id와 중복을 뺀다. 지금 그릴 수 없는 위젯(꺼지거나 지운 애드온 패키지,
// 아직 목록을 받지 못한 패키지)은 지우지 않고 그리지만 않는다. 다시 켜면 돌아온다.
export function storedPlacements(stored: readonly WidgetPlacement[]): WidgetPlacement[] {
  const seen = new Set<string>()
  return stored.filter((p) => {
    if (p.addon.length > 64 || !WIDGET_ID.test(p.addon) || seen.has(p.addon)) return false
    seen.add(p.addon)
    return true
  })
}

// fitSizes는 지금 그릴 수 있는 위젯의 크기를 정의의 범위 안으로 맞춘다(패키지 업데이트로 범위가 줄어든 경우).
// 범위 밖 크기가 남으면 백엔드가 배치 저장을 받지 않는다. 바뀐 것이 없으면 같은 배열을 돌려준다.
export function fitSizes(list: readonly WidgetPlacement[]): readonly WidgetPlacement[] {
  let changed = false
  const out = list.map((p) => {
    const range = widgetSizeRange(p.addon)
    if (!range) return p
    const cols = Math.min(Math.max(p.cols, range.min.cols), range.max.cols)
    const rows = Math.min(Math.max(p.rows, range.min.rows), range.max.rows)
    if (cols === p.cols && rows === p.rows) return p
    changed = true
    return { ...p, cols, rows }
  })
  return changed ? out : list
}
