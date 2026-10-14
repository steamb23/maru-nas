// 바탕화면 위젯 레이어. 바탕화면 아이콘 위, 창 아래에 그린다(끌거나 크기를 바꾸는 동안만 창 위로 올라온다).
// 창 시스템과 분리되어 있다. 저장 배치는 widgets/store.ts, 화면 배치 계산은 widgets/layout.ts가 한다.
import { type Ref, type RefObject, useEffect, useImperativeHandle, useLayoutEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { WidgetPlacement } from '../api/types.gen'
import { requiredPlugins, useCapabilitiesStore, widgetAccess } from '../addons/capabilities'
import { WIDGET_IDS, type WidgetAddonId, getWidget, isWidgetAddon } from '../widgets/catalog'
import { type PxRect, type PxSize, gridSize, reservedCells } from '../widgets/grid'
import { commitPlacement, fitLayout, placeNew, removePlacement } from '../widgets/layout'
import { useWidgetStore } from '../widgets/store'
import type { ContextMenuAction, ContextMenuItem } from './contextMenu'
import { WidgetFrame } from './WidgetFrame'

// WidgetLayerHandle은 바탕화면 메뉴(Desktop)가 위젯 레이어에 묻는 것이다.
export interface WidgetLayerHandle {
  // 바탕화면 우클릭 메뉴에 넣을 항목("위젯 추가 ▸"). 배치를 아직 불러오지 못했으면 빈 목록.
  desktopMenuItems: () => ContextMenuItem[]
}

interface Props {
  // 데스크톱 영역(작업표시줄 제외) 크기
  bounds: PxSize
  // 바탕화면 아이콘 목록. 이 영역에 걸치는 칸에는 위젯을 놓지 않는다.
  iconsRef: RefObject<HTMLElement | null>
  ref?: Ref<WidgetLayerHandle>
}

// 배치를 불러오지 못했을 때 다시 시도하는 간격
const RETRY_MS = 10_000

const { load, clear } = useWidgetStore.getState()

const samePx = (a: PxRect | null, b: PxRect) =>
  a !== null && a.x === b.x && a.y === b.y && a.width === b.width && a.height === b.height

export function WidgetLayer({ bounds, iconsRef, ref }: Props) {
  const { t } = useTranslation()
  const status = useWidgetStore((s) => s.status)
  const placements = useWidgetStore((s) => s.placements)
  const caps = useCapabilitiesStore((s) => s.caps)
  const [icons, setIcons] = useState<PxRect | null>(null)
  // 위젯을 끌거나 크기를 바꾸는 중이면 레이어를 창 위로 올린다.
  const [raised, setRaised] = useState(false)

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

  // 아이콘 목록의 크기는 화면 높이(열 수)와 보이는 아이콘 수에 따라 바뀐다.
  useLayoutEffect(() => {
    const el = iconsRef.current
    if (!el) return
    const measure = () => {
      const next = { x: el.offsetLeft, y: el.offsetTop, width: el.offsetWidth, height: el.offsetHeight }
      setIcons((prev) => (samePx(prev, next) ? prev : next))
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [iconsRef])

  const grid = gridSize(bounds)
  const reserved = reservedCells(icons, bounds)
  // 관리자 전용 위젯은 일반 사용자에게 그리지 않는다(저장 배치에는 남는다).
  const include = (p: WidgetPlacement) => isWidgetAddon(p.addon) && widgetAccess(caps, p.addon).visible
  const { shown } = fitLayout(placements, grid, reserved, include)

  const commit = (edited: WidgetPlacement) => {
    const latest = useWidgetStore.getState().placements
    useWidgetStore.getState().set(commitPlacement(latest, shown, edited, grid))
  }
  const remove = (id: WidgetAddonId) => {
    useWidgetStore.getState().set(removePlacement(useWidgetStore.getState().placements, id))
  }

  // "위젯 추가 ▸" 하위 메뉴. 관리자 전용 위젯은 일반 사용자에게 보이지 않는다.
  // 이미 있는 위젯은 체크 표시와 함께 비활성, 화면에 자리가 없으면 비활성과 이유를 보여 준다.
  useImperativeHandle(ref, () => ({
    desktopMenuItems: () => {
      if (status !== 'ready') return []
      const items = WIDGET_IDS.filter((id) => widgetAccess(caps, id).visible).map((id): ContextMenuAction => {
        const def = getWidget(id)
        const added = placements.some((p) => p.addon === id)
        const spot = added ? null : placeNew(def.defaultSize, shown, grid, reserved)
        return {
          kind: 'action',
          id,
          labelKey: def.manifest.title,
          icon: def.manifest.icon,
          checked: added,
          disabled: added || !spot,
          detailKey: !added && !spot ? 'widgets.noSpace' : undefined,
          onSelect: () => spot && commit({ addon: id, ...spot }),
        }
      })
      if (items.length === 0) return []
      return [{ kind: 'submenu', id: 'add-widget', labelKey: 'widgets.add', icon: 'list-add-symbolic', items }]
    },
  }))

  if (status !== 'ready') return null

  return (
    <div className={raised ? 'widget-layer raised' : 'widget-layer'} aria-label={t('widgets.label')} role="region">
      {shown.map((p) => {
        if (!isWidgetAddon(p.addon)) return null
        const id = p.addon
        return (
          <WidgetFrame
            key={id}
            id={id}
            placement={p}
            area={bounds}
            grid={grid}
            reserved={reserved}
            others={shown.filter((o) => o.addon !== id)}
            notice={widgetAccess(caps, id).notice}
            plugins={requiredPlugins(id)}
            onCommit={commit}
            onInteract={setRaised}
            onRemove={remove}
          />
        )
      })}
    </div>
  )
}
