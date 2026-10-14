// 화면의 컨텍스트 메뉴. contextMenuStore의 메뉴를 그린다. Desktop이 하나만 그린다.
// 항목 모델과 위치 계산은 contextMenu.ts, 여는 쪽은 useContextMenuTrigger.ts를 쓴다.
import { type KeyboardEvent, useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import {
  type ContextMenuAction,
  type ContextMenuItem,
  type ContextMenuSubmenu,
  placeMenu,
  placeSubmenu,
  submenuKey,
} from './contextMenu'
import { type OpenMenu, useContextMenuStore } from './contextMenuStore'
import { MENU_CLOSE_MS, MENU_CLOSE_TO, MENU_OPEN_FROM, MENU_OPEN_MS, MENU_SHOWN } from './menuAnim'
import { moveFocus } from './menuFocus'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

const { close } = useContextMenuStore.getState()

const ITEM_SELECTOR = '[role^="menuitem"]'

const viewport = () => ({ width: window.innerWidth, height: window.innerHeight })

export function ContextMenu() {
  const menu = useContextMenuStore((s) => s.menu)
  // 닫는 애니메이션 동안 마지막 메뉴를 계속 그린다.
  const [shown, setShown] = useState<OpenMenu | null>(menu)
  const [prev, setPrev] = useState(menu)
  if (menu !== prev) {
    setPrev(menu)
    if (menu) setShown(menu)
    else if (!animationsEnabled()) setShown(null)
  }

  const onClosed = useCallback(() => setShown(null), [])

  // 로그아웃 등으로 데스크톱이 사라지면 열린 메뉴도 닫는다(다음 로그인에 남지 않게).
  useEffect(() => close, [])

  if (!shown) return null
  return <MenuRoot key={shown.key} menu={shown} closing={menu === null} onClosed={onClosed} />
}

interface RootProps {
  menu: OpenMenu
  // 닫는 애니메이션 중이면 true. 클릭·포커스를 받지 않는다.
  closing: boolean
  onClosed: () => void
}

function MenuRoot({ menu, closing, onClosed }: RootProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  // 메뉴를 열기 전에 포커스가 있던 요소. Esc로 닫으면 그리로 돌려보낸다.
  const returnFocus = useRef<Element | null>(null)
  // 열린 하위 메뉴(상위 항목 id)와, 열 때 첫 항목에 포커스할지(키보드로 열었는지)
  const [sub, setSub] = useState<{ id: string; focus: boolean } | null>(null)

  // 처음 그릴 때 크기를 재서 화면 안에 놓고 열기 애니메이션을 재생한다.
  useLayoutEffect(() => {
    const el = rootRef.current
    if (!el) return
    returnFocus.current = document.activeElement
    const place = placeMenu(menu.at, { width: el.offsetWidth, height: el.offsetHeight }, viewport())
    Object.assign(el.style, { left: `${place.x}px`, top: `${place.y}px`, transformOrigin: place.origin })
    el.style.visibility = ''
    if (menu.viaKeyboard) el.querySelector<HTMLElement>(ITEM_SELECTOR)?.focus()
    else el.focus()
    if (animationsEnabled()) el.animate([MENU_OPEN_FROM, MENU_SHOWN], { duration: MENU_OPEN_MS, easing: EASING })
  }, [menu])

  useLayoutEffect(() => {
    const el = rootRef.current
    if (!closing || !el) return
    setSub(null)
    const style = getComputedStyle(el)
    el.getAnimations().forEach((a) => a.cancel())
    const anim = el.animate([{ opacity: style.opacity, transform: style.transform }, MENU_CLOSE_TO], {
      duration: MENU_CLOSE_MS,
      easing: CLOSE_EASING,
      fill: 'forwards',
    })
    anim.onfinish = onClosed
  }, [closing, onClosed])

  // 메뉴 밖을 누르거나, 브라우저 창 크기가 바뀌거나 포커스를 잃으면 닫는다. 누른 곳의 동작은 그대로 둔다.
  useEffect(() => {
    if (closing) return
    const onPointerDown = (e: PointerEvent) => {
      const target = e.target as Node
      if (!rootRef.current?.contains(target) && !document.querySelector('.context-menu.submenu')?.contains(target)) close()
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    window.addEventListener('resize', close)
    window.addEventListener('blur', close)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown, true)
      window.removeEventListener('resize', close)
      window.removeEventListener('blur', close)
    }
  }, [closing])

  const closeAndRestore = () => {
    close()
    const el = returnFocus.current
    if (el instanceof HTMLElement && el.isConnected) el.focus()
  }

  const select = (item: ContextMenuAction) => {
    if (item.disabled) return
    close()
    item.onSelect()
  }

  const openSub = (item: ContextMenuSubmenu, focus: boolean) => {
    if (item.disabled) return
    setSub({ id: item.id, focus })
  }

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      closeAndRestore()
      return
    }
    if (e.key === 'Tab') {
      e.preventDefault()
      close()
      return
    }
    const id = (document.activeElement as HTMLElement | null)?.dataset.submenu
    const item = menu.items.find((i): i is ContextMenuSubmenu => i.kind === 'submenu' && i.id === id)
    if (item && submenuKey(e.key, false) === 'open') {
      e.preventDefault()
      openSub(item, true)
      return
    }
    // 다른 항목으로 옮기면 하위 메뉴를 닫는다.
    if (moveFocus(rootRef.current, ITEM_SELECTOR, e)) setSub(null)
  }

  const openItem = sub && menu.items.find((i): i is ContextMenuSubmenu => i.kind === 'submenu' && i.id === sub.id)
  const subId = openItem ? openItem.id : null
  // 하위 메뉴를 여는 상위 항목. 하위 메뉴가 그려진 뒤 위치를 잴 때 찾는다.
  const findAnchor = useCallback(
    () => (subId ? (rootRef.current?.querySelector<HTMLElement>(`[data-submenu="${subId}"]`) ?? null) : null),
    [subId],
  )

  return (
    <>
      <div
        ref={rootRef}
        className={closing ? 'context-menu closing' : 'context-menu'}
        role="menu"
        tabIndex={-1}
        inert={closing}
        // 크기를 재서 자리를 정하기 전에는 보이지 않게 둔다.
        style={{ left: menu.at.x, top: menu.at.y, visibility: 'hidden' }}
        onKeyDown={onKeyDown}
      >
        <MenuItems
          items={menu.items}
          openSub={sub?.id ?? null}
          onSelect={select}
          onSubmenu={openSub}
          onHover={(item) => {
            if (item.kind === 'submenu') openSub(item, false)
            else setSub(null)
          }}
        />
      </div>
      {openItem && !closing && (
        <Submenu
          key={openItem.id}
          item={openItem}
          findAnchor={findAnchor}
          focusFirst={sub.focus}
          onSelect={select}
          onBack={() => {
            findAnchor()?.focus()
            setSub(null)
          }}
          onEscapeAll={() => close()}
        />
      )}
    </>
  )
}

