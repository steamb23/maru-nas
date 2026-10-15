import { describe, expect, it } from 'vitest'
import { DEFAULT_SETTINGS_PAGE, SETTINGS_PAGES, groupPages, resolvePage, visiblePages } from './pages'

describe('visiblePages', () => {
  it('관리자에게는 모든 페이지가 보인다', () => {
    expect(visiblePages({ admin: true }).map((p) => p.id)).toEqual(SETTINGS_PAGES.map((p) => p.id))
  })

  it('일반 사용자에게는 관리자 전용 페이지를 숨긴다', () => {
    expect(visiblePages({ admin: false }).map((p) => p.id)).toEqual(['profile', 'screen', 'general', 'about'])
  })
})

describe('groupPages', () => {
  it('사용자 카드로 여는 프로필은 목록에 넣지 않는다', () => {
    const ids = groupPages(visiblePages({ admin: true })).flatMap((g) => g.pages.map((p) => p.id))
    expect(ids).not.toContain('profile')
  })

  it('이어진 같은 그룹끼리 묶고 머리글 키를 붙인다', () => {
    const groups = groupPages(visiblePages({ admin: true }))
    expect(groups.map((g) => [g.id, g.labelKey, g.pages.length])).toEqual([
      ['user', 'settings.groups.user', 2],
      ['admin', 'settings.groups.admin', 3],
      ['info', null, 1],
    ])
  })

  it('보이는 페이지가 없는 그룹은 만들지 않는다', () => {
    expect(groupPages(visiblePages({ admin: false })).map((g) => g.id)).toEqual(['user', 'info'])
  })
})

describe('resolvePage', () => {
  it('보이는 페이지면 그 페이지다', () => {
    expect(resolvePage('about', visiblePages({ admin: false })).id).toBe('about')
  })

  it('보이지 않는 페이지를 요청하면 첫 페이지다', () => {
    expect(resolvePage('server', visiblePages({ admin: false })).id).toBe('screen')
  })

  it('기본 페이지는 누구에게나 보인다', () => {
    expect(visiblePages({ admin: false }).some((p) => p.id === DEFAULT_SETTINGS_PAGE)).toBe(true)
  })
})
