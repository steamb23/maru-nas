import { describe, expect, it } from 'vitest'
import { WIDGET_IDS, getWidget, isWidgetAddon, knownPlacements } from './catalog'

describe('catalog', () => {
  it('위젯 보기가 있는 애드온만 담는다', () => {
    expect(WIDGET_IDS).toContain('system-info')
    expect(isWidgetAddon('system-info')).toBe(true)
    expect(isWidgetAddon('files')).toBe(false)
  })

  it('크기 범위는 manifest를 따른다', () => {
    expect(getWidget('system-info')).toMatchObject({
      defaultSize: { cols: 3, rows: 2 },
      minSize: { cols: 2, rows: 2 },
      maxSize: { cols: 4, rows: 3 },
    })
  })

  it('knownPlacements는 모르는 위젯을 뺀다', () => {
    const p = { right: 0, top: 0, cols: 3, rows: 2 }
    expect(knownPlacements([{ addon: 'nope', ...p }, { addon: 'system-info', ...p }])).toEqual([
      { addon: 'system-info', ...p },
    ])
  })
})
