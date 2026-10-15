// 프로필 사진 자르기 계산(DOM 없음). 자르기 편집기(settings/AvatarEditor.tsx)가 그린다.
//
// 보기 영역은 한 변이 view(px)인 정사각형이고, 그 안의 원이 프로필 사진이 된다. 확대 1배는 이미지의 짧은 변이
// 보기 영역에 꼭 맞는 크기다. offset은 보기 영역 가운데에서 이미지 가운데까지의 거리(px)이며, 이미지가 늘
// 보기 영역을 다 덮도록 제한한다.

export interface Size {
  width: number
  height: number
}

export interface Offset {
  x: number
  y: number
}

export interface Crop {
  zoom: number
  offset: Offset
}

export const MIN_ZOOM = 1
export const MAX_ZOOM = 4
// 올리는 사진의 한 변(px). 화면에 그리는 가장 큰 아바타의 고해상도 화면에서도 충분하다.
export const AVATAR_OUTPUT_SIZE = 512

export const INITIAL_CROP: Crop = { zoom: MIN_ZOOM, offset: { x: 0, y: 0 } }

export function clampZoom(zoom: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom))
}

// displayScale은 이미지 1px이 보기 영역에서 몇 px인지다.
export function displayScale(img: Size, view: number, zoom: number): number {
  return (view / Math.min(img.width, img.height)) * clampZoom(zoom)
}

// clampOffset은 이미지가 보기 영역을 다 덮는 범위로 위치를 제한한다.
export function clampOffset(img: Size, view: number, zoom: number, offset: Offset): Offset {
  const s = displayScale(img, view, zoom)
  const maxX = Math.max(0, (img.width * s - view) / 2)
  const maxY = Math.max(0, (img.height * s - view) / 2)
  const clamp = (v: number, max: number) => Math.min(max, Math.max(-max, v)) || 0
  return { x: clamp(offset.x, maxX), y: clamp(offset.y, maxY) }
}

// zoomTo는 보기 영역 가운데에 있는 이미지 지점을 그대로 둔 채 확대를 바꾼다.
export function zoomTo(img: Size, view: number, crop: Crop, zoom: number): Crop {
  const next = clampZoom(zoom)
  const ratio = next / clampZoom(crop.zoom)
  return {
    zoom: next,
    offset: clampOffset(img, view, next, { x: crop.offset.x * ratio, y: crop.offset.y * ratio }),
  }
}

// moveBy는 위치를 (dx, dy)만큼 옮긴다(이미지를 끄는 방향).
export function moveBy(img: Size, view: number, crop: Crop, dx: number, dy: number): Crop {
  return { zoom: crop.zoom, offset: clampOffset(img, view, crop.zoom, { x: crop.offset.x + dx, y: crop.offset.y + dy }) }
}

// sourceRect는 보기 영역에 보이는 원본 이미지의 영역(이미지 px)이다. 캔버스 drawImage의 원본 영역으로 쓴다.
export function sourceRect(img: Size, view: number, crop: Crop): { x: number; y: number; width: number; height: number } {
  const s = displayScale(img, view, crop.zoom)
  const { x: ox, y: oy } = clampOffset(img, view, crop.zoom, crop.offset)
  const side = view / s
  const cx = img.width / 2 - ox / s
  const cy = img.height / 2 - oy / s
  return { x: cx - side / 2, y: cy - side / 2, width: side, height: side }
}
