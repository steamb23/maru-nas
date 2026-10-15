import { describe, expect, it } from 'vitest'
import { type WallpaperLayer, initialLayers, pushLayer } from './wallpaperLayers'
import { DEFAULT_VIEW, type WallpaperView, presetView } from './wallpapers'

const image = (url: string): WallpaperView => ({ key: `image:${url}`, kind: 'image', url })
const userImage = image('api/preferences/wallpaper-image?v=1')
const loginImage = image('api/site/images/login-background?v=1')

describe('initialLayers', () => {
  it('프리셋은 확인된 레이어로 시작한다', () => {
    expect(initialLayers(DEFAULT_VIEW)).toEqual([{ id: 0, view: DEFAULT_VIEW, verified: true }])
  })

  it('이미지(힌트)는 받기 전이라 확인되지 않은 레이어로 시작한다', () => {
    expect(initialLayers(userImage)).toEqual([{ id: 0, view: userImage, verified: false }])
  })
})

describe('pushLayer', () => {
  it('맨 위가 확인된 같은 배경이면 그대로 둔다', () => {
    const layers: WallpaperLayer[] = [{ id: 3, view: userImage, verified: true }]
    expect(pushLayer(layers, userImage, 4, true)).toBe(layers)
  })

  it('맨 위가 확인되지 않은 같은 배경이면 확인된 새 레이어로 덮는다', () => {
    const layers = initialLayers(userImage)
    expect(pushLayer(layers, userImage, 1, true)).toEqual([...layers, { id: 1, view: userImage, verified: true }])
  })

  it('다른 배경은 위에 겹친다', () => {
    const layers: WallpaperLayer[] = [{ id: 0, view: DEFAULT_VIEW, verified: true }]
    expect(pushLayer(layers, loginImage, 1, true)).toEqual([...layers, { id: 1, view: loginImage, verified: true }])
  })

  it('애니메이션이 꺼져 있으면 새 레이어만 남긴다', () => {
    const layers: WallpaperLayer[] = [
      { id: 0, view: DEFAULT_VIEW, verified: true },
      { id: 1, view: presetView('ocean'), verified: true },
    ]
    expect(pushLayer(layers, loginImage, 2, false)).toEqual([{ id: 2, view: loginImage, verified: true }])
  })

  it('세션이 끝난 채 새로 고침한 뒤 로그인하면 사용자 바탕화면을 다시 올린다', () => {
    // 힌트(사용자 이미지, 401로 비어 있음) → 받지 못해 기본 배경 → 로그인 배경 → 로그인 뒤 사용자 이미지
    let layers: readonly WallpaperLayer[] = initialLayers(userImage)
    layers = pushLayer(layers, DEFAULT_VIEW, 1, false)
    layers = pushLayer(layers, loginImage, 2, false)
    layers = pushLayer(layers, userImage, 3, false)
    expect(layers).toEqual([{ id: 3, view: userImage, verified: true }])
  })
})
