import { describe, expect, it } from 'vitest'
import { initials, isConsonants, toJamo } from './hangul'

describe('toJamo', () => {
  it('음절을 초성·중성·종성으로 푼다', () => {
    expect(toJamo('네트워크')).toBe('ㄴㅔㅌㅡㅇㅜㅓㅋㅡ')
    expect(toJamo('온도')).toBe('ㅇㅗㄴㄷㅗ')
    expect(toJamo('값')).toBe('ㄱㅏㅂㅅ')
  })

  it('입력 중인 자모와 겹자모도 낱자로', () => {
    expect(toJamo('네ㅌ')).toBe('ㄴㅔㅌ')
    expect(toJamo('ㅘ')).toBe('ㅗㅏ')
    expect(toJamo('ㄳ')).toBe('ㄱㅅ')
  })

  it('된소리는 나누지 않는다', () => {
    expect(toJamo('까')).toBe('ㄲㅏ')
  })

  it('한글이 아닌 글자는 그대로', () => {
    expect(toJamo('smart-1')).toBe('smart-1')
    expect(toJamo('')).toBe('')
  })
})

describe('initials', () => {
  it('음절은 초성으로, 나머지는 그대로', () => {
    expect(initials('디스크건강')).toBe('ㄷㅅㅋㄱㄱ')
    expect(initials('CPU 온도')).toBe('CPU ㅇㄷ')
  })
})

describe('isConsonants', () => {
  it('자음만인지', () => {
    expect(isConsonants('ㄴㅌ')).toBe(true)
    expect(isConsonants('ㄲㄳ')).toBe(true)
    expect(isConsonants('ㄴㅏ')).toBe(false)
    expect(isConsonants('네')).toBe(false)
    expect(isConsonants('n')).toBe(false)
    expect(isConsonants('')).toBe(false)
  })
})
