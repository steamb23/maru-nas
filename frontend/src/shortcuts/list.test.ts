import { describe, expect, it } from 'vitest'
import { APP_IDS, type AppId } from '../windows/apps'
import { defaultShortcuts, knownShortcuts, placeShortcut, removeShortcut, visibleShortcuts } from './list'

// 테스트에서는 smart·services를 숨겨진(관리자 전용) 앱으로 본다.
const hidden = new Set<AppId>(['smart', 'services'])
const include = (id: AppId) => !hidden.has(id)
const all = () => true

describe('defaultShortcuts', () => {
  it('시스템 정보·파일·저장소', () => {
    expect(defaultShortcuts()).toEqual(['system-info', 'files', 'storage'])
  })
})

describe('knownShortcuts', () => {
  it('지금 빌드에 없는 앱과 중복을 뺀다', () => {
    const stored = [{ app: 'storage' }, { app: 'removed-app' }, { app: 'files' }, { app: 'storage' }]
    expect(knownShortcuts(stored)).toEqual(['storage', 'files'])
  })

  it('빈 목록은 빈 목록', () => {
    expect(knownShortcuts([])).toEqual([])
  })
})

describe('visibleShortcuts', () => {
  it('보이는 앱만 순서대로', () => {
    expect(visibleShortcuts(['smart', 'files', 'services', 'storage'], include)).toEqual(['files', 'storage'])
  })
})

describe('placeShortcut', () => {
  const saved: AppId[] = ['system-info', 'files', 'storage']

  it.each([
    [0, ['network', 'system-info', 'files', 'storage']],
    [1, ['system-info', 'network', 'files', 'storage']],
    [3, ['system-info', 'files', 'storage', 'network']],
    [99, ['system-info', 'files', 'storage', 'network']],
    [Infinity, ['system-info', 'files', 'storage', 'network']],
    [-1, ['network', 'system-info', 'files', 'storage']],
  ])('없는 앱은 %s번째에 더한다', (to, want) => {
    expect(placeShortcut(saved, 'network', to, all)).toEqual(want)
  })

  it('있는 앱은 옮긴다', () => {
    expect(placeShortcut(saved, 'system-info', 2, all)).toEqual(['files', 'storage', 'system-info'])
    expect(placeShortcut(saved, 'storage', 0, all)).toEqual(['storage', 'system-info', 'files'])
    expect(placeShortcut(saved, 'files', 1, all)).toEqual(saved)
  })

  it('원래 목록을 바꾸지 않는다', () => {
    const copy = [...saved]
    placeShortcut(saved, 'network', 0, all)
    expect(saved).toEqual(copy)
  })

  describe('숨겨진 앱', () => {
    // 보이는 목록: files, storage, network
    const mixed: AppId[] = ['smart', 'files', 'services', 'storage', 'network']
    const hiddenOrder = (list: AppId[]) => list.filter((id) => hidden.has(id))

    it('보이는 목록 기준 자리에 더하고 숨겨진 앱은 남긴다', () => {
      const next = placeShortcut(mixed, 'system-info', 1, include)
      expect(visibleShortcuts(next, include)).toEqual(['files', 'system-info', 'storage', 'network'])
      expect(hiddenOrder(next)).toEqual(['smart', 'services'])
    })

    it('옮겨도 숨겨진 앱의 순서는 그대로다', () => {
      for (let to = 0; to < 3; to++) {
        const next = placeShortcut(mixed, 'network', to, include)
        const want = ['files', 'storage'].toSpliced(to, 0, 'network')
        expect(visibleShortcuts(next, include)).toEqual(want)
        expect(hiddenOrder(next)).toEqual(['smart', 'services'])
        expect(next).toHaveLength(mixed.length)
      }
    })

    it('맨 앞에 놓으면 첫 보이는 앱 앞에 들어간다', () => {
      expect(placeShortcut(mixed, 'storage', 0, include)).toEqual(['smart', 'storage', 'files', 'services', 'network'])
    })
  })
})

describe('removeShortcut', () => {
  it('그 앱만 뺀다', () => {
    expect(removeShortcut(['smart', 'files', 'storage'], 'files')).toEqual(['smart', 'storage'])
    expect(removeShortcut(['files'], 'storage')).toEqual(['files'])
  })
})

describe('기본값', () => {
  it('기본 바로가기는 모두 앱 목록에 있다', () => {
    for (const id of defaultShortcuts()) expect(APP_IDS).toContain(id)
  })
})
