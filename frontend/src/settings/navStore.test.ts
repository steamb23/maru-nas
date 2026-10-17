import { beforeEach, describe, expect, it } from 'vitest'
import { initialWindowData, useWindowStore } from '../windows/store'
import { initialSettingsNav, useSettingsNavStore } from './navStore'

const bounds = { width: 1280, height: 720 }

describe('settings nav store', () => {
  beforeEach(() => {
    useWindowStore.setState(initialWindowData)
    useSettingsNavStore.setState(initialSettingsNav)
  })

  it('설정 창이 열려 있는 동안 고른 페이지를 둔다', () => {
    useSettingsNavStore.getState().show('profile')
    useWindowStore.getState().open('settings', bounds)
    useWindowStore.getState().open('system-info', bounds)
    expect(useSettingsNavStore.getState().page).toBe('profile')
  })

  it('닫는 동안에는 페이지를 그대로 둔다', () => {
    // 닫힌 창이 닫기 애니메이션 동안 같은 페이지로 그려지게 한다.
    const id = useWindowStore.getState().open('settings', bounds)
    useSettingsNavStore.getState().show('about')
    useWindowStore.getState().close(id)
    expect(useSettingsNavStore.getState().page).toBe('about')
  })

  it('닫은 뒤 다시 열면 기본 페이지로 열린다', () => {
    const id = useWindowStore.getState().open('settings', bounds)
    useSettingsNavStore.getState().show('about')
    useWindowStore.getState().close(id)
    useWindowStore.getState().open('settings', bounds)
    expect(useSettingsNavStore.getState().page).toBe('screen')
  })

  it('닫은 뒤 페이지를 골라 열면 그 페이지로 열린다', () => {
    const id = useWindowStore.getState().open('settings', bounds)
    useSettingsNavStore.getState().show('about')
    useWindowStore.getState().close(id)
    useSettingsNavStore.getState().show('profile')
    useWindowStore.getState().open('settings', bounds)
    expect(useSettingsNavStore.getState().page).toBe('profile')
  })

  it('다른 창을 여닫아도 다음에 열 때 기본 페이지로 돌아간다', () => {
    const id = useWindowStore.getState().open('settings', bounds)
    useSettingsNavStore.getState().show('about')
    useWindowStore.getState().close(id)
    const other = useWindowStore.getState().open('system-info', bounds)
    useWindowStore.getState().close(other)
    useWindowStore.getState().open('settings', bounds)
    expect(useSettingsNavStore.getState().page).toBe('screen')
  })
})
