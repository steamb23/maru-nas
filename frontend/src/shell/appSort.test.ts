import { describe, expect, it } from 'vitest'
import { APP_SORTS, DEFAULT_APP_SORT, sortApps, toAppSort } from './appSort'

const TITLES: Record<string, { en: string; ko: string }> = {
  'system-info': { en: 'System Information', ko: '시스템 정보' },
  storage: { en: 'Storage', ko: '저장소' },
  network: { en: 'Network', ko: '네트워크' },
  temperature: { en: 'Temperature', ko: '온도' },
  smart: { en: 'Disk Health', ko: '디스크 건강' },
  files: { en: 'Files', ko: '파일' },
}
const IDS = Object.keys(TITLES)

describe('toAppSort', () => {
  it('아는 값은 그대로, 없거나 모르는 값은 추천순', () => {
    for (const s of APP_SORTS) expect(toAppSort(s)).toBe(s)
    expect(toAppSort(undefined)).toBe(DEFAULT_APP_SORT)
    expect(toAppSort('random')).toBe('recommended')
  })
})

describe('sortApps', () => {
  it('추천순은 주어진 순서 그대로', () => {
    expect(sortApps(IDS, 'recommended', (id) => TITLES[id].en, 'en')).toEqual(IDS)
  })

  it('이름순은 화면 언어의 이름 순서', () => {
    expect(sortApps(IDS, 'name', (id) => TITLES[id].en, 'en')).toEqual([
      'smart',
      'files',
      'network',
      'storage',
      'system-info',
      'temperature',
    ])
    expect(sortApps(IDS, 'name', (id) => TITLES[id].ko, 'ko')).toEqual([
      'network',
      'smart',
      'system-info',
      'temperature',
      'storage',
      'files',
    ])
  })

  it('대소문자를 가리지 않고, 이름이 같으면 추천순', () => {
    const titles: Record<string, string> = { a: 'beta', b: 'Alpha', c: 'alpha' }
    expect(sortApps(['a', 'b', 'c'], 'name', (id) => titles[id], 'en')).toEqual(['b', 'c', 'a'])
  })

  it('원래 목록을 바꾸지 않는다', () => {
    const ids = [...IDS]
    sortApps(ids, 'name', (id) => TITLES[id].en, 'en')
    expect(ids).toEqual(IDS)
  })
})
