import { afterEach, describe, expect, it } from 'vitest'
import { appAccess, requiredPlugins } from '../addons/capabilities'
import type { CapabilitiesResponse } from '../api/types.gen'
import { getWidget, isWidgetAddon, widgetIds } from '../widgets/catalog'
import { appIds, getApp, getWindowApp, isAppId, isPackageApp } from '../windows/apps'
import { clearPackages, isKnownPackage, localizedText, packageTitleKey, setPackages } from './catalog'
import { testPackage } from './testPackage'

afterEach(() => clearPackages())

describe('애드온 패키지 목록', () => {
  it('지금 목록의 패키지를 앱·위젯 표에 더한다', () => {
    setPackages([
      testPackage('hello', {
        views: {
          window: { entry: 'w.html', defaultSize: { width: 500, height: 300 }, minSize: { width: 300, height: 200 } },
          widget: { entry: 'g.html', defaultSize: { cols: 2, rows: 1 }, maxSize: { cols: 3, rows: 2 } },
        },
        requires: { admin: false, plugins: ['openmediavault-nut'] },
      }),
      testPackage('only-widget', { views: { widget: { entry: 'g.html', defaultSize: { cols: 1, rows: 1 } } } }),
    ])
    expect(appIds()).toContain('hello')
    expect(appIds()).not.toContain('only-widget')
    expect(isAppId('hello')).toBe(true)
    expect(isAppId('only-widget')).toBe(false)
    expect(isPackageApp('hello')).toBe(true)
    const app = getWindowApp('hello' as never)
    expect(app).toMatchObject({ kind: 'addonpkg', titleKey: packageTitleKey('hello'), defaultSize: { width: 500, height: 300 } })
    expect(app.icon).toEqual({ url: 'api/addons/hello/icon?rev=1.0.0-0123abcd' })
    expect(widgetIds()).toEqual(expect.arrayContaining(['hello', 'only-widget']))
    expect(isWidgetAddon('only-widget')).toBe(true)
    expect(getWidget('hello' as never)).toMatchObject({ kind: 'addonpkg', minSize: { cols: 2, rows: 1 }, maxSize: { cols: 3, rows: 2 } })
    expect(requiredPlugins('hello' as never)).toEqual(['openmediavault-nut'])
  })

  it('내장 앱 id와 같은 패키지는 무시한다', () => {
    setPackages([testPackage('files')])
    expect(getApp('files').kind).toBe('shell')
    expect(appIds().filter((id) => id === 'files')).toHaveLength(1)
    expect(isPackageApp('files')).toBe(false)
  })

  it('사라진 패키지는 묘비로 남고 목록에서는 빠진다', () => {
    setPackages([testPackage('hello')])
    setPackages([])
    expect(isKnownPackage('hello')).toBe(true)
    expect(isAppId('hello')).toBe(false)
    expect(appIds()).not.toContain('hello')
    expect(getWindowApp('hello' as never).titleKey).toBe(packageTitleKey('hello'))
    // 본 적 없는 id도 그릴 정의가 있다.
    expect(getWindowApp('never-seen' as never).titleKey).toBe('addonPackages.unavailableTitle')
    clearPackages()
    expect(isKnownPackage('hello')).toBe(false)
  })

  it('capability로 패키지 앱의 표시를 정한다', () => {
    setPackages([testPackage('hello')])
    const caps = (reason?: 'admin_required' | 'plugin_missing'): CapabilitiesResponse => ({
      addons: [{ id: 'hello', available: reason === undefined, reason, sources: [] }],
      plugins: [],
      helper: true,
      terminal: false,
      addonPackages: [],
    })
    expect(appAccess(caps(), 'hello' as never)).toEqual({ visible: true, notice: null })
    expect(appAccess(caps('admin_required'), 'hello' as never).visible).toBe(false)
    expect(appAccess(caps('plugin_missing'), 'hello' as never)).toEqual({ visible: true, notice: 'plugin_missing' })
  })

  it('localizedText는 언어, en, 첫 값 순으로 고른다', () => {
    expect(localizedText({ en: 'Hi', ko: '안녕' }, 'ko')).toBe('안녕')
    expect(localizedText({ en: 'Hi', ko: '안녕' }, 'de')).toBe('Hi')
    expect(localizedText({ fr: 'Salut' }, 'de')).toBe('Salut')
    expect(localizedText(undefined, 'en')).toBe('')
  })
})
