// 접속할 때 sshd가 묻는 질문(비밀번호, 인증 코드)의 답을 터미널 안에서 받는 줄 편집기.
// DOM에 의존하지 않아 node 환경에서 테스트한다. 터미널에 보낼 출력(되돌려 보이기, 지우기)도 함께 만든다.

export interface PromptLine {
  value: string
  // echo가 false면 입력을 화면에 보이지 않는다(비밀번호).
  echo: boolean
}

export type PromptResult =
  // 아직 입력 중. output은 터미널에 쓸 내용이다.
  | { kind: 'editing'; line: PromptLine; output: string }
  // Enter로 답을 마쳤다. Enter 뒤의 입력은 버린다.
  | { kind: 'submit'; value: string; output: string }
  // Ctrl+C, 또는 빈 줄에서 Ctrl+D로 취소했다.
  | { kind: 'cancel'; output: string }

export function newPromptLine(echo: boolean): PromptLine {
  return { value: '', echo }
}

// feedPrompt는 터미널 입력(xterm onData, 붙여 넣기면 여러 글자)을 처리한다.
export function feedPrompt(line: PromptLine, data: string): PromptResult {
  let value = line.value
  let output = ''
  const chars = Array.from(data)
  for (let i = 0; i < chars.length; i++) {
    const ch = chars[i]
    const code = ch.codePointAt(0) ?? 0
    if (ch === '\r' || ch === '\n') {
      return { kind: 'submit', value, output: output + '\r\n' }
    }
    if (code === 0x03 || (code === 0x04 && value === '')) {
      return { kind: 'cancel', output: output + '^C\r\n' }
    }
    if (code === 0x7f || code === 0x08) {
      const last = Array.from(value).pop()
      if (last !== undefined) {
        value = value.slice(0, value.length - last.length)
        if (line.echo) output += erase(last)
      }
      continue
    }
    if (code === 0x15) {
      // Ctrl+U: 줄 전체를 지운다.
      if (line.echo) output += Array.from(value).map(erase).join('')
      value = ''
      continue
    }
    if (code === 0x1b) {
      // 화살표 키 같은 이스케이프 열은 건너뛴다(ESC [ ... 끝 글자, ESC O x).
      i = skipEscape(chars, i)
      continue
    }
    if (code < 0x20) continue
    value += ch
    if (line.echo) output += ch
  }
  return { kind: 'editing', line: { ...line, value }, output }
}

// erase는 글자 하나를 화면에서 지우는 출력이다. 넓은 글자(한글·한자 등)는 두 칸이다.
function erase(ch: string): string {
  return isWide(ch.codePointAt(0) ?? 0) ? '\b\b  \b\b' : '\b \b'
}

// isWide는 터미널에서 두 칸을 차지하는 글자인지의 근사다(동아시아 넓은 글자의 주요 범위).
function isWide(cp: number): boolean {
  return (
    (cp >= 0x1100 && cp <= 0x115f) ||
    (cp >= 0x2e80 && cp <= 0xa4cf) ||
    (cp >= 0xac00 && cp <= 0xd7a3) ||
    (cp >= 0xf900 && cp <= 0xfaff) ||
    (cp >= 0xfe30 && cp <= 0xfe4f) ||
    (cp >= 0xff00 && cp <= 0xff60) ||
    (cp >= 0xffe0 && cp <= 0xffe6) ||
    (cp >= 0x1f300 && cp <= 0x1faff) ||
    (cp >= 0x20000 && cp <= 0x3fffd)
  )
}

function skipEscape(chars: string[], i: number): number {
  const next = chars[i + 1]
  if (next === 'O') return i + 2
  if (next !== '[') return i
  for (let j = i + 2; j < chars.length; j++) {
    const cp = chars[j].codePointAt(0) ?? 0
    if (cp >= 0x40 && cp <= 0x7e) return j
  }
  return chars.length
}
