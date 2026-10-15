// "모든 앱" 화면: 작업표시줄 왼쪽 끝 버튼으로 여는 전체 화면 앱 목록. 검색, 키보드 이동, 바로가기 추가·제거,
// 바탕화면으로 끌어 놓아 바로가기 만들기를 한다.
// 데스크톱 영역 위, 작업표시줄 아래에 그린다(작업표시줄은 열려 있는 동안에도 쓸 수 있다).
// 열림 상태는 appDrawerStore, 검색은 appSearch.ts, 격자 이동은 gridNav.ts가 계산한다.
import { type KeyboardEvent, type RefObject, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { appAccess, useCapabilitiesStore } from '../addons/capabilities'
import { SUPPORTED_LANGUAGES } from '../i18n/languages'
import { IconImage } from '../icons/IconImage'
import { placeShortcut, removeShortcut } from '../shortcuts/list'
import { useShortcutStore } from '../shortcuts/store'
import { APP_IDS, type AppId, getApp, isAppId } from '../windows/apps'
import type { Point, Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'
import {
  DRAWER_CLOSE_MS,
  DRAWER_CONTENT_CLOSE_TO,
  DRAWER_CONTENT_FROM,
  DRAWER_DRAG_FADE_MS,
  DRAWER_HIDDEN,
  DRAWER_OPEN_MS,
  DRAWER_SHOWN,
  GHOST_FADE_MS,
  GHOST_GRAB,
} from './appDrawerAnim'
import { APP_DRAWER_ID, useAppDrawerStore } from './appDrawerStore'
import { searchApps } from './appSearch'
import type { ContextMenuItem } from './contextMenu'
import { DesktopIconFace } from './DesktopIcon'
import type { DesktopShortcutsHandle } from './DesktopShortcuts'
import { gridNextIndex } from './gridNav'
import { usePointerDrag } from './usePointerDrag'
import { mergeHandlers, useContextMenuTrigger } from './useContextMenuTrigger'
import { animationsEnabled } from './useWindowAnimation'
import { CLOSE_EASING, EASING } from './windowAnim'

interface Props {
  // 데스크톱 영역(작업표시줄 제외) 크기. 창을 열 때 쓴다.
  bounds: Size
  // 바탕화면 바로가기. 타일을 바탕화면으로 끌어 놓을 때 놓을 칸을 보여 주고 넣는다.
  shortcuts: RefObject<DesktopShortcutsHandle | null>
}

// 이 거리 이상 끌어야 끌기로 본다. 그 전에 놓으면 클릭(앱 열기)이다. 바탕화면 아이콘과 같다.
const DRAG_THRESHOLD = 4

// TileDrag는 타일을 누른 순간부터 놓을 때까지의 정보다. 판단은 렌더링이 아닌 이 값으로 한다.
interface TileDrag {
  app: AppId
  // 임계 거리를 넘어 끌기가 시작됐는지
  started: boolean
  // 마지막 포인터 위치(화면 좌표)
  pointer: Point
}

const GRID_ID = `${APP_DRAWER_ID}-grid`

const { close } = useAppDrawerStore.getState()

// 바깥을 눌러도 닫지 않는 곳: 앱 타일, 검색 상자, 타일 메뉴, 작업표시줄의 "모든 앱" 버튼(누르면 닫는다)과
// 창 버튼(누르면 Taskbar가 닫고 그 창을 보인다. 여기서 먼저 닫으면 활성 창 버튼이 창을 최소화한다).
const KEEP_OPEN = '.app-drawer-tile, .app-drawer-search, .context-menu, .taskbar-launcher, .taskbar-items .task'

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

export function AppDrawer({ bounds, shortcuts }: Props) {
  const { t, i18n } = useTranslation()
  const open = useAppDrawerStore((s) => s.open)
  const viaTouch = useAppDrawerStore((s) => s.viaTouch)
  const instant = useAppDrawerStore((s) => s.instant)
  const caps = useCapabilitiesStore((s) => s.caps)
  // 닫기 애니메이션 중이면 true. 그동안 화면을 그려 두되 클릭·포커스는 받지 않는다.
  const [closing, setClosing] = useState(false)
  const [query, setQuery] = useState('')
  // 키보드로 고를 타일(로빙 tabindex). 검색 결과가 바뀌면 처음으로 돌아간다.
  const [active, setActive] = useState(0)
  const [prevOpen, setPrevOpen] = useState(open)
  // 끌고 있는 앱. 고스트를 그리고 검색 상자를 잠근다(고스트 위치는 렌더 없이 ghostRef로 옮긴다).
  const [dragging, setDragging] = useState<AppId | null>(null)
  if (open !== prevOpen) {
    setPrevOpen(open)
    setClosing(!open && !instant && animationsEnabled())
    if (!open) setDragging(null)
    if (open) {
      setQuery('')
      setActive(0)
    }
  }
  const rootRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const gridRef = useRef<HTMLUListElement>(null)
  const ghostRef = useRef<HTMLDivElement>(null)
  const tileDrag = useRef<TileDrag | null>(null)
  // 끌기·길게 누르기 뒤에 오는 click은 무시한다(앱을 열지 않는다). 다음 누름에서 풀린다.
  const suppressClick = useRef(false)

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
    if (!root || !content || !animationsEnabled() || (!open && instant)) return
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
  }, [open, instant])

  // 열리면 검색 상자에 포커스한다. 터치로 열었으면 가상 키보드가 화면을 가리지 않게 하지 않는다.
  // 지난번 끌기가 남긴 click 무시 표시도 지운다(끌어 놓은 타일은 click을 받기 전에 사라진다).
  useLayoutEffect(() => {
    if (!open) return
    suppressClick.current = false
    if (!viaTouch) searchRef.current?.focus({ preventScroll: true })
  }, [open, viaTouch])

  // endDrag는 타일 끌기를 끝내고 화면을 닫는다(endTileDrag).
  const endDrag = (dropped: boolean) => {
    tileDrag.current = null
    endTileDrag(dropped, shortcuts.current, ghostRef.current)
  }

  // 열려 있는 동안 바깥(배경, 격자의 빈 곳, 작업표시줄의 다른 요소)을 누르면 닫는다. 누른 곳의 동작은 그대로 둔다.
  // Esc로도 닫는다(끄는 중이면 끌기를 취소하며 닫는다). 타일 메뉴의 Esc는 메뉴가 처리(preventDefault)하므로
  // 메뉴만 닫힌다. 끄는 동안 브라우저 창이 포커스를 잃거나 크기가 바뀌면 끌기를 취소한다.
  useEffect(() => {
    if (!open) return
    const onPointerDown = (e: PointerEvent) => {
      const target = e.target as Element
      if (tileDrag.current?.started || target.closest(KEEP_OPEN)) return
      // 격자 스크롤 막대를 누른 것이면 닫지 않는다.
      const grid = gridRef.current
      if (target === grid && e.offsetX >= grid.clientWidth) return
      close()
    }
    const onKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key !== 'Escape' || e.defaultPrevented) return
      e.preventDefault()
      if (tileDrag.current?.started) cancelDrag()
      else close()
      focusLauncher()
    }
    const cancelDrag = () => {
      if (!tileDrag.current?.started) return
      tileDrag.current = null
      suppressClick.current = true
      endTileDrag(false, shortcuts.current, ghostRef.current)
    }
    document.addEventListener('pointerdown', onPointerDown, true)
    document.addEventListener('keydown', onKeyDown)
    window.addEventListener('blur', cancelDrag)
    window.addEventListener('resize', cancelDrag)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown, true)
      document.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('blur', cancelDrag)
      window.removeEventListener('resize', cancelDrag)
    }
  }, [open, shortcuts])

  // 끄는 중에 화면이 닫히면(모달) 바탕화면의 미리보기를 지운다.
  useEffect(() => {
    if (open || !tileDrag.current) return
    tileDrag.current = null
    shortcuts.current?.dragEnd()
  }, [open, shortcuts])

  // 로그아웃·만료로 데스크톱이 사라지면 닫는다(다음 로그인 때 닫힌 상태로 시작하게).
  // 끄는 중이었으면 바탕화면의 미리보기도 지운다.
  useEffect(
    () => () => {
      if (tileDrag.current) shortcuts.current?.dragEnd()
      tileDrag.current = null
      close(true)
    },
    [shortcuts],
  )

  // 고스트가 처음 그려지면 포인터 자리에 놓는다. 그 뒤로는 렌더 없이 끌기 처리기가 옮긴다.
  useLayoutEffect(() => {
    const p = tileDrag.current
    if (dragging && p) moveGhost(ghostRef.current, p.pointer)
  }, [dragging])

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

  // 타일 끌기. 포인터 capture는 누른 타일이 갖고, 타일은 놓을 때까지 그대로 둔다(지우거나 inert로 만들면
  // capture가 풀린다). 화면만 투명하게 만들어 뒤의 바탕화면을 보이고, 고스트가 포인터를 따라간다.
  const drag = usePointerDrag({
    threshold: DRAG_THRESHOLD,
    onStart: (e) => {
      suppressClick.current = false
      const app = e.currentTarget.dataset.app
      // 터치로는 끌지 않는다. 격자를 스크롤하려는 손가락이 끌기가 되기 때문이다(타일 메뉴의 "바탕화면에
      // 바로가기 추가"를 쓴다). 바로가기 목록을 불러오지 못했으면 끌어 놓을 수 없다. 둘 다 클릭은 그대로 된다.
      if (e.pointerType === 'touch') return false
      if (!app || !isAppId(app) || closing || useShortcutStore.getState().status !== 'ready') return false
      tileDrag.current = { app, started: false, pointer: { x: e.clientX, y: e.clientY } }
    },
    onMove: (_d, e) => {
      const p = tileDrag.current
      // 길게 눌러 메뉴를 연 누름은 끌기로 이어 가지 않는다.
      if (!p || menu.openedByPress()) return
      p.pointer = { x: e.clientX, y: e.clientY }
      if (!p.started) {
        p.started = true
        setDragging(p.app)
        fadeOut(rootRef.current)
      } else moveGhost(ghostRef.current, p.pointer)
      shortcuts.current?.dragOver(p.app, p.pointer)
    },
    onEnd: (_moved, _d, e) => {
      const p = tileDrag.current
      if (menu.openedByPress()) suppressClick.current = true
      if (!p?.started) {
        tileDrag.current = null
        return
      }
      suppressClick.current = true
      const ghost = ghostRef.current?.getBoundingClientRect()
      const pointer = { x: e.clientX, y: e.clientY }
      endDrag(!!ghost && !menu.openedByPress() && !!shortcuts.current?.dropAt(p.app, pointer, ghost))
    },
    onCancel: () => {
      if (tileDrag.current?.started) endDrag(false)
      else tileDrag.current = null
    },
  })

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
    <>
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
              aria-controls={results.length > 0 ? GRID_ID : undefined}
              autoComplete="off"
              spellCheck={false}
              readOnly={dragging !== null}
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
                      {...mergeHandlers(menu.handlers, drag)}
                      onKeyDown={(e) => onTileKeyDown(e, i)}
                      onFocus={() => setActive(i)}
                      onClick={(e) => {
                        // 끌기·길게 누르기 뒤의 click은 무시하지만 키보드(detail 0)로 누른 것은 늘 연다.
                        const suppressed = suppressClick.current
                        suppressClick.current = false
                        if (!suppressed || e.detail === 0) openApp(id)
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
      {dragging && (
        <div ref={ghostRef} className="desktop-icon app-drawer-ghost" aria-hidden="true">
          <DesktopIconFace appId={dragging} />
        </div>
      )}
    </>
  )
}

