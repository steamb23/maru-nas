// t()의 키를 타입 검사한다. 원본 카탈로그는 en.json이며, 다른 언어는 catalog.test.ts가 같은 키를 쓰는지 확인한다.
import 'i18next'
import type en from './locales/en.json'

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation'
    resources: { translation: typeof en }
  }
}
