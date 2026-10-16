import { describe, expect, it } from 'vitest'
import type { CapabilitiesResponse } from '../api/types.gen'
import { linkConfigured, linkHref, userEditRoute, workbenchHref } from './links'

const caps = (workbenchUrl?: string): CapabilitiesResponse => ({ addons: [], plugins: [], workbenchUrl, helper: false })

describe('workbenchHref', () => {
  const base = 'https://nas.example.lan/desk/'

  it('설정이 없으면 null', () => {
    expect(workbenchHref(undefined, base)).toBeNull()
    expect(workbenchHref('', base)).toBeNull()
  })

  it('절대 주소에 첫 화면 라우트를 붙인다', () => {
    expect(workbenchHref('https://vault.example.lan/', base)).toBe('https://vault.example.lan/#/')
    expect(workbenchHref('http://192.0.2.10:8001', base)).toBe('http://192.0.2.10:8001/#/')
  })

  it('경로 끝 슬래시를 보정한다', () => {
    expect(workbenchHref('https://nas.example.lan/omv', base)).toBe('https://nas.example.lan/omv/#/')
  })

  it('경로 참조는 대시보드 문서 주소 기준으로 해석한다', () => {
    expect(workbenchHref('/', base)).toBe('https://nas.example.lan/#/')
    expect(workbenchHref('../', base)).toBe('https://nas.example.lan/#/')
    expect(workbenchHref('./omv/', base)).toBe('https://nas.example.lan/desk/omv/#/')
    expect(workbenchHref('/', 'https://nas.example.lan/desk/index.html')).toBe('https://nas.example.lan/#/')
  })

  it('http(s)가 아닌 주소는 열지 않는다', () => {
    expect(workbenchHref('javascript:alert(1)', base)).toBeNull()
  })
})

describe('linkHref·linkConfigured', () => {
  it('capability의 workbenchUrl로 정한다', () => {
    expect(linkConfigured('workbench', null)).toBe(false)
    expect(linkConfigured('workbench', caps())).toBe(false)
    expect(linkConfigured('workbench', caps('https://vault.example.lan/'))).toBe(true)
    expect(linkHref('workbench', null, 'https://nas.example.lan/')).toBeNull()
    expect(linkHref('workbench', caps('/'), 'https://nas.example.lan/desk/')).toBe('https://nas.example.lan/#/')
  })
})

describe('userEditRoute', () => {
  it('OMV 사용자 편집 라우트를 사용자 이름을 인코딩해 만든다', () => {
    const base = 'https://nas.example.lan/desk/'
    expect(workbenchHref('https://nas.example.lan/', base, userEditRoute('totp'))).toBe(
      'https://nas.example.lan/#/usermgmt/users/edit/totp',
    )
    expect(userEditRoute('a b/c')).toBe('usermgmt/users/edit/a%20b%2Fc')
  })
})
