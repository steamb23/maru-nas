import { describe, expect, it } from 'vitest'
import { isTextInput } from './nativeGuard'

describe('isTextInput', () => {
  it.each([
    ['TEXTAREA', null],
    ['INPUT', null],
    ['INPUT', 'text'],
    ['INPUT', 'password'],
    ['INPUT', 'search'],
    ['INPUT', 'number'],
    ['INPUT', 'email'],
    ['INPUT', ' Text '],
    ['input', 'url'],
  ])('%s[type=%s]는 텍스트 입력이다', (tag, type) => {
    expect(isTextInput(tag, type)).toBe(true)
  })

  it.each([
    ['INPUT', 'checkbox'],
    ['INPUT', 'radio'],
    ['INPUT', 'button'],
    ['INPUT', 'SUBMIT'],
    ['INPUT', 'range'],
    ['INPUT', 'file'],
    ['INPUT', 'hidden'],
    ['BUTTON', null],
    ['DIV', null],
    ['SELECT', null],
  ])('%s[type=%s]는 텍스트 입력이 아니다', (tag, type) => {
    expect(isTextInput(tag, type)).toBe(false)
  })
})
