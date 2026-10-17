// 웹 터미널 창. 대시보드 세션 사용자로 같은 호스트의 sshd에 접속한 셸을 xterm.js로 그린다.
// 접속·인증은 백엔드가 하고(GET /api/terminal, WebSocket), 이 창은 바이트를 주고받고 sshd의 질문(비밀번호 등)에
// 터미널 안에서 답한다. 창을 닫거나 다시 연결하면 셸을 끝낸다(셸을 유지하려면 tmux를 쓴다).
//
// xterm.js의 스타일은 이 보기가 처음 열릴 때만 필요하므로 index.css가 아니라 여기서 import해 지연 청크에 싣는다
// (제3자 스타일이라 .xterm 클래스만 다루고 순서에 영향이 없다).
import '@xterm/xterm/css/xterm.css'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import type { ParseKeys } from 'i18next'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { API_BASE } from '../api/client'
import type { TerminalPrompt } from '../api/types.gen'
import { useAuthStore } from '../auth/store'
import { requestStepUp } from '../security/stepUpDialog'
import { useDialogs } from '../shell/dialogPresets'
import { type CloseNotice, closeNotice } from './messages'
import { type PromptLine, feedPrompt, newPromptLine } from './promptLine'
import { answerMessage, cancelMessage, inputChunks, parseServerMessage, resizeMessage, terminalUrl } from './protocol'
import { terminalTheme } from './theme'

const FONT_FAMILY = "ui-monospace, 'Cascadia Mono', 'DejaVu Sans Mono', Menlo, Consolas, monospace"

export function TerminalWindow() {
  const { t } = useTranslation()
  const user = useAuthStore((s) => s.user?.username ?? '')
  const dialogs = useDialogs()
  const hostRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<{ term: Terminal; fit: FitAddon } | null>(null)
  // 언어를 바꿔도 이미 연 접속의 문구 함수를 새것으로 쓰게 한다.
  const tRef = useRef(t)
  useEffect(() => {
    tRef.current = t
  })
  const [closed, setClosed] = useState<CloseNotice | null>(null)
  // generation이 바뀌면 다시 연결한다.
  const [generation, setGeneration] = useState(0)

  // 터미널(xterm)은 창이 열려 있는 동안 하나만 둔다. 다시 연결해도 지난 출력은 남는다.
  useEffect(() => {
    const host = hostRef.current
    if (!host) return
    const term = new Terminal({
      fontFamily: FONT_FAMILY,
      fontSize: 14,
      cursorBlink: true,
      scrollback: 5000,
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    const applyTheme = () => {
      const style = getComputedStyle(host)
      term.options.theme = terminalTheme(style.backgroundColor, style.color)
    }
    applyTheme()
    fit.fit()
    // Ctrl+Shift+C는 고른 글자를 복사하고, Ctrl+Shift+V는 브라우저가 붙여 넣게 한다(xterm이 입력으로 받는다).
    term.attachCustomKeyEventHandler((e) => {
      if (e.type !== 'keydown' || !e.ctrlKey || !e.shiftKey) return true
      if (e.code === 'KeyC') {
        if (term.hasSelection()) copyText(term.getSelection())
        term.focus()
        e.preventDefault()
        return false
      }
      return e.code !== 'KeyV'
    })
    termRef.current = { term, fit }

    // 창 크기가 바뀌면 칸 수를 다시 맞춘다. 최소화된 창(크기 0)은 FitAddon이 건너뛴다.
    let frame = 0
    const ro = new ResizeObserver(() => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => fit.fit())
    })
    ro.observe(host)
    // 테마(<html data-theme>, 시스템 설정)가 바뀌면 색을 다시 계산한다.
    const mo = new MutationObserver(applyTheme)
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] })
    const scheme = matchMedia('(prefers-color-scheme: dark)')
    scheme.addEventListener('change', applyTheme)
    term.focus()
    return () => {
      cancelAnimationFrame(frame)
      ro.disconnect()
      mo.disconnect()
      scheme.removeEventListener('change', applyTheme)
      termRef.current = null
      term.dispose()
    }
  }, [])

  useEffect(() => {
    const ctx = termRef.current
    if (!ctx) return
    setClosed(null)
    return connect(ctx.term, user, (key, params) => tRef.current(key, params), setClosed)
  }, [generation, user])

  const reconnect = () => {
    termRef.current?.term.write('\r\n')
    setGeneration((g) => g + 1)
    termRef.current?.term.focus()
  }

  // 보안 강화 모드의 확인을 거친 뒤 다시 연결한다.
  const stepUp = async (level: NonNullable<CloseNotice['stepUp']>['level']) => {
    if (await requestStepUp(dialogs.scope, { level, target: 'terminal' })) reconnect()
    else termRef.current?.term.focus()
  }

  return (
    <div className="terminal-window">
      <div className="terminal-screen" ref={hostRef} />
      {closed && (
        <div className="terminal-bar" role="status">
          <span className="terminal-bar-text">{t(closed.key, closed.params)}</span>
          {closed.reconnect && (
            <button type="button" className="addon-button primary" onClick={reconnect}>
              {t('terminal.reconnect')}
            </button>
          )}
          {closed.stepUp && (
            <button type="button" className="addon-button primary" onClick={() => void stepUp(closed.stepUp?.level)}>
              {t('terminal.verify')}
            </button>
          )}
        </div>
      )}
    </div>
  )
}

