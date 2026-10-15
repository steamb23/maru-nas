// 바탕화면·로그인 배경의 프리셋과 배경 서술자 해석. DOM에 의존하지 않아 node 환경에서 테스트한다.
//
// 배경 서술자(api Wallpaper)는 preset(여기의 id) 또는 image(올린 이미지)다. 서술자는 자리마다 후보 순서대로
// 해석하고(사용자 값 → 관리자 기본값 → 대시보드 기본), 모르는 프리셋이나 없는 이미지는 다음 후보로 넘어간다.
// 바탕화면 아이콘 글자가 흰색이므로 프리셋은 어둡거나 중간 밝기로만 둔다.
import type { ParseKeys } from 'i18next'
import type { ImageInfo, Wallpaper } from '../api/types.gen'

export interface WallpaperPreset {
  id: string
  nameKey: ParseKeys
  // CSS background 값
  background: string
}

export const WALLPAPER_PRESETS: readonly WallpaperPreset[] = [
  {
    id: 'dusk',
    nameKey: 'appearance.presets.dusk',
    background: 'linear-gradient(160deg, #2b5876 0%, #4e4376 100%)',
  },
  {
    id: 'aurora',
    nameKey: 'appearance.presets.aurora',
    background:
      'radial-gradient(at 18% 22%, #2f6fd6 0, transparent 52%), radial-gradient(at 82% 28%, #7a3fb0 0, transparent 50%), radial-gradient(at 55% 92%, #138a72 0, transparent 55%), #161b29',
  },
  {
    id: 'ocean',
    nameKey: 'appearance.presets.ocean',
    background: 'linear-gradient(160deg, #134e6f 0%, #1f7a8c 55%, #2a9d8f 100%)',
  },
  {
    id: 'forest',
    nameKey: 'appearance.presets.forest',
    background: 'linear-gradient(160deg, #1d3b2a 0%, #2f5d43 50%, #4d7c55 100%)',
  },
  {
    id: 'sunset',
    nameKey: 'appearance.presets.sunset',
    background: 'linear-gradient(160deg, #3a2149 0%, #8a3b5c 55%, #c46a4a 100%)',
  },
  {
    id: 'night',
    nameKey: 'appearance.presets.night',
    background: 'linear-gradient(160deg, #0f1a2e 0%, #1f3352 100%)',
  },
  { id: 'graphite', nameKey: 'appearance.presets.graphite', background: '#3a3f47' },
  { id: 'slate', nameKey: 'appearance.presets.slate', background: '#3d5068' },
  { id: 'plum', nameKey: 'appearance.presets.plum', background: '#4a3b5c' },
]

// DEFAULT_PRESET은 대시보드 기본 배경이다(설정이 생기기 전부터 쓰던 그라데이션).
export const DEFAULT_PRESET = 'dusk'

export function findPreset(id: string | undefined): WallpaperPreset | undefined {
  return WALLPAPER_PRESETS.find((p) => p.id === id)
}

// WallpaperView는 화면에 그릴 배경이다. key가 같으면 같은 배경이다(바뀔 때만 페이드한다).
export type WallpaperView = { key: string; kind: 'css'; background: string } | { key: string; kind: 'image'; url: string }

export function presetView(id: string): WallpaperView {
  const p = findPreset(id) ?? findPreset(DEFAULT_PRESET)!
  return { key: `preset:${p.id}`, kind: 'css', background: p.background }
}

export const DEFAULT_VIEW = presetView(DEFAULT_PRESET)

// WallpaperCandidate는 해석할 후보다. imageUrl은 서술자가 image일 때 그 자리 이미지의 주소다(없으면 null).
export interface WallpaperCandidate {
  wallpaper: Wallpaper | undefined
  imageUrl: string | null
}

// resolveWallpaper는 후보를 순서대로 보고 처음으로 그릴 수 있는 배경이다. 없으면 대시보드 기본 배경이다.
export function resolveWallpaper(candidates: readonly WallpaperCandidate[]): WallpaperView {
  for (const { wallpaper, imageUrl } of candidates) {
    if (!wallpaper) continue
    if (wallpaper.kind === 'preset' && findPreset(wallpaper.preset)) return presetView(wallpaper.preset!)
    if (wallpaper.kind === 'image' && imageUrl) return { key: `image:${imageUrl}`, kind: 'image', url: imageUrl }
  }
  return DEFAULT_VIEW
}

// imageUrlOf는 이미지 정보가 있으면 주소를 만든다.
export function imageUrlOf(info: ImageInfo | undefined | null, url: (rev: string) => string): string | null {
  return info ? url(info.rev) : null
}

// toWallpaperView는 저장해 둔 힌트(localStorage)를 읽는다. 모양이 맞지 않으면 null.
export function toWallpaperView(v: unknown): WallpaperView | null {
  if (typeof v !== 'object' || v === null) return null
  const o = v as Record<string, unknown>
  if (typeof o.key !== 'string') return null
  if (o.kind === 'css' && typeof o.background === 'string') return { key: o.key, kind: 'css', background: o.background }
  if (o.kind === 'image' && typeof o.url === 'string') return { key: o.key, kind: 'image', url: o.url }
  return null
}

// WallpaperHint는 이 브라우저에 남겨 두는 마지막 바탕화면이다(appearance/dom.ts).
// until은 그 세션의 예상 만료 시각(이 브라우저의 Date.now() 기준 ms)이다. null이면 만료 시각을 모르고
// (자동 로그아웃이 꺼져 있는 등), 0이면 로그아웃했거나 세션이 끝난 것을 확인했다.
export interface WallpaperHint {
  user: string
  view: WallpaperView
  until: number | null
}

// toWallpaperHint는 저장해 둔 바탕화면 힌트를 읽는다. 모양이 맞지 않으면 null. until이 없는 예전 힌트는 null로 읽는다.
export function toWallpaperHint(v: unknown): WallpaperHint | null {
  if (typeof v !== 'object' || v === null) return null
  const o = v as Record<string, unknown>
  const view = toWallpaperView(o.view)
  if (typeof o.user !== 'string' || !view) return null
  return { user: o.user, view, until: typeof o.until === 'number' ? o.until : null }
}

// viewBeforeSessionCheck는 세션을 확인하는 동안 그릴 바탕화면이다. 세션이 이미 끝났을 것 같으면(로그아웃했거나
// 예상 만료 시각이 지남) null이고, 그때는 곧 나올 로그인 화면의 배경을 그린다. 끝난 세션으로 사용자 이미지를
// 요청하면 401이 나고, 그리더라도 바로 로그인 배경으로 바뀌기 때문이다.
export function viewBeforeSessionCheck(hint: WallpaperHint | null, now: number): WallpaperView | null {
  if (!hint) return null
  if (hint.until !== null && hint.until <= now) return null
  return hint.view
}

// cssBackground는 배경 레이어의 CSS background 값이다. 이미지는 화면을 덮도록 가운데에 맞춘다.
export function cssBackground(view: WallpaperView): string {
  if (view.kind === 'css') return view.background
  return `center / cover no-repeat url("${view.url.replace(/["\\\n]/g, '')}")`
}
