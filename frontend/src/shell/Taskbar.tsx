import {
  type CSSProperties,
  type PointerEvent,
  type TransitionEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import { flushSync } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { useAppearanceStore } from '../appearance/store'
import { taskbarAlignOf } from '../appearance/taskbar'
import { useLanguage } from '../i18n'
import { IconImage } from '../icons/IconImage'
import type { SettingsPageId } from '../settings/pages'
import { type WindowAppId, getApp } from '../windows/apps'
import { type WindowState, type WindowStore, selectActiveId, useWindowStore } from '../windows/store'
import { windowTitle, windowTitleLabel } from '../windows/title'
import { APP_DRAWER_ID, useAppDrawerStore } from './appDrawerStore'
import { Clock } from './Clock'
import type { ContextMenuAction } from './contextMenu'
import { useContextMenuStore } from './contextMenuStore'
import { ProfileMenu } from './ProfileMenu'
import { TASK_ENTER_MS, TASK_LEAVE_MS, withLeaving } from './taskbarAnim'
import { type DragLayout, type Slot, dragLayout } from './taskbarDrag'
import { TASK_ICON_WIDTH, TASK_MAX_WIDTH, TASK_TITLE_GAP, type TaskbarFit, fitTasks, sameFit } from './taskbarFit'
import { useContextMenuTrigger } from './useContextMenuTrigger'
import { useClosingWindows } from './useClosingWindows'
import { animationsEnabled, captureWindowLayout } from './useWindowAnimation'
import { EASING } from './windowAnim'
import { type WindowMenuActions, taskMenuItems } from './windowMenu'

const { closeMany, minimize, raiseGroup, reorder, restore, toggleMaximize } = useWindowStore.getState()

// 이 거리 이상 가로로 끌어야 드래그로 본다. 그보다 짧으면 클릭이다.
const DRAG_THRESHOLD = 4
// Taskbar.css의 작업표시줄 li 전환 시간과 맞춘다. transitionend를 놓쳤을 때의 대비용이다.
const SETTLE_MS = 200
// 작업표시줄의 앱 아이콘 크기. 작업표시줄 두께(base.css의 --taskbar-height)와 함께 바꾼다.
const ICON_SIZE = 24
// "모든 앱" 버튼 아이콘 크기. 창 버튼과 구분되게 앱 아이콘보다 크게 그린다.
// 16px 격자의 symbolic 아이콘이라 2배인 32px에서 점이 정수 픽셀에 맞는다.
const LAUNCHER_ICON_SIZE = 32
// "더 보기" 메뉴와 버튼 사이 간격. 프로필 메뉴(ProfileMenu.css의 .profile-menu bottom)와 맞춘다.
const MENU_GAP = 12

// 작업표시줄 버튼: 활성 창은 최소화, 그 밖의 창은 복원해 맨 앞으로. 타일 그룹의 창이면 짝 창도 함께 앞으로 온다.
// "모든 앱" 화면이 열려 있으면 닫고 그 창을 보인다(화면에 가려져 있던 활성 창을 최소화하지 않는다).
function onTaskClick(w: WindowState, active: boolean) {
  const drawer = useAppDrawerStore.getState()
  const drawerOpen = drawer.open
  drawer.close()
  if (active && !w.minimized && !drawerOpen) minimize(w.id)
  else raiseGroup(w.id)
}

// 버튼 우클릭 메뉴의 동작. 최대화는 제목 표시줄 버튼처럼 바뀌기 전 위치에서 애니메이션한다.
const taskMenuActions: WindowMenuActions = {
  closeMany,
  minimize,
  raiseGroup,
  restore,
  toggleMaximize: (id) => {
    captureWindowLayout(id)
    toggleMaximize(id)
  },
}

// Press는 버튼을 누른 순간부터 드래그가 끝날 때까지의 포인터 정보다.
// 포인터 이벤트가 렌더링보다 먼저 이어서 들어올 수 있어, 판단은 상태가 아닌 이 값으로 한다.
interface Press {
  pointerId: number
  id: string
  from: number
  startX: number
  // press: 아직 클릭과 구별되지 않음, drag: 끄는 중, settle: 놓은 뒤 제자리로 가는 중
  phase: 'press' | 'drag' | 'settle'
  // 드래그가 시작되면(임계 거리를 넘으면) 잰 버튼 위치와 간격
  slots: Slot[]
  gap: number
  layout: DragLayout | null
  // 애니메이션이 끝나면 반영할 순서 변경
  commit: { id: string; to: number } | null
  // 누른 시점의 창 id 순서. 그 사이 창이 열리거나 닫히면 드래그를 취소하는 기준이다.
  ids: string[]
}

// DragView는 화면에 반영할 드래그 상태다.
interface DragView {
  id: string
  layout: DragLayout
  settling: boolean
}

// 닫힌 창의 버튼은 마지막 상태로 퇴장 애니메이션을 재생한다.
// overflowed: 닫힐 때 "더 보기" 메뉴로 접혀 있었는지. 그랬으면 보이지 않게 애니메이션 없이 뺀다.
interface LeavingTask {
  win: WindowState
  overflowed: boolean
}

// "더 보기" 메뉴로 접힌 버튼인지. 접힌 버튼은 자리만 빼고 보이지 않게 둔다(Taskbar.css의 li.overflowed).
const isOverflowed = (li: Element) => li.classList.contains('overflowed')

// 버튼 사이 간격(px). 등장·퇴장 중인 버튼이 차지하는 간격도 함께 늘리고 줄여, 끝날 때 버튼이 튀지 않게 한다.
const gapOf = (el: Element | null) => (el ? parseFloat(getComputedStyle(el).columnGap) || 0 : 0)

// enterTask는 새로 그려진 버튼을 너비 0에서 제 너비로 펼친다. 같은 함수를 ref로 넘기므로
// 버튼이 처음 그려질 때만 불린다(순서가 바뀌어 DOM이 옮겨질 때는 불리지 않는다).
// "더 보기" 메뉴로 접혀 숨은 채 그려진 버튼은 펼치지 않는다.
function enterTask(li: HTMLLIElement | null) {
  if (!li || isOverflowed(li) || !animationsEnabled()) return
  // 개발 모드(StrictMode)에서는 ref가 두 번 불리므로, 앞서 건 애니메이션을 지운 뒤 제 너비를 잰다.
  li.getAnimations().forEach((a) => a.cancel())
  const width = li.getBoundingClientRect().width
  const gap = gapOf(li.parentElement)
  li.animate(
    [
      { width: '0px', marginRight: `${-gap}px`, opacity: 0, overflow: 'hidden' },
      { width: `${width}px`, marginRight: '0px', opacity: 1, overflow: 'hidden' },
    ],
    { duration: TASK_ENTER_MS, easing: EASING },
  )
}

// sameIds는 창 id 순서가 같은지 비교한다.
const sameIds = (a: readonly string[], b: readonly WindowState[]) =>
  a.length === b.length && a.every((id, i) => id === b[i].id)

const widthOf = (el: Element) => el.getBoundingClientRect().width

// 재기 전(처음 그릴 때)의 배치. 줄이지 않는다.
const UNFITTED: TaskbarFit = { mode: 'labels', cap: TASK_MAX_WIDTH, truncated: [] }

// measureFit은 작업표시줄의 지금 너비로 창 버튼을 어떻게 그릴지 정한다(taskbarFit.ts).
// 잰 값은 결과(제목 너비 제한, 아이콘만, 숨김)에 따라 바뀌지 않는 것만 쓴다. 그래야 결과로 다시 그린 뒤
// 다시 재도 같은 결과가 나온다.
function measureFit(nav: HTMLElement, launcher: HTMLElement, list: HTMLElement, tray: HTMLElement): TaskbarFit {
  const navStyle = getComputedStyle(nav)
  const navGap = parseFloat(navStyle.columnGap) || 0
  const inner = nav.clientWidth - (parseFloat(navStyle.paddingLeft) || 0) - (parseFloat(navStyle.paddingRight) || 0)
  // 트레이는 가운데 정렬이면 남는 너비만큼 늘어나 있으므로 자식 너비의 합으로 잰다.
  const trayItems = Array.from(tray.children)
  const trayWidth = trayItems.reduce((sum, el) => sum + widthOf(el), 0) + gapOf(tray) * Math.max(0, trayItems.length - 1)
  // nav 자식 사이 간격: "모든 앱" | 목록 | 트레이, 가운데 정렬이면 앞쪽 빈 칸(::before, 0까지 줄어든다) 뒤에 하나 더
  const gaps = navGap * (nav.classList.contains('centered') ? 3 : 2)
  // 버튼 너비는 제목 글자 너비로 잰다. 제목이 잘리거나 아이콘만 그려도 span은 남아 있어 scrollWidth는 글자 전체 너비다.
  // 접힌 버튼도 display: none이 아니라 보이지만 않게 두는 것은 이 때문이다(display: none이면 0이 되어, 넓어진 화면에서
  // 다 들어간다고 잘못 판단하고 다시 잴 계기도 없다).
  // scrollWidth는 정수로 반올림되므로 1px 여유를 둔다(조금이라도 모자라면 말줄임이 붙는다).
  const natural = Array.from(list.children)
    .filter((li) => !li.classList.contains('leaving'))
    .map((li) => TASK_ICON_WIDTH + TASK_TITLE_GAP + (li.querySelector('.task-title')?.scrollWidth ?? 0) + 1)
  return fitTasks({
    available: inner - widthOf(launcher) - trayWidth - gaps,
    gap: gapOf(list),
    // "더 보기" 버튼은 "모든 앱" 버튼과 같은 크기다.
    moreSpace: widthOf(launcher) + navGap,
    natural,
  })
}

// inert이면 작업표시줄 전체가 입력을 받지 않는다(데스크톱 범위 모달이 열려 있을 때).
// onOpenSettings는 프로필 메뉴 머리글이 설정 창을 열 때, onNewWindow는 버튼 메뉴의 "새 창"이 부른다
// (작업표시줄은 데스크톱 크기를 모른다).
export function Taskbar({
  inert = false,
  onOpenSettings,
  onNewWindow,
}: {
  inert?: boolean
  onOpenSettings: (page: SettingsPageId) => void
  onNewWindow: (appId: WindowAppId) => void
}) {
  const { t } = useTranslation()
  const lang = useLanguage()
  const windows = useWindowStore((s) => s.windows)
  const activeId = useWindowStore(selectActiveId)
  const drawerOpen = useAppDrawerStore((s) => s.open)
  // 사용자 화면 설정의 정렬. 불러오기 전에는 기본값(가운데)이다.
  const centered = useAppearanceStore((s) => taskbarAlignOf(s.prefs.taskbarAlign) === 'center')
  // "모든 앱" 버튼을 마지막으로 누른 포인터 종류. 터치로 열면 검색 상자에 포커스하지 않는다.
  const launcherPointer = useRef('')
  const navRef = useRef<HTMLElement>(null)
  const launcherRef = useRef<HTMLButtonElement>(null)
  const listRef = useRef<HTMLUListElement>(null)
  const moreRef = useRef<HTMLButtonElement>(null)
  const trayRef = useRef<HTMLDivElement>(null)
  // 창 버튼 배치(제목 줄이기, 아이콘만, 접기). 다시 잰 결과가 같으면 그대로 둔다.
  const [fit, setFit] = useState<TaskbarFit>(UNFITTED)
  // 지금 그려진 배치에서 보이는 창 버튼 수. 창이 닫히는 순간(저장소 구독) 그 버튼이 접혀 있었는지 판단한다.
  const visibleNow = useRef(Infinity)
  useLayoutEffect(() => {
    visibleNow.current = fit.mode === 'icons' ? fit.visible : Infinity
  }, [fit])
  const leavingSnapshot = useCallback(
    (win: WindowState, prev: WindowStore): LeavingTask => ({
      win,
      overflowed: prev.windows.findIndex((w) => w.id === win.id) >= visibleNow.current,
    }),
    [],
  )
  // 닫힌 창의 버튼은 저장소에서 바로 지워지지만, 퇴장 애니메이션이 끝나 onLeft가 불릴 때까지
  // 원래 자리에 마지막 상태로 남긴다.
  const [leaving, onLeft] = useClosingWindows(leavingSnapshot)
  const [order, setOrder] = useState<string[]>([])
  const renderIds = withLeaving(
    order,
    windows.map((w) => w.id),
    leaving,
  )
  if (renderIds.length !== order.length || renderIds.some((id, i) => id !== order[i])) setOrder(renderIds)
  // 퇴장 애니메이션을 시작한 버튼 id
  const leaveStarted = useRef(new Set<string>())
  const press = useRef<Press | null>(null)
  const [view, setView] = useState<DragView | null>(null)
  // 드래그를 놓은 직후 버튼에 들어오는 click을 무시한다.
  const suppressClick = useRef(false)
  const settleTimer = useRef<number | undefined>(undefined)
  // 버튼 우클릭 메뉴. 대상 창은 입력을 받은 버튼의 data-window-id다.
  // 끌거나 놓은 버튼이 제자리로 가는 동안에는 열지 않는다(터치로 길게 누르는 사이 끌기가 시작된 경우 포함).
  const dragging = () => press.current !== null && press.current.phase !== 'press'
  const menu = useContextMenuTrigger({
    items: (el) =>
      dragging()
        ? []
        : taskMenuItems(useWindowStore.getState().windows, el.dataset.windowId ?? '', {
            ...taskMenuActions,
            newWindow: onNewWindow,
          }),
    accept: () => !dragging(),
  })

  const reset = () => {
    window.clearTimeout(settleTimer.current)
    settleTimer.current = undefined
    press.current = null
  }

  // finish는 드래그를 끝내고 순서를 반영한다. 버튼들이 이미 새 자리에 그려져 있으므로
  // 순서 변경과 transform 해제를 한 번에 그려야 버튼이 튀지 않는다.
  const finish = () => {
    const commit = press.current?.commit
    reset()
    flushSync(() => {
      if (commit) reorder(commit.id, commit.to)
      setView(null)
    })
  }

  // settle은 드래그를 놓았을 때 드래그 버튼을 도착 위치로 ease 애니메이션시키고, 끝나면 finish한다.
  // keep이 false면 순서를 바꾸지 않고 모든 버튼을 원래 자리로 돌려보낸다.
  const settle = (p: Press, keep: boolean) => {
    const layout = p.layout
    if (!layout) return
    p.phase = 'settle'
    p.commit = keep && layout.to !== p.from ? { id: p.id, to: layout.to } : null
    const end = keep ? layout : { ...layout, settle: 0, shifts: layout.shifts.map(() => 0) }
    // 이미 도착 위치에 있으면 transform이 바뀌지 않아 transitionend가 오지 않는다.
    // 애니메이션이 꺼져 있으면 CSS 전환도 꺼져 있으므로(base.css) 기다리지 않는다.
    if (Math.abs(end.settle - layout.dx) < 0.5 || !animationsEnabled()) {
      finish()
      return
    }
    setView({ id: p.id, layout: { ...end, dx: end.settle }, settling: true })
    settleTimer.current = window.setTimeout(finish, SETTLE_MS + 50)
  }

  // 드래그 중 창이 열리거나 닫히면 잰 버튼 위치가 맞지 않으므로 드래그를 취소한다.
  useEffect(() => {
    const p = press.current
    if (p && !sameIds(p.ids, windows)) {
      reset()
      setView(null)
    }
  }, [windows])

  useEffect(() => () => window.clearTimeout(settleTimer.current), [])

  // 창 버튼 배치를 잰다. 창 목록·제목(언어)·정렬이 바뀌면 그리기 직후에, 작업표시줄·트레이 크기가 바뀌면
  // (화면 크기, 시계 글자) ResizeObserver로, 글꼴을 받으면 다시 잰다.
  // ref와 setFit만 쓰므로 처음 그릴 때 만든 함수를 계속 써도 된다.
  const measure = () => {
    const nav = navRef.current
    const launcher = launcherRef.current
    const list = listRef.current
    const tray = trayRef.current
    if (!nav || !launcher || !list || !tray) return
    const next = measureFit(nav, launcher, list, tray)
    setFit((prev) => (sameFit(prev, next) ? prev : next))
  }
  const renderKey = renderIds.join(' ')
  useLayoutEffect(measure, [renderKey, lang, centered])
  useLayoutEffect(() => {
    const ro = new ResizeObserver(measure)
    for (const el of [navRef.current, launcherRef.current, ...Array.from(trayRef.current?.children ?? [])]) {
      if (el) ro.observe(el)
    }
    void document.fonts.ready.then(measure)
    return () => ro.disconnect()
  }, [])

  // 끄는 동안 배치가 바뀌면(창 크기 변화) 잰 버튼 위치가 맞지 않으므로 드래그를 취소한다.
  useEffect(() => {
    if (!press.current || press.current.phase === 'press') return
    reset()
    setView(null)
  }, [fit])

  // 닫힌 창의 버튼을 지금 너비·투명도에서 접으며 사라지게 한다. 등장 도중이어도 이어 간다.
  useLayoutEffect(() => {
    for (const id of leaving.keys()) {
      if (leaveStarted.current.has(id)) continue
      const li = listRef.current?.querySelector<HTMLLIElement>(`li[data-task-id="${CSS.escape(id)}"]`)
      // "더 보기" 메뉴로 접혀 있던 버튼은 보이지 않으므로 애니메이션 없이 뺀다.
      if (!li || isOverflowed(li)) {
        onLeft(id)
        continue
      }
      leaveStarted.current.add(id)
      const width = li.getBoundingClientRect().width
      const style = getComputedStyle(li)
      const { opacity, marginRight } = style
      const gap = gapOf(li.parentElement)
      li.getAnimations().forEach((a) => a.cancel())
      // 끝난 뒤 버튼이 빠질 때까지 다시 보이지 않도록 마지막 프레임을 유지한다.
      const a = li.animate(
        [
          { width: `${width}px`, marginRight, opacity, overflow: 'hidden' },
          { width: '0px', marginRight: `${-gap}px`, opacity: 0, overflow: 'hidden' },
        ],
        { duration: TASK_LEAVE_MS, easing: EASING, fill: 'forwards' },
      )
      a.onfinish = () => {
        leaveStarted.current.delete(id)
        onLeft(id)
      }
    }
  }, [leaving, onLeft])

  // 포인터 처리기는 메뉴 처리기(길게 누르기)를 먼저 부른다.
  const onPointerDown = (e: PointerEvent<HTMLButtonElement>, w: WindowState, index: number) => {
    menu.handlers.onPointerDown(e)
    suppressClick.current = false
    if (e.button !== 0 || press.current) return
    press.current = {
      pointerId: e.pointerId,
      id: w.id,
      from: index,
      startX: e.clientX,
      phase: 'press',
      slots: [],
      gap: 0,
      layout: null,
      commit: null,
      ids: windows.map((o) => o.id),
    }
    e.currentTarget.setPointerCapture(e.pointerId)
  }

  const onPointerMove = (e: PointerEvent<HTMLButtonElement>) => {
    menu.handlers.onPointerMove(e)
    const p = press.current
    if (!p || p.pointerId !== e.pointerId || p.phase === 'settle') return
    const dx = e.clientX - p.startX
    if (p.phase === 'press') {
      // 길게 눌러 메뉴를 연 누름은 끌기로 이어 가지 않는다.
      if (menu.openedByPress()) return
      const list = listRef.current
      if (Math.abs(dx) < DRAG_THRESHOLD || !list) return
      // 드래그를 시작할 때 한 번만 잰다. 이후로는 transform만 바뀌어 배치가 그대로다.
      // 퇴장 중인 버튼은 빼고 잰다. 남은 버튼의 순서가 windows[] 순서와 같다.
      // "더 보기" 메뉴로 접힌 버튼(뒤쪽)도 뺀다. 숨은 버튼은 끌 수 없으므로 from은 그대로 맞는다.
      p.slots = Array.from(list.children)
        .filter((li) => !li.classList.contains('leaving') && !isOverflowed(li))
        .map((li) => {
          const r = li.getBoundingClientRect()
          return { left: r.left, width: r.width }
        })
      p.gap = parseFloat(getComputedStyle(list).columnGap) || 0
      p.phase = 'drag'
    }
    p.layout = dragLayout(p.slots, p.gap, p.from, dx)
    setView({ id: p.id, layout: p.layout, settling: false })
  }

  const onPointerUp = (e: PointerEvent<HTMLButtonElement>) => {
    menu.handlers.onPointerUp(e)
    const p = press.current
    if (!p || p.pointerId !== e.pointerId) return
    if (p.phase === 'press') {
      // 임계 거리를 넘지 않았으면 그냥 클릭이다. 길게 눌러 메뉴를 연 누름은 클릭으로 보지 않는다.
      press.current = null
      if (menu.openedByPress()) suppressClick.current = true
    } else if (p.phase === 'drag') {
      suppressClick.current = true
      settle(p, true)
    }
  }

  // 포인터를 잃으면(취소 등) 순서를 바꾸지 않고 원래 자리로 돌려보낸다.
  // 놓은 뒤에도 lostpointercapture가 오는데, 그때는 이미 settle 단계라 무시된다.
  const onPointerLost = (e: PointerEvent<HTMLButtonElement>) => {
    menu.handlers.onPointerCancel(e)
    const p = press.current
    if (!p || p.pointerId !== e.pointerId) return
    if (p.phase === 'press') press.current = null
    else if (p.phase === 'drag') settle(p, false)
  }

  const onTransitionEnd = (e: TransitionEvent<HTMLLIElement>, id: string) => {
    if (e.target !== e.currentTarget || e.propertyName !== 'transform') return
    const p = press.current
    if (p?.phase === 'settle' && p.id === id) finish()
  }

  // 아이콘만으로도 넘쳐 "더 보기" 메뉴로 접힌 창. 순서는 그대로이므로 windows[]의 뒤쪽이다.
  const visibleCount = fit.mode === 'icons' ? fit.visible : windows.length
  const overflowedWindows = windows.slice(visibleCount)

  // "더 보기" 메뉴는 버튼 위로 연다. 항목을 고르면 그 창의 작업표시줄 버튼을 누른 것과 같다.
  // 버튼은 넘칠 때만 있어 트레이 바로 옆(화면 오른쪽)이므로, 오른쪽 끝을 기준점으로 주면 메뉴가 왼쪽으로
  // 뒤집혀 버튼 오른쪽 끝에 맞춰 열린다.
  const openMoreMenu = (viaKeyboard: boolean) => {
    const r = moreRef.current?.getBoundingClientRect()
    if (!r) return
    const items = overflowedWindows.map((w): ContextMenuAction => {
      const app = getApp(w.appId)
      const active = w.id === activeId
      return {
        kind: 'action',
        id: w.id,
        ...windowTitleLabel(t, w),
        icon: app.icon,
        checked: active,
        onSelect: () => onTaskClick(w, active),
      }
    })
    useContextMenuStore.getState().openAt({ x: r.right, y: r.top - MENU_GAP }, items, viaKeyboard)
  }

  return (
    <div className="taskbar-dock" inert={inert}>
      <nav ref={navRef} className={centered ? 'taskbar centered' : 'taskbar'} aria-label={t('taskbar.label')}>
        {/* "모든 앱" 버튼. 창 버튼 목록(드래그 정렬이 자식 순서를 잰다) 밖에 둔다. */}
        <button
          ref={launcherRef}
          type="button"
          className="taskbar-launcher"
          aria-label={t('drawer.title')}
          title={t('drawer.title')}
          aria-haspopup="dialog"
          aria-expanded={drawerOpen}
          aria-controls={drawerOpen ? APP_DRAWER_ID : undefined}
          onPointerDown={(e) => {
            launcherPointer.current = e.pointerType
          }}
          onClick={() => {
            const viaTouch = launcherPointer.current === 'touch'
            launcherPointer.current = ''
            useAppDrawerStore.getState().toggle(viaTouch)
          }}
        >
          <IconImage name="view-app-grid-symbolic" size={LAUNCHER_ICON_SIZE} />
        </button>
        <ul
          ref={listRef}
          className={fit.mode === 'icons' ? 'taskbar-items icons' : 'taskbar-items'}
          style={fit.mode === 'labels' ? ({ '--task-cap': `${fit.cap}px` } as CSSProperties) : undefined}
        >
          {renderIds.map((id) => {
            const ghost = leaving.get(id)
            const i = windows.findIndex((w) => w.id === id)
            if (i < 0) {
              if (!ghost) return null
              // 퇴장 중인 버튼: 클릭·포커스를 받지 않고, 창 최소화 애니메이션의 목표(data-window-id)도 아니다.
              return (
                <li key={id} className={ghost.overflowed ? 'leaving overflowed' : 'leaving'} data-task-id={id} inert>
                  <button type="button" className="task">
                    <IconImage name={getApp(ghost.win.appId).icon} size={ICON_SIZE} app />
                    <span className="task-title">{windowTitle(t, ghost.win)}</span>
                  </button>
                </li>
              )
            }
            const w = windows[i]
            const active = w.id === activeId
            const classes = ['task', active && 'active', w.minimized && 'minimized'].filter(Boolean).join(' ')
            const dragging = view?.id === w.id
            const offset = view ? (dragging ? view.layout.dx : view.layout.shifts[i]) : 0
            let liClasses: string | undefined
            if (i >= visibleCount) liClasses = 'overflowed'
            else if (dragging) liClasses = view.settling ? 'dragging settling' : 'dragging'
            else if (view) liClasses = 'shifting'
            const title = windowTitle(t, w)
            // 제목이 잘리거나 아이콘만 그리면 마우스를 올렸을 때 제목을 보인다.
            const tip = fit.mode === 'icons' || fit.truncated[i] ? title : undefined

            return (
              <li
                key={w.id}
                ref={enterTask}
                data-task-id={w.id}
                className={liClasses}
                style={offset ? { transform: `translateX(${offset}px)` } : undefined}
                onTransitionEnd={(e) => onTransitionEnd(e, w.id)}
              >
                {/* data-window-id: 창 최소화 애니메이션이 이 버튼 위치를 찾는다(useWindowAnimation.ts). */}
                <button
                  type="button"
                  className={classes}
                  aria-pressed={active}
                  title={tip}
                  data-window-id={w.id}
                  onContextMenu={menu.handlers.onContextMenu}
                  onKeyDown={menu.handlers.onKeyDown}
                  onPointerDown={(e) => onPointerDown(e, w, i)}
                  onPointerMove={onPointerMove}
                  onPointerUp={onPointerUp}
                  onPointerCancel={onPointerLost}
                  onLostPointerCapture={onPointerLost}
                  onClick={() => {
                    if (suppressClick.current) suppressClick.current = false
                    else onTaskClick(w, active)
                  }}
                >
                  <IconImage name={getApp(w.appId).icon} size={ICON_SIZE} app />
                  <span className="task-title">{title}</span>
                </button>
              </li>
            )
          })}
        </ul>
        {/* "더 보기" 버튼: 접힌 창이 있을 때만. 드래그 정렬이 목록의 자식을 재므로 목록 밖에 둔다.
            접힌 창 중에 활성 창이 있으면 창 버튼처럼 밑줄로 표시한다. */}
        {overflowedWindows.length > 0 && (
          <button
            ref={moreRef}
            type="button"
            className={overflowedWindows.some((w) => w.id === activeId) ? 'taskbar-more active' : 'taskbar-more'}
            aria-haspopup="menu"
            aria-label={t('taskbar.more')}
            title={t('taskbar.more')}
            onClick={(e) => openMoreMenu(e.detail === 0)}
          >
            <IconImage name="view-more-symbolic" size={ICON_SIZE} />
          </button>
        )}
        {/* 트레이: 시계, 그리고 오른쪽 끝에 고정된 프로필 메뉴 */}
        <div ref={trayRef} className="taskbar-tray">
          <Clock />
          <ProfileMenu onOpenSettings={onOpenSettings} />
        </div>
      </nav>
    </div>
  )
}