// endTileDrag는 타일 끌기가 끝난 뒤 화면을 닫는다. 화면은 끌기를 시작할 때 이미 사라졌으므로 애니메이션 없이
// 닫는다. dropped가 아니면(바탕화면 밖에 놓음, 취소) 바탕화면의 미리보기를 지우고 고스트를 흐리게 한 뒤 닫는다.
function endTileDrag(dropped: boolean, shortcuts: DesktopShortcutsHandle | null, ghost: HTMLElement | null) {
  if (dropped) {
    close(true)
    return
  }
  shortcuts?.dragEnd()
  if (!ghost || !animationsEnabled()) {
    close(true)
    return
  }
  const anim = ghost.animate([{ opacity: 1 }, { opacity: 0 }], { duration: GHOST_FADE_MS, fill: 'forwards' })
  anim.onfinish = () => close(true)
}

// moveGhost는 포인터가 고스트의 잡는 점(GHOST_GRAB)에 오도록 고스트를 옮긴다.
function moveGhost(ghost: HTMLElement | null, pointer: Point) {
  if (ghost) ghost.style.transform = `translate(${pointer.x - GHOST_GRAB.x}px, ${pointer.y - GHOST_GRAB.y}px)`
}

// fadeOut은 끌기를 시작할 때 화면을 지금 투명도에서 흐려지게 해 뒤의 바탕화면을 보인다. 끝난 뒤에도 유지한다.
function fadeOut(root: HTMLElement | null) {
  if (!root) return
  const from: Keyframe = { opacity: getComputedStyle(root).opacity }
  for (const a of root.getAnimations()) a.cancel()
  root.animate([from, DRAWER_HIDDEN], {
    duration: animationsEnabled() ? DRAWER_DRAG_FADE_MS : 0,
    easing: CLOSE_EASING,
    fill: 'forwards',
  })
}
