// "모든 앱" 화면: 작업표시줄 왼쪽 끝 버튼으로 여는 전체 화면 앱 목록. 검색, 키보드 이동, 바로가기 추가·제거를 한다.
// 데스크톱 영역 위, 작업표시줄 아래에 그린다(작업표시줄은 열려 있는 동안에도 쓸 수 있다).
// 열림 상태는 appDrawerStore, 검색은 appSearch.ts, 격자 이동은 gridNav.ts가 계산한다.
import { type KeyboardEvent, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { appAccess, useCapabilitiesStore } from '../addons/capabilities'
import { SUPPORTED_LANGUAGES } from '../i18n/languages'
import { IconImage } from '../icons/IconImage'
import { placeShortcut, removeShortcut } from '../shortcuts/list'
import { useShortcutStore } from '../shortcuts/store'
import { APP_IDS, type AppId, getApp, isAppId } from '../windows/apps'
import type { Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'
import {
  DRAWER_CLOSE_MS,
  DRAWER_CONTENT_CLOSE_TO,
  DRAWER_CONTENT_FROM,
  DRAWER_HIDDEN,
  DRAWER_OPEN_MS,
  DRAWER_SHOWN,
} from './appDrawerAnim'
import { APP_DRAWER_ID, useAppDrawerStore } from './appDrawerStore'
import { searchApps } from './appSearch'
import type { ContextMenuItem } from './contextMenu'
import { gridNextIndex } from './gridNav'
import { useContextMenuTrigger } from './useContextMenuTrigger'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

interface Props {
  // 데스크톱 영역(작업표시줄 제외) 크기. 창을 열 때 쓴다.
  bounds: Size
}

const GRID_ID = `${APP_DRAWER_ID}-grid`

const { close } = useAppDrawerStore.getState()

// 바깥을 눌러도 닫지 않는 곳: 앱 타일, 검색 상자, 타일 메뉴, 작업표시줄의 "모든 앱" 버튼(누르면 닫는다).
const KEEP_OPEN = '.app-drawer-tile, .app-drawer-search, .context-menu, .taskbar-launcher'

// 저장 목록을 바꾼다. 계산은 늘 저장소의 최신 목록으로 한다.
const updateShortcuts = (change: (apps: AppId[]) => AppId[]) => {
  const s = useShortcutStore.getState()
  s.set(change(s.apps))
}

const focusLauncher = () => document.querySelector<HTMLElement>('.taskbar-launcher')?.focus()

// columnsOf는 격자 한 줄의 타일 수다. 첫 타일과 같은 높이에 놓인 타일을 센다.
function columnsOf(grid: HTMLElement | null): number {
  const items = Array.from(grid?.children ?? []) as HTMLElement[]
  if (items.length === 0) return 1
  const top = items[0].offsetTop
  return items.filter((li) => li.offsetTop === top).length
}

export function AppDrawer({ bounds }: Props) {
  const { t, i18n } = useTranslation()
  const open = useAppDrawerStore((s) => s.open)
  const viaTouch = useAppDrawerStore((s) => s.viaTouch)
  const caps = useCapabilitiesStore((s) => s.caps)
  // 닫기 애니메이션 중이면 true. 그동안 화면을 그려 두되 클릭·포커스는 받지 않는다.
  const [closing, setClosing] = useState(false)
  const [query, setQuery] = useState('')
  // 키보드로 고를 타일(로빙 tabindex). 검색 결과가 바뀌면 처음으로 돌아간다.
  const [active, setActive] = useState(0)
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    setClosing(!open && animationsEnabled())
    if (open) {
      setQuery('')
      setActive(0)
    }
  }
  const rootRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const gridRef = useRef<HTMLUListElement>(null)

  // 관리자 전용 앱은 일반 사용자에게 보이지 않는다.
  const include = (id: AppId) => appAccess(caps, id).visible

  // 검색어 후보: 지원하는 모든 언어의 앱 이름과 앱 id. 화면 언어와 무관하게 어느 언어로 쳐도 찾게 한다.
  // 모든 언어 번들이 로드되어 있어야 한다(i18n/index.ts의 resources).
  const terms = useMemo(() => {
    const fixed = SUPPORTED_LANGUAGES.map((lng) => i18n.getFixedT(lng))
    return new Map(APP_IDS.map((id) => [id, [...fixed.map((tl) => tl(getApp(id).titleKey)), id]]))
  }, [i18n])
  const results = searchApps(APP_IDS.filter(include), (id) => terms.get(id) ?? [id], query)
  const activeIndex = Math.min(active, Math.max(results.length - 1, 0))

  const openApp = (id: AppId) => {
    close()
    useWindowStore.getState().open(id, bounds)
  }

  // 열기·닫기 애니메이션. 닫는 중에 다시 열면 지금 보이는 투명도·크기에서 이어 간다.
  useLayoutEffect(() => {
    const root = rootRef.current
    const content = contentRef.current
    if (!root || !content || !animationsEnabled()) return
    const rootFrom: Keyframe = { opacity: getComputedStyle(root).opacity }
    const cs = getComputedStyle(content)
    const contentFrom: Keyframe = { opacity: cs.opacity, transform: cs.transform }
    const running = root.getAnimations().length > 0
    for (const el of [root, content]) for (const a of el.getAnimations()) a.cancel()
    if (open) {
      const options = { duration: DRAWER_OPEN_MS, easing: EASING }
      root.animate([running ? rootFrom : DRAWER_HIDDEN, { opacity: 1 }], options)
      content.animate([running ? contentFrom : DRAWER_CONTENT_FROM, DRAWER_SHOWN], options)
      return
    }
    // 끝난 뒤 화면이 지워지는 렌더링까지 원래 모습이 한 프레임 비치지 않도록 마지막 상태를 유지한다.
    const options: KeyframeAnimationOptions = { duration: DRAWER_CLOSE_MS, easing: CLOSE_EASING, fill: 'forwards' }
    root.animate([rootFrom, DRAWER_HIDDEN], options)
    const anim = content.animate([contentFrom, DRAWER_CONTENT_CLOSE_TO], options)
    anim.onfinish = () => setClosing(false)
    // 닫는 도중 다시 열면 이 애니메이션은 취소된다. 그래도 onfinish가 남아 있으면 열린 화면을 지우지 않게 뗀다.
    return () => {
      anim.onfinish = null
    }
  }, [open])

  // 열리면 검색 상자에 포커스한다. 터치로 열었으면 가상 키보드가 화면을 가리지 않게 하지 않는다.
  useLayoutEffect(() => {
    if (open && !viaTouch) searchRef.current?.focus({ preventScroll: true })
  }, [open, viaTouch])

  // 열려 있는 동안 바깥(배경, 격자의 빈 곳, 작업표시줄의 다른 요소)을 누르면 닫는다. 누른 곳의 동작은 그대로 둔다.
  // Esc로도 닫는다. 타일 메뉴의 Esc는 메뉴가 처리(preventDefault)하므로 메뉴만 닫힌다.
  useEffect(() => {
    if (!open) return
    const onPointerDown = (e: PointerEvent) => {
      const target = e.target as Element
      if (target.closest(KEEP_OPEN)) return
      // 격자 스크롤 막대를 누른 것이면 닫지 않는다.
      const grid = gridRef.current
      if (target === grid && e.offsetX >= grid.clientWidth) return
      close()
    }
    const onKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      e.preventDefault()
      close()
      focusLauncher()
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown, true)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  // 로그아웃·만료로 데스크톱이 사라지면 닫는다(다음 로그인 때 닫힌 상태로 시작하게).
  useEffect(() => close, [])

  // 타일 우클릭·메뉴 키·길게 누르기 메뉴. 대상 앱은 입력을 받은 타일의 data-app이다.
  const menu = useContextMenuTrigger({
    items: (el) => {
      const id = el.dataset.app
      if (!id || !isAppId(id)) return []
      const items: ContextMenuItem[] = [
        { kind: 'action', id: 'open', labelKey: 'shortcuts.open', icon: 'document-open-symbolic', onSelect: () => openApp(id) },
      ]
      // 바로가기 목록을 아직 불러오지 못했으면 추가·제거를 두지 않는다.
      const shortcuts = useShortcutStore.getState()
      if (shortcuts.status !== 'ready') return items
      items.push({ kind: 'separator', id: 'sep' })
      if (shortcuts.apps.includes(id)) {
        items.push({
          kind: 'action',
          id: 'remove',
          labelKey: 'shortcuts.remove',
          icon: 'list-remove-symbolic',
          onSelect: () => updateShortcuts((list) => removeShortcut(list, id)),
        })
      } else {
        items.push({
          kind: 'action',
          id: 'add',
          labelKey: 'shortcuts.addToDesktop',
          icon: 'list-add-symbolic',
          onSelect: () => updateShortcuts((list) => placeShortcut(list, id, Infinity, include)),
        })
      }
      return items
    },
  })

  // 길게 눌러 메뉴를 연 누름 뒤에 오는 click은 무시한다(앱을 열지 않는다). 다음 누름에서 풀린다.
  const suppressClick = useRef(false)

  const focusTile = (i: number) => gridRef.current?.querySelectorAll<HTMLElement>('.app-drawer-tile')[i]?.focus()

  const onSearchKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      if (results.length > 0) openApp(results[0])
    } else if (e.key === 'ArrowDown' && results.length > 0) {
      e.preventDefault()
      focusTile(activeIndex)
    } else if (e.key === 'Escape' && query !== '') {
      // 검색어가 있으면 먼저 지운다. 한 번 더 누르면 닫는다.
      e.preventDefault()
      setQuery('')
    }
  }

  const onTileKeyDown = (e: KeyboardEvent<HTMLButtonElement>, i: number) => {
    menu.handlers.onKeyDown(e)
    if (e.defaultPrevented) return
    const next = gridNextIndex(i, results.length, columnsOf(gridRef.current), e.key)
    if (next === i) return
    e.preventDefault()
    if (next < 0) searchRef.current?.focus()
    else focusTile(next)
  }

  if (!open && !closing) return null

  return (
    <div
      id={APP_DRAWER_ID}
      ref={rootRef}
      className={closing ? 'app-drawer closing' : 'app-drawer'}
      role="dialog"
      aria-label={t('drawer.title')}
      inert={closing}
    >
      <div className="app-drawer-content" ref={contentRef}>
        <label className="app-drawer-search">
          <IconImage name="edit-find-symbolic" size={16} />
          <input
            ref={searchRef}
            type="search"
            value={query}
            placeholder={t('drawer.search')}
            aria-label={t('drawer.search')}
            aria-controls={GRID_ID}
            autoComplete="off"
            spellCheck={false}
            onChange={(e) => {
              setQuery(e.target.value)
              setActive(0)
            }}
            onKeyDown={onSearchKeyDown}
          />
        </label>
        {results.length === 0 ? (
          <p className="app-drawer-empty" role="status">
            {t('drawer.empty')}
          </p>
        ) : (
          <ul id={GRID_ID} ref={gridRef} className="app-drawer-grid" aria-label={t('drawer.title')}>
            {results.map((id, i) => {
              const app = getApp(id)
              return (
                <li key={id}>
                  <button
                    type="button"
                    className="app-drawer-tile"
                    data-app={id}
                    tabIndex={i === activeIndex ? 0 : -1}
                    {...menu.handlers}
                    onPointerDown={(e) => {
                      menu.handlers.onPointerDown(e)
                      suppressClick.current = false
                    }}
                    onPointerUp={(e) => {
                      menu.handlers.onPointerUp(e)
                      if (menu.openedByPress()) suppressClick.current = true
                    }}
                    onKeyDown={(e) => onTileKeyDown(e, i)}
                    onFocus={() => setActive(i)}
                    onClick={() => {
                      if (suppressClick.current) suppressClick.current = false
                      else openApp(id)
                    }}
                  >
                    <IconImage name={app.icon} size={64} />
                    <span className="app-drawer-tile-label">{t(app.titleKey)}</span>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </div>
  )
}
