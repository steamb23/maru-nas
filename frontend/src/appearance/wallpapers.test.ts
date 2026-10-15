import { describe, expect, it } from 'vitest'
import {
  DEFAULT_VIEW,
  WALLPAPER_PRESETS,
  cssBackground,
  presetView,
  resolveWallpaper,
  toWallpaperHint,
  toWallpaperView,
  viewBeforeSessionCheck,
} from './wallpapers'

describe('WALLPAPER_PRESETS', () => {
  it('id가 겹치지 않고 백엔드 형식(^[a-z0-9-]{1,32}$)을 따른다', () => {
    const ids = WALLPAPER_PRESETS.map((p) => p.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const id of ids) expect(id).toMatch(/^[a-z0-9-]{1,32}$/)
  })
})

describe('resolveWallpaper', () => {
  const img = 'api/preferences/wallpaper-image?v=1'

  it('처음으로 그릴 수 있는 후보를 쓴다', () => {
    expect(
      resolveWallpaper([
        { wallpaper: undefined, imageUrl: null },
        { wallpaper: { kind: 'preset', preset: 'ocean' }, imageUrl: null },
      ]),
    ).toEqual(presetView('ocean'))
  })

  it('이미지 서술자는 이미지가 있을 때만 쓴다', () => {
    expect(resolveWallpaper([{ wallpaper: { kind: 'image' }, imageUrl: img }])).toEqual({
      key: `image:${img}`,
      kind: 'image',
      url: img,
    })
    expect(
      resolveWallpaper([
        { wallpaper: { kind: 'image' }, imageUrl: null },
        { wallpaper: { kind: 'preset', preset: 'forest' }, imageUrl: null },
      ]),
    ).toEqual(presetView('forest'))
  })

  it('모르는 프리셋은 다음 후보로 넘어가고 후보가 없으면 기본 배경이다', () => {
    expect(resolveWallpaper([{ wallpaper: { kind: 'preset', preset: 'removed' }, imageUrl: null }])).toEqual(DEFAULT_VIEW)
    expect(resolveWallpaper([])).toEqual(DEFAULT_VIEW)
  })
})

describe('toWallpaperView', () => {
  it('저장한 힌트를 그대로 읽는다', () => {
    const v = presetView('night')
    expect(toWallpaperView(JSON.parse(JSON.stringify(v)))).toEqual(v)
    expect(toWallpaperView({ key: 'image:x', kind: 'image', url: 'x' })).toEqual({ key: 'image:x', kind: 'image', url: 'x' })
  })

  it('모양이 맞지 않으면 null이다', () => {
    expect(toWallpaperView(null)).toBeNull()
    expect(toWallpaperView({ key: 'k', kind: 'css' })).toBeNull()
    expect(toWallpaperView({ kind: 'image', url: 'x' })).toBeNull()
  })
})

describe('toWallpaperHint', () => {
  const view = presetView('ocean')

  it('저장한 힌트를 그대로 읽는다', () => {
    const hint = { user: 'alice', view, until: 1000 }
    expect(toWallpaperHint(JSON.parse(JSON.stringify(hint)))).toEqual(hint)
  })

  it('until이 없는 예전 힌트는 만료 시각을 모르는 것으로 읽는다', () => {
    expect(toWallpaperHint({ user: 'alice', view })).toEqual({ user: 'alice', view, until: null })
  })

  it('모양이 맞지 않으면 null', () => {
    expect(toWallpaperHint(null)).toBeNull()
    expect(toWallpaperHint({ view })).toBeNull()
    expect(toWallpaperHint({ user: 'alice', view: { key: 'k' } })).toBeNull()
  })
})

describe('viewBeforeSessionCheck', () => {
  const view = { key: 'image:u', kind: 'image', url: 'u' } as const

  it('예상 만료 시각 전이면 바탕화면을 그린다', () => {
    expect(viewBeforeSessionCheck({ user: 'alice', view, until: 2000 }, 1000)).toBe(view)
  })

  it('만료 시각을 모르면 바탕화면을 그린다', () => {
    expect(viewBeforeSessionCheck({ user: 'alice', view, until: null }, 1000)).toBe(view)
  })

  it('예상 만료 시각이 지났거나 로그아웃했으면 그리지 않는다', () => {
    expect(viewBeforeSessionCheck({ user: 'alice', view, until: 1000 }, 1000)).toBeNull()
    expect(viewBeforeSessionCheck({ user: 'alice', view, until: 0 }, 1000)).toBeNull()
  })

  it('힌트가 없으면 null', () => {
    expect(viewBeforeSessionCheck(null, 1000)).toBeNull()
  })
})

describe('cssBackground', () => {
  it('이미지는 화면을 덮게 그리고 따옴표를 벗어나지 못하게 한다', () => {
    expect(cssBackground({ key: 'k', kind: 'image', url: 'a"b' })).toBe('center / cover no-repeat url("ab")')
    expect(cssBackground(presetView('graphite'))).toBe('#3a3f47')
  })
})
