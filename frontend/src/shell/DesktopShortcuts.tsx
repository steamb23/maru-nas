// 바탕화면 바로가기: 사용자가 고른 앱 아이콘을 왼쪽 위부터 열 단위로 놓는다. 위젯 레이어와 창 아래에 그리고,
// 끄는 동안만 창 위로 올라온다. 끌어서 순서를 바꾸고, "모든 앱"에서 끌어 온 앱을 원하는 자리에 끼운다.
// 목록은 shortcuts/store.ts(사용자별 저장), 순서 계산은 shortcuts/list.ts, 칸 계산은 shortcuts/layout.ts가 한다.
import { type MouseEvent, type Ref, type RefObject, useEffect, useImperativeHandle, useRef, useState } from 'react'
import { flushSync } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { appAccess, useCapabilitiesStore } from '../addons/capabilities'
import { ICON_HEIGHT, ICON_WIDTH, iconCenter, inBounds, previewOffsets, rowsPerColumn, slotAt } from '../shortcuts/layout'
import { placeShortcut, removeShortcut, visibleShortcuts } from '../shortcuts/list'
import { useShortcutStore } from '../shortcuts/store'
import { APP_IDS, type AppId, getApp, isAppId } from '../windows/apps'
import type { Point, Rect, Size } from '../windows/geometry'
import { GHOST_GRAB } from './appDrawerAnim'
import { launchApp } from './launchApp'
import type { ContextMenuAction, ContextMenuItem } from './contextMenu'
import { DesktopIcon } from './DesktopIcon'
import { usePointerDrag } from './usePointerDrag'
import { mergeHandlers, useContextMenuTrigger } from './useContextMenuTrigger'
import { animationsEnabled } from './useWindowAnimation'
import { DROP_MS, slideKeyframes } from './widgetPreviewAnim'
import { EASING } from './windowAnim'

// DesktopShortcutsHandle은 바탕화면 메뉴(Desktop)가 바로가기에 묻는 것이다.
export interface DesktopShortcutsHandle {
  // 바탕화면 우클릭 메뉴에 넣을 항목("바로가기 추가 ▸"). 목록을 아직 불러오지 못했으면 빈 목록.
  desktopMenuItems: () => ContextMenuItem[]
  // 진행 중인 끌기를 취소한다(데스크톱 범위 모달이 열릴 때).
  cancelDrag: () => void
  // "모든 앱"에서 끄는 앱 app의 고스트를 포인터(화면 좌표)가 잡고 있다. 놓을 칸을 비워 보여 준다.
  // 데스크톱 영역 밖이면 미리보기를 지운다.
  dragOver: (app: AppId, pointer: Point) => void
  // 끌던 앱을 놓는다. 데스크톱 영역 안이면 그 칸에 바로가기를 넣고(이미 있으면 옮기고) true,
  // 밖이면 미리보기만 지우고 false. ghost는 고스트의 화면 자리로, 새 아이콘이 그 자리에서 미끄러져 들어온다.
  dropAt: (app: AppId, pointer: Point, ghost: Rect) => boolean
  // 놓지 않고 끝난 끌기의 미리보기를 지운다.
  dragEnd: () => void
}

interface Props {
  // 데스크톱 영역(작업표시줄 제외) 크기. 창을 열 때 쓴다.
  bounds: Size
  // 아이콘 목록 요소. 위젯 레이어가 크기를 재서 이 영역을 피하고, 바탕화면 메뉴가 빈 곳인지 판단한다.
  listRef: RefObject<HTMLUListElement | null>
  ref?: Ref<DesktopShortcutsHandle>
}

// 목록을 불러오지 못했을 때 다시 시도하는 간격
const RETRY_MS = 10_000
// 이 거리 이상 끌어야 끌기로 본다. 그 전에 놓으면 클릭(창 열기)이다. 창·위젯과 같다.
const DRAG_THRESHOLD = 4

