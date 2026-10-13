import type { Size } from '../windows/geometry'
import type { TileSide } from '../windows/tiling'
import { useTileSplitDrag } from './useTileSplitDrag'

interface Props {
  side: TileSide
  bounds: Size
}

// TileResizeHandle은 타일 창의 안쪽 변(왼쪽 타일은 오른쪽 변, 오른쪽 타일은 왼쪽 변) 크기 조절 핸들이다.
// 바깥쪽 변은 화면 가장자리에 붙어 있으므로 핸들을 두지 않는다.
export function TileResizeHandle({ side, bounds }: Props) {
  const { handlers } = useTileSplitDrag(bounds)
  const edge = side === 'left' ? 'e' : 'w'
  return <div className={`resize-handle resize-${edge} tile-resize`} aria-hidden="true" {...handlers} />
}
