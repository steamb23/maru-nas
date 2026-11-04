import { describe, expect, it } from 'vitest'
import { WIDGET_IDS, getWidget, isWidgetAddon, fitSizes, storedPlacements } from './catalog'

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

  it('storedPlacements는 모르는 위젯을 남기고 형식이 틀린 id와 중복만 뺀다', () => {
    const p = { right: 0, top: 0, cols: 3, rows: 2 }
    expect(
      storedPlacements([
        { addon: 'nope', ...p },
        { addon: 'system-info', ...p },
        { addon: 'Bad', ...p },
        { addon: 'nope', ...p, top: 5 },
      ]),
    ).toEqual([
      { addon: 'nope', ...p },
      { addon: 'system-info', ...p },
    ])
  })

  it('fitSizes는 아는 위젯의 크기만 범위 안으로 맞춘다', () => {
    const list = [
      { addon: 'system-info', right: 0, top: 0, cols: 9, rows: 1 },
      { addon: 'nope', right: 9, top: 0, cols: 12, rows: 12 },
    ]
    expect(fitSizes(list)).toEqual([
      { addon: 'system-info', right: 0, top: 0, cols: 4, rows: 2 },
      { addon: 'nope', right: 9, top: 0, cols: 12, rows: 12 },
    ])
    const ok = [{ addon: 'system-info', right: 0, top: 0, cols: 3, rows: 2 }]
    expect(fitSizes(ok)).toBe(ok)
  })
})
