import type { ParseKeys } from 'i18next'
import { describe, expect, it } from 'vitest'
import { windowTitle, windowTitleLabel } from './title'

// t는 키와 치환값을 그대로 보이는 가짜 번역 함수다.
const t = (key: ParseKeys, params?: Record<string, string | number>) =>
  key === 'window.numberedTitle' && params ? `${params.title} ${params.number}` : `[${key}]`

describe('windowTitle', () => {
  it('첫 창은 앱 이름만이다', () => {
    expect(windowTitle(t, { appId: 'terminal', instance: 1 })).toBe('[apps.terminal]')
  })

  it('둘째 창부터는 앱 이름 뒤에 번호를 붙인다', () => {
    expect(windowTitle(t, { appId: 'terminal', instance: 2 })).toBe('[apps.terminal] 2')
  })
})

describe('windowTitleLabel', () => {
  it('첫 창은 앱 이름 키 그대로다', () => {
    expect(windowTitleLabel(t, { appId: 'terminal', instance: 1 })).toEqual({ labelKey: 'apps.terminal' })
  })

  it('둘째 창부터는 번호 붙은 제목 키와 번역한 앱 이름이다', () => {
    expect(windowTitleLabel(t, { appId: 'terminal', instance: 3 })).toEqual({
      labelKey: 'window.numberedTitle',
      labelParams: { title: '[apps.terminal]', number: 3 },
    })
  })
})
