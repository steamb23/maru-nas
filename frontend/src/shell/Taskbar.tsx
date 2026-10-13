import { type PointerEvent, type TransitionEvent, useEffect, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { IconImage } from '../icons/IconImage'
import { getApp } from '../windows/apps'
import { type WindowState, selectActiveId, useWindowStore } from '../windows/store'
import { type DragLayout, type Slot, dragLayout } from './taskbarDrag'

const { focus, minimize, restore, reorder } = useWindowStore.getState()

// 이 거리 이상 가로로 끌어야 드래그로 본다. 그보다 짧으면 클릭이다.
const DRAG_THRESHOLD = 4
// index.css의 작업표시줄 li 전환 시간과 맞춘다. transitionend를 놓쳤을 때의 대비용이다.
const SETTLE_MS = 200

// 작업표시줄 버튼: 최소화된 창은 복원, 활성 창은 최소화, 그 밖의 창은 맨 앞으로.
function onTaskClick(w: WindowState, active: boolean) {
  if (w.minimized) restore(w.id)
  else if (active) minimize(w.id)
  else focus(w.id)
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

// sameIds는 창 id 순서가 같은지 비교한다.
const sameIds = (a: readonly string[], b: readonly WindowState[]) =>
  a.length === b.length && a.every((id, i) => id === b[i].id)

export function Taskbar() {
  const windows = useWindowStore((s) => s.windows)
  const activeId = useWindowStore(selectActiveId)
  const listRef = useRef<HTMLUListElement>(null)
  const press = useRef<Press | null>(null)
  const [view, setView] = useState<DragView | null>(null)
  // 드래그를 놓은 직후 버튼에 들어오는 click을 무시한다.
  const suppressClick = useRef(false)
  const settleTimer = useRef<number | undefined>(undefined)

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

  const onPointerDown = (e: PointerEvent<HTMLButtonElement>, w: WindowState, index: number) => {
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
    const p = press.current
    if (!p || p.pointerId !== e.pointerId || p.phase === 'settle') return
    const dx = e.clientX - p.startX
    if (p.phase === 'press') {
      const list = listRef.current
      if (Math.abs(dx) < DRAG_THRESHOLD || !list) return
      // 드래그를 시작할 때 한 번만 잰다. 이후로는 transform만 바뀌어 배치가 그대로다.
      p.slots = Array.from(list.children, (li) => {
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
    const p = press.current
    if (!p || p.pointerId !== e.pointerId) return
    if (p.phase === 'press') {
      // 임계 거리를 넘지 않았으면 그냥 클릭이다.
      press.current = null
    } else if (p.phase === 'drag') {
      suppressClick.current = true
      settle(p, true)
    }
  }

  // 포인터를 잃으면(취소 등) 순서를 바꾸지 않고 원래 자리로 돌려보낸다.
  // 놓은 뒤에도 lostpointercapture가 오는데, 그때는 이미 settle 단계라 무시된다.
  const onPointerLost = (e: PointerEvent<HTMLButtonElement>) => {
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
      <nav className="taskbar" aria-label="작업표시줄">
        <ul ref={listRef} className="taskbar-items">
          {windows.map((w, i) => {
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
                className={liClasses}
                style={offset ? { transform: `translateX(${offset}px)` } : undefined}
                onTransitionEnd={(e) => onTransitionEnd(e, w.id)}
              >
                <button
                  type="button"
                  className={classes}
                  aria-pressed={active}
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
                  <span className="task-title">{w.title}</span>
                </button>
              </li>
            )
          })}
        </ul>
      </nav>
    </div>
  )
}
