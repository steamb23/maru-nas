import { BRIDGE_CODES, MAX_INFLIGHT, MAX_MESSAGE_BYTES, interpolate, parseFrameMessage, parseHostMessage } from '@maru-sdk/protocol'
import { describe, expect, it } from 'vitest'
import { BridgeLimits, GESTURE_WINDOW_MS, admit, gestureAllowed, parseParams, plainText } from './bridge'

describe('프로토콜 메시지', () => {
  it('프레임 메시지 형식을 검사한다', () => {
    expect(parseFrameMessage({ maru: 1, type: 'hello', sdk: 1 })).toEqual({ maru: 1, type: 'hello', sdk: 1 })
    expect(parseFrameMessage({ maru: 1, type: 'request', id: 3, method: 'rpc', params: { a: 1 } })).toMatchObject({ id: 3 })
    expect(parseFrameMessage({ maru: 1, type: 'request', id: 3, method: 'eval' })).toBeNull()
    expect(parseFrameMessage({ maru: 1, type: 'request', id: -1, method: 'rpc' })).toBeNull()
    expect(parseFrameMessage({ maru: 2, type: 'hello', sdk: 1 })).toBeNull()
    expect(parseFrameMessage({ maru: 1, type: 'input', kind: 'wheel' })).toBeNull()
    expect(parseFrameMessage('hello')).toBeNull()
    // 메시지에 애드온 id를 넣어도 형식에 없으므로 버린다(호스트는 프레임 바인딩으로 정한다).
    expect(parseFrameMessage({ maru: 1, type: 'request', id: 1, method: 'data', addonId: 'other' })).not.toHaveProperty('addonId')
  })

  it('호스트 메시지 형식을 검사한다', () => {
    expect(parseHostMessage({ maru: 1, type: 'response', id: 1, ok: true, result: 2 })).not.toBeNull()
    expect(parseHostMessage({ maru: 1, type: 'response', id: 1, ok: false })).toBeNull()
    expect(parseHostMessage({ maru: 1, type: 'init' })).toBeNull()
  })

  it('interpolate는 아는 이름만 바꾼다', () => {
    expect(interpolate('Host {{ name }}: {{x}}', { name: 'nas' })).toBe('Host nas: {{x}}')
    expect(interpolate('{{toString}}', {})).toBe('{{toString}}')
  })
})

describe('요청 판정', () => {
  const yes = () => true
  const no = () => false
  it('위젯은 읽기 요청만 받는다', () => {
    expect(admit('data', 'widget', 10, yes)).toBeNull()
    expect(admit('storage.get', 'widget', 10, yes)).toBeNull()
    expect(admit('storage.set', 'widget', 10, yes)).toBe(BRIDGE_CODES.notAllowed)
    expect(admit('confirm', 'widget', 10, yes)).toBe(BRIDGE_CODES.notAllowed)
    expect(admit('action', 'widget', 10, yes)).toBe(BRIDGE_CODES.notAllowed)
  })
  it('조작과 워크벤치 열기는 사용자 조작이 있어야 받는다', () => {
    expect(admit('action', 'window', 10, no)).toBe(BRIDGE_CODES.gesture)
    expect(admit('openWorkbench', 'window', 10, no)).toBe(BRIDGE_CODES.gesture)
    expect(admit('action', 'window', 10, yes)).toBeNull()
    // query는 창을 열 때 데이터를 읽는 데 쓰므로 막지 않는다.
    expect(admit('query', 'window', 10, no)).toBeNull()
  })
  it('큰 메시지는 받지 않는다', () => {
    expect(admit('data', 'window', MAX_MESSAGE_BYTES + 1, yes)).toBe(BRIDGE_CODES.invalid)
  })
  it('사용자 조작은 포커스한 그 프레임의 최근 입력이어야 한다', () => {
    const base = { activation: true, focused: true, front: true, lastInput: 1000, now: 1500 }
    expect(gestureAllowed(base)).toBe(true)
    expect(gestureAllowed({ ...base, activation: false })).toBe(false)
    expect(gestureAllowed({ ...base, focused: false })).toBe(false)
    expect(gestureAllowed({ ...base, front: false })).toBe(false)
    expect(gestureAllowed({ ...base, lastInput: null })).toBe(false)
    expect(gestureAllowed({ ...base, now: 1000 + GESTURE_WINDOW_MS })).toBe(false)
  })
})

describe('BridgeLimits', () => {
  it('동시 요청 수를 센다', () => {
    const l = new BridgeLimits()
    for (let i = 0; i < MAX_INFLIGHT; i++) expect(l.start('data', i * 100)).toBeNull()
    expect(l.start('data', 99_999)).toBe(BRIDGE_CODES.busy)
    l.done()
    expect(l.start('rpc', 99_999)).toBeNull()
  })
  it('요청마다 빈도를 센다', () => {
    const l = new BridgeLimits()
    expect(l.start('action', 0)).toBeNull()
    l.done()
    expect(l.start('action', 500)).toBe(BRIDGE_CODES.rate)
    expect(l.start('action', 1000)).toBeNull()
    l.done()
    for (let i = 0; i < 3; i++) {
      expect(l.start('notify', i)).toBeNull()
      l.done()
    }
    expect(l.start('notify', 59_000)).toBe(BRIDGE_CODES.rate)
    expect(l.start('notify', 60_001)).toBeNull()
  })
})

describe('parseParams', () => {
  it('맞는 파라미터를 받는다', () => {
    expect(parseParams('rpc', { service: 'System', method: 'getInformation' })).toEqual({ service: 'System', method: 'getInformation', params: undefined })
    expect(parseParams('data', { query: { a: '1' } })).toEqual({ query: { a: '1' } })
    expect(parseParams('openWorkbench', { route: 'storage/smart/devices' })).toEqual({ route: 'storage/smart/devices' })
    expect(parseParams('notify', { level: 'info', key: 'saved', params: { n: 1 } })).toMatchObject({ key: 'saved' })
    expect(parseParams('storage.set', { data: { a: 1 } })).toEqual({ data: { a: 1 } })
  })
  it('틀린 파라미터는 버린다', () => {
    expect(parseParams('rpc', { service: 'System.x', method: 'get' })).toBeNull()
    expect(parseParams('rpc', 'x')).toBeNull()
    expect(parseParams('openWorkbench', { route: '/x' })).toBeNull()
    expect(parseParams('openWorkbench', { route: 'https://evil.example' })).toBeNull()
    expect(parseParams('openWorkbench', { route: 'a/../b' })).toBeNull()
    expect(parseParams('action', { name: 'Go' })).toBeNull()
    expect(parseParams('action', { name: 'go', input: [1] })).toBeNull()
    expect(parseParams('notify', { level: 'fatal', key: 'x' })).toBeNull()
    expect(parseParams('confirm', { message: '' })).toBeNull()
    expect(parseParams('storage.set', {})).toBeNull()
  })
})

describe('plainText', () => {
  it('제어·서식 문자를 지우고 줄인다', () => {
    const rlo = String.fromCharCode(0x202e)
    expect(plainText(`a${rlo}b\n\tc`, 100)).toBe('a b c')
    expect(plainText('abcdef', 4)).toBe('abc…')
  })
})
