import { describe, expect, it } from 'vitest'
import type { PasskeyOptions } from '../api/types.gen'
import { assertionJSON, b64urlDecode, b64urlEncode, creationOptions, prfResult, registrationJSON, requestOptions } from './webauthn'

const bytes = (...b: number[]) => new Uint8Array(b).buffer

describe('base64url', () => {
  it('패딩 없이 URL에 안전한 문자로 쓰고 되돌린다', () => {
    const data = new Uint8Array([0xfb, 0xff, 0xbf, 0x00, 0x10])
    const s = b64urlEncode(data)
    expect(s).toBe('-_-_ABA')
    expect(Array.from(b64urlDecode(s))).toEqual(Array.from(data))
    expect(Array.from(b64urlDecode('-_-_ABA='))).toEqual(Array.from(data))
    expect(b64urlEncode(new Uint8Array())).toBe('')
  })

  it('형식이 틀리면 예외를 던진다', () => {
    expect(() => b64urlDecode('a')).toThrow()
  })
})

const assertOpts: PasskeyOptions = {
  challenge: 'AAEC',
  rpId: 'desk.example.lan',
  timeoutMs: 120000,
  credentials: [{ id: 'CQk', transports: ['internal', 'bogus'] }],
}

describe('requestOptions', () => {
  it('확인 옵션으로 바꾼다', () => {
    const o = requestOptions(assertOpts)
    expect(Array.from(new Uint8Array(o.challenge as ArrayBuffer))).toEqual([0, 1, 2])
    expect(o.rpId).toBe('desk.example.lan')
    expect(o.userVerification).toBe('required')
    expect(o.allowCredentials).toHaveLength(1)
    expect(o.allowCredentials?.[0].transports).toEqual(['internal'])
    expect(o.extensions).toBeUndefined()
  })

  it('PRF 입력이 있으면 확장으로 요청한다', () => {
    const salt = new Uint8Array([5, 6, 7])
    const o = requestOptions(assertOpts, salt)
    expect(o.extensions?.prf?.eval?.first).toBe(salt)
    expect(o.userVerification).toBe('required')
  })
})

describe('prfResult', () => {
  it('PRF 첫 결과를 ArrayBuffer로 꺼내고 없으면 null이다', () => {
    expect(prfResult({})).toBeNull()
    expect(prfResult({ prf: { enabled: false } })).toBeNull()
    const buf = bytes(1, 2, 3)
    expect(prfResult({ prf: { results: { first: buf } } })).toBe(buf)
    const view = new Uint8Array([9, 1, 2, 3, 9]).subarray(1, 4)
    expect(Array.from(new Uint8Array(prfResult({ prf: { results: { first: view } } }) ?? new ArrayBuffer(0)))).toEqual([1, 2, 3])
  })
})

describe('creationOptions', () => {
  it('등록 옵션으로 바꾼다', () => {
    const o = creationOptions({
      ...assertOpts,
      rpName: 'Maru NAS',
      user: { id: 'BwcH', name: 'alice', displayName: 'alice' },
      algorithms: [-7, -8, -257],
    })
    expect(o.rp).toEqual({ id: 'desk.example.lan', name: 'Maru NAS' })
    expect(Array.from(new Uint8Array(o.user.id as ArrayBuffer))).toEqual([7, 7, 7])
    expect(o.pubKeyCredParams.map((p) => p.alg)).toEqual([-7, -8, -257])
    expect(o.attestation).toBe('none')
    expect(o.authenticatorSelection?.userVerification).toBe('required')
    expect(o.excludeCredentials).toHaveLength(1)
    // 보안 키의 hmac-secret은 만들 때 요청해야 생기므로 등록할 때 PRF를 요청한다(입력 없이).
    expect(o.extensions).toEqual({ prf: {} })
  })

  it('등록 옵션이 아니면 예외를 던진다', () => {
    expect(() => creationOptions(assertOpts)).toThrow()
  })
})

describe('결과 JSON', () => {
  it('확인 결과를 바꾼다', () => {
    expect(
      assertionJSON({
        rawId: bytes(1),
        response: { clientDataJSON: bytes(2), authenticatorData: bytes(3), signature: bytes(4), userHandle: null },
      }),
    ).toEqual({ id: 'AQ', clientDataJSON: 'Ag', authenticatorData: 'Aw', signature: 'BA', userHandle: undefined })
  })

  it('등록 결과를 바꾸고 공개 키가 없으면 비운다', () => {
    const reg = registrationJSON({
      rawId: bytes(1),
      response: {
        clientDataJSON: bytes(2),
        getAuthenticatorData: () => bytes(3),
        getPublicKey: () => null,
        getPublicKeyAlgorithm: () => -7,
        getTransports: () => ['internal'],
      },
    })
    expect(reg).toEqual({
      id: 'AQ',
      clientDataJSON: 'Ag',
      authenticatorData: 'Aw',
      publicKey: '',
      publicKeyAlgorithm: -7,
      transports: ['internal'],
    })
  })
})