// DragView는 끄는 동안 화면에 반영할 미리보기다. source는 끌기를 시작한 곳(바탕화면 아이콘, "모든 앱" 타일),
// from은 끄는 앱의 보이는 순서(바탕화면에 없으면 null), to는 놓을 칸(놓을 곳이 없으면 null)이다.
interface DragView {
  source: 'desktop' | 'drawer'
  from: number | null
  to: number | null
  // 끌기를 시작할 때 잰 한 열의 아이콘 수
  rows: number
}

// DrawerDrag는 "모든 앱"에서 끌고 있는 앱과, 시작할 때 잰 바탕화면 정보다.
interface DrawerDrag {
  app: AppId
  from: number | null
  count: number
  rows: number
  to: number | null
}

// Press는 아이콘을 누른 순간부터 놓을 때까지의 정보다. 포인터 이벤트가 렌더링보다 먼저 이어서 들어올 수
// 있어, 판단은 상태가 아닌 이 값으로 한다.
interface Press {
  app: AppId
  from: number
  button: HTMLElement
  // 누른 시점의 보이는 순서와 데스크톱 영역 높이. 끄는 사이 바뀌면 끌기를 취소하는 기준이다.
  ids: AppId[]
  height: number
  rows: number
  // 누른 시점의 데스크톱 영역 왼쪽 위(화면 좌표)
  origin: Point
  to: number
  // 임계 거리를 넘어 끌기가 시작됐는지
  moved: boolean
}

const { load, clear } = useShortcutStore.getState()

// 저장 목록을 바꾼다. 계산은 늘 저장소의 최신 목록으로 한다.
const update = (change: (apps: AppId[]) => AppId[]) => {
  const s = useShortcutStore.getState()
  s.set(change(s.apps))
}

