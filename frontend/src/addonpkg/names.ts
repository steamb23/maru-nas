// 애드온 패키지 이름을 번역 리소스에 넣는다. 앱·위젯 정의의 titleKey(addonPackageNames.<id>)가 이 키를 가리키므로,
// 제목을 그리는 곳은 내장 앱과 같이 t(titleKey)를 쓴다. 이름은 언어별 평문이고 없는 언어는 en으로 대신한다.
// 패키지 문구에는 i18next 문법({{, }}, $t()이 없다(백엔드 format.Validate가 거부한다).
import i18n from 'i18next'
import type { AddonPackageView } from '../api/types.gen'
import { SUPPORTED_LANGUAGES } from '../i18n/languages'
import { localizedText } from './catalog'

export function registerPackageNames(list: readonly AddonPackageView[]) {
  for (const lang of SUPPORTED_LANGUAGES) {
    const names: Record<string, string> = {}
    for (const p of list) names[p.id] = localizedText(p.name, lang)
    i18n.addResourceBundle(lang, 'translation', { addonPackageNames: names }, true, true)
  }
}
