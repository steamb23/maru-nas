import { describe, expect, it } from 'vitest'
import vendorReadme from '../../../vendor/icons/README.md?raw'
import { ICON_SOURCES, getIcon } from './resolver'

// vendor/icons/<출처>/의 라이선스 기록 파일. 출처 폴더마다 LICENSE 또는 NOTICE가 있어야 한다.
const licenseFiles = Object.keys(import.meta.glob('../../../vendor/icons/*/{LICENSE,NOTICE}'))
const licensedSources = new Set(licenseFiles.map((p) => p.split('/').at(-2)))
const readmeSections = [...vendorReadme.matchAll(/^## ([^/\s]+)\/$/gm)].map((m) => m[1])

describe('getIcon', () => {
  it('folder는 Colloid places 아이콘으로 해석된다', () => {
    const icon = getIcon('folder')
    expect(icon.kind).toBe('color')
    expect(icon.url).toMatch(/folder\.svg$|^data:image\/svg\+xml/)
  })

  it('openmediavault는 OMV 로고 원본으로 해석된다', () => {
    const icon = getIcon('openmediavault')
    expect(icon.kind).toBe('color')
    expect(icon.url).toMatch(/favicon\.svg$|^data:image\/svg\+xml/)
  })
})

describe('ICON_SOURCES', () => {
  const sources = Object.keys(ICON_SOURCES)

  it('출처 사이에 같은 아이콘 이름이 없다', () => {
    const names = Object.values(ICON_SOURCES).flatMap((table) => Object.keys(table))
    expect(names.filter((n, i) => names.indexOf(n) !== i)).toEqual([])
  })

  it('출처마다 vendor/icons/README.md에 절이 있다', () => {
    expect([...readmeSections].sort()).toEqual([...sources].sort())
  })

  it('출처마다 vendor/icons/<출처>/에 LICENSE 또는 NOTICE가 있다', () => {
    expect(sources.filter((s) => !licensedSources.has(s))).toEqual([])
  })
})