export function DesktopShortcuts({ bounds, listRef, ref }: Props) {
  const { t } = useTranslation()
  const status = useShortcutStore((s) => s.status)
  const apps = useShortcutStore((s) => s.apps)
  const caps = useCapabilitiesStore((s) => s.caps)

  // 로그인한 동안만 그려진다. 로그아웃·만료로 언마운트되면 다음 사용자를 위해 비운다.
  useEffect(() => {
    void load()
    return clear
  }, [])

  useEffect(() => {
    if (status !== 'error') return
    const timer = setTimeout(() => void load(), RETRY_MS)
    return () => clearTimeout(timer)
  }, [status])

  // 관리자 전용 앱은 일반 사용자에게 그리지 않는다(저장 목록에는 남는다).
  const include = (id: AppId) => appAccess(caps, id).visible
  const visible = status === 'ready' ? visibleShortcuts(apps, include) : []

  const visibleKey = visible.join(' ')

  // measureRows는 지금 그려진 아이콘 목록의 한 열 아이콘 수다. 브라우저 확대 등으로 영역 높이가 소수점이면
  // 정수로 반올림한 bounds로 계산한 값과 CSS의 행 수가 다를 수 있어, 그려진 행 수를 쓴다.
  const measureRows = () => {
    const list = listRef.current
    const tracks = list ? getComputedStyle(list).gridTemplateRows : 'none'
    const n = tracks === 'none' ? 0 : tracks.split(' ').length
    return n > 0 ? n : rowsPerColumn(bounds.height)
  }

  const open = (id: AppId) => launchApp(id, bounds)

  const press = useRef<Press | null>(null)
  const drawerDrag = useRef<DrawerDrag | null>(null)
  const [view, setView] = useState<DragView | null>(null)
  // 끌거나 놓은 아이콘이 제자리로 가는 동안 목록을 창 위로 올린다.
  const [raised, setRaised] = useState(false)
  // 진행 중인 착지 애니메이션 묶음 수. 모두 끝나면 목록을 내린다.
  const landing = useRef(0)
  // 끌기·길게 누르기 뒤에 오는 click은 무시한다(창을 열지 않는다). 다음 누름에서 풀린다.
  const suppressClick = useRef(false)

  // settle은 끌기를 끝낸다. next가 있으면 그 목록으로 바꾸고, 아이콘마다 지금 보이는 자리에서 새 자리로
  // 미끄러지게 한다(FLIP). 새로 들어온 아이콘은 seed 자리(끌던 고스트)에서 출발한다.
  // 순서(li의 DOM 순서)는 여기서만 바뀐다. 끄는 동안 옮기면 pointer capture가 풀린다.
  const settle = (next: AppId[] | null, dragged: HTMLElement | null, seed?: { app: AppId; rect: Rect }) => {
    const list = listRef.current
    const buttons = () => (list ? Array.from(list.querySelectorAll<HTMLElement>('.desktop-icon')) : [])
    const before = new Map<string, Rect>()
    for (const b of buttons()) {
      before.set(b.dataset.app ?? '', b.getBoundingClientRect())
      for (const a of b.getAnimations()) a.cancel()
    }
    if (seed) before.set(seed.app, seed.rect)
    // 목록과 미리보기 해제를 한 번에 그린다. li의 transform과 전환 효과(.shifting)가 함께 빠지므로 튀지 않는다.
    flushSync(() => {
      if (next) update(() => next)
      setView(null)
    })
    if (dragged) dragged.style.transform = ''
    const anims: Animation[] = []
    if (animationsEnabled()) {
      for (const b of buttons()) {
        const from = before.get(b.dataset.app ?? '')
        const frames = from && slideKeyframes(b.getBoundingClientRect(), from)
        if (frames) anims.push(b.animate(frames, { duration: DROP_MS, easing: EASING }))
      }
    }
    // 착지가 끝나도 그사이 새 끌기가 시작됐으면 목록을 내리지 않는다(그 끌기가 끝날 때 내린다).
    const lower = () => {
      if (landing.current === 0 && !press.current?.moved && !drawerDrag.current) setRaised(false)
    }
    if (anims.length === 0) {
      lower()
      return
    }
    landing.current++
    void Promise.allSettled(anims.map((a) => a.finished)).then(() => {
      landing.current--
      lower()
    })
  }

  // cancel은 진행 중인 끌기를 순서를 바꾸지 않고 끝낸다(Esc, 포커스 잃음, 목록·화면 높이 변화, 모달).
  // 포인터는 아직 눌려 있을 수 있으므로 이어지는 click을 무시한다. effect 안에서도 불리므로 flushSync를 쓰는
  // settle은 microtask로 미룬다(React가 커밋하는 중에는 flushSync를 할 수 없다).
  const cancel = () => {
    const p = press.current
    if (!p) return
    press.current = null
    if (!p.moved) return
    suppressClick.current = true
    queueMicrotask(() => settle(null, p.button))
  }

  // 아이콘 우클릭·메뉴 키·길게 누르기 메뉴. 대상 앱은 입력을 받은 버튼의 data-app이다.
  const menu = useContextMenuTrigger({
    items: (el) => {
      const id = el.dataset.app
      if (!id || !isAppId(id)) return []
      return [
        { kind: 'action', id: 'open', labelKey: 'shortcuts.open', icon: 'document-open-symbolic', onSelect: () => open(id) },
        { kind: 'separator', id: 'sep' },
        {
          kind: 'action',
          id: 'remove',
          labelKey: 'shortcuts.remove',
          icon: 'list-remove-symbolic',
          onSelect: () => update((list) => removeShortcut(list, id)),
        },
      ]
    },
  })

  // 아이콘 끌기. 포인터 capture는 누른 버튼이 갖는다. 끄는 동안 버튼은 transform으로만 움직이고(렌더 없음),
  // 다른 아이콘은 놓을 칸이 바뀔 때만 미리보기 자리로 옮겨 그린다.
  const drag = usePointerDrag({
    threshold: DRAG_THRESHOLD,
    onStart: (e) => {
      suppressClick.current = false
      const button = e.currentTarget
      const app = button.dataset.app
      const area = listRef.current?.parentElement
      const from = app && isAppId(app) ? visible.indexOf(app) : -1
      if (!app || !isAppId(app) || !area || from < 0 || drawerDrag.current) return false
      const r = area.getBoundingClientRect()
      press.current = {
        app,
        from,
        button,
        ids: visible,
        height: bounds.height,
        rows: measureRows(),
        origin: { x: r.x, y: r.y },
        to: from,
        moved: false,
      }
    },
    onMove: (d, e) => {
      const p = press.current
      // 길게 눌러 메뉴를 연 누름은 끌기로 이어 가지 않는다.
      if (!p || menu.openedByPress()) return
      if (!p.moved) {
        p.moved = true
        // 손가락이 조금 움직여 끌기가 시작됐으면 길게 누르기 메뉴는 열지 않는다.
        menu.cancelLongPress()
        for (const a of p.button.getAnimations()) a.cancel()
        setRaised(true)
        setView({ source: 'desktop', from: p.from, to: p.from, rows: p.rows })
      }
      p.button.style.transform = `translate(${d.dx}px, ${d.dy}px)`
      // 영역 밖(작업표시줄 등)이면 놓아도 제자리로 돌아가므로 미리보기도 제자리다.
      const pointer = { x: e.clientX - p.origin.x, y: e.clientY - p.origin.y }
      const start = iconCenter(p.from, p.rows)
      const to = inBounds(pointer, bounds)
        ? slotAt({ x: start.x + d.dx, y: start.y + d.dy }, p.rows, p.ids.length - 1)
        : p.from
      if (to === p.to) return
      p.to = to
      setView({ source: 'desktop', from: p.from, to, rows: p.rows })
    },
    onEnd: (moved, _d, e) => {
      const p = press.current
      press.current = null
      if (menu.openedByPress()) suppressClick.current = true
      if (!p || !p.moved) return
      suppressClick.current = true
      const pointer = { x: e.clientX - p.origin.x, y: e.clientY - p.origin.y }
      const keep = moved && !menu.openedByPress() && inBounds(pointer, bounds) && p.to !== p.from
      settle(keep ? placeShortcut(useShortcutStore.getState().apps, p.app, p.to, include) : null, p.button)
    },
    onCancel: () => {
      const p = press.current
      press.current = null
      if (p?.moved) settle(null, p.button)
    },
  })

  // 바탕화면 아이콘을 끄는 동안 Esc를 누르거나 브라우저 창이 포커스를 잃으면 취소한다.
  // ("모든 앱"에서 끄는 동안은 그쪽이 처리한다.)
  const dragging = view?.source === 'desktop'
  useEffect(() => {
    if (!dragging) return
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      e.preventDefault()
      cancel()
    }
    window.addEventListener('keydown', onKeyDown, true)
    window.addEventListener('blur', cancel)
    return () => {
      window.removeEventListener('keydown', onKeyDown, true)
      window.removeEventListener('blur', cancel)
    }
  })

  // 끄는 사이 보이는 목록(capability, 다른 탭의 저장)이나 화면 높이(열 나눔)가 바뀌면 잰 자리가 맞지 않으므로 취소한다.
  useEffect(() => {
    const p = press.current
    if (p && (p.ids.join(' ') !== visibleKey || p.height !== bounds.height)) cancel()
  })

  // 끌기·길게 누르기 뒤의 click은 무시하지만 키보드(Enter·Space, detail 0)로 누른 것은 늘 연다.
  // 끈 버튼이 놓는 순간 DOM에서 옮겨지면 브라우저가 click을 보내지 않으므로 표시는 여기서 늘 지운다.
  const onClick = (id: AppId, e: MouseEvent) => {
    const suppressed = suppressClick.current
    suppressClick.current = false
    if (!suppressed || e.detail === 0) open(id)
  }

  // 데스크톱 영역 기준 좌표와, 고스트를 그 자리에 놓았을 때의 아이콘 가운데
  const areaPoint = (pointer: Point) => {
    const r = listRef.current?.parentElement?.getBoundingClientRect()
    return r ? { x: pointer.x - r.x, y: pointer.y - r.y } : null
  }
  const ghostCenter = (local: Point) => ({
    x: local.x - GHOST_GRAB.x + ICON_WIDTH / 2,
    y: local.y - GHOST_GRAB.y + ICON_HEIGHT / 2,
  })
  const drawerSlot = (d: DrawerDrag, local: Point | null) =>
    local && inBounds(local, bounds)
      ? slotAt(ghostCenter(local), d.rows, d.from === null ? d.count : d.count - 1)
      : null

  useImperativeHandle(ref, () => ({
    cancelDrag: cancel,
    dragOver: (app, pointer) => {
      if (status !== 'ready' || press.current) return
      let d = drawerDrag.current
      if (d?.app !== app) {
        const from = visible.indexOf(app)
        d = { app, from: from < 0 ? null : from, count: visible.length, rows: measureRows(), to: null }
        drawerDrag.current = d
        setRaised(true)
      }
      const to = drawerSlot(d, areaPoint(pointer))
      if (to === d.to && view) return
      d.to = to
      setView({ source: 'drawer', from: d.from, to, rows: d.rows })
    },
    dropAt: (app, pointer, ghost) => {
      const d = drawerDrag.current
      drawerDrag.current = null
      const to = d?.app === app ? drawerSlot(d, areaPoint(pointer)) : null
      if (!d || to === null) {
        settle(null, null)
        return false
      }
      const next = placeShortcut(useShortcutStore.getState().apps, app, to, include)
      // 놓은 앱은 고스트 자리에서 놓을 칸으로 미끄러져 들어온다(이미 바탕화면에 있던 앱도 끄는 동안 숨겨 두었다).
      settle(next, null, { app, rect: ghost })
      return true
    },
    dragEnd: () => {
      if (!drawerDrag.current) return
      drawerDrag.current = null
      // 언마운트(모달, 로그아웃) 정리에서도 불리므로 flushSync를 쓰는 settle은 microtask로 미룬다.
      queueMicrotask(() => settle(null, null))
    },
    // "바로가기 추가 ▸" 하위 메뉴. 볼 수 있는 앱마다 하나씩, 이미 있는 앱은 체크 표시와 함께 비활성이다.
    desktopMenuItems: () => {
      if (status !== 'ready') return []
      const items = APP_IDS.filter(include).map((id): ContextMenuAction => {
        const app = getApp(id)
        const added = apps.includes(id)
        return {
          kind: 'action',
          id,
          labelKey: app.titleKey,
          icon: app.icon,
          checked: added,
          disabled: added,
          onSelect: () => update((list) => placeShortcut(list, id, Infinity, include)),
        }
      })
      if (items.length === 0) return []
      return [{ kind: 'submenu', id: 'add-shortcut', labelKey: 'shortcuts.add', icon: 'list-add-symbolic', items }]
    },
  }))

  const offsets = view ? previewOffsets(visible.length, view.from, view.to, view.rows) : null

  return (
    <ul className={raised ? 'desktop-icons raised' : 'desktop-icons'} aria-label={t('desktop.label')} ref={listRef}>
      {visible.map((id, i) => {
        // 바탕화면에서 끄는 아이콘은 버튼이 포인터를 따라가므로 li는 그대로 둔다.
        // "모든 앱"에서 끄는 앱이 이미 바탕화면에 있으면 고스트가 대신 보이므로 그 아이콘은 숨기고 놓을 칸을 차지한다.
        const dragged = view?.source === 'desktop' && view.from === i
        const picked = view?.source === 'drawer' && view.from === i
        const o = offsets && !dragged ? offsets[i] : null
        let className: string | undefined
        if (view) className = dragged ? 'dragging' : picked ? 'shifting picked' : 'shifting'
        return (
          <li
            key={id}
            className={className}
            style={o && (o.x || o.y) ? { transform: `translate(${o.x}px, ${o.y}px)` } : undefined}
          >
            <DesktopIcon appId={id} {...mergeHandlers(menu.handlers, drag)} onClick={(e) => onClick(id, e)} />
          </li>
        )
      })}
    </ul>
  )
}
