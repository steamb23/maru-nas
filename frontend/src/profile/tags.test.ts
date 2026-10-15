import { describe, expect, it } from 'vitest'
import { MAX_TAGS, addTags, sameTags, validEmail, validTags } from './tags'

describe('addTags', () => {
  it('쉼표로 나누고 공백·빈 값·중복을 뺀다', () => {
    expect(addTags(['a'], ' b , ,a, c ')).toEqual(['a', 'b', 'c'])
    expect(addTags([], '')).toEqual([])
  })
})

describe('validTags', () => {
  it('콜론·제어 문자·너무 긴 태그를 거른다', () => {
    expect(validTags(['nas', 'backup'])).toBe(true)
    expect(validTags(['a:b'])).toBe(false)
    expect(validTags(['a\tb'])).toBe(false)
    expect(validTags(['가'.repeat(86)])).toBe(false)
    expect(validTags(Array.from({ length: MAX_TAGS + 1 }, (_, i) => `t${i}`))).toBe(false)
  })
})

describe('validEmail', () => {
  it('빈 값과 주소 모양을 받는다', () => {
    expect(validEmail('')).toBe(true)
    expect(validEmail('me@example.com')).toBe(true)
    expect(validEmail('not-an-email')).toBe(false)
    expect(validEmail('Me <me@example.com>')).toBe(false)
  })
})

describe('sameTags', () => {
  it('순서까지 같아야 같다', () => {
    expect(sameTags(['a', 'b'], ['a', 'b'])).toBe(true)
    expect(sameTags(['a', 'b'], ['b', 'a'])).toBe(false)
  })
})
