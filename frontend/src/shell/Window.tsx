import { type PointerEvent, useRef } from 'react'
import { IconImage } from '../icons/IconImage'
import { SystemInfoWindow } from '../sysinfo/SystemInfoWindow'
import { getApp } from '../windows/apps'
import { type Point, type Rect, type Size, restoreGrab } from '../windows/geometry'
import { selectActiveId, useWindowStore } from '../windows/store'
import { PlaceholderWindow } from './PlaceholderWindow'
import { ResizeHandles } from './ResizeHandles'
import { useWindowAnimation } from './useWindowAnimation'

interface Props {
  id: string
  bounds: Size
}

interface Drag {
  pointerId: number
  // 포인터와 창 왼쪽 위 모서리 사이의 거리
  dx: number
  dy: number
  // 최대화된 창을 누른 경우 누른 지점과 최대화 창의 위치·크기. 임계 거리를 넘게 끌면
  // 창을 복원하고 일반 드래그로 이어 간다. 그 전에는 클릭·더블클릭이다.
  restore?: { grab: Point; maxRect: Rect }
}

// 최대화된 창을 이 거리 이상 끌어야 복원한다.
const RESTORE_THRESHOLD = 4

const { focus, close, minimize, toggleMaximize, unmaximizeAt, moveTo } = useWindowStore.getState()

// 버튼 위에서 시작한 포인터 동작은 드래그·더블클릭으로 다루지 않는다.
const fromButton = (e: { target: EventTarget }) => e.target instanceof Element && e.target.closest('button') !== null

export function Window({ id, bounds }: Props) {
  const win = useWindowStore((s) => s.windows.find((w) => w.id === id))
  const active = useWindowStore((s) => selectActiveId(s) === id)
  const drag = useRef<Drag | null>(null)
  const sectionRef = useRef<HTMLElement>(null)
  const anim = useWindowAnimation(sectionRef, id, win?.minimized ?? false, win?.maximized ?? false)

  if (!win) return null
  const app = getApp(win.appId)

  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0 || fromButton(e)) return
    if (win.maximized) {
      // 최대화된 창의 위치는 CSS가 정하므로 화면에 그려진 위치를 잰다. 좌표는 데스크톱 영역 기준이다.
      const section = e.currentTarget.parentElement
      if (!section) return
      const maxRect = { x: section.offsetLeft, y: section.offsetTop, width: section.offsetWidth, height: section.offsetHeight }
      drag.current = { pointerId: e.pointerId, dx: 0, dy: 0, restore: { grab: { x: e.clientX, y: e.clientY }, maxRect } }
    } else {
      drag.current = { pointerId: e.pointerId, dx: e.clientX - win.x, dy: e.clientY - win.y }
    }
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  const onPointerMove = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current
    if (!d || d.pointerId !== e.pointerId) return
    if (d.restore) {
      const { grab, maxRect } = d.restore
      if (Math.hypot(e.clientX - grab.x, e.clientY - grab.y) < RESTORE_THRESHOLD) return
      // 잡은 지점이 복원된 창에서도 같은 비율 위치에 오게 놓고, 이후로는 일반 드래그로 옮긴다.
      const offset = restoreGrab(grab, maxRect, win)
      d.dx = offset.x
      d.dy = offset.y
      d.restore = undefined
      // 최대화 창에서 포인터 쪽으로 줄어드는 애니메이션. 애니메이션은 창 위치에 얹히는 상대 이동이라
      // 이어지는 드래그로 창이 움직여도 함께 따라가고, 잡은 지점은 포인터 아래에 남는다.
      anim.captureFrom()
      unmaximizeAt(id, e.clientX - d.dx, e.clientY - d.dy, bounds)
      return
    }
    moveTo(id, e.clientX - d.dx, e.clientY - d.dy, bounds)
  }
  const endDrag = (e: PointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId === e.pointerId) drag.current = null
  }
  // 최대화를 바꾸기 직전 위치를 남겨 애니메이션이 그 위치에서 시작되게 한다.
  const onToggleMaximize = () => {
    anim.captureFrom()
    toggleMaximize(id)
  }

  const classes = ['window', active && 'active', win.maximized && 'maximized', anim.minimizing && 'minimizing']
    .filter(Boolean)
    .join(' ')
  // 최소화한 창은 언마운트하지 않고 숨긴다. 창 내용의 상태가 유지된다. 최소화 애니메이션이 끝난 뒤에 숨긴다.
  const style = win.maximized
    ? { zIndex: win.z }
    : { zIndex: win.z, left: win.x, top: win.y, width: win.width, height: win.height }

  return (
    <section
      ref={sectionRef}
      className={classes}
      style={style}
      hidden={anim.hidden}
      aria-label={win.title}
      onPointerDownCapture={() => focus(id)}
    >
      <header
        className="titlebar"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={endDrag}
        onPointerCancel={endDrag}
        onLostPointerCapture={endDrag}
        onDoubleClick={(e) => {
          if (!fromButton(e)) onToggleMaximize()
        }}
      >
        <IconImage name={app.icon} size={20} />
        <h2 className="titlebar-title">{win.title}</h2>
        <div className="titlebar-buttons">
          <button type="button" className="win-btn" aria-label="최소화" title="최소화" onClick={() => minimize(id)}>
            <IconImage name="window-minimize-symbolic" size={16} />
          </button>
          <button
            type="button"
            className="win-btn"
            aria-label={win.maximized ? '이전 크기로' : '최대화'}
            title={win.maximized ? '이전 크기로' : '최대화'}
            onClick={onToggleMaximize}
          >
            <IconImage name={win.maximized ? 'window-restore-symbolic' : 'window-maximize-symbolic'} size={16} />
          </button>
          <button type="button" className="win-btn close" aria-label="닫기" title="닫기" onClick={() => close(id)}>
            <IconImage name="window-close-symbolic" size={16} />
          </button>
        </div>
      </header>
      <div className="window-body">
        {app.kind === 'system-info' ? <SystemInfoWindow active={!win.minimized} /> : <PlaceholderWindow />}
      </div>
      {!win.maximized && <ResizeHandles id={id} rect={win} bounds={bounds} minSize={app.minSize} />}
    </section>
  )
}
