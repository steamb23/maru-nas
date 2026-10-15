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

  // 루트 <svg>에 SVG 이름공간이 없으면 브라우저가 이미지(mask-image 포함)로 읽지 못한다. 원본에 없으면
  // vite.config.ts의 svgNamespace가 붙여 data URL로 넣는다. 파일 경로로 남은 아이콘은 검사하지 않는다.
  it('data URL 아이콘은 루트 <svg>에 SVG 이름공간이 있다', () => {
    const missing = Object.values(ICON_SOURCES).flatMap((table) =>
      Object.entries(table)
        .filter(([, icon]) => icon.url.startsWith('data:') && !hasSvgNamespace(decodeDataUrl(icon.url)))
        .map(([name]) => name),
    )
    expect(missing).toEqual([])
  })
})

function decodeDataUrl(url: string): string {
  const comma = url.indexOf(',')
  const body = url.slice(comma + 1)
  return url.slice(0, comma).endsWith(';base64') ? atob(body) : decodeURIComponent(body)
}

function hasSvgNamespace(svg: string): boolean {
  const root = svg.match(/<svg\b[^>]*>/)?.[0] ?? ''
  return /\sxmlns=['"]http:\/\/www\.w3\.org\/2000\/svg['"]/.test(root)
}
