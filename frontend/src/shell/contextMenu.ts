// 컨텍스트 메뉴(우클릭 메뉴)의 항목 모델과 위치 계산. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 바탕화면·위젯 등 우클릭 메뉴는 모두 이 모델로 항목을 만들어 contextMenuStore의 openAt으로 연다.
// 화면에는 ContextMenu 하나만 그린다(한 번에 메뉴 하나).
import type { ParseKeys } from 'i18next'
import type { IconName } from '../icons/resolver'

interface ItemBase {
  // 같은 메뉴 안에서 고유한 값. React key로 쓴다.
  id: string
  labelKey: ParseKeys
  icon?: IconName
  // 비활성 항목은 보이고 포커스도 받지만 누를 수 없다(aria-disabled).
  disabled?: boolean
}

export interface ContextMenuAction extends ItemBase {
  kind: 'action'
  // 켜져 있음 표시(체크). 있으면 항목은 menuitemcheckbox가 된다.
  checked?: boolean
  // 항목 오른쪽에 흐리게 붙이는 짧은 설명(비활성 이유 등)
  detailKey?: ParseKeys
  onSelect: () => void
}

export interface ContextMenuSeparator {
  kind: 'separator'
  id: string
}

// 하위 메뉴는 한 단계까지만 둔다(하위 메뉴 안에 하위 메뉴 없음).
export interface ContextMenuSubmenu extends ItemBase {
  kind: 'submenu'
  items: readonly (ContextMenuAction | ContextMenuSeparator)[]
}

export type ContextMenuItem = ContextMenuAction | ContextMenuSeparator | ContextMenuSubmenu

export interface Point {
  x: number
  y: number
}

export interface Size {
  width: number
  height: number
}

export interface Rect extends Point, Size {}

// 메뉴가 화면 가장자리와 두는 최소 간격
export const MENU_MARGIN = 8
// 하위 메뉴와 상위 메뉴 사이 간격, 상위 메뉴의 안쪽 여백(index.css의 .context-menu padding)
export const SUBMENU_GAP = 4
export const MENU_PADDING = 6

const clamp = (v: number, lo: number, hi: number) => Math.min(Math.max(v, lo), Math.max(lo, hi))

export interface MenuPlacement extends Point {
  // 열기 애니메이션의 기준점. 화면 끝에서 뒤집히면 메뉴가 누른 지점의 반대쪽으로 열리므로 기준점도 바뀐다.
  origin: 'left top' | 'right top' | 'left bottom' | 'right bottom'
}

// placeMenu는 at(누른 지점)에 여는 메뉴의 왼쪽 위 위치다. 오른쪽·아래로 열고, 화면 밖으로 나가면
// 그 방향을 뒤집는다. 뒤집어도 들어가지 않으면 화면 안으로 민다.
export function placeMenu(at: Point, size: Size, viewport: Size): MenuPlacement {
  const flipX = at.x + size.width > viewport.width - MENU_MARGIN && at.x - size.width >= MENU_MARGIN
  const flipY = at.y + size.height > viewport.height - MENU_MARGIN && at.y - size.height >= MENU_MARGIN
  const x = clamp(flipX ? at.x - size.width : at.x, MENU_MARGIN, viewport.width - MENU_MARGIN - size.width)
  const y = clamp(flipY ? at.y - size.height : at.y, MENU_MARGIN, viewport.height - MENU_MARGIN - size.height)
  return { x, y, origin: `${flipX ? 'right' : 'left'} ${flipY ? 'bottom' : 'top'}` }
}

export interface SubmenuPlacement extends Point {
  side: 'right' | 'left'
}

// placeSubmenu는 anchor 옆에 여는 하위 메뉴의 왼쪽 위 위치다. anchor의 가로는 상위 메뉴 테두리, 세로는 하위
// 메뉴를 여는 항목이다. 첫 항목이 그 항목과 같은 높이에 오게 오른쪽에 열고, 오른쪽에 자리가 없으면 왼쪽에 연다.
// 아래로 넘치면 위로 올린다.
export function placeSubmenu(anchor: Rect, size: Size, viewport: Size): SubmenuPlacement {
  const rightX = anchor.x + anchor.width + SUBMENU_GAP
  const leftX = anchor.x - SUBMENU_GAP - size.width
  const side = rightX + size.width <= viewport.width - MENU_MARGIN || leftX < MENU_MARGIN ? 'right' : 'left'
  const x = clamp(side === 'right' ? rightX : leftX, MENU_MARGIN, viewport.width - MENU_MARGIN - size.width)
  const y = clamp(anchor.y - MENU_PADDING, MENU_MARGIN, viewport.height - MENU_MARGIN - size.height)
  return { x, y, side }
}

// submenuKey는 하위 메뉴를 여닫는 키인지 판단한다. 보통의 데스크톱 메뉴처럼 오른쪽 화살표로 열고
// 왼쪽 화살표·Esc로 닫는다(하위 메뉴가 왼쪽으로 뒤집혀 열려도 같다). Enter·Space는 버튼 클릭으로 처리된다.
export function submenuKey(key: string, inSubmenu: boolean): 'open' | 'close' | null {
  if (inSubmenu) return key === 'ArrowLeft' || key === 'Escape' ? 'close' : null
  return key === 'ArrowRight' ? 'open' : null
}

// isContextMenuKey는 키보드로 컨텍스트 메뉴를 여는 키(메뉴 키, Shift+F10)인지 판단한다.
export function isContextMenuKey(key: string, shiftKey: boolean): boolean {
  return key === 'ContextMenu' || (shiftKey && key === 'F10')
}
