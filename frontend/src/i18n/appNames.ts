// "모든 앱" 검색에 쓰는 모든 지원 언어의 앱 이름. DOM·i18next에 의존하지 않아 node 환경에서 테스트한다.
//
// 카탈로그 전체는 언어를 고를 때 받으므로(index.ts) 다른 언어의 앱 이름은 i18next에 없다. 그래서 카탈로그의
// apps 부분만 남긴 모듈(locales/<언어>.json?apps, vite.config.ts의 catalogApps)을 진입 청크에 넣는다.
import { SUPPORTED_LANGUAGES } from './languages'

type AppNames = { apps?: Record<string, string> }

const modules = import.meta.glob<AppNames>('./locales/*.json', { query: '?apps', import: 'default', eager: true })

const byLanguage = new Map(
  Object.entries(modules).map(([path, names]) => [path.replace(/^.*\/(.+)\.json$/, '$1'), names.apps ?? {}]),
)

// appTitles는 앱 제목 키(apps.<이름>)를 지원 언어 순서대로 모든 언어의 이름으로 바꾼다.
// 번역이 없는 언어와 apps 밖의 키는 건너뛴다.
export function appTitles(titleKey: string): string[] {
  const m = /^apps\.([^.]+)$/.exec(titleKey)
  if (!m) return []
  return SUPPORTED_LANGUAGES.flatMap((lang) => {
    const name = byLanguage.get(lang)?.[m[1]]
    return name ? [name] : []
  })
}
