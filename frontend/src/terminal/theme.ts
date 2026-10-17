// 터미널 색. DOM에 의존하지 않아 node 환경에서 테스트한다.
// 배경·글자색은 창 틀의 CSS 토큰(--terminal-bg, --terminal-fg, light-dark)을 브라우저가 계산한 값으로 받고,
// ANSI 16색은 배경이 어두운지에 따라 고른다(One Dark·One Light 계열).
import type { ITheme } from '@xterm/xterm'

const DARK = {
  black: '#3f4451',
  red: '#e06c75',
  green: '#98c379',
  yellow: '#d19a66',
  blue: '#61afef',
  magenta: '#c678dd',
  cyan: '#56b6c2',
  white: '#d7dae0',
  brightBlack: '#5c6370',
  brightRed: '#ef7b84',
  brightGreen: '#a9d48a',
  brightYellow: '#e5c07b',
  brightBlue: '#74bdf7',
  brightMagenta: '#d58ee8',
  brightCyan: '#69c7d3',
  brightWhite: '#ffffff',
} as const satisfies ITheme

const LIGHT = {
  black: '#383a42',
  red: '#e45649',
  green: '#3f8f3e',
  yellow: '#a77300',
  blue: '#4078f2',
  magenta: '#a626a4',
  cyan: '#0184bc',
  white: '#9d9fa6',
  brightBlack: '#696c77',
  brightRed: '#ca3f33',
  brightGreen: '#2f7a2e',
  brightYellow: '#8a5f00',
  brightBlue: '#2b62d6',
  brightMagenta: '#8a1f88',
  brightCyan: '#016d9c',
  brightWhite: '#c8c9cc',
} as const satisfies ITheme

// parseRgb는 getComputedStyle이 돌려주는 rgb()/rgba() 색을 읽는다. 읽지 못하면 null이다.
export function parseRgb(color: string): [number, number, number] | null {
  const m = /^rgba?\(\s*([\d.]+)[\s,]+([\d.]+)[\s,]+([\d.]+)/.exec(color.trim())
  if (!m) return null
  return [Number(m[1]), Number(m[2]), Number(m[3])]
}

// isDark는 배경색이 어두운지다(상대 휘도 기준). 읽지 못하면 어둡다고 본다.
export function isDark(color: string): boolean {
  const rgb = parseRgb(color)
  if (!rgb) return true
  const [r, g, b] = rgb.map((c) => {
    const s = c / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  })
  return 0.2126 * r + 0.7152 * g + 0.0722 * b < 0.4
}

// terminalTheme은 xterm 테마다. background·foreground는 CSS에서 계산한 색이다.
export function terminalTheme(background: string, foreground: string): ITheme {
  const palette = isDark(background) ? DARK : LIGHT
  return {
    ...palette,
    background,
    foreground,
    cursor: foreground,
    cursorAccent: background,
    selectionBackground: 'rgba(60, 132, 247, 0.35)',
  }
}