interface SubmenuProps {
  item: ContextMenuSubmenu
  findAnchor: () => HTMLElement | null
  focusFirst: boolean
  onSelect: (item: ContextMenuAction) => void
  // 왼쪽 화살표·Esc: 하위 메뉴만 닫고 상위 항목으로 돌아간다.
  onBack: () => void
  // Tab: 메뉴 전체를 닫는다.
  onEscapeAll: () => void
}

function Submenu({ item, findAnchor, focusFirst, onSelect, onBack, onEscapeAll }: SubmenuProps) {
  const { t } = useTranslation()
  const ref = useRef<HTMLDivElement>(null)

  useLayoutEffect(() => {
    const el = ref.current
    const anchor = findAnchor()
    if (!el || !anchor) return
    // 가로는 상위 메뉴 테두리, 세로는 상위 항목에 맞춘다.
    const a = anchor.getBoundingClientRect()
    const panel = anchor.closest('.context-menu')?.getBoundingClientRect() ?? a
    const place = placeSubmenu(
      { x: panel.x, y: a.y, width: panel.width, height: a.height },
      { width: el.offsetWidth, height: el.offsetHeight },
      viewport(),
    )
    Object.assign(el.style, {
      left: `${place.x}px`,
      top: `${place.y}px`,
      transformOrigin: place.side === 'right' ? 'left top' : 'right top',
    })
    el.style.visibility = ''
    if (focusFirst) el.querySelector<HTMLElement>(ITEM_SELECTOR)?.focus()
    if (animationsEnabled()) el.animate([MENU_OPEN_FROM, MENU_SHOWN], { duration: MENU_OPEN_MS, easing: EASING })
  }, [findAnchor, focusFirst])

  // 하위 메뉴의 키 입력은 상위 메뉴까지 올리지 않는다(DOM에서 형제라 원래 올라가지 않는다).
  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Tab') {
      e.preventDefault()
      onEscapeAll()
      return
    }
    if (submenuKey(e.key, true) === 'close') {
      e.preventDefault()
      onBack()
      return
    }
    moveFocus(ref.current, ITEM_SELECTOR, e)
  }

  return (
    <div
      ref={ref}
      className="context-menu submenu"
      role="menu"
      aria-label={t(item.labelKey)}
      style={{ visibility: 'hidden' }}
      onKeyDown={onKeyDown}
    >
      <MenuItems items={item.items} openSub={null} onSelect={onSelect} />
    </div>
  )
}

