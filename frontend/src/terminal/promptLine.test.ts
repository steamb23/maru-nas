import { describe, expect, it } from 'vitest'
import { type PromptLine, feedPrompt, newPromptLine } from './promptLine'

// type은 입력을 차례로 넣고 마지막 결과와 지금까지의 출력을 돌려준다.
function type(line: PromptLine, ...inputs: string[]) {
  let output = ''
  let result = feedPrompt(line, '')
  for (const data of inputs) {
    if (result.kind !== 'editing') break
    result = feedPrompt(result.line, data)
    output += result.output
  }
  return { result, output }
}

describe('feedPrompt', () => {
  it('비밀번호는 보이지 않게 받고 Enter로 마친다', () => {
    const { result, output } = type(newPromptLine(false), 'se', 'cret', '\r')
    expect(result).toEqual({ kind: 'submit', value: 'secret', output: '\r\n' })
    expect(output).toBe('\r\n')
  })

  it('echo면 입력을 되돌려 보인다', () => {
    const { result, output } = type(newPromptLine(true), '1234', '56\r')
    expect(result.kind === 'submit' && result.value).toBe('123456')
    expect(output).toBe('123456\r\n')
  })

  it('Enter 뒤의 입력은 버린다(붙여 넣기)', () => {
    const r = feedPrompt(newPromptLine(false), 'pw\rls\r')
    expect(r).toEqual({ kind: 'submit', value: 'pw', output: '\r\n' })
  })

  it('Backspace·Ctrl+U로 지운다', () => {
    const { result, output } = type(newPromptLine(true), 'ab', '\x7f', 'c', '\x15', 'xy\r')
    expect(result.kind === 'submit' && result.value).toBe('xy')
    expect(output).toBe('ab\b \bc\b \b\b \bxy\r\n')
    // 넓은 글자는 두 칸을 지운다. 빈 줄에서 지우면 아무것도 쓰지 않는다.
    expect(type(newPromptLine(true), '가', '\x7f', '\x7f').output).toBe('가\b\b  \b\b')
  })

  it('Ctrl+C와 빈 줄의 Ctrl+D는 취소다', () => {
    expect(feedPrompt(newPromptLine(false), 'ab\x03').kind).toBe('cancel')
    expect(feedPrompt(newPromptLine(false), '\x04').kind).toBe('cancel')
    expect(feedPrompt({ value: 'a', echo: false }, '\x04').kind).toBe('editing')
  })

  it('화살표 키 같은 이스케이프 열과 제어 문자는 무시한다', () => {
    const { result } = type(newPromptLine(true), 'a\x1b[A\x1b[1;5Cb\x1bOHc\tz\r')
    expect(result.kind === 'submit' && result.value).toBe('abcz')
  })
})
