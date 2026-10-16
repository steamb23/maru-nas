// 지원 언어 목록과 언어 선택 규칙. DOM·i18next에 의존하지 않아 node 환경에서 테스트한다.
//
// 언어를 추가하려면 locales/<code>.json을 만들고 SUPPORTED_LANGUAGES에 넣는다.

// 지원 언어. 순서는 언어 선택 목록의 순서이며, 그 언어 자신의 이름(languageName)을 라틴 문자, 한자·가나·한글
// 순으로 놓았다. 지역·문자 변형은 BCP 47 태그로 쓴다(zh-Hans 간체, zh-Hant 번체, pt-PT 유럽 포르투갈어).
export const SUPPORTED_LANGUAGES = [
  'de',
  'en',
  'es',
  'fr',
  'it',
  'nl',
  'pt-PT',
  'ja',
  'zh-Hans',
  'zh-Hant',
  'ko',
] as const

export type Language = (typeof SUPPORTED_LANGUAGES)[number]

// 지원하지 않는 브라우저 언어의 기본값이자, 번역이 빠진 키의 대체 언어다.
export const FALLBACK_LANGUAGE: Language = 'en'

export function isLanguage(code: string | null | undefined): code is Language {
  return (SUPPORTED_LANGUAGES as readonly string[]).includes(code ?? '')
}

// matchLanguage는 BCP 47 언어 태그를 지원 언어로 바꾼다. 대소문자는 구분하지 않는다.
// 정확히 일치하는 언어가 없으면 같은 언어 중에서 문자가 같은 것(zh-TW·zh-HK → zh-Hant, zh·zh-CN → zh-Hans),
// 그것도 없으면 같은 언어의 첫 지원 언어(pt-BR → pt-PT)를 고른다. 문자는 Intl.Locale.maximize()로 채워 비교한다.
function matchLanguage(tag: string): Language | null {
  const lower = tag.toLowerCase()
  const exact = SUPPORTED_LANGUAGES.find((l) => l.toLowerCase() === lower)
  if (exact) return exact
  const want = maximize(tag)
  if (!want) return null
  const same = SUPPORTED_LANGUAGES.filter((l) => maximize(l)?.language === want.language)
  return same.find((l) => maximize(l)?.script === want.script) ?? same[0] ?? null
}

// maximize는 태그의 언어와 (생략된 경우 추정한) 문자를 돌려준다. 올바른 태그가 아니면 null이다.
function maximize(tag: string): { language: string; script: string | undefined } | null {
  try {
    const l = new Intl.Locale(tag).maximize()
    return { language: l.language, script: l.script }
  } catch {
    return null
  }
}

// pickLanguage는 저장된 선택 → 브라우저 선호 언어 순서로 처음 지원되는 언어를 고르고,
// 없으면 FALLBACK_LANGUAGE를 돌려준다. 지원하지 않는 저장값은 무시한다.
export function pickLanguage(saved: string | null, preferred: readonly string[]): Language {
  if (isLanguage(saved)) return saved
  for (const tag of preferred) {
    const lang = matchLanguage(tag)
    if (lang) return lang
  }
  return FALLBACK_LANGUAGE
}

// languageName은 언어를 그 언어 자신의 이름(한국어, English)으로 표시한다.
// 언어 선택 목록은 현재 언어와 상관없이 이 이름을 쓴다.
export function languageName(code: Language): string {
  const name = new Intl.DisplayNames([code], { type: 'language' }).of(code) ?? code
  // 일부 언어는 소문자로 시작하는 이름을 돌려준다(예: français). 목록에서는 첫 글자를 대문자로 쓴다.
  return name.charAt(0).toLocaleUpperCase(code) + name.slice(1)
}
