import { type PointerEvent, type TransitionEvent, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { getApp } from '../windows/apps'
import { type WindowState, selectActiveId, useWindowStore } from '../windows/store'
import { Clock } from './Clock'
import { ProfileMenu } from './ProfileMenu'
import { TASK_ENTER_MS, TASK_LEAVE_MS, withLeaving } from './taskbarAnim'
import { type DragLayout, type Slot, dragLayout } from './taskbarDrag'
import { useContextMenuTrigger } from './useContextMenuTrigger'
import { useClosingWindows } from './useClosingWindows'
import { animationsEnabled, captureWindowLayout } from './useWindowAnimation'
import { EASING } from './windowAnim'
import { type WindowMenuActions, taskMenuItems } from './windowMenu'

const { closeMany, minimize, raiseGroup, reorder, restore, toggleMaximize } = useWindowStore.getState()

// 이 거리 이상 가로로 끌어야 드래그로 본다. 그보다 짧으면 클릭이다.
const DRAG_THRESHOLD = 4
// index.css의 작업표시줄 li 전환 시간과 맞춘다. transitionend를 놓쳤을 때의 대비용이다.
const SETTLE_MS = 200

// 작업표시줄 버튼: 활성 창은 최소화, 그 밖의 창은 복원해 맨 앞으로. 타일 그룹의 창이면 짝 창도 함께 앞으로 온다.
function onTaskClick(w: WindowState, active: boolean) {
  if (active && !w.minimized) minimize(w.id)
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
const leavingSnapshot = (w: WindowState) => w

// 버튼 사이 간격(px). 등장·퇴장 중인 버튼이 차지하는 간격도 함께 늘리고 줄여, 끝날 때 버튼이 튀지 않게 한다.
const gapOf = (el: Element | null) => (el ? parseFloat(getComputedStyle(el).columnGap) || 0 : 0)

// enterTask는 새로 그려진 버튼을 너비 0에서 제 너비로 펼친다. 같은 함수를 ref로 넘기므로
// 버튼이 처음 그려질 때만 불린다(순서가 바뀌어 DOM이 옮겨질 때는 불리지 않는다).
function enterTask(li: HTMLLIElement | null) {
  if (!li || !animationsEnabled()) return
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

export function Taskbar() {
  const { t } = useTranslation()
  const windows = useWindowStore((s) => s.windows)
  const activeId = useWindowStore(selectActiveId)
  const listRef = useRef<HTMLUListElement>(null)
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
      dragging() ? [] : taskMenuItems(useWindowStore.getState().windows, el.dataset.windowId ?? '', taskMenuActions),
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
    if (Math.abs(end.settle - layout.dx) < 0.5) {
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

  // 닫힌 창의 버튼을 지금 너비·투명도에서 접으며 사라지게 한다. 등장 도중이어도 이어 간다.
  useLayoutEffect(() => {
    for (const id of leaving.keys()) {
      if (leaveStarted.current.has(id)) continue
      const li = listRef.current?.querySelector<HTMLLIElement>(`li[data-task-id="${CSS.escape(id)}"]`)
      if (!li) {
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
      p.slots = Array.from(list.children)
        .filter((li) => !li.classList.contains('leaving'))
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

  return (
    <div className="taskbar-dock">
      <nav className="taskbar" aria-label={t('taskbar.label')}>
        <ul ref={listRef} className="taskbar-items">
          {renderIds.map((id) => {
            const ghost = leaving.get(id)
            const i = windows.findIndex((w) => w.id === id)
            if (i < 0) {
              if (!ghost) return null
              // 퇴장 중인 버튼: 클릭·포커스를 받지 않고, 창 최소화 애니메이션의 목표(data-window-id)도 아니다.
              return (
                <li key={id} className="leaving" data-task-id={id} inert>
                  <button type="button" className="task">
                    <IconImage name={getApp(ghost.appId).icon} size={20} />
                    <span className="task-title">{t(getApp(ghost.appId).titleKey)}</span>
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
            if (dragging) liClasses = view.settling ? 'dragging settling' : 'dragging'
            else if (view) liClasses = 'shifting'

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
                  <IconImage name={getApp(w.appId).icon} size={20} />
                  <span className="task-title">{t(getApp(w.appId).titleKey)}</span>
                </button>
              </li>
            )
          })}
        </ul>
        {/* 트레이: 시계, 그리고 오른쪽 끝에 고정된 프로필 메뉴 */}
        <div className="taskbar-tray">
          <Clock />
          <ProfileMenu />
        </div>
      </nav>
    </div>
  )
}
