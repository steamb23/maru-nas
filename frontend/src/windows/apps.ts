// 데스크톱에서 열 수 있는 앱 목록. "모든 앱" 화면은 이 순서대로 놓인다(창 보기가 있는 애드온, 준비 중 앱,
// 셸 앱, 링크 앱).
// 바탕화면에는 사용자가 고른 바로가기만 놓인다(src/shortcuts).
import type { ParseKeys } from 'i18next'
import { type AddonId, MANIFESTS } from '../addons/manifests.gen'
import type { AddonManifest } from '../addons/types'
import type { IconName } from '../icons/resolver'
import type { Size } from './geometry'

// addon: 내장 애드온의 창 보기(addons/registry.ts). placeholder: "준비 중" 빈 창.
// shell: 애드온이 아니라 셸이 직접 그리는 창(shell/shellViews.ts, 설정 앱).
// link: 창 없이 브라우저 탭으로 여는 앱(shell/launchApp.ts). 창 저장소에는 들어가지 않는다.
export type AppKind = 'addon' | 'placeholder' | 'shell' | 'link'

interface AppBase {
  // 앱 이름의 번역 키. 창 상태에는 제목을 복사하지 않고 그릴 때 번역한다(언어를 바꾸면 바로 바뀌게).
  titleKey: ParseKeys
  icon: IconName
}

export interface WindowAppDef extends AppBase {
  kind: 'addon' | 'placeholder' | 'shell'
  defaultSize: Size
  // 크기 조절로 줄일 수 있는 최소 크기. 생략하면 공통 하한(geometry의 MIN_WIDTH·MIN_HEIGHT)을 쓰며,
  // 공통 하한보다 작은 값은 공통 하한으로 올라간다.
  minSize?: Size
  // 한 탭에서 창을 여러 개 열 수 있는지. 생략하면 앱마다 창 하나다(다시 열면 그 창을 앞으로 가져온다).
  // 켜면 앱을 열 때마다(바로가기·서랍 클릭, "열기", 작업표시줄 버튼 메뉴의 "새 창") 새 창을 연다.
  multiInstance?: boolean
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
const PLACEHOLDER_APPS = {} as const satisfies Record<string, WindowAppDef>

// 셸이 직접 그리는 창 앱. 보기 컴포넌트는 shell/shellViews.ts에 있다(이 파일은 node 테스트 대상이라 import하지 않는다).
const SHELL_APPS = {
  // 파일 탐색기. 홈 폴더와 공유 폴더를 권한 도우미가 사용자 권한으로 다룬다(src/files). 도우미가 있을 때만 보인다.
  // 두 폴더를 나란히 보도록 창을 여러 개 연다.
  files: {
    titleKey: 'apps.files',
    icon: 'folder',
    kind: 'shell',
    defaultSize: { width: 1200, height: 680 },
    minSize: { width: 400, height: 360 },
    multiInstance: true,
  },
  // 웹 터미널. 같은 호스트의 sshd에 대시보드 사용자로 접속한 셸이다(src/terminal). 관리자가 허용했을 때만 보인다.
  terminal: {
    titleKey: 'apps.terminal',
    icon: 'utilities-terminal',
    kind: 'shell',
    defaultSize: { width: 800, height: 500 },
    minSize: { width: 400, height: 240 },
    // 창마다 셸 하나다. 세션당 창 수 제한(백엔드 terminal.MaxWindows)은 다른 탭과 함께 세므로
    // 여기서 막지 않고, 넘치면 그 창 안에 안내한다(too_many_windows).
    multiInstance: true,
  },
  // 파일 작업. 파일 탐색기의 복사와 올리기를 보인다(src/files/transferStore.ts가 연다). 창 하나에 모든 작업을
  // 보이며, "모든 앱"과 바로가기에는 보이지 않는다(appAccess).
  transfers: {
    titleKey: 'apps.transfers',
    icon: 'folder-download',
    kind: 'shell',
    defaultSize: { width: 460, height: 340 },
    minSize: { width: 360, height: 220 },
  },
  // 미리보기. 파일 탐색기에서 고른 파일(이미지, 동영상, 오디오, PDF, 텍스트)을 보인다(src/viewer). 탐색기가 여는 창이라
  // "모든 앱"과 바로가기에는 보이지 않는다(appAccess). 창 하나를 다시 쓰며, 다른 파일을 열면 그 파일로 바뀐다.
  viewer: {
    titleKey: 'apps.viewer',
    icon: 'accessories-document-viewer',
    kind: 'shell',
    defaultSize: { width: 900, height: 640 },
    minSize: { width: 320, height: 240 },
  },
  // 설정. 화면(테마·바탕화면) 같은 사용자 설정과 관리자의 전역 설정, 정보 페이지를 둔다(src/settings).
  settings: {
    titleKey: 'apps.settings',
    icon: 'preferences-system',
    kind: 'shell',
    defaultSize: { width: 860, height: 580 },
    minSize: { width: 360, height: 360 },
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
export type ShellAppId = keyof typeof SHELL_APPS
export type WindowAppId = WindowAddonId | keyof typeof PLACEHOLDER_APPS | ShellAppId
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

const WINDOW_APPS: Record<WindowAppId, WindowAppDef> = { ...ADDON_APPS, ...PLACEHOLDER_APPS, ...SHELL_APPS }

export const APPS: Record<AppId, AppDef> = { ...WINDOW_APPS, ...LINK_APPS }

export const APP_IDS = Object.keys(APPS) as AppId[]

export function getApp(id: AppId): AppDef {
  return APPS[id]
}

// getWindowApp은 창 앱의 정의(크기 포함)다.
export function getWindowApp(id: WindowAppId): WindowAppDef {
  return WINDOW_APPS[id]
}

// isMultiInstance는 한 탭에서 창을 여러 개 열 수 있는 앱인지 확인한다.
export function isMultiInstance(id: AppId): id is WindowAppId {
  return Object.hasOwn(WINDOW_APPS, id) && WINDOW_APPS[id as WindowAppId].multiInstance === true
}

// isAppId는 지금 빌드에 있는 앱 id인지 확인한다. 저장된 바로가기처럼 밖에서 온 id를 거를 때 쓴다.
export function isAppId(id: string): id is AppId {
  return Object.hasOwn(APPS, id)
}

// isAddonApp은 id가 애드온의 창 보기인지 확인한다. 보기 컴포넌트는 addons/registry.ts에서 찾는다.
export function isAddonApp(id: AppId): id is WindowAddonId {
  return Object.hasOwn(ADDON_APPS, id)
}

// isShellApp은 id가 셸이 직접 그리는 창 앱인지 확인한다. 보기 컴포넌트는 shell/shellViews.ts에서 찾는다.
export function isShellApp(id: AppId): id is ShellAppId {
  return Object.hasOwn(SHELL_APPS, id)
}

// isLinkApp은 id가 창 없이 브라우저 탭으로 여는 앱인지 확인한다.
export function isLinkApp(id: AppId): id is LinkAppId {
  return Object.hasOwn(LINK_APPS, id)
}
