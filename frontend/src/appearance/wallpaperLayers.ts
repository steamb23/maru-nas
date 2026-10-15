// 배경 레이어 목록 계산(DOM 없음). shell/Wallpaper.tsx가 이 목록을 아래에서 위로 겹쳐 그린다.
//
// 레이어는 그릴 수 있는지 확인한 배경(verified)과 아닌 배경으로 나뉜다. 이미지는 받아 디코드한 뒤에야
// 확인된다. 첫 레이어는 깜빡임을 줄이려고 힌트를 확인 없이 바로 그리므로, 이미지면 받지 못했을 수 있다
// (세션이 끝난 채 새로 고침하면 사용자 바탕화면은 401로 비어 있다). 그래서 같은 배경이라도 확인되지 않은
// 레이어는 확인된 새 레이어로 덮는다. 같은 이미지를 겹치는 페이드라 화면에는 보이지 않는다.
import type { WallpaperView } from './wallpapers'

export interface WallpaperLayer {
  id: number
  view: WallpaperView
  verified: boolean
}

// initialLayers는 처음 그릴 목록이다. 이미지가 아닌 배경은 언제나 그릴 수 있으므로 확인된 것으로 본다.
export function initialLayers(view: WallpaperView): WallpaperLayer[] {
  return [{ id: 0, view, verified: view.kind !== 'image' }]
}

// pushLayer는 받아 확인한 배경 ready를 맨 위에 올린 목록이다. 맨 위가 이미 확인된 같은 배경이면 그대로 둔다.
// animate가 아니면(애니메이션 꺼짐) 새 레이어만 남긴다.
export function pushLayer(
  layers: readonly WallpaperLayer[],
  ready: WallpaperView,
  id: number,
  animate: boolean,
): readonly WallpaperLayer[] {
  const top = layers[layers.length - 1]
  if (top.verified && top.view.key === ready.key) return layers
  const layer: WallpaperLayer = { id, view: ready, verified: true }
  return animate ? [...layers, layer] : [layer]
}
