import { type PointerEvent, useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { IconImage } from '../icons/IconImage'
import { AddonUnavailable } from '../addons/AddonUnavailable'
import { appAccess, requiredPlugins, useCapabilitiesStore } from '../addons/capabilities'
import { ADDON_VIEWS } from '../addons/registry'
import { getApp, isAddonApp } from '../windows/apps'
import { type Point, type Rect, type Size, restoreGrab } from '../windows/geometry'
import { type WindowState, selectActiveId, useWindowStore } from '../windows/store'
import { type TileZone, layoutOf, tileRect, tileZoneAt } from '../windows/tiling'
import { PlaceholderWindow } from './PlaceholderWindow'
import { ResizeHandles } from './ResizeHandles'
import { TilePreview } from './TilePreview'
import { TileResizeHandle } from './TileResizeHandle'
import { useWindowAnimation } from './useWindowAnimation'

// ClosingWindow는 저장소에서 지워진(닫힌) 창의 마지막 상태와, 그때의 타일 그룹 비율이다.
export interface ClosingWindow {
  win: WindowState
  tileSplit: number
}

interface Props {
  id: string
  bounds: Size
  // 닫힌 창이면 마지막 상태. 닫기 애니메이션 동안 이 상태로 그린다.
  closingState?: ClosingWindow
  // 닫기 애니메이션이 끝나면 부른다. Desktop이 이때 창을 목록에서 뺀다.
  onClosed: (id: string) => void
}

interface Drag {
  pointerId: number
  // 포인터와 창 왼쪽 위 모서리 사이의 거리
  dx: number
  dy: number
  // 누른 지점. 여기서 임계 거리를 넘게 끌어야 드래그로 본다. 그 전에는 클릭·더블클릭이다.
  start: Point
  moved: boolean
  // 최대화·타일 창을 누른 경우 그 창의 위치·크기. 드래그가 시작되면 창을 떼어 내 원래 크기로
  // 되돌리고 일반 드래그로 이어 간다.
  fixedRect?: Rect
  // 지금 놓으면 들어갈 타일링 영역. 렌더링을 기다리지 않고 pointerup에서 바로 쓰려고 여기에도 둔다.
  zone: TileZone | null
}

// 이 거리 이상 끌어야 드래그로 본다. 창을 화면 위쪽에 붙여 둔 채 클릭할 때 조금 흔들려도
// 최대화되지 않게 하고, 최대화·타일 창도 이 거리를 넘겨야 떼어 낸다.
const DRAG_THRESHOLD = 4

const { focus, close, minimize, toggleMaximize, tile, detachAt, moveTo } = useWindowStore.getState()

// 버튼 위에서 시작한 포인터 동작은 드래그·더블클릭으로 다루지 않는다.
const fromButton = (e: { target: EventTarget }) => e.target instanceof Element && e.target.closest('button') !== null

export function Window({ id, bounds, closingState, onClosed }: Props) {
  const { t } = useTranslation()
  const stored = useWindowStore((s) => s.windows.find((w) => w.id === id))
  const win = stored ?? closingState?.win
  const closing = stored === undefined && closingState !== undefined
  const active = useWindowStore((s) => selectActiveId(s) === id)
  // 타일 그룹의 비율은 타일 창만 구독한다. 다른 창은 비율이 바뀌어도 다시 그리지 않는다.
  // 닫힌 창은 닫을 때의 비율로 그린다(저장소에서 빠져 구독 결과가 null이 되므로).
  const storedSplit = useWindowStore((s) => (s.windows.find((w) => w.id === id)?.tile ? s.tileSplit : null))
  const tileSplit = closing ? closingState.tileSplit : storedSplit
  // capability가 바뀌어도 이 창의 안내 여부가 그대로면 다시 그리지 않는다.
  const notice = useCapabilitiesStore((s) => (win ? appAccess(s.caps, win.appId).notice : null))
  const drag = useRef<Drag | null>(null)
  // 끄는 동안 놓으면 들어갈 타일링 영역. 가이드(TilePreview)를 그린다.
  const [zone, setZone] = useState<TileZone | null>(null)
  const sectionRef = useRef<HTMLElement>(null)
  const handleClosed = useCallback(() => onClosed(id), [onClosed, id])
  const anim = useWindowAnimation(
    sectionRef,
    id,
    win?.minimized ?? false,
    win ? layoutOf(win) : 'free',
    closing,
    handleClosed,
  )

  if (!win) return null
  const app = getApp(win.appId)
  const View = isAddonApp(win.appId) ? ADDON_VIEWS[win.appId].Window : null
  const title = t(app.titleKey)
  const maximizeLabel = win.maximized ? t('window.restore') : t('window.maximize')
  const layout = layoutOf(win)

  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    if (e.button !== 0 || fromButton(e)) return
    const start = { x: e.clientX, y: e.clientY }
    if (layout !== 'free') {
      // 최대화·타일 창은 화면에 그려진 위치를 잰다. 좌표는 데스크톱 영역 기준이다.
      const section = e.currentTarget.parentElement
      if (!section) return
      const fixedRect = { x: section.offsetLeft, y: section.offsetTop, width: section.offsetWidth, height: section.offsetHeight }
      drag.current = { pointerId: e.pointerId, dx: 0, dy: 0, start, moved: false, fixedRect, zone: null }
    } else {
      drag.current = { pointerId: e.pointerId, dx: e.clientX - win.x, dy: e.clientY - win.y, start, moved: false, zone: null }
    }
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  const onPointerMove = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current
    if (!d || d.pointerId !== e.pointerId) return
    if (!d.moved) {
      if (Math.hypot(e.clientX - d.start.x, e.clientY - d.start.y) < DRAG_THRESHOLD) return
      d.moved = true
    }
    if (d.fixedRect) {
      // 잡은 지점이 떼어 낸 창에서도 같은 비율 위치에 오게 놓고, 이후로는 일반 드래그로 옮긴다.
      const offset = restoreGrab(d.start, d.fixedRect, win)
      d.dx = offset.x
      d.dy = offset.y
      d.fixedRect = undefined
      // 최대화·타일 창에서 포인터 쪽으로 줄어드는 애니메이션. 애니메이션은 창 위치에 얹히는 상대 이동이라
      // 이어지는 드래그로 창이 움직여도 함께 따라가고, 잡은 지점은 포인터 아래에 남는다.
      anim.captureFrom()
      detachAt(id, e.clientX - d.dx, e.clientY - d.dy, bounds)
    } else {
      moveTo(id, e.clientX - d.dx, e.clientY - d.dy, bounds)
    }
    // 영역은 창이 아니라 포인터 위치로 정한다. 창 위치는 clampPosition으로 제한되지만 포인터는 가장자리까지 간다.
    d.zone = tileZoneAt({ x: e.clientX, y: e.clientY }, bounds)
    setZone(d.zone)
  }
  // 놓았을 때 타일링 영역 안이면 그 자리에 배치한다.
  const onPointerUp = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current
    if (!d || d.pointerId !== e.pointerId) return
    drag.current = null
    setZone(null)
    if (d.zone) {
      anim.captureFrom()
      tile(id, d.zone)
    }
  }
  // 취소되면 배치하지 않고 가이드만 지운다. pointerup 뒤에 오는 lostpointercapture는 이미 끝난 드래그라 무시된다.
  const cancelDrag = (e: PointerEvent<HTMLElement>) => {
    if (drag.current?.pointerId !== e.pointerId) return
    drag.current = null
    setZone(null)
  }
  // 최대화를 바꾸기 직전 위치를 남겨 애니메이션이 그 위치에서 시작되게 한다.
  const onToggleMaximize = () => {
    anim.captureFrom()
    toggleMaximize(id)
  }

  const classes = [
    'window',
    active && 'active',
    win.maximized && 'maximized',
    win.tile && 'tiled',
    anim.minimizing && 'minimizing',
    closing && 'closing',
  ]
    .filter(Boolean)
    .join(' ')
  // 최소화한 창은 언마운트하지 않고 숨긴다. 창 내용의 상태가 유지된다. 최소화 애니메이션이 끝난 뒤에 숨긴다.
  // 최대화·타일 창의 위치·크기는 tileRect가 데스크톱 영역 크기에서 계산한다(가이드와 같은 계산).
  const rect = layout === 'free' ? win : tileRect(layout, tileSplit ?? 0, bounds)
  const style = { zIndex: win.z, left: rect.x, top: rect.y, width: rect.width, height: rect.height }

  return (
    <>
      {zone && <TilePreview key={zone} id={id} zone={zone} bounds={bounds} zIndex={win.z} />}
      <section
        ref={sectionRef}
        className={classes}
        style={style}
        hidden={anim.hidden}
        aria-label={title}
        aria-hidden={closing || undefined}
        onPointerDownCapture={() => focus(id)}
      >
        <header
          className="titlebar"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={cancelDrag}
          onLostPointerCapture={cancelDrag}
          onDoubleClick={(e) => {
            if (!fromButton(e)) onToggleMaximize()
          }}
        >
          <IconImage name={app.icon} size={20} />
          <h2 className="titlebar-title">{title}</h2>
          <div className="titlebar-buttons">
            <button type="button" className="win-btn" aria-label={t('window.minimize')}
              title={t('window.minimize')}
              onClick={() => minimize(id)}>
              <IconImage name="window-minimize-symbolic" size={16} />
            </button>
            <button
              type="button"
              className="win-btn"
              aria-label={maximizeLabel}
              title={maximizeLabel}
              onClick={onToggleMaximize}
            >
              <IconImage name={win.maximized ? 'window-restore-symbolic' : 'window-maximize-symbolic'} size={16} />
            </button>
            <button type="button" className="win-btn close" aria-label={t('window.close')}
              title={t('window.close')}
              onClick={() => close(id)}>
              <IconImage name="window-close-symbolic" size={16} />
            </button>
          </div>
        </header>
        <div className="window-body">
          {notice ? (
            <AddonUnavailable reason={notice} plugins={requiredPlugins(win.appId)} />
          ) : View ? (
            <View active={!win.minimized && !closing} />
          ) : (
            <PlaceholderWindow />
          )}
        </div>
        {layout === 'free' && <ResizeHandles id={id} rect={win} bounds={bounds} minSize={app.minSize} />}
        {win.tile && <TileResizeHandle side={win.tile} bounds={bounds} />}
      </section>
    </>
  )
}
