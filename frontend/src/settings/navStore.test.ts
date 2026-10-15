import { beforeEach, describe, expect, it } from 'vitest'
import { initialWindowData, useWindowStore } from '../windows/store'
import { useSettingsNavStore } from './navStore'

const bounds = { width: 1280, height: 720 }

describe('settings nav store', () => {
  beforeEach(() => {
    useWindowStore.setState(initialWindowData)
    useSettingsNavStore.getState().reset()
  })

  it('설정 창이 열려 있는 동안 고른 페이지를 둔다', () => {
    useSettingsNavStore.getState().show('profile')
    useWindowStore.getState().open('settings', bounds)
    useWindowStore.getState().open('system-info', bounds)
    expect(useSettingsNavStore.getState().page).toBe('profile')
  })

  it('설정 창이 닫히면 기본 페이지로 돌아간다', () => {
    useSettingsNavStore.getState().show('about')
    const id = useWindowStore.getState().open('settings', bounds)
    useWindowStore.getState().close(id)
    expect(useSettingsNavStore.getState().page).toBe('screen')
  })
})
