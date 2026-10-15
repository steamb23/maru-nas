// 데스크톱에서 열 수 있는 앱 목록. "모든 앱" 화면은 이 순서대로 놓인다(창 보기가 있는 애드온, 준비 중 앱, 링크 앱).
// 바탕화면에는 사용자가 고른 바로가기만 놓인다(src/shortcuts).
import type { ParseKeys } from 'i18next'
import { type AddonId, MANIFESTS } from '../addons/manifests.gen'
import type { AddonManifest } from '../addons/types'
import type { IconName } from '../icons/resolver'
import type { Size } from './geometry'

// addon: 내장 애드온의 창 보기(addons/registry.ts). placeholder: "준비 중" 빈 창.
// link: 창 없이 브라우저 탭으로 여는 앱(shell/launchApp.ts). 창 저장소에는 들어가지 않는다.
export type AppKind = 'addon' | 'placeholder' | 'link'

interface AppBase {
  // 앱 이름의 번역 키. 창 상태에는 제목을 복사하지 않고 그릴 때 번역한다(언어를 바꾸면 바로 바뀌게).
  titleKey: ParseKeys
  icon: IconName
}

export interface WindowAppDef extends AppBase {
  kind: 'addon' | 'placeholder'
  defaultSize: Size
  // 크기 조절로 줄일 수 있는 최소 크기. 생략하면 공통 하한(geometry의 MIN_WIDTH·MIN_HEIGHT)을 쓰며,
  // 공통 하한보다 작은 값은 공통 하한으로 올라간다.
  minSize?: Size
}

export interface LinkAppDef extends AppBase {
  kind: 'link'
}

export type AppDef = WindowAppDef | LinkAppDef

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
} as const satisfies Record<string, WindowAppDef>

// 창 없이 브라우저 탭으로 여는 앱. 여는 주소와 탭은 shell/launchApp.ts의 openLink가 정한다.
const LINK_APPS = {
  // OMV 웹 UI(workbench). 주소는 백엔드 설정(-workbench-url)이다.
  workbench: {
    titleKey: 'apps.workbench',
    icon: 'openmediavault',
    kind: 'link',
  },
} as const satisfies Record<string, LinkAppDef>

// WindowAppId는 창으로 여는 앱이다. 창 저장소(store.ts)는 이 id만 받는다.
export type WindowAppId = WindowAddonId | keyof typeof PLACEHOLDER_APPS
export type LinkAppId = keyof typeof LINK_APPS
export type AppId = WindowAppId | LinkAppId

function addonApp(m: AddonManifest): WindowAppDef | null {
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
) as Record<WindowAddonId, WindowAppDef>

const WINDOW_APPS: Record<WindowAppId, WindowAppDef> = { ...ADDON_APPS, ...PLACEHOLDER_APPS }

export const APPS: Record<AppId, AppDef> = { ...WINDOW_APPS, ...LINK_APPS }

export const APP_IDS = Object.keys(APPS) as AppId[]

export function getApp(id: AppId): AppDef {
  return APPS[id]
}

// getWindowApp은 창 앱의 정의(크기 포함)다.
export function getWindowApp(id: WindowAppId): WindowAppDef {
  return WINDOW_APPS[id]
}

// isAppId는 지금 빌드에 있는 앱 id인지 확인한다. 저장된 바로가기처럼 밖에서 온 id를 거를 때 쓴다.
export function isAppId(id: string): id is AppId {
  return Object.hasOwn(APPS, id)
}

// isAddonApp은 id가 애드온의 창 보기인지 확인한다. 보기 컴포넌트는 addons/registry.ts에서 찾는다.
export function isAddonApp(id: AppId): id is WindowAddonId {
  return Object.hasOwn(ADDON_APPS, id)
}

// isLinkApp은 id가 창 없이 브라우저 탭으로 여는 앱인지 확인한다.
export function isLinkApp(id: AppId): id is LinkAppId {
  return Object.hasOwn(LINK_APPS, id)
}
