import { describe, expect, it } from 'vitest'
import { type ProfileMenuItem, PROFILE_MENU_ITEMS, groupItems, nextIndex, visibleItems } from './profileMenu'

const items: ProfileMenuItem[] = [
  { id: 'logout', label: 'a', icon: 'folder', group: 'g1' },
  { id: 'logout', label: 'b', icon: 'folder', group: 'g1', requires: 'admin' },
  { id: 'logout', label: 'c', icon: 'folder', group: 'g2' },
]

describe('profileMenu', () => {
  it('관리자 전용 항목은 관리자에게만 보인다', () => {
    expect(visibleItems(items, { username: 'u', admin: false }).map((i) => i.label)).toEqual(['a', 'c'])
    expect(visibleItems(items, { username: 'a', admin: true }).map((i) => i.label)).toEqual(['a', 'b', 'c'])
  })

  it('이어진 같은 group끼리 묶는다', () => {
    expect(groupItems(items).map((g) => g.map((i) => i.label))).toEqual([['a', 'b'], ['c']])
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

  it('기본 메뉴에는 로그아웃이 있다', () => {
    expect(PROFILE_MENU_ITEMS.some((i) => i.id === 'logout' && !i.requires)).toBe(true)
  })
})
