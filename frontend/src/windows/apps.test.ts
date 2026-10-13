import { describe, expect, it } from 'vitest'
import { APP_IDS, getApp } from './apps'
import { MIN_HEIGHT, MIN_WIDTH } from './geometry'

describe('APPS', () => {
  it.each(APP_IDS)('%s의 기본 크기는 최소 크기 이상이다', (id) => {
    const app = getApp(id)
    expect(app.defaultSize.width).toBeGreaterThanOrEqual(Math.max(app.minSize?.width ?? 0, MIN_WIDTH))
    expect(app.defaultSize.height).toBeGreaterThanOrEqual(Math.max(app.minSize?.height ?? 0, MIN_HEIGHT))
  })
})
