// 데스크톱에서 열 수 있는 앱 목록. "모든 앱" 화면은 이 순서대로 놓인다(창 보기가 있는 애드온, 준비 중 앱).
// 바탕화면에는 사용자가 고른 바로가기만 놓인다(src/shortcuts).
import type { ParseKeys } from 'i18next'
import { type AddonId, MANIFESTS } from '../addons/manifests.gen'
import type { AddonManifest } from '../addons/types'
import type { IconName } from '../icons/resolver'
import type { Size } from './geometry'

// addon: 내장 애드온의 창 보기(addons/registry.ts). placeholder: "준비 중" 빈 창.
export type AppKind = 'addon' | 'placeholder'

export interface AppDef {
  // 창 제목의 번역 키. 창 상태에는 제목을 복사하지 않고 그릴 때 번역한다(언어를 바꾸면 바로 바뀌게).
  titleKey: ParseKeys
  icon: IconName
  kind: AppKind
  defaultSize: Size
  // 크기 조절로 줄일 수 있는 최소 크기. 생략하면 공통 하한(geometry의 MIN_WIDTH·MIN_HEIGHT)을 쓰며,
  // 공통 하한보다 작은 값은 공통 하한으로 올라간다.
  minSize?: Size
}

// WindowAddonId는 창 보기가 있는 애드온 id다. 위젯만 있는 애드온은 앱 목록에 넣지 않는다.
export type WindowAddonId = {
  [K in AddonId]: (typeof MANIFESTS)[K]['views'] extends { window: object } ? K : never
}[AddonId]

// 아직 애드온이 없는 앱. 애드온을 만들면 여기서 지운다(같은 id를 함께 두지 않는다).
const PLACEHOLDER_APPS = {
  files: {
    titleKey: 'apps.files',
    icon: 'folder',
    kind: 'placeholder',
    defaultSize: { width: 640, height: 420 },
  },
} as const satisfies Record<string, AppDef>

export type AppId = WindowAddonId | keyof typeof PLACEHOLDER_APPS

function addonApp(m: AddonManifest): AppDef | null {
  const win = m.views.window
  if (!win) return null
  return {
    titleKey: m.title,
    icon: m.icon,
    kind: 'addon',
    defaultSize: { ...win.defaultSize },
    minSize: win.minSize && { ...win.minSize },
  }
}

const ADDON_APPS = Object.fromEntries(
  Object.values(MANIFESTS as Record<AddonId, AddonManifest>).flatMap((m) => {
    const app = addonApp(m)
    return app ? [[m.id, app]] : []
  }),
) as Record<WindowAddonId, AppDef>

export const APPS: Record<AppId, AppDef> = { ...ADDON_APPS, ...PLACEHOLDER_APPS }

export const APP_IDS = Object.keys(APPS) as AppId[]

export function getApp(id: AppId): AppDef {
  return APPS[id]
}

// isAppId는 지금 빌드에 있는 앱 id인지 확인한다. 저장된 바로가기처럼 밖에서 온 id를 거를 때 쓴다.
export function isAppId(id: string): id is AppId {
  return Object.hasOwn(APPS, id)
}

// isAddonApp은 id가 애드온의 창 보기인지 확인한다. 보기 컴포넌트는 addons/registry.ts에서 찾는다.
export function isAddonApp(id: AppId): id is WindowAddonId {
  return Object.hasOwn(ADDON_APPS, id)
}
