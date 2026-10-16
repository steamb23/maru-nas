import { describe, expect, it } from 'vitest'
import { SUPPORTED_LANGUAGES } from './languages'
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

// 카탈로그 파일 원문. 보이지 않는 문자가 그대로 들어갔는지 본다.
const sources = Object.fromEntries(
  Object.entries(import.meta.glob<string>('./locales/*.json', { eager: true, query: '?raw', import: 'default' })).map(
    ([path, src]) => [path.replace(/^.*\/(.+)\.json$/, '$1'), src],
  ),
)

// 편집기에서 보이지 않는 문자: 서식 문자(소프트 하이픈, 폭 없는 공백, 방향 표시 등)와 줄바꿈 없는 공백.
// 카탈로그에는 \u 이스케이프로 써야 사람이 읽고 고칠 수 있다.
const INVISIBLE = /[\p{Cf}\xA0]/gu

const base = flatten(en)

// 복수형 키는 <기준>_<범주>다(i18next JSON v4). 범주는 언어마다 다르다(Intl.PluralRules):
// en·de는 one·other, ru·pl·cs·uk는 one·few·many·other, ja·zh·ko는 other뿐이다.
const PLURAL_KEY = /^(.+)_(zero|one|two|few|many|other)$/

// en에서 <기준>_other가 있는 복수형 기준 키들.
const pluralBases = new Set(
  [...base.keys()].flatMap((k) => {
    const m = PLURAL_KEY.exec(k)
    return m && m[2] === 'other' ? [m[1]] : []
  }),
)

function pluralCategories(lang: string): string[] {
  return new Intl.PluralRules(lang).resolvedOptions().pluralCategories
}

// reference는 그 언어의 키 k에 대응하는 en 문구다. en에 같은 키가 없어도 그 언어의 복수형 범주이면
// (ru의 _few 등) en의 <기준>_other와 대응한다. 대응하는 문구가 없으면 undefined다.
function reference(lang: string, k: string): string | undefined {
  if (base.has(k)) return base.get(k)
  const m = PLURAL_KEY.exec(k)
  if (m && pluralBases.has(m[1]) && pluralCategories(lang).includes(m[2])) return base.get(`${m[1]}_other`)
  return undefined
}

describe('번역 카탈로그', () => {
  it('1차 지원 언어 카탈로그가 모두 있다', () => {
    for (const lang of COMPLETE) expect(Object.keys(catalogs)).toContain(lang)
  })

  it('지원 언어마다 카탈로그가 하나씩 있다', () => {
    expect(Object.keys(catalogs).sort()).toEqual([...SUPPORTED_LANGUAGES].sort())
  })

  describe.each(Object.keys(catalogs))('%s', (lang) => {
    const flat = flatten(catalogs[lang])

    it('en에 대응하는 문구가 없는 키가 없다', () => {
      expect([...flat.keys()].filter((k) => reference(lang, k) === undefined)).toEqual([])
    })

    it('치환자가 en과 같다', () => {
      for (const [k, v] of flat) expect(placeholders(v), k).toEqual(placeholders(reference(lang, k) ?? ''))
    })

    it('보이지 않는 문자를 이스케이프로 쓴다', () => {
      const lines = sources[lang].split('\n')
      const found = lines.flatMap((line, i) =>
        [...line.matchAll(INVISIBLE)].map((m) => `${i + 1}행 U+${m[0].codePointAt(0)!.toString(16).toUpperCase().padStart(4, '0')}`),
      )
      expect(found).toEqual([])
    })

    it('빈 문구가 없다', () => {
      expect([...flat].filter(([, v]) => v.trim() === '').map(([k]) => k)).toEqual([])
    })

    it('번역한 복수형은 그 언어의 범주를 모두 갖는다', () => {
      // 범주가 하나라도 빠지면 그 수에서는 영어 문구가 나온다.
      const missing = [...pluralBases].flatMap((b) => {
        const translated = [...flat.keys()].some((k) => PLURAL_KEY.exec(k)?.[1] === b)
        return translated ? pluralCategories(lang).flatMap((c) => (flat.has(`${b}_${c}`) ? [] : [`${b}_${c}`])) : []
      })
      expect(missing).toEqual([])
    })

    it.runIf(COMPLETE.includes(lang))('빠진 키가 없다', () => {
      expect([...base.keys()].filter((k) => !flat.has(k))).toEqual([])
    })
  })
})
