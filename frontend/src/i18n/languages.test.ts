import { describe, expect, it } from 'vitest'
import { SUPPORTED_LANGUAGES, isLanguage, languageName, pickLanguage } from './languages'

describe('pickLanguage', () => {
  it('저장된 선택을 브라우저 언어보다 먼저 쓴다', () => {
    expect(pickLanguage('en', ['ko-KR'])).toBe('en')
    expect(pickLanguage('ko', ['en-US'])).toBe('ko')
  })

  it('지원하지 않는 저장값은 무시한다', () => {
    expect(pickLanguage('sv', ['ko-KR'])).toBe('ko')
    expect(pickLanguage('', ['ko'])).toBe('ko')
    expect(pickLanguage('zh', ['ko'])).toBe('ko')
  })

  it('브라우저 언어 태그의 기본 언어 부분으로 찾는다', () => {
    expect(pickLanguage(null, ['ko-KR'])).toBe('ko')
    expect(pickLanguage(null, ['EN-gb'])).toBe('en')
    expect(pickLanguage(null, ['de-AT'])).toBe('de')
  })

  it('지역·문자 변형이 있는 언어는 태그 그대로 찾는다(대소문자 무시)', () => {
    expect(pickLanguage(null, ['zh-Hant'])).toBe('zh-Hant')
    expect(pickLanguage(null, ['ZH-HANS'])).toBe('zh-Hans')
    expect(pickLanguage(null, ['pt-pt'])).toBe('pt-PT')
  })

  it('중국어는 문자로 간체·번체를 가른다', () => {
    expect(pickLanguage(null, ['zh-TW'])).toBe('zh-Hant')
    expect(pickLanguage(null, ['zh-HK'])).toBe('zh-Hant')
    expect(pickLanguage(null, ['zh-MO'])).toBe('zh-Hant')
    expect(pickLanguage(null, ['zh-Hant-TW'])).toBe('zh-Hant')
    expect(pickLanguage(null, ['zh'])).toBe('zh-Hans')
    expect(pickLanguage(null, ['zh-CN'])).toBe('zh-Hans')
    expect(pickLanguage(null, ['zh-SG'])).toBe('zh-Hans')
    expect(pickLanguage(null, ['zh-Hans-CN'])).toBe('zh-Hans')
  })

  it('같은 언어의 다른 지역은 그 언어의 지원 변형으로 찾는다', () => {
    expect(pickLanguage(null, ['pt-BR'])).toBe('pt-PT')
    expect(pickLanguage(null, ['pt'])).toBe('pt-PT')
  })

  it('올바르지 않은 태그는 건너뛴다', () => {
    expect(pickLanguage(null, ['', '*', 'ko'])).toBe('ko')
  })

  it('브라우저 선호 순서대로 처음 지원되는 언어를 고른다', () => {
    expect(pickLanguage(null, ['sv-SE', 'ko', 'en'])).toBe('ko')
  })

  it('지원 언어가 없으면 영어', () => {
    expect(pickLanguage(null, ['sv-SE', 'hu'])).toBe('en')
    expect(pickLanguage(null, [])).toBe('en')
  })
})

describe('languageName', () => {
  it('그 언어 자신의 이름으로 표시한다', () => {
    expect(languageName('ko')).toBe('한국어')
    expect(languageName('en')).toBe('English')
    expect(languageName('zh-Hant')).toBe('繁體中文')
  })

  it('소문자로 시작하는 이름은 첫 글자를 대문자로 쓴다', () => {
    expect(languageName('fr')).toBe('Français')
    expect(languageName('es')).toBe('Español')
  })

  it('모든 지원 언어가 이름을 가진다', () => {
    for (const lang of SUPPORTED_LANGUAGES) expect(languageName(lang)).not.toBe(lang)
  })
})

describe('isLanguage', () => {
  it('지원 언어만 받는다', () => {
    expect(isLanguage('ko')).toBe(true)
    expect(isLanguage('ko-KR')).toBe(false)
    expect(isLanguage('zh')).toBe(false)
    expect(isLanguage(null)).toBe(false)
  })
})
