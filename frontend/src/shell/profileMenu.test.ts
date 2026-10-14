import { describe, expect, it } from 'vitest'
import {
  type ProfileMenuItem,
  PROFILE_MENU_ITEMS,
  groupItems,
  nextIndex,
  submenuKey,
  visibleItems,
} from './profileMenu'

const items: ProfileMenuItem[] = [
  { kind: 'action', id: 'logout', labelKey: 'apps.files', icon: 'folder', group: 'g1' },
  { kind: 'action', id: 'logout', labelKey: 'apps.storage', icon: 'folder', group: 'g1', requires: 'admin' },
  { kind: 'submenu', id: 'language', labelKey: 'apps.systemInfo', icon: 'folder', group: 'g2' },
]
const keys = (list: ProfileMenuItem[]) => list.map((i) => i.labelKey)

describe('profileMenu', () => {
  it('관리자 전용 항목은 관리자에게만 보인다', () => {
    expect(keys(visibleItems(items, { username: 'u', admin: false }))).toEqual(['apps.files', 'apps.systemInfo'])
    expect(keys(visibleItems(items, { username: 'a', admin: true }))).toEqual([
      'apps.files',
      'apps.storage',
      'apps.systemInfo',
    ])
  })

  it('이어진 같은 group끼리 묶는다', () => {
    expect(groupItems(items).map(keys)).toEqual([['apps.files', 'apps.storage'], ['apps.systemInfo']])
    expect(groupItems([])).toEqual([])
  })

  it('화살표 키 이동은 양 끝에서 넘어간다', () => {
    expect(nextIndex(-1, 3, 'ArrowDown')).toBe(0)
    expect(nextIndex(2, 3, 'ArrowDown')).toBe(0)
    expect(nextIndex(-1, 3, 'ArrowUp')).toBe(2)
    expect(nextIndex(0, 3, 'ArrowUp')).toBe(2)
    expect(nextIndex(1, 3, 'End')).toBe(2)
    expect(nextIndex(1, 3, 'Home')).toBe(0)
    expect(nextIndex(1, 3, 'x')).toBe(1)
    expect(nextIndex(0, 0, 'ArrowDown')).toBe(-1)
  })

  it('좌우 화살표로 하위 메뉴를 열고, 하위 메뉴에서는 좌우 화살표·Esc로 닫는다', () => {
    expect(submenuKey('ArrowLeft', false)).toBe('open')
    expect(submenuKey('ArrowRight', false)).toBe('open')
    expect(submenuKey('Escape', false)).toBeNull()
    expect(submenuKey('ArrowDown', false)).toBeNull()
    expect(submenuKey('ArrowLeft', true)).toBe('close')
    expect(submenuKey('ArrowRight', true)).toBe('close')
    expect(submenuKey('Escape', true)).toBe('close')
    expect(submenuKey('ArrowDown', true)).toBeNull()
  })

  it('기본 메뉴에는 언어와 로그아웃이 있다', () => {
    expect(PROFILE_MENU_ITEMS.some((i) => i.id === 'logout' && i.kind === 'action' && !i.requires)).toBe(true)
    expect(PROFILE_MENU_ITEMS.some((i) => i.id === 'language' && i.kind === 'submenu' && !i.requires)).toBe(true)
  })
})
