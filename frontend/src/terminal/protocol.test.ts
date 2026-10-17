import { describe, expect, it } from 'vitest'
import {
  MAX_INPUT_CHUNK,
  answerMessage,
  cancelMessage,
  inputChunks,
  parseServerMessage,
  resizeMessage,
  terminalUrl,
} from './protocol'

describe('parseServerMessage', () => {
  it('상태 메시지를 읽는다', () => {
    expect(parseServerMessage('{"type":"status","state":"connecting"}')).toEqual({ type: 'status', state: 'connecting' })
    expect(parseServerMessage('{"type":"status","state":"closed","reason":"exit","exitCode":2}')).toEqual({
      type: 'status',
      state: 'closed',
      reason: 'exit',
      exitCode: 2,
    })
    expect(
      parseServerMessage(
        '{"type":"status","state":"closed","reason":"ssh_auth_failed","keyCode":"no_home","notAllowedGroups":["_ssh"]}',
      ),
    ).toEqual({ type: 'status', state: 'closed', reason: 'ssh_auth_failed', keyCode: 'no_home', notAllowedGroups: ['_ssh'] })
  })

  it('모르는 닫힘 이유는 ssh_failed로 본다', () => {
    expect(parseServerMessage('{"type":"status","state":"closed","reason":"new_reason"}')?.reason).toBe('ssh_failed')
  })

  it('질문 메시지를 읽는다', () => {
    expect(
      parseServerMessage('{"type":"prompt","kind":"keyboard-interactive","prompts":[{"text":"Password: ","echo":false}]}'),
    ).toEqual({
      type: 'prompt',
      kind: 'keyboard-interactive',
      instruction: '',
      prompts: [{ text: 'Password: ', echo: false }],
    })
  })

  it('형식이 틀리면 null', () => {
    for (const bad of ['not json', '[]', '{"type":"status","state":"x"}', '{"type":"prompt","kind":"password","prompts":[]}']) {
      expect(parseServerMessage(bad)).toBeNull()
    }
  })
})

describe('client messages', () => {
  it('제어 메시지를 만든다', () => {
    expect(JSON.parse(resizeMessage(80, 24))).toEqual({ type: 'resize', cols: 80, rows: 24 })
    expect(JSON.parse(answerMessage(['pw']))).toEqual({ type: 'answer', answers: ['pw'] })
    expect(JSON.parse(cancelMessage())).toEqual({ type: 'cancel' })
  })

  it('큰 입력을 나눈다', () => {
    const data = new Uint8Array(MAX_INPUT_CHUNK * 2 + 5)
    expect(inputChunks(data).map((c) => c.length)).toEqual([MAX_INPUT_CHUNK, MAX_INPUT_CHUNK, 5])
    expect(inputChunks(new Uint8Array(0))).toEqual([])
  })
})

describe('terminalUrl', () => {
  it('문서 주소 기준으로 ws·wss 주소를 만든다', () => {
    expect(terminalUrl('https://nas.example.lan/desk/#x', './api', 100, 30)).toBe(
      'wss://nas.example.lan/desk/api/terminal?cols=100&rows=30',
    )
    expect(terminalUrl('http://192.0.2.10:5173/', '/api', 80, 24)).toBe('ws://192.0.2.10:5173/api/terminal?cols=80&rows=24')
  })
})
