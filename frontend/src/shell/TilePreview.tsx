import type { Size } from '../windows/geometry'
import { useWindowStore } from '../windows/store'
import { type TileZone, splitForTiling, tileRect } from '../windows/tiling'
import { animationsEnabled } from './useWindowAnimation'

interface Props {
  // 끌고 있는 창
  id: string
  zone: TileZone
  bounds: Size
  // 끌고 있는 창의 z-index. DOM에서 창보다 앞에 그려 그 창 바로 아래, 다른 창들 위에 놓인다.
  zIndex: number
}

// TilePreview는 창을 끄는 동안 놓으면 들어갈 자리를 보여 주는 가이드다. 놓았을 때와 같은 계산
// (splitForTiling, tileRect)을 써서 가이드와 결과가 같은 자리가 된다.
// 영역이 바뀔 때마다 등장 애니메이션을 다시 재생하도록 호출하는 쪽에서 zone을 key로 준다.
export function TilePreview({ id, zone, bounds, zIndex }: Props) {
  const split = useWindowStore((s) => (zone === 'max' ? s.tileSplit : splitForTiling(s.windows, id, zone, s.tileSplit)))
  const rect = tileRect(zone, split, bounds)
  return (
    <div
      className={animationsEnabled() ? 'tile-preview animated' : 'tile-preview'}
      style={{ zIndex, left: rect.x, top: rect.y, width: rect.width, height: rect.height }}
      aria-hidden="true"
    />
  )
}
