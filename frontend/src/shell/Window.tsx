import { type PointerEvent, useRef } from 'react'
import { IconImage } from '../icons/IconImage'
import { getApp } from '../windows/apps'
import type { Size } from '../windows/geometry'
import { selectActiveId, useWindowStore } from '../windows/store'
import { PlaceholderWindow } from './PlaceholderWindow'

interface Props {
  id: string
  bounds: Size
}

interface Drag {
  pointerId: number
  // 포인터와 창 왼쪽 위 모서리 사이의 거리
  dx: number
  dy: number
}

const { focus, close, minimize, toggleMaximize, moveTo } = useWindowStore.getState()

// 버튼 위에서 시작한 포인터 동작은 드래그·더블클릭으로 다루지 않는다.
const fromButton = (e: { target: EventTarget }) => e.target instanceof Element && e.target.closest('button') !== null

export function Window({ id, bounds }: Props) {
  const win = useWindowStore((s) => s.windows.find((w) => w.id === id))
  const active = useWindowStore((s) => selectActiveId(s) === id)
  const drag = useRef<Drag | null>(null)

  if (!win) return null
  const app = getApp(win.appId)

  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0 || win.maximized || fromButton(e)) return
    drag.current = { pointerId: e.pointerId, dx: e.clientX - win.x, dy: e.clientY - win.y }
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  const onPointerMove = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current
    if (!d || d.pointerId !== e.pointerId) return
    moveTo(id, e.clientX - d.dx, e.clientY - d.dy, bounds)
  }
  const endDrag = (e: PointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId === e.pointerId) drag.current = null
  }

  const classes = ['window', active && 'active', win.maximized && 'maximized'].filter(Boolean).join(' ')
  // 최소화한 창은 언마운트하지 않고 숨긴다. 창 내용의 상태가 유지된다.
  const style = win.maximized
    ? { zIndex: win.z }
    : { zIndex: win.z, left: win.x, top: win.y, width: win.width, height: win.height }

  return (
    <section
      className={classes}
      style={style}
      hidden={win.minimized}
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
          if (!fromButton(e)) toggleMaximize(id)
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
            onClick={() => toggleMaximize(id)}
          >
            <IconImage name={win.maximized ? 'window-restore-symbolic' : 'window-maximize-symbolic'} size={16} />
          </button>
          <button type="button" className="win-btn close" aria-label="닫기" title="닫기" onClick={() => close(id)}>
            <IconImage name="window-close-symbolic" size={16} />
          </button>
        </div>
      </header>
      <div className="window-body">
        <PlaceholderWindow />
      </div>
    </section>
  )
}
