// 지원 언어 목록과 언어 선택 규칙. DOM·i18next에 의존하지 않아 node 환경에서 테스트한다.
//
// 언어를 추가하려면 locales/<code>.json을 만들고 SUPPORTED_LANGUAGES와 index.ts의 resources에 넣는다.

export const SUPPORTED_LANGUAGES = ['en', 'ko'] as const

export type Language = (typeof SUPPORTED_LANGUAGES)[number]

// 지원하지 않는 브라우저 언어의 기본값이자, 번역이 빠진 키의 대체 언어다.
export const FALLBACK_LANGUAGE: Language = 'en'

export function isLanguage(code: string | null | undefined): code is Language {
  return (SUPPORTED_LANGUAGES as readonly string[]).includes(code ?? '')
}

// matchLanguage는 BCP 47 언어 태그를 지원 언어로 바꾼다. 정확히 일치하는 언어가 없으면
// 기본 언어 부분(ko-KR → ko)으로 찾는다. 대소문자는 구분하지 않는다.
function matchLanguage(tag: string): Language | null {
  const lower = tag.toLowerCase()
  const exact = SUPPORTED_LANGUAGES.find((l) => l.toLowerCase() === lower)
  if (exact) return exact
  const base = lower.split('-')[0]
  return SUPPORTED_LANGUAGES.find((l) => l.toLowerCase() === base) ?? null
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