type Translate = (key: ParseKeys, params?: Record<string, string | number>) => string

// connect는 WebSocket을 열어 term과 잇고, 끊는 함수를 돌려준다. 닫히면 onClosed로 안내를 알린다.
function connect(term: Terminal, user: string, t: Translate, onClosed: (n: CloseNotice) => void): () => void {
  const ws = new WebSocket(terminalUrl(document.baseURI, API_BASE, term.cols, term.rows))
  ws.binaryType = 'arraybuffer'
  const encoder = new TextEncoder()
  let state: 'connecting' | 'prompt' | 'connected' | 'closed' = 'connecting'
  let opened = false
  // asked는 이 접속에서 비밀번호 등을 물었는지다(실패 안내를 고를 때 쓴다).
  let asked = false
  let prompts: TerminalPrompt[] = []
  let answers: string[] = []
  let line: PromptLine | null = null
  let kind: 'password' | 'keyboard-interactive' = 'password'

  const dim = (s: string) => `\x1b[2m${s.replace(/\n/g, '\r\n')}\x1b[0m`
  const send = (data: string | Uint8Array<ArrayBuffer>) => {
    if (ws.readyState === WebSocket.OPEN) ws.send(data)
  }
  const finish = (n: CloseNotice) => {
    if (state === 'closed') return
    state = 'closed'
    line = null
    term.write(`\r\n${dim(t(n.key, n.params))}\r\n`)
    onClosed(n)
  }
  const ask = (p: TerminalPrompt) => {
    // password 방식은 질문 문구가 없어 현지화한 문구를 쓰고, keyboard-interactive는 sshd(PAM)의 문구 그대로다.
    term.write(kind === 'password' ? t('terminal.passwordPrompt', { user }) : p.text)
    line = newPromptLine(p.echo)
  }

  ws.onopen = () => {
    opened = true
  }
  ws.onmessage = (ev: MessageEvent<ArrayBuffer | string>) => {
    if (typeof ev.data !== 'string') {
      term.write(new Uint8Array(ev.data))
      return
    }
    const msg = parseServerMessage(ev.data)
    if (!msg || state === 'closed') return
    if (msg.type === 'prompt') {
      // 앞 질문과 같은 질문이 다시 오면 답이 틀린 것이다(인증 코드처럼 이어지는 질문은 문구가 다르다).
      if (asked && samePrompts(prompts, msg.prompts ?? [])) term.write(`${dim(t('terminal.retry'))}\r\n`)
      asked = true
      state = 'prompt'
      kind = msg.kind ?? 'password'
      prompts = msg.prompts ?? []
      answers = []
      if (msg.instruction) term.write(`${msg.instruction.replace(/\n/g, '\r\n')}\r\n`)
      ask(prompts[0])
      return
    }
    if (msg.state === 'connecting') {
      term.write(`${dim(t('terminal.connecting', { user }))}\r\n`)
    } else if (msg.state === 'connected') {
      state = 'connected'
      send(resizeMessage(term.cols, term.rows))
    } else if (msg.state === 'closed') {
      finish(closeNotice(msg, asked))
    }
  }
  ws.onclose = () => {
    // closed 메시지 없이 끊겼다: 연결을 열지 못했거나(허용 안 됨, 세션 없음) 네트워크가 끊겼다.
    finish({ key: opened ? 'terminal.closed.connectionLost' : 'terminal.closed.openFailed', reconnect: true })
  }

  const subs = [
    term.onData((data) => {
      if (state === 'prompt' && line) {
        const r = feedPrompt(line, data)
        term.write(r.output)
        if (r.kind === 'editing') {
          line = r.line
        } else if (r.kind === 'cancel') {
          line = null
          state = 'connecting'
          send(cancelMessage())
        } else {
          answers.push(r.value)
          if (answers.length < prompts.length) {
            ask(prompts[answers.length])
          } else {
            line = null
            state = 'connecting'
            send(answerMessage(answers))
          }
        }
        return
      }
      if (state === 'connected') for (const chunk of inputChunks(encoder.encode(data))) send(chunk)
    }),
    // 일부 마우스 보고는 바이트 문자열(0~255)로 온다.
    term.onBinary((data) => {
      if (state === 'connected') send(Uint8Array.from(data, (c) => c.charCodeAt(0) & 0xff))
    }),
    term.onResize(({ cols, rows }) => send(resizeMessage(cols, rows))),
  ]
  return () => {
    for (const s of subs) s.dispose()
    ws.onopen = ws.onmessage = ws.onclose = null
    ws.close()
  }
}

function samePrompts(a: TerminalPrompt[], b: TerminalPrompt[]): boolean {
  return a.length === b.length && a.every((p, i) => p.text === b[i].text && p.echo === b[i].echo)
}

// copyText는 글자를 클립보드에 넣는다. 평문 HTTP처럼 Clipboard API가 없으면 execCommand로 한다.
function copyText(text: string) {
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).catch(() => {})
    return
  }
  const area = document.createElement('textarea')
  area.value = text
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.append(area)
  area.select()
  document.execCommand('copy')
  area.remove()
}
