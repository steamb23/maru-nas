import { describe, expect, it } from 'vitest'
import indexCss from './index.css?raw'

// 컴포넌트가 CSS를 직접 import하지 않으므로, index.css 목록에 빠진 파일은 조용히 로드되지 않는다.
const cssFiles = Object.keys(import.meta.glob('./**/*.css')).filter((p) => p !== './index.css')

const imports = [...indexCss.matchAll(/^@import '([^']+)';$/gm)].map((m) => m[1])

describe('index.css', () => {
  it('imports every stylesheet under src', () => {
    expect(cssFiles.filter((p) => !imports.includes(p))).toEqual([])
  })

  it('imports only existing stylesheets, once each', () => {
    expect(imports.filter((p) => !cssFiles.includes(p))).toEqual([])
    expect(new Set(imports).size).toBe(imports.length)
  })

  it('contains only comments and @import lines', () => {
    const rest = indexCss
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l !== '' && !/^@import '[^']+';$/.test(l))
    expect(rest).toEqual([])
  })
})
