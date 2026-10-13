import { describe, expect, it } from 'vitest'
import { withLeaving } from './taskbarAnim'

describe('withLeaving', () => {
  it('퇴장 중인 버튼이 없으면 next 순서 그대로', () => {
    expect(withLeaving(['a', 'b'], ['b', 'a', 'c'], new Set())).toEqual(['b', 'a', 'c'])
  })

  it('닫힌 버튼은 퇴장 중이면 원래 앞 버튼 뒤에 남는다', () => {
    expect(withLeaving(['a', 'b', 'c'], ['a', 'c'], new Set(['b']))).toEqual(['a', 'b', 'c'])
  })

  it('퇴장이 끝난 id는 빠진다', () => {
    expect(withLeaving(['a', 'b', 'c'], ['a', 'c'], new Set())).toEqual(['a', 'c'])
  })

  it('맨 앞 버튼이 닫히면 맨 앞에 남는다', () => {
    expect(withLeaving(['a', 'b'], ['b'], new Set(['a']))).toEqual(['a', 'b'])
  })

  it('연달아 닫힌 버튼은 순서를 유지한다', () => {
    expect(withLeaving(['a', 'b', 'c', 'd'], ['a', 'd'], new Set(['b', 'c']))).toEqual(['a', 'b', 'c', 'd'])
  })

  it('앞 버튼의 자리가 바뀌면 함께 따라간다', () => {
    expect(withLeaving(['a', 'b', 'c'], ['c', 'a'], new Set(['b']))).toEqual(['c', 'a', 'b'])
  })
})
