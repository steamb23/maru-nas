import { describe, expect, it } from 'vitest'
import { MANIFESTS } from '../addons/manifests.gen'
import {
  APP_IDS,
  type WindowAppId,
  getApp,
  getWindowApp,
  isAddonApp,
  isAppId,
  isLinkApp,
  isMultiInstance,
  isShellApp,
} from './apps'
import { MIN_HEIGHT, MIN_WIDTH } from './geometry'

const windowAppIds = APP_IDS.filter((id): id is WindowAppId => !isLinkApp(id))

describe('APPS', () => {
  it.each(windowAppIds)('%s의 기본 크기는 최소 크기 이상이다', (id) => {
    const app = getWindowApp(id)
    expect(app.defaultSize.width).toBeGreaterThanOrEqual(Math.max(app.minSize?.width ?? 0, MIN_WIDTH))
    expect(app.defaultSize.height).toBeGreaterThanOrEqual(Math.max(app.minSize?.height ?? 0, MIN_HEIGHT))
  })
})

describe('애드온 앱', () => {
  it('창 보기가 있는 애드온은 manifest의 크기로 앱이 된다', () => {
    expect(isAddonApp('system-info')).toBe(true)
    expect(getApp('system-info')).toEqual({
      titleKey: 'apps.systemInfo',
      icon: 'utilities-system-monitor',
      kind: 'addon',
      defaultSize: { width: 960, height: 600 },
      minSize: { width: 440, height: 360 },
    })
  })

  it('파일 탐색기는 셸 앱이다', () => {
    expect(isAddonApp('files')).toBe(false)
    expect(getApp('files').kind).toBe('shell')
    expect(getApp('storage').kind).toBe('addon')
  })

  it('셸 앱·링크 앱과 애드온의 id가 겹치지 않는다', () => {
    const others = APP_IDS.filter((id) => !isAddonApp(id))
    for (const id of others) expect(Object.hasOwn(MANIFESTS, id)).toBe(false)
  })

  it('애드온 앱, 준비 중 앱, 셸 앱, 링크 앱 순서로 놓인다', () => {
    const windowAddons = Object.values(MANIFESTS)
      .filter((m) => 'window' in m.views)
      .map((m) => m.id)
    expect(APP_IDS).toEqual([...windowAddons, 'files', 'terminal', 'transfers', 'viewer', 'settings', 'workbench'])
  })
})

describe('셸 앱', () => {
  it('설정은 셸이 그리는 창 앱이다', () => {
    expect(isShellApp('settings')).toBe(true)
    expect(isAddonApp('settings')).toBe(false)
    expect(isLinkApp('settings')).toBe(false)
    expect(getApp('settings').kind).toBe('shell')
    expect(isShellApp('files')).toBe(true)
    expect(isShellApp('workbench')).toBe(false)
  })
})

describe('링크 앱', () => {
  it('OMV 워크벤치는 창 없이 여는 링크 앱이다', () => {
    expect(isLinkApp('workbench')).toBe(true)
    expect(isAddonApp('workbench')).toBe(false)
    expect(getApp('workbench')).toEqual({ titleKey: 'apps.workbench', icon: 'openmediavault', kind: 'link' })
    expect(isLinkApp('files')).toBe(false)
    expect(isLinkApp('system-info')).toBe(false)
  })
})

describe('isAppId', () => {
  it('앱 목록에 있는 id만 받는다', () => {
    for (const id of APP_IDS) expect(isAppId(id)).toBe(true)
    expect(isAppId('removed-app')).toBe(false)
    expect(isAppId('toString')).toBe(false)
    expect(isAppId('')).toBe(false)
  })
})

describe('isMultiInstance', () => {
  it('창을 여러 개 여는 앱은 파일 탐색기와 웹 터미널이다', () => {
    expect(APP_IDS.filter(isMultiInstance)).toEqual(['files', 'terminal'])
  })
})
