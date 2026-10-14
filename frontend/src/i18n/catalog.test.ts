import { describe, expect, it } from 'vitest'
import en from './locales/en.json'

// 1차 지원 언어. 카탈로그가 완전해야 한다(키가 빠지면 실패). 나중에 추가하는 언어는 일부만 번역해도 되고
// 빠진 키는 영어로 표시된다.
const COMPLETE = ['en', 'ko']

type Catalog = { [key: string]: string | Catalog }

const catalogs = Object.fromEntries(
  Object.entries(import.meta.glob<Catalog>('./locales/*.json', { eager: true, import: 'default' })).map(
    ([path, mod]) => [path.replace(/^.*\/(.+)\.json$/, '$1'), mod],
  ),
)

// flatten은 중첩 카탈로그를 "a.b.c" 키의 평면 맵으로 바꾼다.
function flatten(c: Catalog, prefix = ''): Map<string, string> {
  const out = new Map<string, string>()
  for (const [k, v] of Object.entries(c)) {
    const key = prefix + k
    if (typeof v === 'string') out.set(key, v)
    else for (const [kk, vv] of flatten(v, `${key}.`)) out.set(kk, vv)
  }
  return out
}

// placeholders는 문구 안의 {{name}} 치환자 이름을 정렬해 돌려준다.
function placeholders(s: string): string[] {
  return [...s.matchAll(/\{\{\s*([\w.]+)[^}]*\}\}/g)].map((m) => m[1]).sort()
}

const base = flatten(en)

describe('번역 카탈로그', () => {
  it('1차 지원 언어 카탈로그가 모두 있다', () => {
    for (const lang of COMPLETE) expect(Object.keys(catalogs)).toContain(lang)
  })

  describe.each(Object.keys(catalogs))('%s', (lang) => {
    const flat = flatten(catalogs[lang])

    it('en에 없는 키가 없다', () => {
      expect([...flat.keys()].filter((k) => !base.has(k))).toEqual([])
    })

    it('치환자가 en과 같다', () => {
      for (const [k, v] of flat) expect(placeholders(v), k).toEqual(placeholders(base.get(k) ?? ''))
    })

    it('빈 문구가 없다', () => {
      expect([...flat].filter(([, v]) => v.trim() === '').map(([k]) => k)).toEqual([])
    })

    it.runIf(COMPLETE.includes(lang))('빠진 키가 없다', () => {
      expect([...base.keys()].filter((k) => !flat.has(k))).toEqual([])
    })
  })
})
