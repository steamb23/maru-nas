import type { Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'
import { DEFAULT_TILE_SPLIT, clampSplit, tileDividerRect, tileMinWidth, visibleTileGroup } from '../windows/tiling'
import { useTileSplitDrag } from './useTileSplitDrag'

interface Props {
  bounds: Size
}

const { setTileSplit } = useWindowStore.getState()

// TileDivider는 화면에 보이는 좌우 타일 그룹 사이 간격이다. 끌면 두 창의 크기가 함께 바뀌고,
// 더블클릭하면 반반으로 되돌린다. 두 창의 안쪽 변 핸들과 합쳐 경계 전체를 잡을 수 있다.
//
// z-index는 두 타일 창 중 뒤쪽 창의 값이고 DOM에서는 창들보다 앞에 그린다. 그래서 두 타일 창보다
// 앞에 있는 떠 있는 창이 간격을 덮으면 그 창이 클릭을 받는다.
export function TileDivider({ bounds }: Props) {
  const zIndex = useWindowStore((s) => {
    const group = visibleTileGroup(s.windows)
    return group ? Math.min(group.left.z, group.right.z) : null
  })
  const split = useWindowStore((s) => s.tileSplit)
  const { dragging, handlers } = useTileSplitDrag(bounds)

  if (zIndex === null) return null
  const rect = tileDividerRect(split, bounds)
  const onDoubleClick = () => {
    const { windows } = useWindowStore.getState()
    setTileSplit(clampSplit(DEFAULT_TILE_SPLIT, bounds, tileMinWidth(windows, 'left'), tileMinWidth(windows, 'right')))
  }

  return (
    <div
      className={dragging ? 'tile-divider dragging' : 'tile-divider'}
      style={{ zIndex, left: rect.x, top: rect.y, width: rect.width, height: rect.height }}
      aria-hidden="true"
      onDoubleClick={onDoubleClick}
      {...handlers}
    >
      <span className="tile-divider-grip" />
    </div>
  )
}
