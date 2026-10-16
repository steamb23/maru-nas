// i18next 초기화와 언어 전환. main.tsx가 App보다 먼저 import하고, i18nReady를 기다린 뒤 그린다.
//
// document·localStorage를 만지고 react-i18next를 등록하므로 컴포넌트와 main.tsx에서만 import한다.
// node 환경에서 테스트하는 모듈(서식 함수, 저장소, 메뉴 데이터 등)은 이 파일을 import하지 않고
// 번역 키나 locale을 인자로 받는다.
import i18n from 'i18next'
import { initReactI18next, useTranslation } from 'react-i18next'
import { FALLBACK_LANGUAGE, type Language, SUPPORTED_LANGUAGES, isLanguage, pickLanguage } from './languages'
import en from './locales/en.json'

const STORAGE_KEY = 'onmaru.language'

// 영어는 빠진 키의 대체 언어라 진입 청크에 넣는다. 다른 언어는 언어마다 청크 하나로 나뉘어 고를 때 받는다.
// 언어 수만큼 늘어나는 카탈로그를 모든 사용자가 받지 않게 하려는 것이다.
const loaders = import.meta.glob<Record<string, unknown>>(['./locales/*.json', '!./locales/en.json'], {
  import: 'default',
})

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
  resources: { [FALLBACK_LANGUAGE]: { translation: en } },
  lng: FALLBACK_LANGUAGE,
  fallbackLng: FALLBACK_LANGUAGE,
  supportedLngs: [...SUPPORTED_LANGUAGES],
  // 영어를 정적으로 넣으므로 초기화를 동기로 끝낸다. 다른 언어는 받은 뒤에 바꾸므로(applyLanguage)
  // useTranslation이 suspend하지 않는다.
  initAsync: false,
  // React가 이스케이프하므로 i18next는 하지 않는다.
  interpolation: { escapeValue: false },
})

syncDocument()
i18n.on('languageChanged', syncDocument)

// loadCatalog는 언어의 카탈로그를 받아 i18next에 넣는다. 이미 있으면 바로 끝난다.
async function loadCatalog(lang: Language): Promise<void> {
  if (i18n.hasResourceBundle(lang, 'translation')) return
  const load = loaders[`./locales/${lang}.json`]
  if (!load) throw new Error(`no catalog for language ${lang}`)
  i18n.addResourceBundle(lang, 'translation', await load())
}

let latestRequest = 0

// applyLanguage는 카탈로그를 받은 뒤 언어를 바꾼다. 카탈로그가 먼저 있어야 resolvedLanguage가 그 언어가 된다.
// 받는 동안 다른 언어를 고르면 먼저 고른 쪽은 버린다. 받지 못하면(서버 연결이 끊겼거나 업그레이드로 청크가
// 바뀜) 언어를 그대로 두고 false를 돌려준다.
async function applyLanguage(lang: Language): Promise<boolean> {
  const request = ++latestRequest
  try {
    await loadCatalog(lang)
    if (request !== latestRequest) return false
    await i18n.changeLanguage(lang)
    return true
  } catch (err) {
    console.warn(`failed to load language ${lang}`, err)
    return false
  }
}

// 처음 고른 언어를 기다리는 최대 시간(ms). 넘으면 영어로 먼저 그리고, 받는 대로 그 언어로 바뀐다.
const READY_TIMEOUT_MS = 3000

// i18nReady는 처음 고른 언어로 바뀌면 끝난다. 그 언어를 받지 못하거나 READY_TIMEOUT_MS가 지나면 영어인 채로
// 끝난다(실패하지 않는다). main.tsx는 영어로 먼저 그렸다가 바뀌지 않게 이걸 기다린 뒤 그린다.
export const i18nReady: Promise<void> = Promise.race([
  applyLanguage(pickLanguage(readSaved(), navigator.languages)).then(() => {}),
  new Promise<void>((resolve) => setTimeout(resolve, READY_TIMEOUT_MS)),
])

// currentLanguage는 지금 쓰는 지원 언어다.
export function currentLanguage(): Language {
  const lang = i18n.resolvedLanguage
  return isLanguage(lang) ? lang : FALLBACK_LANGUAGE
}

// setLanguage는 그 언어의 카탈로그를 받아 언어를 바꾸고 이 브라우저에 기억한다. 받지 못하면 바꾸지도
// 기억하지도 않는다.
export async function setLanguage(lang: Language): Promise<void> {
  if (await applyLanguage(lang)) save(lang)
}

// useLanguage는 현재 언어를 돌려주고, 언어가 바뀌면 컴포넌트를 다시 그린다.
// 서식 함수(Intl)에 locale로 넘길 때 쓴다.
export function useLanguage(): Language {
  const { i18n: instance } = useTranslation()
  return isLanguage(instance.resolvedLanguage) ? instance.resolvedLanguage : FALLBACK_LANGUAGE
}
