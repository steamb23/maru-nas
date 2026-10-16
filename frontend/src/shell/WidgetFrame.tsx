// 바탕화면 위젯 하나의 틀: 제목, 보기, 끌어서 옮기기, 변·모서리 크기 조절(창과 같은 8방향), 누르면 창 열기.
// 위젯은 창 시스템과 분리되어 있다. 창 저장소는 누를 때 창을 여는 데만 쓴다.
import { type KeyboardEvent, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { GridSize, WidgetPlacement } from '../api/types.gen'
import { AddonUnavailable } from '../addons/AddonUnavailable'
import type { UnavailableReason } from '../addons/capabilities'
import { usePageVisible } from '../addons/common/usePageVisible'
import { ADDON_VIEWS } from '../addons/registry'
import { IconImage } from '../icons/IconImage'
import { type WidgetAddonId, getWidget } from '../widgets/catalog'
import {
  type CellRect,
  type PxRect,
  type PxSize,
  RESIZE_EDGES,
  type ResizeEdge,
  cellToPx,
  resizeCells,
  resizePx,
  snapCell,
} from '../widgets/grid'
import { canPlace } from '../widgets/layout'
import { isAddonApp } from '../windows/apps'
import { useWindowStore } from '../windows/store'
import type { ContextMenuItem } from './contextMenu'
import { type DragDelta, usePointerDrag } from './usePointerDrag'
import { ViewBoundary } from './ViewBoundary'
import { mergeHandlers, useContextMenuTrigger } from './useContextMenuTrigger'
import { animationsEnabled } from './useWindowAnimation'
import { WidgetPreview, type WidgetPreviewHandle } from './WidgetPreview'
import { DROP_MS, slideKeyframes } from './widgetPreviewAnim'
import { EASING } from './windowAnim'

interface Props {
  id: WidgetAddonId
  // 화면 배치에서의 자리
  placement: WidgetPlacement
  area: PxSize
  grid: GridSize
  reserved: CellRect | null
  // 다른 위젯들의 화면 자리. 이 자리들과 겹치는 곳에는 놓을 수 없다.
  others: readonly CellRect[]
  notice: UnavailableReason | null
  plugins: readonly string[]
  // 위젯 잠금(화면 설정). 옮기거나 크기를 바꾸거나 메뉴로 제거하지 못한다. 누르면 창을 여는 것은 그대로다.
  locked: boolean
  // 옮기거나 크기를 바꾼 자리를 저장한다.
  onCommit: (edited: WidgetPlacement) => void
  // 끌거나 크기를 바꾸는 동안 true. 레이어를 창 위로 올린다.
  onInteract: (active: boolean) => void
  // 위젯을 바탕화면에서 뺀다(위젯 메뉴의 "위젯 제거").
  onRemove: (id: WidgetAddonId) => void
}

type Mode = 'idle' | 'drag' | 'resize'

// 창과 같다(Window.tsx). 이 거리 전에 놓으면 클릭으로 보고 창을 연다.
const DRAG_THRESHOLD = 4

const sameCell = (a: CellRect | null, b: CellRect | null) =>
  a === b || (a !== null && b !== null && a.right === b.right && a.top === b.top && a.cols === b.cols && a.rows === b.rows)

// applyPx는 끄는 동안 위젯의 위치·크기를 직접 바꾼다(렌더 없이 포인터를 따라가게).
function applyPx(el: HTMLElement, r: PxRect) {
  el.style.left = `${r.x}px`
  el.style.top = `${r.y}px`
  el.style.width = `${r.width}px`
  el.style.height = `${r.height}px`
}

export function WidgetFrame({
  id,
  placement,
  area,
  grid,
  reserved,
  others,
  notice,
  plugins,
  locked,
  onCommit,
  onInteract,
  onRemove,
}: Props) {
  const { t } = useTranslation()
  const def = getWidget(id)
  const View = ADDON_VIEWS[id].Widget
  const pageVisible = usePageVisible()
  const ref = useRef<HTMLElement>(null)
  const [mode, setMode] = useState<Mode>('idle')
  // 지금 놓을 자리. 놓을 수 없는 자리면 null. 가이드는 렌더 없이 previewRef로 움직인다.
  const targetRef = useRef<CellRect | null>(null)
  const previewRef = useRef<WidgetPreviewHandle>(null)
  // 끌기를 시작할 때의 놓을 자리. 가이드는 시작 렌더에서 그려지므로 첫 이동의 자리를 이것으로 넘긴다.
  const [previewStart, setPreviewStart] = useState<PxRect | null>(null)
  // 끌기를 끝낸 순간 위젯이 보이던 자리. 다음 렌더에서 그 자리부터 제자리로 미끄러지게 한다.
  const dropFrom = useRef<PxRect | null>(null)
  const lastDelta = useRef<DragDelta>({ dx: 0, dy: 0 })
  // 이번 누름에서 옮기기를 시작했는지. 놓기 전에 mode가 아직 렌더되지 않았을 수 있어 ref로 둔다.
  const moving = useRef(false)
  // 크기 조절 중인 핸들의 방향
  const resizeEdge = useRef<ResizeEdge>('se')

  const rect = cellToPx(placement, area)
  const cells: CellRect = { right: placement.right, top: placement.top, cols: placement.cols, rows: placement.rows }
  const opensWindow = isAddonApp(id)
  const title = t(def.manifest.title)

  useLayoutEffect(() => {
    const from = dropFrom.current
    dropFrom.current = null
    const el = ref.current
    if (!from || !el || !animationsEnabled()) return
    const frames = slideKeyframes(rect, from)
    if (frames) el.animate(frames, { duration: DROP_MS, easing: EASING })
  })

  const showTarget = (next: CellRect | null) => {
    const valid = next && canPlace(next, others, grid, reserved) ? next : null
    targetRef.current = valid
    previewRef.current?.moveTo(valid && cellToPx(valid, area))
  }

  const begin = (m: Mode) => {
    if (mode === m) return
    setMode(m)
    setPreviewStart(targetRef.current && cellToPx(targetRef.current, area))
    onInteract(true)
  }

  // finish는 끌기·크기 조절을 끝낸다. commit이면 가이드 자리에 놓는다(가이드가 없으면 제자리로 돌아간다).
  const finish = (commit: boolean) => {
    const el = ref.current
    const placed = targetRef.current
    if (el) {
      // 위치·크기는 React가 style로 관리하는 값이다. 지우면 값이 바뀌지 않은 쪽은 React가 다시 넣지 않으므로
      // 지금 자리의 값으로 되돌린다(놓은 자리가 바뀌면 다음 렌더에서 React가 새 값을 넣는다).
      el.style.transform = ''
      applyPx(el, rect)
    }
    targetRef.current = null
    moving.current = false
    setMode('idle')
    onInteract(false)
    if (commit && placed && !sameCell(placed, cells)) onCommit({ addon: id, ...placed })
  }

  const openWindow = () => {
    if (opensWindow) useWindowStore.getState().open(id, area)
  }

  // 우클릭·메뉴 키·길게 누르기로 여는 위젯 메뉴
  const menu = useContextMenuTrigger({
    items: () => {
      const items: ContextMenuItem[] = []
      if (opensWindow) {
        items.push(
          { kind: 'action', id: 'open', labelKey: 'widgets.open', icon: 'document-open-symbolic', onSelect: openWindow },
          { kind: 'separator', id: 'sep' },
        )
      }
      items.push({
        kind: 'action',
        id: 'remove',
        labelKey: 'widgets.remove',
        icon: 'list-remove-symbolic',
        disabled: locked,
        detailKey: locked ? 'widgets.locked' : undefined,
        onSelect: () => onRemove(id),
      })
      return items
    },
  })

  const moveHandlers = usePointerDrag({
    threshold: DRAG_THRESHOLD,
    onMove: (d) => {
      // 길게 눌러 메뉴를 연 누름은 끌기로 이어 가지 않는다.
      if (menu.openedByPress()) return
      // 잠겨 있으면 끌기를 시작하지 않는다. 끄는 도중 잠겼으면 시작한 끌기는 그대로 끝낸다.
      if (locked && !moving.current) return
      moving.current = true
      lastDelta.current = d
      if (ref.current) ref.current.style.transform = `translate(${d.dx}px, ${d.dy}px)`
      // 놓을 자리를 먼저 정해야 begin이 가이드의 첫 자리로 쓴다.
      showTarget(snapCell({ x: rect.x + d.dx, y: rect.y + d.dy }, cells, area))
      begin('drag')
    },
    onEnd: (moved, d) => {
      if (menu.openedByPress()) {
        if (moving.current) finish(false)
        return
      }
      if (!moved) {
        openWindow()
        return
      }
      // 잠겨 있어 끌기를 시작하지 않았으면 클릭도 아니므로 아무것도 하지 않는다.
      if (!moving.current) return
      dropFrom.current = { ...rect, x: rect.x + d.dx, y: rect.y + d.dy }
      finish(true)
    },
    onCancel: () => {
      if (!moving.current) return
      const d = lastDelta.current
      dropFrom.current = { ...rect, x: rect.x + d.dx, y: rect.y + d.dy }
      finish(false)
    },
  })

  const resizeHandlers = usePointerDrag({
    threshold: 0,
    onStart: (e) => {
      // 위젯 틀의 끌기가 시작되지 않게 한다. 끄는 동안 텍스트 선택도 막는다.
      e.stopPropagation()
      e.preventDefault()
      resizeEdge.current = (e.currentTarget.dataset.edge as ResizeEdge | undefined) ?? 'se'
    },
    onMove: (d) => {
      const edge = resizeEdge.current
      // 위젯은 포인터를 따라 픽셀 단위로 늘어나고, 가이드는 칸에 맞춘 크기를 보여 준다.
      const gridPx = cellToPx({ right: 0, top: 0, ...grid }, area)
      if (ref.current) applyPx(ref.current, resizePx(rect, edge, d.dx, d.dy, def.minSize, def.maxSize, gridPx))
      showTarget(resizeCells(cells, edge, d.dx, d.dy, def.minSize, def.maxSize, grid))
      begin('resize')
    },
    onEnd: () => finish(true),
    onCancel: () => finish(false),
  })

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.target !== e.currentTarget) return
    menu.handlers.onKeyDown(e)
    if (e.key !== 'Enter' && e.key !== ' ') return
    e.preventDefault()
    openWindow()
  }

  const resizable =
    !locked && (def.minSize.cols !== def.maxSize.cols || def.minSize.rows !== def.maxSize.rows)
  const classes = ['widget', mode !== 'idle' && mode, opensWindow && 'opens-window'].filter(Boolean).join(' ')

  return (
    <>
      {mode !== 'idle' && <WidgetPreview ref={previewRef} initial={previewStart} />}
      <section
        ref={ref}
        className={classes}
        style={{ left: rect.x, top: rect.y, width: rect.width, height: rect.height }}
        aria-label={title}
        tabIndex={0}
        {...mergeHandlers(menu.handlers, moveHandlers)}
        onKeyDown={onKeyDown}
      >
        <header className="widget-title">
          <IconImage name={def.manifest.icon} size={16} />
          <h2>{title}</h2>
        </header>
        <div className="widget-body">
          {notice ? (
            <AddonUnavailable reason={notice} plugins={plugins} compact />
          ) : (
            <ViewBoundary compact>
              <View active={pageVisible} size={{ cols: placement.cols, rows: placement.rows }} />
            </ViewBoundary>
          )}
        </div>
        {resizable &&
          RESIZE_EDGES.map((edge) => (
            <div
              key={edge}
              className={`resize-handle resize-${edge}`}
              data-edge={edge}
              aria-hidden="true"
              {...resizeHandlers}
            />
          ))}
      </section>
    </>
  )
}
