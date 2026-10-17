import { describe, expect, it } from 'vitest'
import type { ServerSettings } from '../api/types.gen'
import { overridesWith, runtimeLabelKey } from './server'

const base: ServerSettings = {
  workbenchUrl: { value: 'https://nas.example.lan/', base: 'https://nas.example.lan/', overridden: false },
  smartRefreshAt: { value: '06:00', base: '00:00', overridden: true },
  terminal: { value: 'false', base: 'false', overridden: false },
  runtime: [],
}

describe('overridesWith', () => {
  it('바꾸는 값과 이미 덮어쓴 다른 값을 함께 보낸다', () => {
    expect(overridesWith(base, 'workbenchUrl', '')).toEqual({ workbenchUrl: '', smartRefreshAt: '06:00' })
  })

  it('null이면 그 값의 덮어쓰기를 지운다', () => {
    expect(overridesWith(base, 'smartRefreshAt', null)).toEqual({})
  })

  it('터미널 허용은 "true"·"false" 문자열로 덮어쓴다', () => {
    expect(overridesWith(base, 'terminal', 'true')).toEqual({ smartRefreshAt: '06:00', terminal: 'true' })
  })
})

describe('runtimeLabelKey', () => {
  it('아는 플래그만 문구 키가 있다', () => {
    expect(runtimeLabelKey('omv-url')).toBe('settings.server.flags.omvUrl')
    expect(runtimeLabelKey('toString')).toBeNull()
  })
})
