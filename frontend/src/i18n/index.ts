// i18next 초기화와 언어 전환. main.tsx가 App보다 먼저 import한다.
//
// document·localStorage를 만지고 react-i18next를 등록하므로 컴포넌트와 main.tsx에서만 import한다.
// node 환경에서 테스트하는 모듈(서식 함수, 저장소, 메뉴 데이터 등)은 이 파일을 import하지 않고
// 번역 키나 locale을 인자로 받는다.
import i18n from 'i18next'
import { initReactI18next, useTranslation } from 'react-i18next'
import { FALLBACK_LANGUAGE, type Language, SUPPORTED_LANGUAGES, isLanguage, pickLanguage } from './languages'
import en from './locales/en.json'
import ko from './locales/ko.json'

const STORAGE_KEY = 'omvdesk.language'

const resources = {
  en: { translation: en },
  ko: { translation: ko },
} satisfies Record<Language, { translation: object }>

// 언어 선택은 브라우저별 편의 설정이라 localStorage에 둔다. 사생활 보호 모드 등에서 저장소를 쓸 수 없으면
// 저장하지 않고 브라우저 언어로 동작한다.
function readSaved(): string | null {
  try {
    return localStorage.getItem(STORAGE_KEY)
  } catch {
    return null
  }
}

function save(lang: Language) {
  try {
    localStorage.setItem(STORAGE_KEY, lang)
  } catch {
    // 저장하지 못해도 이번 탭에서는 바뀐 언어로 동작한다.
  }
}

// syncDocument는 문서 언어와 탭 제목을 현재 언어에 맞춘다.
function syncDocument() {
  document.documentElement.lang = currentLanguage()
  document.title = i18n.t('app.title')
}

void i18n.use(initReactI18next).init({
  resources,
  lng: pickLanguage(readSaved(), navigator.languages),
  fallbackLng: FALLBACK_LANGUAGE,
  supportedLngs: [...SUPPORTED_LANGUAGES],
  // 리소스를 정적으로 넣으므로 초기화를 동기로 끝낸다. 그래야 useTranslation이 suspend하지 않는다.
  initAsync: false,
  // React가 이스케이프하므로 i18next는 하지 않는다.
  interpolation: { escapeValue: false },
})

syncDocument()
i18n.on('languageChanged', syncDocument)

// currentLanguage는 지금 쓰는 지원 언어다.
export function currentLanguage(): Language {
  const lang = i18n.resolvedLanguage
  return isLanguage(lang) ? lang : FALLBACK_LANGUAGE
}

// setLanguage는 언어를 바꾸고 이 브라우저에 기억한다.
export function setLanguage(lang: Language) {
  save(lang)
  void i18n.changeLanguage(lang)
}

// useLanguage는 현재 언어를 돌려주고, 언어가 바뀌면 컴포넌트를 다시 그린다.
// 서식 함수(Intl)에 locale로 넘길 때 쓴다.
export function useLanguage(): Language {
  const { i18n: instance } = useTranslation()
  return isLanguage(instance.resolvedLanguage) ? instance.resolvedLanguage : FALLBACK_LANGUAGE
}
