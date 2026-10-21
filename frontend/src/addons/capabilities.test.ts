import { describe, expect, it } from 'vitest'
import type { CapabilitiesResponse } from '../api/types.gen'
import { appAccess, requiredPlugins, useCapabilitiesStore } from './capabilities'

const caps = (addons: CapabilitiesResponse['addons']): CapabilitiesResponse => ({ addons, plugins: [], helper: false, terminal: false })

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

  it('응답에 없는 애드온은 그대로 그린다', () => {
    const c = caps([])
    expect(appAccess(c, 'system-info')).toEqual({ visible: true, notice: null })
  })

  it('파일 탐색기는 권한 도우미가 있을 때만 보인다(받기 전에는 보인다)', () => {
    expect(appAccess(null, 'files')).toEqual({ visible: true, notice: null })
    expect(appAccess(caps([]), 'files')).toEqual({ visible: true, notice: 'helper_missing' })
    expect(appAccess({ ...caps([]), helper: true }, 'files')).toEqual({ visible: true, notice: null })
    // 파일 작업 창은 탐색기가 여는 창이라 아이콘을 두지 않는다.
    expect(appAccess(null, 'transfers')).toEqual({ visible: false, notice: null })
  })

  it('웹 터미널은 관리자가 허용했을 때만 보인다(받기 전에는 숨긴다)', () => {
    expect(appAccess(null, 'terminal')).toEqual({ visible: false, notice: null })
    expect(appAccess(caps([]), 'terminal')).toEqual({ visible: false, notice: null })
    expect(appAccess({ ...caps([]), terminal: true }, 'terminal')).toEqual({ visible: true, notice: null })
  })

  it('링크 앱은 주소가 설정되어 있을 때만 보인다(받기 전에는 숨긴다)', () => {
    expect(appAccess(null, 'workbench')).toEqual({ visible: false, notice: null })
    expect(appAccess(caps([]), 'workbench')).toEqual({ visible: false, notice: null })
    const c: CapabilitiesResponse = { ...caps([]), workbenchUrl: 'https://vault.example.lan/' }
    expect(appAccess(c, 'workbench')).toEqual({ visible: true, notice: null })
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
