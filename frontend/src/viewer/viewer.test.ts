import { describe, expect, it } from 'vitest'
import { decodeText } from './text'

const enc = new TextEncoder()

describe('텍스트 보기', () => {
  it('UTF-8을 읽고 BOM을 뺀다', () => {
    const bytes = enc.encode('﻿안녕\nhello')
    expect(decodeText(bytes, bytes.length)).toEqual({ text: '안녕\nhello', truncated: false, binary: false })
  })

  it('잘린 끝의 덜 받은 글자는 버린다', () => {
    const full = enc.encode('가나다')
    // '다'의 첫 두 바이트까지만 받았다.
    const cut = full.subarray(0, 7)
    expect(decodeText(cut, full.length)).toEqual({ text: '가나', truncated: true, binary: false })
  })

  it('NUL 바이트가 있으면 이진 파일로 본다', () => {
    expect(decodeText(new Uint8Array([0x50, 0x4b, 0x00, 0x01]), 4)).toMatchObject({ binary: true, text: '' })
  })

  it('UTF-8이 아닌 바이트는 대체 문자로 보인다', () => {
    expect(decodeText(new Uint8Array([0x61, 0xff, 0x62]), 3).text).toBe('a�b')
  })
})
