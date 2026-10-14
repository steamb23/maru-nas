import { describe, expect, it } from 'vitest'
import { MENU_MARGIN, isContextMenuKey, placeMenu, placeSubmenu, submenuKey } from './contextMenu'
import { useContextMenuStore } from './contextMenuStore'

const viewport = { width: 1000, height: 600 }
const size = { width: 200, height: 120 }

describe('placeMenu', () => {
  it('누른 지점의 오른쪽 아래로 연다', () => {
    expect(placeMenu({ x: 100, y: 50 }, size, viewport)).toEqual({ x: 100, y: 50, origin: 'left top' })
  })
  it('오른쪽·아래 끝에서는 뒤집는다', () => {
    expect(placeMenu({ x: 900, y: 550 }, size, viewport)).toEqual({ x: 700, y: 430, origin: 'right bottom' })
    expect(placeMenu({ x: 900, y: 50 }, size, viewport)).toMatchObject({ x: 700, origin: 'right top' })
  })
  it('뒤집어도 들어가지 않으면 화면 안으로 민다', () => {
    const tiny = { width: 250, height: 150 }
    expect(placeMenu({ x: 150, y: 100 }, size, tiny)).toEqual({ x: 250 - MENU_MARGIN - 200, y: 150 - MENU_MARGIN - 120, origin: 'left top' })
  })
})

describe('placeSubmenu', () => {
  const anchor = { x: 100, y: 80, width: 200, height: 32 }
  it('오른쪽에 첫 항목을 맞춰 연다', () => {
    expect(placeSubmenu(anchor, size, viewport)).toEqual({ x: 304, y: 74, side: 'right' })
  })
  it('오른쪽에 자리가 없으면 왼쪽에 연다', () => {
    expect(placeSubmenu({ ...anchor, x: 700 }, size, viewport)).toEqual({ x: 496, y: 74, side: 'left' })
  })
  it('아래로 넘치면 위로 올린다', () => {
    expect(placeSubmenu({ ...anchor, y: 560 }, size, viewport).y).toBe(600 - MENU_MARGIN - 120)
  })
})

describe('keys', () => {
  it('오른쪽 화살표로 열고 왼쪽 화살표·Esc로 닫는다', () => {
    expect(submenuKey('ArrowRight', false)).toBe('open')
    expect(submenuKey('ArrowLeft', false)).toBeNull()
    expect(submenuKey('ArrowLeft', true)).toBe('close')
    expect(submenuKey('Escape', true)).toBe('close')
    expect(submenuKey('ArrowRight', true)).toBeNull()
  })
  it('메뉴 키와 Shift+F10', () => {
    expect(isContextMenuKey('ContextMenu', false)).toBe(true)
    expect(isContextMenuKey('F10', true)).toBe(true)
    expect(isContextMenuKey('F10', false)).toBe(false)
  })
})

describe('useContextMenuStore', () => {
  const item = { kind: 'action' as const, id: 'a', labelKey: 'apps.files' as const, onSelect: () => {} }
  it('한 번에 하나만 열고, 열 때마다 새 key를 준다', () => {
    const s = useContextMenuStore.getState()
    s.openAt({ x: 1, y: 2 }, [item])
    const first = useContextMenuStore.getState().menu
    s.openAt({ x: 1, y: 2 }, [item], true)
    const second = useContextMenuStore.getState().menu
    expect(second?.key).not.toBe(first?.key)
    expect(second?.viaKeyboard).toBe(true)
    s.close()
    expect(useContextMenuStore.getState().menu).toBeNull()
  })
  it('항목이 없으면 열지 않는다', () => {
    useContextMenuStore.getState().openAt({ x: 0, y: 0 }, [])
    expect(useContextMenuStore.getState().menu).toBeNull()
  })
})
