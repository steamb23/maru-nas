import { describe, expect, it } from 'vitest'
import { APP_IDS, getApp } from '../windows/apps'
import { appTitles } from './appNames'
import en from './locales/en.json'
import ko from './locales/ko.json'

const catalogs = import.meta.glob<{ apps?: Record<string, string> }>('./locales/*.json', {
  eager: true,
  import: 'default',
})

describe('appTitles', () => {
  it('모든 언어의 앱 이름을 돌려준다', () => {
    expect(appTitles('apps.files')).toEqual(expect.arrayContaining([en.apps.files, ko.apps.files]))
  })

  it('언어마다 카탈로그의 앱 이름과 같다', () => {
    const all = Object.values(catalogs).flatMap((c) => Object.values(c.apps ?? {}))
    const fromTitles = Object.keys(en.apps).flatMap((id) => appTitles(`apps.${id}`))
    expect(fromTitles.sort()).toEqual(all.sort())
  })

  it('apps 밖의 키는 이름이 없다', () => {
    expect(appTitles('window.close')).toEqual([])
    expect(appTitles('apps')).toEqual([])
  })

  // 검색용 앱 이름은 카탈로그의 apps 부분만 담으므로 제목 키가 그 밖에 있으면 검색되지 않는다.
  it.each(APP_IDS)('%s의 제목 키는 apps 아래에 있다', (id) => {
    expect(appTitles(getApp(id).titleKey)).not.toEqual([])
  })
})
