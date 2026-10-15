import { describe, expect, it } from 'vitest'
import { MANIFESTS } from '../addons/manifests.gen'
import { APP_IDS, getApp, isAddonApp, isAppId } from './apps'
import { MIN_HEIGHT, MIN_WIDTH } from './geometry'

describe('APPS', () => {
  it.each(APP_IDS)('%s의 기본 크기는 최소 크기 이상이다', (id) => {
    const app = getApp(id)
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
      defaultSize: { width: 420, height: 360 },
      minSize: { width: 320, height: 270 },
    })
  })

  it('준비 중 앱은 애드온이 아니다', () => {
    expect(isAddonApp('files')).toBe(false)
    expect(getApp('files').kind).toBe('placeholder')
    expect(getApp('storage').kind).toBe('addon')
  })

  it('준비 중 앱과 애드온의 id가 겹치지 않는다', () => {
    const placeholders = APP_IDS.filter((id) => !isAddonApp(id))
    for (const id of placeholders) expect(Object.hasOwn(MANIFESTS, id)).toBe(false)
  })

  it('애드온 앱이 앞에, 준비 중 앱이 뒤에 놓인다', () => {
    const windowAddons = Object.values(MANIFESTS)
      .filter((m) => 'window' in m.views)
      .map((m) => m.id)
    expect(APP_IDS).toEqual([...windowAddons, 'files'])
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