interface ItemsProps {
  items: readonly ContextMenuItem[]
  openSub: string | null
  onSelect: (item: ContextMenuAction) => void
  onSubmenu?: (item: ContextMenuSubmenu, focus: boolean) => void
  // 마우스를 올린 항목. 하위 메뉴 항목이면 하위 메뉴를 열고, 아니면 닫는다.
  onHover?: (item: ContextMenuAction | ContextMenuSubmenu) => void
}

function MenuItems({ items, openSub, onSelect, onSubmenu, onHover }: ItemsProps) {
  const { t } = useTranslation()
  return items.map((item) => {
    if (item.kind === 'separator') return <div key={item.id} className="context-menu-separator" role="separator" />
    const hover = onHover && (() => onHover(item))
    if (item.kind === 'submenu') {
      return (
        <button
          key={item.id}
          type="button"
          role="menuitem"
          className="context-menu-item"
          data-submenu={item.id}
          aria-haspopup="menu"
          aria-expanded={openSub === item.id}
          aria-disabled={item.disabled || undefined}
          onPointerEnter={hover}
          onClick={() => onSubmenu?.(item, false)}
        >
          <span className="context-menu-icon">{item.icon && <IconImage name={item.icon} size={16} />}</span>
          <span className="context-menu-label">{t(item.labelKey)}</span>
          <IconImage name="pan-end-symbolic" size={16} className="context-menu-chevron" />
        </button>
      )
    }
    const checkable = item.checked !== undefined
    return (
      <button
        key={item.id}
        type="button"
        role={checkable ? 'menuitemcheckbox' : 'menuitem'}
        aria-checked={checkable ? item.checked : undefined}
        aria-disabled={item.disabled || undefined}
        className="context-menu-item"
        onPointerEnter={hover}
        onClick={() => onSelect(item)}
      >
        <span className="context-menu-icon">
          {item.checked ? (
            <IconImage name="object-select-symbolic" size={16} />
          ) : (
            item.icon && <IconImage name={item.icon} size={16} />
          )}
        </span>
        <span className="context-menu-label">{t(item.labelKey)}</span>
        {item.detailKey && <span className="context-menu-detail">{t(item.detailKey)}</span>}
      </button>
    )
  })
}
