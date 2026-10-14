import { describe, expect, it } from 'vitest'
import type { CapabilitiesResponse } from '../api/types.gen'
import { appAccess, requiredPlugins, useCapabilitiesStore } from './capabilities'

const caps = (addons: CapabilitiesResponse['addons']): CapabilitiesResponse => ({ addons, plugins: [] })

describe('appAccess', () => {
  it('아직 받지 못했으면 모두 그린다', () => {
    expect(appAccess(null, 'system-info')).toEqual({ visible: true, notice: null })
  })

  it('쓸 수 있는 애드온은 그대로 그린다', () => {
    const c = caps([{ id: 'system-info', available: true, sources: ['proc'] }])
    expect(appAccess(c, 'system-info')).toEqual({ visible: true, notice: null })
  })

  it('관리자 전용이면 아이콘을 숨기고 창 안에 안내한다', () => {
    const c = caps([{ id: 'system-info', available: false, reason: 'admin_required', sources: [] }])
    expect(appAccess(c, 'system-info')).toEqual({ visible: false, notice: 'admin_required' })
  })

  it('플러그인이 없으면 아이콘을 두고 창 안에 안내한다', () => {
    const c = caps([{ id: 'system-info', available: false, reason: 'plugin_missing', sources: [] }])
    expect(appAccess(c, 'system-info')).toEqual({ visible: true, notice: 'plugin_missing' })
  })

  it('응답에 없는 애드온과 준비 중 앱은 그대로 그린다', () => {
    const c = caps([])
    expect(appAccess(c, 'system-info')).toEqual({ visible: true, notice: null })
    expect(appAccess(c, 'files')).toEqual({ visible: true, notice: null })
  })
})

describe('requiredPlugins', () => {
  it('manifest에 선언한 플러그인을 돌려준다', () => {
    expect(requiredPlugins('system-info')).toEqual([])
    expect(requiredPlugins('storage')).toEqual([])
  })
})

describe('useCapabilitiesStore', () => {
  it('set과 clear', () => {
    const c = caps([])
    useCapabilitiesStore.getState().set(c)
    expect(useCapabilitiesStore.getState().caps).toBe(c)
    useCapabilitiesStore.getState().clear()
    expect(useCapabilitiesStore.getState().caps).toBeNull()
  })
})
