import { describe, expect, it } from 'vitest'
import { SUPPORTED_LANGUAGES, isLanguage, languageName, pickLanguage } from './languages'

describe('pickLanguage', () => {
  it('저장된 선택을 브라우저 언어보다 먼저 쓴다', () => {
    expect(pickLanguage('en', ['ko-KR'])).toBe('en')
    expect(pickLanguage('ko', ['en-US'])).toBe('ko')
  })

  it('지원하지 않는 저장값은 무시한다', () => {
    expect(pickLanguage('fr', ['ko-KR'])).toBe('ko')
    expect(pickLanguage('', ['ko'])).toBe('ko')
  })

  it('브라우저 언어 태그의 기본 언어 부분으로 찾는다', () => {
    expect(pickLanguage(null, ['ko-KR'])).toBe('ko')
    expect(pickLanguage(null, ['EN-gb'])).toBe('en')
  })

  it('브라우저 선호 순서대로 처음 지원되는 언어를 고른다', () => {
    expect(pickLanguage(null, ['fr-FR', 'ko', 'en'])).toBe('ko')
  })

  it('지원 언어가 없으면 영어', () => {
    expect(pickLanguage(null, ['fr-FR', 'de'])).toBe('en')
    expect(pickLanguage(null, [])).toBe('en')
  })
})

describe('languageName', () => {
  it('그 언어 자신의 이름으로 표시한다', () => {
    expect(languageName('ko')).toBe('한국어')
    expect(languageName('en')).toBe('English')
  })

  it('모든 지원 언어가 이름을 가진다', () => {
    for (const lang of SUPPORTED_LANGUAGES) expect(languageName(lang)).not.toBe(lang)
  })
})

describe('isLanguage', () => {
  it('지원 언어만 받는다', () => {
    expect(isLanguage('ko')).toBe(true)
    expect(isLanguage('ko-KR')).toBe(false)
    expect(isLanguage(null)).toBe(false)
  })
})
