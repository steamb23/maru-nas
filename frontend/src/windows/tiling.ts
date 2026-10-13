// 창 타일링: 창을 화면 가장자리로 끌어 최대화하거나 좌우 절반에 배치한다. DOM에 의존하지 않는다.
//
// 좌우에 타일링된 창들은 하나의 비율(split)을 공유하는 타일 그룹이다. 비율을 바꾸면 양쪽 창이 함께
// 다시 배치된다. 좌표는 geometry.ts와 같이 데스크톱 영역(작업표시줄 제외) 왼쪽 위가 원점이다.
import { type AppId, getApp } from './apps'
import { FLOAT_GAP, MIN_WIDTH, type Point, type Rect, type Size } from './geometry'

export type TileSide = 'left' | 'right'
// 창을 놓을 수 있는 타일링 영역. max는 최대화다.
export type TileZone = 'max' | TileSide
// 창의 배치 방식. free는 사용자가 정한 위치·크기다.
export type WindowLayout = 'free' | TileZone

// 포인터가 데스크톱 영역 가장자리에서 이 거리 안에 들어오면 그 가장자리의 타일링 영역이 된다.
export const TILE_EDGE = 8
// 새 타일 그룹의 왼쪽 비율
export const DEFAULT_TILE_SPLIT = 0.5

// 타일링 계산에 필요한 창 정보. store의 WindowState가 이 모양을 만족한다.
export interface TileWindow {
  id: string
  appId: AppId
  z: number
  minimized: boolean
  maximized: boolean
  tile: TileSide | null
}

export function oppositeSide(side: TileSide): TileSide {
  return side === 'left' ? 'right' : 'left'
}

export function layoutOf(w: Pick<TileWindow, 'maximized' | 'tile'>): WindowLayout {
  return w.maximized ? 'max' : (w.tile ?? 'free')
}

// tileWidth는 좌우 타일 두 개가 나눠 쓰는 너비다(가장자리 간격 2개와 가운데 간격 1개를 뺀다).
function tileWidth(bounds: Size): number {
  return Math.max(0, bounds.width - 3 * FLOAT_GAP)
}

// tileZoneAt은 창을 끄는 포인터 위치에 해당하는 타일링 영역을 반환한다. 포인터 캡처 중에는 브라우저
// 밖으로 나간 포인터가 음수나 bounds를 넘는 좌표로 들어오는데, 이것도 가장자리로 본다.
// 모서리에서는 좌우를 위보다 우선한다. 데스크톱이 좁아 좌우 타일이 최소 너비를 못 가지면 좌우는 끈다.
export function tileZoneAt(pointer: Point, bounds: Size): TileZone | null {
  const sides = tileWidth(bounds) >= 2 * MIN_WIDTH
  if (sides && pointer.x <= TILE_EDGE) return 'left'
  if (sides && pointer.x >= bounds.width - TILE_EDGE) return 'right'
  if (pointer.y <= TILE_EDGE) return 'max'
  return null
}

// tileRect는 타일링 영역에 놓인 창의 위치·크기다. 가이드와 실제 배치가 모두 이 값을 쓴다.
// 최대화·타일 창은 작업표시줄처럼 가장자리와 서로에서 FLOAT_GAP만큼 떨어져 떠 있다.
export function tileRect(zone: TileZone, split: number, bounds: Size): Rect {
  const height = Math.max(0, bounds.height - 2 * FLOAT_GAP)
  if (zone === 'max') {
    return { x: FLOAT_GAP, y: FLOAT_GAP, width: Math.max(0, bounds.width - 2 * FLOAT_GAP), height }
  }
  const total = tileWidth(bounds)
  const leftWidth = Math.round(total * split)
  if (zone === 'left') return { x: FLOAT_GAP, y: FLOAT_GAP, width: leftWidth, height }
  return { x: 2 * FLOAT_GAP + leftWidth, y: FLOAT_GAP, width: total - leftWidth, height }
}

// tileDividerRect는 좌우 타일 사이 간격의 위치·크기다. 이 간격을 끌어 비율을 바꾼다.
export function tileDividerRect(split: number, bounds: Size): Rect {
  const left = tileRect('left', split, bounds)
  return { x: left.x + left.width, y: left.y, width: FLOAT_GAP, height: left.height }
}

// clampSplit은 좌우 타일 너비가 각각 minLeft·minRight 이상이 되게 비율을 제한한다.
// 데스크톱이 좁아 둘 다 만족할 수 없으면 반반으로 나눈다.
export function clampSplit(split: number, bounds: Size, minLeft: number, minRight: number): number {
  const total = tileWidth(bounds)
  if (total <= 0) return DEFAULT_TILE_SPLIT
  const lo = minLeft / total
  const hi = 1 - minRight / total
  if (lo > hi) return DEFAULT_TILE_SPLIT
  return Math.min(Math.max(split, lo), hi)
}

// splitFromDrag는 startSplit일 때의 경계를 가로로 dx만큼 끈 비율이다. 제한은 clampSplit이 한다.
export function splitFromDrag(startSplit: number, dx: number, bounds: Size): number {
  const total = tileWidth(bounds)
  return total > 0 ? startSplit + dx / total : startSplit
}

// tileMinWidth는 side에 타일링된 창들이 모두 최소 크기를 지킬 수 있는 너비다.
export function tileMinWidth(windows: readonly TileWindow[], side: TileSide): number {
  let min = MIN_WIDTH
  for (const w of windows) {
    if (w.tile === side) min = Math.max(min, getApp(w.appId).minSize?.width ?? 0)
  }
  return min
}

// topTiled는 side에 타일링된 창 중 맨 앞 창이다. visibleOnly면 최소화된 창은 뺀다.
function topTiled(
  windows: readonly TileWindow[],
  side: TileSide,
  exclude: string | null,
  visibleOnly: boolean,
): TileWindow | null {
  let top: TileWindow | null = null
  for (const w of windows) {
    if (w.tile !== side || w.id === exclude || (visibleOnly && w.minimized)) continue
    if (top === null || w.z > top.z) top = w
  }
  return top
}

// tilePartner는 타일링된 창 id와 함께 그룹을 이루는 창, 즉 반대쪽에 타일링된 창 중 맨 앞 창이다.
// 최소화된 창도 짝이 될 수 있다. id가 타일링되지 않았거나 반대쪽이 비어 있으면 null.
export function tilePartner(windows: readonly TileWindow[], id: string): TileWindow | null {
  const w = windows.find((o) => o.id === id)
  if (!w?.tile) return null
  return topTiled(windows, oppositeSide(w.tile), id, false)
}

// visibleTileGroup은 화면에 보이는 타일 그룹, 즉 좌우 각각에서 최소화되지 않은 맨 앞 타일 창이다.
// 한쪽이라도 비어 있으면 null.
export function visibleTileGroup(windows: readonly TileWindow[]): { left: TileWindow; right: TileWindow } | null {
  const left = topTiled(windows, 'left', null, true)
  const right = topTiled(windows, 'right', null, true)
  return left && right ? { left, right } : null
}

// splitForTiling은 창 id를 side에 타일링할 때 쓸 비율이다. 반대쪽에 다른 타일 창이 있으면 그 그룹의
// 비율을 그대로 쓰고, 없으면 새 그룹이므로 반반으로 나눈다. 가이드와 놓은 결과가 같은 값을 쓴다.
export function splitForTiling(windows: readonly TileWindow[], id: string, side: TileSide, split: number): number {
  return topTiled(windows, oppositeSide(side), id, false) ? split : DEFAULT_TILE_SPLIT
}
